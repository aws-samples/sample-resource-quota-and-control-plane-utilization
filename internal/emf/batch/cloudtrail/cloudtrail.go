// Package cloudtrail provides file-based batching of CloudTrail events into EMF records.
// It handles event accumulation, threshold-based flushing, and aggregation for rate limiting metrics.
package cloudtrail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// Error constants for better error handling and testing
var (
	ErrInvalidRegion     = errors.New("invalid region name")
	ErrCounterFileRead   = errors.New("failed to read counter file")
	ErrCounterFileWrite  = errors.New("failed to write counter file")
	ErrInvalidConfig     = errors.New("invalid batcher configuration")
	ErrFlushFailed       = errors.New("failed to flush records")
	ErrTimeCalculation   = errors.New("failed to calculate elapsed time")
)

// FileSystem interface for dependency injection in testing
type FileSystem interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
	Remove(filename string) error
	Rename(oldpath, newpath string) error
	Glob(pattern string) ([]string, error)
}

// DefaultFileSystem implements FileSystem using os package
type DefaultFileSystem struct{}

func (DefaultFileSystem) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}

func (DefaultFileSystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (DefaultFileSystem) Remove(filename string) error {
	return os.Remove(filename)
}

func (DefaultFileSystem) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (DefaultFileSystem) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// TimeProvider interface for time operations
type TimeProvider interface {
	Now() time.Time
	Parse(layout, value string) (time.Time, error)
}

// DefaultTimeProvider implements TimeProvider using time package
type DefaultTimeProvider struct{}

func (DefaultTimeProvider) Now() time.Time {
	return time.Now()
}

func (DefaultTimeProvider) Parse(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}



// Batcher defines the interface for batching CloudTrail events and flushing them as EMF records.
type Batcher interface {
	Add(ctx context.Context, region string, ct types.CloudTrailEvent)
	FlushAll(ctx context.Context, time time.Time) error
}

// CTFileBatcher batches CloudTrail events into region-specific counter files,
// manages threshold-based flushing, and creates EMF records from counters.
type CTFileBatcher struct {
	baseDir            string
	namespace          string
	metricName         string
	lastFlushFilePath  string
	lambdaInitFilePath string
	propagrateInvoker  bool

	emfFlusher   emf.EMFFlusher
	logger       logger.Logger
	fs           FileSystem
	timeProvider TimeProvider
	counterMgr   *CounterManager
	timeCalc     *TimeCalculator
}

// CounterManager handles counter file operations
type CounterManager struct {
	fs      FileSystem
	baseDir string
	logger  logger.Logger
}

// NewCounterManager creates a new counter manager
func NewCounterManager(fs FileSystem, baseDir string, logger logger.Logger) *CounterManager {
	return &CounterManager{
		fs:      fs,
		baseDir: baseDir,
		logger:  logger,
	}
}

// TimeCalculator handles flush time calculations
type TimeCalculator struct {
	fs            FileSystem
	timeProvider  TimeProvider
	baseDir       string
	lastFlushPath string
	initFilePath  string
	logger        logger.Logger
}

// NewTimeCalculator creates a new time calculator
func NewTimeCalculator(fs FileSystem, timeProvider TimeProvider, baseDir, lastFlushPath, initFilePath string, logger logger.Logger) *TimeCalculator {
	return &TimeCalculator{
		fs:            fs,
		timeProvider:  timeProvider,
		baseDir:       baseDir,
		lastFlushPath: lastFlushPath,
		initFilePath:  initFilePath,
		logger:        logger,
	}
}

// CTFileBatcherConfig holds all configuration parameters needed to create
// and configure a CTFileBatcher instance.
type CTFileBatcherConfig struct {
	BaseDir            string
	Namespace          string
	MetricName         string
	LastFlushFilePath  string
	LambdaInitFilePath string
	PropagateInvoker   bool
	EmfFlusher         emf.EMFFlusher
	Logger             logger.Logger
	FileSystem         FileSystem     // Optional, defaults to DefaultFileSystem
	TimeProvider       TimeProvider   // Optional, defaults to DefaultTimeProvider
}

// Validate checks if the configuration is valid
func (cfg CTFileBatcherConfig) Validate() error {
	if cfg.BaseDir == "" {
		return fmt.Errorf("%w: base directory is empty", ErrInvalidConfig)
	}
	if cfg.Namespace == "" {
		return fmt.Errorf("%w: namespace is empty", ErrInvalidConfig)
	}
	if cfg.MetricName == "" {
		return fmt.Errorf("%w: metric name is empty", ErrInvalidConfig)
	}
	if cfg.EmfFlusher == nil {
		return fmt.Errorf("%w: EMF flusher is nil", ErrInvalidConfig)
	}
	return nil
}

// NewCTFileBatcher constructs a new file-based CloudTrail batcher with
// the provided configuration and initializes internal state.
func NewCTFileBatcher(cfg CTFileBatcherConfig) (Batcher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	
	if cfg.Logger == nil {
		cfg.Logger = logger.Get()
	}
	if cfg.FileSystem == nil {
		cfg.FileSystem = DefaultFileSystem{}
	}
	if cfg.TimeProvider == nil {
		cfg.TimeProvider = DefaultTimeProvider{}
	}
	
	counterMgr := NewCounterManager(cfg.FileSystem, cfg.BaseDir, cfg.Logger)
	timeCalc := NewTimeCalculator(cfg.FileSystem, cfg.TimeProvider, cfg.BaseDir, cfg.LastFlushFilePath, cfg.LambdaInitFilePath, cfg.Logger)
	
	return &CTFileBatcher{
		baseDir:            cfg.BaseDir,
		namespace:          cfg.Namespace,
		metricName:         cfg.MetricName,
		lastFlushFilePath:  cfg.LastFlushFilePath,
		lambdaInitFilePath: cfg.LambdaInitFilePath,
		propagrateInvoker:  cfg.PropagateInvoker,
		emfFlusher:         cfg.EmfFlusher,
		logger:             cfg.Logger,
		fs:                 cfg.FileSystem,
		timeProvider:       cfg.TimeProvider,
		counterMgr:         counterMgr,
		timeCalc:           timeCalc,
	}, nil
}

// Add processes a CloudTrail event by creating EMF records, checking thresholds,
// writing to region-specific files, and triggering flushes when limits are exceeded.
func (fb *CTFileBatcher) Add(ctx context.Context, region string, ct types.CloudTrailEvent) {
	if !utils.IsValidRegion(region) {
		fb.logger.Error("Add: %v: %s", ErrInvalidRegion, region)
		return
	}

	fb.logger.Debug("processing event: %+v", ct)

	// Generate counter keys
	keys := GenerateCounterKeys(ct, fb.propagrateInvoker)

	// Update counters for each key
	for _, key := range keys {
		if err := fb.counterMgr.IncrementCounter(region, key); err != nil {
			fb.logger.Error("increment counter %s/%s: %v", region, key, err)
		} else {
			fb.logger.Debug("incremented counter %s/%s", region, key)
		}
	}
}

// GenerateCounterKeys generates counter keys from CloudTrail event
func GenerateCounterKeys(ct types.CloudTrailEvent, propagateInvoker bool) []string {
	keys := []string{ct.EventName} // Always have simple eventName counter

	if propagateInvoker {
		invoker := ExtractInvoker(ct)
		invokerKey := fmt.Sprintf("%s:%s", ct.EventName, invoker)
		keys = append(keys, invokerKey)
	}

	return keys
}

// ReadCounters reads the counter file for a specific region
func (cm *CounterManager) ReadCounters(region string) (map[string]int, error) {
	filePath := filepath.Join(cm.baseDir, fmt.Sprintf("counters_%s.json", region))
	
	data, err := cm.fs.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]int), nil // Return empty map if file doesn't exist
		}
		return nil, fmt.Errorf("%w: %v", ErrCounterFileRead, err)
	}
	
	var counters map[string]int
	if err := json.Unmarshal(data, &counters); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", ErrCounterFileRead, err)
	}
	
	return counters, nil
}

