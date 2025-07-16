// Package metrics provides in-memory batching of CloudWatch metrics into EMF records.
// It handles metric accumulation, threshold-based flushing, and EMF record creation.
package metrics

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// EMFBuilder interface for building EMF records
type EMFBuilder interface {
	Build(input builder.EMFInput, logger logger.Logger) (builder.EMFRecord, error)
}

// DefaultEMFBuilder implements EMFBuilder using the builder package
type DefaultEMFBuilder struct{}

func (DefaultEMFBuilder) Build(input builder.EMFInput, logger logger.Logger) (builder.EMFRecord, error) {
	return builder.Build(input, logger)
}

// ThresholdChecker interface for checking flush thresholds
type ThresholdChecker interface {
	ShouldFlush(currentCount int, currentSize int64, newRecordSize int64, maxCount int, maxBytes int64) bool
}

// DefaultThresholdChecker implements ThresholdChecker
type DefaultThresholdChecker struct{}

func (DefaultThresholdChecker) ShouldFlush(currentCount int, currentSize int64, newRecordSize int64, maxCount int, maxBytes int64) bool {
	newCount := currentCount + 1
	newSize := currentSize + newRecordSize

	return (maxCount > 0 && newCount >= maxCount) ||
		(maxBytes > 0 && newSize >= maxBytes)
}

// Error constants for better error handling and testing
var (
	ErrInvalidConfig    = errors.New("invalid batcher configuration")
	ErrEmptyNamespace   = errors.New("namespace cannot be empty")
	ErrEmptyRegion      = errors.New("region cannot be empty")
	ErrNilEMFFlusher    = errors.New("EMF flusher cannot be nil")
	ErrInvalidThreshold = errors.New("invalid threshold configuration")
	ErrBuildFailed      = errors.New("failed to build EMF record")
	ErrFlushFailed      = errors.New("failed to flush batch")
)

// Batcher defines the interface for batching CloudWatch metrics and flushing them as EMF records.
type Batcher interface {
	Add(ctx context.Context, m types.CloudWatchMetric)
	FlushAll(ctx context.Context)
}

// MetricsBatcher batches CloudWatchMetric items in-memory with threshold-based flushing,
// converting metrics to EMF records for CloudWatch ingestion.
type MetricsBatcher struct {
	namespace string
	logGroup  string
	logStream string
	region    string
	maxCount  int
	maxBytes  int64

	emfFlusher       emf.EMFFlusher
	logger           logger.Logger
	emfBuilder       EMFBuilder
	thresholdChecker ThresholdChecker

	mu      sync.Mutex
	records []builder.EMFRecord
	count   int
	size    int64
}

// MetricsBatcherConfig defines all configuration parameters needed to create
// and configure a MetricsBatcher instance.
type MetricsBatcherConfig struct {
	Namespace        string           // EMF namespace
	LogGroup         string           // CloudWatch Logs group
	LogStream        string           // CloudWatch Logs stream
	Region           string           // AWS region
	MaxCount         int              // max records before pre-flush
	MaxBytes         int64            // max bytes before pre-flush
	EmfFlusher       emf.EMFFlusher   // EMF flusher implementation
	Logger           logger.Logger    // Logger instance
	EMFBuilder       EMFBuilder       // Optional, defaults to DefaultEMFBuilder
	ThresholdChecker ThresholdChecker // Optional, defaults to DefaultThresholdChecker
}

// Validate checks if the configuration is valid
func (cfg MetricsBatcherConfig) Validate() error {
	if strings.TrimSpace(cfg.Namespace) == "" {
		return ErrEmptyNamespace
	}
	if strings.TrimSpace(cfg.Region) == "" {
		return ErrEmptyRegion
	}
	if cfg.EmfFlusher == nil {
		return ErrNilEMFFlusher
	}
	if cfg.MaxCount < 0 || cfg.MaxBytes < 0 {
		return ErrInvalidThreshold
	}
	return nil
}

