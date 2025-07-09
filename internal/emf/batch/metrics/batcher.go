// internal/emf/batch/metrics/batcher.go
package metrics

import (
	"context"
	"sync"

	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

type Batcher interface {
	Add(ctx context.Context, m sharedTypes.CloudWatchMetric)
	FlushAll(ctx context.Context)
}

// MetricsBatcher batches CloudWatchMetric items in-memory and flushes them as EMF records.
type MetricsBatcher struct {
	namespace string
	logGroup  string
	logStream string
	region    string
	maxCount  int
	maxBytes  int64

	emfFlusher emf.EMFFlusher
	logger     logger.Logger

	mu      sync.Mutex
	records []builder.EMFRecord
	count   int
	size    int64
}

// MetricsBatcherConfig defines settings for a MetricsBatcher.
type MetricsBatcherConfig struct {
	Namespace  string         // EMF namespace
	LogGroup   string         // CloudWatch Logs group
	LogStream  string         // CloudWatch Logs stream
	Region     string         // AWS region
	MaxCount   int            // max records before pre-flush
	MaxBytes   int64          // max bytes before pre-flush
	EmfFlusher emf.EMFFlusher // EMF flusher implementation
	Logger     logger.Logger  // Logger instance
}

// NewMetricsBatcher constructs a new in-memory MetricsBatcher.
func NewMetricsBatcher(cfg MetricsBatcherConfig) Batcher {
	if cfg.Logger == nil {
		cfg.Logger = logger.Get()
	}
	return &MetricsBatcher{
		namespace:  cfg.Namespace,
		logGroup:   cfg.LogGroup,
		logStream:  cfg.LogStream,
		region:     cfg.Region,
		maxCount:   cfg.MaxCount,
		maxBytes:   cfg.MaxBytes,
		emfFlusher: cfg.EmfFlusher,
		logger:     cfg.Logger,
		records:    make([]builder.EMFRecord, 0),
	}
}

// Add builds an EMF record for the given metric and appends to the batch.
// Triggers a pre-flush if thresholds would be exceeded.
func (mb *MetricsBatcher) Add(ctx context.Context, m sharedTypes.CloudWatchMetric) {
	err := m.Unit.Validate()
	if err != nil {
		m.Unit = sharedTypes.UnitPercent
	}
	rec, err := builder.Build(builder.EMFInput{
		Namespace:  mb.namespace,
		MetricName: m.Name,
		Value:      m.Value,
		Unit:       m.Unit.UnitToString(),
		Dimensions: func() [][]string {
			dims := make([][]string, 0, len(m.Metadata))
			for k, v := range m.Metadata {
				dims = append(dims, []string{k, v})
			}
			return dims
		}(),
		Timestamp: m.Timestamp,
	}, mb.logger)
	if err != nil {
		mb.logger.Error("Add metric: build error: %v", err)
		return
	}

	mb.mu.Lock()
	currCount := mb.count
	currSize := mb.size
	newCount := currCount + 1
	recSize := int64(len(rec.Payload) + 1)
	newSize := currSize + recSize
	// Pre-flush if needed
	if (mb.maxCount > 0 && newCount > mb.maxCount) ||
		(mb.maxBytes > 0 && newSize > mb.maxBytes) {
		mb.mu.Unlock()
		mb.logger.Info("MetricsBatcher: pre-threshold reached (count %d/%d, size %d/%d), flushing", newCount, mb.maxCount, newSize, mb.maxBytes)
		mb.FlushAll(ctx)
		mb.mu.Lock()
	}
	// Append record
	mb.records = append(mb.records, rec)
	mb.count++
	mb.size += recSize
	mb.mu.Unlock()
}

// FlushAll flushes all accumulated EMF records and resets the batch.
func (mb *MetricsBatcher) FlushAll(ctx context.Context) {
	mb.mu.Lock()
	batch := mb.records
	mb.mu.Unlock()

	if len(batch) == 0 {
		return
	}

	if err := mb.emfFlusher.Flush(ctx, mb.region, batch); err != nil {
		mb.logger.Error("MetricsBatcher: flush error: %v", err)
	}

	mb.mu.Lock()
	mb.records = make([]builder.EMFRecord, 0)
	mb.count = 0
	mb.size = 0
	mb.mu.Unlock()
}