// WriteCounters writes counters to file for a specific region
func (cm *CounterManager) WriteCounters(region string, counters map[string]int) error {
	filePath := filepath.Join(cm.baseDir, fmt.Sprintf("counters_%s.json", region))
	
	data, err := json.Marshal(counters)
	if err != nil {
		return fmt.Errorf("%w: marshal failed: %v", ErrCounterFileWrite, err)
	}
	
	// Atomic write pattern
	tempFile := filePath + ".tmp"
	if err := cm.fs.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("%w: temp file write failed: %v", ErrCounterFileWrite, err)
	}
	
	if err := cm.fs.Rename(tempFile, filePath); err != nil {
		return fmt.Errorf("%w: atomic rename failed: %v", ErrCounterFileWrite, err)
	}
	
	return nil
}

// IncrementCounter atomically increments a counter for a specific region and key
func (cm *CounterManager) IncrementCounter(region, key string) error {
	counters, err := cm.ReadCounters(region)
	if err != nil {
		return err
	}
	
	counters[key]++
	
	return cm.WriteCounters(region, counters)
}

// ClearCounters removes the counter file for a region
func (cm *CounterManager) ClearCounters(region string) error {
	filePath := filepath.Join(cm.baseDir, fmt.Sprintf("counters_%s.json", region))
	if err := cm.fs.Remove(filePath); err != nil && !os.IsNotExist(err) {
		cm.logger.Error("clear counter file %s: %v", filePath, err)
		return err
	}
	return nil
}