// NewMetricsBatcher constructs a new in-memory MetricsBatcher with
// the provided configuration and initializes internal state.
func NewMetricsBatcher(cfg MetricsBatcherConfig) (Batcher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.Logger == nil {
		cfg.Logger = logger.Get()
	}
	if cfg.EMFBuilder == nil {
		cfg.EMFBuilder = DefaultEMFBuilder{}
	}
	if cfg.ThresholdChecker == nil {
		cfg.ThresholdChecker = DefaultThresholdChecker{}
	}

	mb := &MetricsBatcher{
		namespace:        cfg.Namespace,
		logGroup:         cfg.LogGroup,
		logStream:        cfg.LogStream,
		region:           cfg.Region,
		maxCount:         cfg.MaxCount,
		maxBytes:         cfg.MaxBytes,
		emfFlusher:       cfg.EmfFlusher,
		logger:           cfg.Logger,
		emfBuilder:       cfg.EMFBuilder,
		thresholdChecker: cfg.ThresholdChecker,
		records:          make([]builder.EMFRecord, 0),
	}
	mb.logger.Info("MetricsBatcher initialized for namespace %s region %s", cfg.Namespace, cfg.Region)
	return mb, nil
}

// Add converts a CloudWatch metric to an EMF record, adds it to the batch,
// and triggers pre-flush if size or count thresholds would be exceeded.
func (mb *MetricsBatcher) Add(ctx context.Context, m types.CloudWatchMetric) {
	err := m.Unit.Validate()
	if err != nil {
		m.Unit = types.UnitPercent
	}
	rec, err := mb.emfBuilder.Build(builder.EMFInput{
		Namespace:  mb.namespace,
		MetricName: m.Name,
		Value:      m.Value,
		Unit:       m.Unit.String(),
		Dimensions: BuildDimensions(m.Metadata),
		Timestamp:  m.Timestamp,
	}, mb.logger)
	if err != nil {
		mb.logger.Error("Add metric: %v: %v", ErrBuildFailed, err)
		return
	}

	mb.mu.Lock()
	currCount := mb.count
	currSize := mb.size
	recSize := int64(len(rec.Payload) + 1)

	// Pre-flush if needed
	if mb.thresholdChecker.ShouldFlush(currCount, currSize, recSize, mb.maxCount, mb.maxBytes) {
		mb.mu.Unlock()
		mb.logger.Info("pre-threshold reached (count %d/%d, size %d/%d), flushing", currCount+1, mb.maxCount, currSize+recSize, mb.maxBytes)
		mb.FlushAll(ctx)
		mb.mu.Lock()
	}

	// Append record
	mb.records = append(mb.records, rec)
	mb.count++
	mb.size += recSize
	mb.logger.Debug("added metric %s: count=%d size=%d", m.Name, mb.count, mb.size)
	mb.mu.Unlock()
}

// FlushAll sends all accumulated EMF records to CloudWatch Logs and
// resets the batch state for the next accumulation cycle.
func (mb *MetricsBatcher) FlushAll(ctx context.Context) {
	mb.mu.Lock()
	batch := mb.records
	mb.mu.Unlock()

	if len(batch) == 0 {
		return
	}

	mb.logger.Info("flushing %d EMF records to CloudWatch", len(batch))
	if err := mb.emfFlusher.Flush(ctx, mb.region, batch); err != nil {
		mb.logger.Error("flush failed: %v", err)
		return
	}

	mb.mu.Lock()
	mb.records = make([]builder.EMFRecord, 0)
	mb.count = 0
	mb.size = 0
	mb.mu.Unlock()
}

// BuildDimensions converts metadata map to EMF dimensions format
func BuildDimensions(metadata map[string]string) [][]string {
	if len(metadata) == 0 {
		return nil
	}
	dims := make([][]string, 0, len(metadata))
	for k, v := range metadata {
		pair := make([]string, 2)
		pair[0] = k
		pair[1] = v
		dims = append(dims, pair)
	}
	return dims
}
