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
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// Error constants for better error handling and testing
var (
	ErrInvalidRegion    = errors.New("invalid region name")
	ErrCounterFileRead  = errors.New("failed to read counter file")
	ErrCounterFileWrite = errors.New("failed to write counter file")
	ErrInvalidConfig    = errors.New("invalid batcher configuration")
	ErrFlushFailed      = errors.New("failed to flush records")
	ErrTimeCalculation  = errors.New("failed to calculate elapsed time")
	ErrBaseDirEmpty     = errors.New("base directory is empty")
	ErrNamespaceEmpty   = errors.New("namespace is empty")
	ErrMetricNameEmpty  = errors.New("metric name is empty")
	ErrEMFFlusherNil    = errors.New("EMF flusher is nil")
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
	AddCounters(ctx context.Context, counters map[string]map[string]int) error
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

// PropagateInvoker returns whether to propagate invoker information
func (fb *CTFileBatcher) PropagateInvoker() bool {
	return fb.propagrateInvoker
}

// CounterManager handles counter file operations
type CounterManager struct {
	fs       FileSystem
	baseDir  string
	filePath string
	logger   logger.Logger
}

// NewCounterManager creates a new counter manager
func NewCounterManager(fs FileSystem, baseDir string, logger logger.Logger) *CounterManager {
	return &CounterManager{
		fs:       fs,
		baseDir:  baseDir,
		filePath: filepath.Join(baseDir, "counters.json"),
		logger:   logger,
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
	FileSystem         FileSystem   // Optional, defaults to DefaultFileSystem
	TimeProvider       TimeProvider // Optional, defaults to DefaultTimeProvider
}

// Validate checks if the configuration is valid
func (cfg CTFileBatcherConfig) Validate() error {
	if cfg.BaseDir == "" {
		return ErrBaseDirEmpty
	}
	if cfg.Namespace == "" {
		return ErrNamespaceEmpty
	}
	if cfg.MetricName == "" {
		return ErrMetricNameEmpty
	}
	if cfg.EmfFlusher == nil {
		return ErrEMFFlusherNil
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

// AddCounters adds multiple counters at once for multiple regions
func (fb *CTFileBatcher) AddCounters(ctx context.Context, counters map[string]map[string]int) error {
	// Validate regions
	for region := range counters {
		if !utils.IsValidRegion(region) {
			fb.logger.Error("AddCounters: %v: %s", ErrInvalidRegion, region)
			delete(counters, region) // Remove invalid region
		}
	}
	
	// Add all counters in a single operation
	if err := fb.counterMgr.AddCounters(counters); err != nil {
		fb.logger.Error("add counters: %v", err)
		return err
	}
	
	fb.logger.Debug("added batch counters for %d regions", len(counters))
	return nil
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

// ReadCounters reads all counters from the single counter file
func (cm *CounterManager) ReadCounters() (map[string]map[string]int, error) {
	data, err := cm.fs.ReadFile(cm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]map[string]int), nil // Return empty map if file doesn't exist
		}
		return nil, fmt.Errorf("%w: %v", ErrCounterFileRead, err)
	}

	var counters map[string]map[string]int
	if err := json.Unmarshal(data, &counters); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %v", ErrCounterFileRead, err)
	}

	return counters, nil
}

// ReadRegionCounters reads counters for a specific region
func (cm *CounterManager) ReadRegionCounters(region string) (map[string]int, error) {
	allCounters, err := cm.ReadCounters()
	if err != nil {
		return nil, err
	}
	
	if regionCounters, exists := allCounters[region]; exists {
		return regionCounters, nil
	}
	
	return make(map[string]int), nil
}

// WriteCounters writes all counters to the single counter file
func (cm *CounterManager) WriteCounters(counters map[string]map[string]int) error {
	data, err := json.Marshal(counters)
	if err != nil {
		return fmt.Errorf("%w: marshal failed: %v", ErrCounterFileWrite, err)
	}

	// Atomic write pattern
	tempFile := cm.filePath + ".tmp"
	if err := cm.fs.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("%w: temp file write failed: %v", ErrCounterFileWrite, err)
	}

	if err := cm.fs.Rename(tempFile, cm.filePath); err != nil {
		return fmt.Errorf("%w: atomic rename failed: %v", ErrCounterFileWrite, err)
	}

	return nil
}

// IncrementCounter atomically increments a counter for a specific region and key
func (cm *CounterManager) IncrementCounter(region, key string) error {
	allCounters, err := cm.ReadCounters()
	if err != nil {
		return err
	}

	// Ensure region map exists
	if allCounters[region] == nil {
		allCounters[region] = make(map[string]int)
	}

	// Increment counter
	allCounters[region][key]++

	return cm.WriteCounters(allCounters)
}

// MergeCounters merges new counters into existing counters
func (cm *CounterManager) MergeCounters(existing, new map[string]map[string]int) map[string]map[string]int {
	// Start with a copy of existing data
	result := make(map[string]map[string]int)
	
	// Copy all existing data first
	for region, counters := range existing {
		result[region] = make(map[string]int)
		for key, count := range counters {
			result[region][key] = count
		}
	}
	
	// Then add/update only the keys in the new data
	for region, counters := range new {
		if result[region] == nil {
			result[region] = make(map[string]int)
		}
		for key, count := range counters {
			result[region][key] += count
		}
	}
	
	return result
}

// AddCounters adds multiple counters at once
func (cm *CounterManager) AddCounters(newCounters map[string]map[string]int) error {
	// Read existing counters
	existingCounters, err := cm.ReadCounters()
	if err != nil {
		return err
	}
	
	// Merge counters
	mergedCounters := cm.MergeCounters(existingCounters, newCounters)
	
	// Write back to file
	return cm.WriteCounters(mergedCounters)
}

// ClearRegionCounters removes a region's counters from the file
func (cm *CounterManager) ClearRegionCounters(region string) error {
	allCounters, err := cm.ReadCounters()
	if err != nil {
		return err
	}
	
	// Remove the region
	delete(allCounters, region)
	
	// Write back to file
	return cm.WriteCounters(allCounters)
}

// ClearAllCounters removes the counter file entirely
func (cm *CounterManager) ClearAllCounters() error {
	if err := cm.fs.Remove(cm.filePath); err != nil && !os.IsNotExist(err) {
		cm.logger.Error("clear counter file %s: %v", cm.filePath, err)
		return err
	}
	return nil
}

// GetRegions returns all regions that have counters
func (cm *CounterManager) GetRegions() ([]string, error) {
	allCounters, err := cm.ReadCounters()
	if err != nil {
		return nil, err
	}
	
	regions := make([]string, 0, len(allCounters))
	for region := range allCounters {
		regions = append(regions, region)
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

	// Read all counters
	allCounters, err := fb.counterMgr.ReadCounters()
	if err != nil {
		fb.logger.Error("read counters: %v", err)
		return fmt.Errorf("%w: %v", ErrFlushFailed, err)
	}
	
	// Process each region's counters
	for region, counters := range allCounters {
		if !utils.IsValidRegion(region) {
			fb.logger.Error("invalid region in counters: %s", region)
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
				// Remove this region's counters
				delete(allCounters, region)
			}
		}
	}
	
	// Write back remaining counters (if any)
	if len(allCounters) > 0 {
		if err := fb.counterMgr.WriteCounters(allCounters); err != nil {
			fb.logger.Error("write remaining counters: %v", err)
		}
	} else {
		// All regions were flushed, clear the file
		if err := fb.counterMgr.ClearAllCounters(); err != nil {
			fb.logger.Error("clear all counters: %v", err)
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
			// amazonq-ignore-next-line
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