// GetRegions returns all regions that have counter files
func (cm *CounterManager) GetRegions() ([]string, error) {
	files, err := cm.fs.Glob(filepath.Join(cm.baseDir, "counters_*.json"))
	if err != nil {
		return nil, err
	}
	
	var regions []string
	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasPrefix(base, "counters_") && strings.HasSuffix(base, ".json") {
			region := base[9 : len(base)-5] // Remove "counters_" and ".json"
			regions = append(regions, region)
		}
	}
	return regions, nil
}

// CalculateElapsedTime calculates elapsed time since last flush
func (tc *TimeCalculator) CalculateElapsedTime(flushTime time.Time) (float64, error) {
	lastFlush := time.Time{}
	
	// Try to read last flush time
	if data, err := tc.fs.ReadFile(tc.lastFlushPath); err == nil {
		if t, err := tc.timeProvider.Parse(time.RFC3339Nano, string(data)); err == nil {
			lastFlush = t
		}
	}
	
	// If no last flush time, try init file
	if lastFlush.IsZero() {
		initPath := filepath.Join(tc.baseDir, tc.initFilePath)
		if data, err := tc.fs.ReadFile(initPath); err == nil {
			if t, err := tc.timeProvider.Parse(time.RFC3339Nano, string(data)); err == nil {
				lastFlush = t
			}
		}
	}
	
	// Default to 60 seconds ago if no timestamp found
	if lastFlush.IsZero() {
		lastFlush = flushTime.Add(-60 * time.Second)
	}
	
	elapsed := flushTime.Sub(lastFlush).Seconds()
	if elapsed <= 0 {
		elapsed = 60
	}
	
	return elapsed, nil
}

// SaveFlushTime saves the flush timestamp
func (tc *TimeCalculator) SaveFlushTime(flushTime time.Time) error {
	if err := tc.fs.WriteFile(tc.lastFlushPath,
		[]byte(flushTime.Format(time.RFC3339Nano)),
		0o644,
	); err != nil {
		tc.logger.Error("write lastFlushTimestamp %s: %v", tc.lastFlushPath, err)
		return fmt.Errorf("%w: %v", ErrTimeCalculation, err)
	}
	return nil
}

// ParseCounterKey parses a counter key into EMF dimensions
func ParseCounterKey(key string) [][]string {
	parts := strings.Split(key, ":")
	
	if len(parts) == 1 {
		// Simple case: just eventName
		return [][]string{{"eventName", parts[0]}}
	}
	
	if len(parts) >= 3 {
		// Complex case: eventName:invokerType:invokerName
		eventName := parts[0]
		invoker := strings.Join(parts[1:], ":")
		
		return [][]string{
			{"eventName", eventName},
			{"invoker", invoker},
		}
	}
	
	// Fallback for malformed keys
	return [][]string{{"eventName", key}}
}

// FlushAll reads counters, creates EMF records, and flushes to CloudWatch
func (fb *CTFileBatcher) FlushAll(ctx context.Context, flushTime time.Time) error {
	// Calculate elapsed time
	elapsed, err := fb.timeCalc.CalculateElapsedTime(flushTime)
	if err != nil {
		fb.logger.Error("calculate elapsed time: %v", err)
		elapsed = 60 // Default fallback
	}

	// Process each region's counters
	regions, err := fb.counterMgr.GetRegions()
	if err != nil {
		fb.logger.Error("get regions: %v", err)
		return fmt.Errorf("%w: %v", ErrFlushFailed, err)
	}
	
	for _, region := range regions {
		counters, err := fb.counterMgr.ReadCounters(region)
		if err != nil {
			fb.logger.Error("read counters for region %s: %v", region, err)
			continue
		}

		// Create EMF records from counters
		records, err := fb.createEMFRecords(counters, elapsed, flushTime)
		if err != nil {
			fb.logger.Error("create EMF records for region %s: %v", region, err)
			continue
		}

		// Flush records
		if len(records) > 0 {
			if err := fb.emfFlusher.Flush(ctx, region, records); err != nil {
				fb.logger.Error("flush region %s: %v", region, err)
			} else {
				fb.counterMgr.ClearCounters(region)
			}
		}
	}

	// Save flush timestamp
	return fb.timeCalc.SaveFlushTime(flushTime)
}

