// Package cloudtrail provides file-based batching of CloudTrail events into EMF records.
// It handles event accumulation, threshold-based flushing, and aggregation for rate limiting metrics.
package cloudtrail

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// isValidRegion validates that the region string contains only safe characters
// and matches AWS region naming patterns to prevent path traversal attacks.
func isValidRegion(region string) bool {
	// AWS regions follow pattern: us-east-1, eu-west-2, etc.
	validRegion := regexp.MustCompile(`^[a-z0-9-]+$`)
	return len(region) > 0 && len(region) < 50 && validRegion.MatchString(region)
}

// Batcher defines the interface for batching CloudTrail events and flushing them as EMF records.
type Batcher interface {
	Add(ctx context.Context, region string, ct sharedTypes.CloudTrailEvent)
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

	emfFlusher emf.EMFFlusher
	logger     logger.Logger
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
}

// NewCTFileBatcher constructs a new file-based CloudTrail batcher with
// the provided configuration and initializes internal state.
func NewCTFileBatcher(cfg CTFileBatcherConfig) Batcher {
	if cfg.Logger == nil {
		cfg.Logger = logger.Get()
	}
	return &CTFileBatcher{
		baseDir:            cfg.BaseDir,
		namespace:          cfg.Namespace,
		metricName:         cfg.MetricName,
		lastFlushFilePath:  cfg.LastFlushFilePath,
		lambdaInitFilePath: cfg.LambdaInitFilePath,
		propagrateInvoker:  cfg.PropagateInvoker,
		emfFlusher:         cfg.EmfFlusher,
		logger:             cfg.Logger,
	}
}

// Add processes a CloudTrail event by creating EMF records, checking thresholds,
// writing to region-specific files, and triggering flushes when limits are exceeded.
func (fb *CTFileBatcher) Add(ctx context.Context, region string, ct sharedTypes.CloudTrailEvent) {
	if !isValidRegion(region) {
		fb.logger.Error("Add: invalid region name: %s", region)
		return
	}

	fb.logger.Debug("processing event: %+v", ct)

	// Generate counter keys
	keys := []string{ct.EventName} // Always have simple eventName counter

	if fb.propagrateInvoker {
		invoker := ExtractInvoker(ct)
		fb.logger.Debug("invoker=%s", invoker)
		invokerKey := fmt.Sprintf("%s:%s", ct.EventName, invoker)
		keys = append(keys, invokerKey)
	}

	// Update counters for each key
	for _, key := range keys {
		if err := fb.incrementCounter(region, key); err != nil {
			fb.logger.Error("increment counter %s/%s: %v", region, key, err)
		} else {
			fb.logger.Debug("incremented counter %s/%s", region, key)
		}
	}
}

// incrementCounter atomically updates the counter for a specific region and key.
func (fb *CTFileBatcher) incrementCounter(region, key string) error {
	filePath := filepath.Join(fb.baseDir, fmt.Sprintf("counters_%s.json", region))
	
	// Read existing counters
	counters, err := fb.readCounterFile(region)
	if err != nil {
		return err
	}
	
	// Increment counter
	counters[key]++
	
	// Write back atomically
	data, err := json.Marshal(counters)
	if err != nil {
		return err
	}
	
	// Atomic write pattern
	tempFile := filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return err
	}
	
	return os.Rename(tempFile, filePath)
}

// readCounterFile reads the counter file for a specific region.
func (fb *CTFileBatcher) readCounterFile(region string) (map[string]int, error) {
	filePath := filepath.Join(fb.baseDir, fmt.Sprintf("counters_%s.json", region))
	
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]int), nil // Return empty map if file doesn't exist
		}
		return nil, err
	}
	
	var counters map[string]int
	if err := json.Unmarshal(data, &counters); err != nil {
		return nil, err
	}
	
	return counters, nil
}

// parseCounterKey parses a counter key into EMF dimensions.
func (fb *CTFileBatcher) parseCounterKey(key string) [][]string {
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

// getRegions returns all regions that have counter files.
func (fb *CTFileBatcher) getRegions() []string {
	files, err := filepath.Glob(filepath.Join(fb.baseDir, "counters_*.json"))
	if err != nil {
		return nil
	}
	
	var regions []string
	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasPrefix(base, "counters_") && strings.HasSuffix(base, ".json") {
			region := base[9 : len(base)-5] // Remove "counters_" and ".json"
			regions = append(regions, region)
		}
	}
	return regions
}

// clearCounterFile removes the counter file for a region after successful flush.
func (fb *CTFileBatcher) clearCounterFile(region string) {
	filePath := filepath.Join(fb.baseDir, fmt.Sprintf("counters_%s.json", region))
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		fb.logger.Error("clear counter file %s: %v", filePath, err)
	}
}

// FlushAll reads counters, creates EMF records, and flushes to CloudWatch.
func (fb *CTFileBatcher) FlushAll(ctx context.Context, flushTime time.Time) error {
	// Calculate elapsed time
	lastFlush := time.Time{}
	if data, err := os.ReadFile(fb.lastFlushFilePath); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, string(data)); err == nil {
			lastFlush = t
		}
	}
	if lastFlush.IsZero() {
		initPath := filepath.Join(fb.baseDir, fb.lambdaInitFilePath)
		if data, err := os.ReadFile(initPath); err == nil {
			if t, err := time.Parse(time.RFC3339Nano, string(data)); err == nil {
				lastFlush = t
			}
		}
	}
	if lastFlush.IsZero() {
		lastFlush = flushTime.Add(-60 * time.Second)
	}

	elapsed := flushTime.Sub(lastFlush).Seconds()
	if elapsed <= 0 {
		elapsed = 60
	}

	// Process each region's counters
	regions := fb.getRegions()
	for _, region := range regions {
		counters, err := fb.readCounterFile(region)
		if err != nil {
			fb.logger.Error("read counters for region %s: %v", region, err)
			continue
		}

		// Create EMF records from counters
		var records []builder.EMFRecord
		for key, count := range counters {
			if count == 0 {
				continue
			}

			// Calculate rate
			rate := float64(count) / elapsed

			// Parse dimensions from key
			dimensions := fb.parseCounterKey(key)

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

		// Flush records
		if len(records) > 0 {
			if err := fb.emfFlusher.Flush(ctx, region, records); err != nil {
				fb.logger.Error("flush region %s: %v", region, err)
			} else {
				fb.clearCounterFile(region)
			}
		}
	}

	// Save flush timestamp
	if err := os.WriteFile(fb.lastFlushFilePath,
		[]byte(flushTime.Format(time.RFC3339Nano)),
		0o644,
	); err != nil {
		fb.logger.Error("write lastFlushTimestamp %s: %v", fb.lastFlushFilePath, err)
	}

	return nil
}

// ExtractInvoker extracts the invoker identity from a CloudTrail event,
// returning a formatted string "<Type>:<Invoker>" based on AWS identity types.
func ExtractInvoker(ct sharedTypes.CloudTrailEvent) string {
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