// createEMFRecords creates EMF records from counter data
func (fb *CTFileBatcher) createEMFRecords(counters map[string]int, elapsed float64, flushTime time.Time) ([]builder.EMFRecord, error) {
	var records []builder.EMFRecord
	
	for key, count := range counters {
		if count == 0 {
			continue
		}

		// Calculate rate
		rate := float64(count) / elapsed

		// Parse dimensions from key
		dimensions := ParseCounterKey(key)

		// Create EMF record
		record, err := builder.Build(builder.EMFInput{
			Namespace:  fb.namespace,
			MetricName: fb.metricName,
			Value:      rate,
			Unit:       builder.MetricUnitCount,
			Dimensions: dimensions,
			Timestamp:  flushTime,
		}, fb.logger)
		if err != nil {
			fb.logger.Error("build EMF record: %v", err)
			continue
		}
		records = append(records, record)
	}
	
	return records, nil
}

// ExtractInvoker extracts the invoker identity from a CloudTrail event,
// returning a formatted string "<Type>:<Invoker>" based on AWS identity types.
func ExtractInvoker(ct types.CloudTrailEvent) string {
	id := ct.UserIdentity
	t := id.Type

	var inv string
	switch t {
	case "Root":
		// no userName field; ARN ends in alias or account-id
		inv = lastSegment(id.ARN)
		if inv == "" {
			inv = id.AccountId
		}

	case "IAMUser":
		// userName isn't in our struct, so fall back to last ARN segment
		inv = lastSegment(id.ARN)

	case "AssumedRole", "Role":
		// try sessionIssuer.userName, else extract role name from ARN
		if id.SessionContext != nil && id.SessionContext.SessionIssuer.UserName != "" {
			inv = id.SessionContext.SessionIssuer.UserName
		} else {
			inv = extractAfterPrefix(id.ARN, "assumed-role")
		}

	case "FederatedUser":
		// same as above but look for "federated-user"
		if id.SessionContext != nil && id.SessionContext.SessionIssuer.UserName != "" {
			inv = id.SessionContext.SessionIssuer.UserName
		} else {
			inv = extractAfterPrefix(id.ARN, "federated-user")
		}

	case "Directory", "Unknown":
		// directory can include account alias or email in ARN
		inv = lastSegment(id.ARN)

	case "AWSService":
		inv = id.InvokedBy

	case "AWSAccount":
		inv = id.AccountId

	case "IdentityCenterUser":
		// if you have OnBehalfOf in SessionContext, use it; else fallback
		// assume we've extended SessionContext with OnBehalfOf.UserId
		if id.SessionContext != nil {
			type issuer = struct {
				UserId string
			}
			if raw, ok := any(id.SessionContext).(interface {
				GetOnBehalfOf() (issuer, bool)
			}); ok {
				if ob, found := raw.GetOnBehalfOf(); found && ob.UserId != "" {
					inv = ob.UserId
				}
			}
		}
		if inv == "" {
			inv = lastSegment(id.ARN)
		}

	default:
		// anything else
		inv = lastSegment(id.ARN)
	}

	if inv == "" {
		inv = "unknownInvoker"
	}
	return fmt.Sprintf("%s:%s", t, inv)
}

// lastSegment extracts the final segment after the last '/' in an ARN string.
func lastSegment(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractAfterPrefix finds "<prefix>/<name>" in arn and returns <name>.
// E.g. prefix="assumed-role", arn="arn:...:assumed-role/MyRole/..." → "MyRole".
// extractAfterPrefix finds a specific prefix in an ARN and returns the next segment.
// Used to extract role names from assumed-role or federated-user ARNs.
func extractAfterPrefix(arn, prefix string) string {
	parts := strings.Split(arn, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == prefix {
			return parts[i+1]
		}
	}
	return ""
}