package cloudtrail

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

type Batcher interface {
	Add(ctx context.Context, region string, ct sharedTypes.CloudTrailEvent)
	FlushAll(ctx context.Context, time time.Time) error
}

// CTFileBatcher batches CloudTrail events into files and flushes aggregated EMFs.
type CTFileBatcher struct {
	baseDir            string
	namespace          string
	metricName         string
	lastFlushFilePath  string
	lambdaInitFilePath string
	maxCount           int
	maxBytes           int64
	propagrateInvoker  bool

	emfFlusher emf.EMFFlusher
	aggregator EMFAggregator
	logger     logger.Logger

	mu     sync.Mutex
	counts map[string]int
	sizes  map[string]int64
}

// CTFileBatcherConfig holds configuration for CTFileBatcher.
type CTFileBatcherConfig struct {
	BaseDir            string
	Namespace          string
	MetricName         string
	LastFlushFilePath  string
	LambdaInitFilePath string
	MaxCount           int
	MaxBytes           int64
	PropagateInvoker   bool
	Aggregator         EMFAggregator
	EmfFlusher         emf.EMFFlusher
	Logger             logger.Logger
}

// NewCTFileBatcher constructs a new file-based CloudTrail batcher.
func NewCTFileBatcher(cfg CTFileBatcherConfig) Batcher {
	if cfg.Logger == nil {
		cfg.Logger = logger.Get()
	}
	return &CTFileBatcher{
		baseDir:            cfg.BaseDir,
		namespace:          cfg.Namespace,
		metricName:         cfg.MetricName,
		maxCount:           cfg.MaxCount,
		maxBytes:           cfg.MaxBytes,
		lastFlushFilePath:  cfg.LastFlushFilePath,
		lambdaInitFilePath: cfg.LambdaInitFilePath,
		propagrateInvoker:  cfg.PropagateInvoker,
		emfFlusher:         cfg.EmfFlusher,
		aggregator:         cfg.Aggregator,
		logger:             cfg.Logger,
		counts:             make(map[string]int),
		sizes:              make(map[string]int64),
	}
}

// Add writes one (or two, if enabled) EMF records for a CloudTrail event,
// performs a pre‐add threshold check (flushing if necessary), then appends
// each record to the region’s NDJSON file, updates counters, and kicks off
// an async post‐add flush if thresholds are exceeded again.
func (fb *CTFileBatcher) Add(ctx context.Context, region string, ct sharedTypes.CloudTrailEvent) {
	// 1) Build EMF record(s)
	var records []builder.EMFRecord
	fb.logger.Debug("creating emf for event: %+v", ct)
	// overall event count
	overallRec, err := builder.Build(builder.EMFInput{
		Namespace:  fb.namespace,
		MetricName: fb.metricName,
		Value:      1,
		Unit:       builder.MetricUnitCount,
		Dimensions: [][]string{{"eventName", ct.EventName}},
		Timestamp:  ct.EventTime,
	}, fb.logger)
	if err != nil {
		fb.logger.Error("Add: build overall EMF error: %v", err)
		return
	}
	records = append(records, overallRec)
	fb.logger.Info("created emf record %s", string(overallRec.Payload))

	// optional per‐principal count
	if fb.propagrateInvoker {
		fb.logger.Debug("propagate invoker=%v. creating multi-dimensional metric", fb.propagrateInvoker)
		invoker := ExtractInvoker(ct)
		fb.logger.Debug("invoker=%s", invoker)
		princRec, err := builder.Build(builder.EMFInput{
			Namespace:  fb.namespace,
			MetricName: fb.metricName,
			Value:      1,
			Unit:       builder.MetricUnitCount,
			Dimensions: [][]string{
				{"eventName", ct.EventName},
				{"invoker", invoker},
			},
			Timestamp: ct.EventTime,
		}, fb.logger)
		if err != nil {
			fb.logger.Error("Add: build principal EMF error: %v", err)
		} else {
			records = append(records, princRec)
			fb.logger.Debug("created multi-dimensional emf record %s", string(princRec.Payload))
		}
	}

	// 2) Pre‐add threshold check
	fb.mu.Lock()
	currCount := fb.counts[region]
	currSize := fb.sizes[region]
	fb.mu.Unlock()

	// Calculate total after adding these records
	newCount := currCount + len(records)
	var newBytes int64
	for _, r := range records {
		newBytes += int64(len(r.Payload) + 1)
	}
	newSize := currSize + newBytes

	if (fb.maxCount > 0 && newCount > fb.maxCount) ||
		(fb.maxBytes > 0 && newSize > fb.maxBytes) {
		fb.logger.Info("Add: pre‐threshold reached for region %s (count %d/%d, size %d/%d), flushing first",
			region, newCount, fb.maxCount, newSize, fb.maxBytes)
		if err := fb.FlushAll(ctx, time.Now()); err != nil {
			fb.logger.Error("Add: flush error (pre‐threshold) for region %s: %v", region, err)
		}
		// reset our view for post‐flush
		currCount = 0
		currSize = 0
	}

	// 3) Write each record to file & update counters
	filePath := filepath.Join(fb.baseDir, fmt.Sprintf("emf_%s.ndjson", region))
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fb.logger.Error("Add: unable to open file %s: %v", filePath, err)
		return
	}
	defer f.Close()

	for _, r := range records {
		fb.logger.Debug("adding emf record %s", string(r.Payload))
		if _, err := f.Write(r.Payload); err != nil {
			fb.logger.Error("Add: write payload error: %v", err)
			continue
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			fb.logger.Error("Add: write newline error: %v", err)
			continue
		}
		recSize := int64(len(r.Payload) + 1)
		fb.mu.Lock()
		fb.counts[region]++
		fb.sizes[region] += recSize
		fb.mu.Unlock()
		fb.logger.Debug("successfully added emf record %s", string(r.Payload))
	}

	// 4) Post‐add threshold check (async flush)
	fb.mu.Lock()
	finalCount := fb.counts[region]
	finalSize := fb.sizes[region]
	fb.mu.Unlock()

	if (fb.maxCount > 0 && finalCount >= fb.maxCount) ||
		(fb.maxBytes > 0 && finalSize >= fb.maxBytes) {
		fb.logger.Info("Add: post‐threshold reached for region %s (count %d, size %d), flushing async",
			region, finalCount, finalSize)
		go func(r string) {
			if err := fb.FlushAll(ctx, time.Now()); err != nil {
				fb.logger.Error("Add: async flush error for region %s: %v", r, err)
			}
		}(region)
	}
}

func (fb *CTFileBatcher) FlushAll(ctx context.Context, flushTime time.Time) error {
	// 0a) Try the last‐flush file that was injected
	lastFlush := time.Time{}
	if data, err := os.ReadFile(fb.lastFlushFilePath); err == nil {
		if t, err := time.Parse(time.RFC3339Nano, string(data)); err == nil {
			lastFlush = t
			fb.logger.Debug("last flush file found. timestamp, %s", string(data))
		}
	}
	// 0b) Fallback: try the Lambda init timestamp file
	if lastFlush.IsZero() {
		initPath := filepath.Join(fb.baseDir, fb.lambdaInitFilePath)
		if data, err := os.ReadFile(initPath); err == nil {
			if t, err := time.Parse(time.RFC3339Nano, string(data)); err == nil {
				lastFlush = t
				fb.logger.Debug("falling back to lambda init fime, %s", string(data))
			}
		}
	}
	// 0c) Ultimate fallback: assume 60s
	if lastFlush.IsZero() {
		lastFlush = flushTime.Add(-60 * time.Second)
		fb.logger.Warn("last flush and init time empty.  using fallback to 60s")
	}

	// 1) Compute elapsed and guard against non‐positive
	elapsed := flushTime.Sub(lastFlush).Seconds()
	if elapsed <= 0 {
		elapsed = 60
	}
	fb.logger.Info("elapsed time=%.2f", elapsed)

	// 2) For each region: read file, aggregate with NormFactor = 1/elapsed, flush, reset
	regions := fb.snapshotRegions()
	for _, region := range regions {
		raw, err := fb.readRegionFile(region)
		if err != nil {
			fb.logger.Error("read region %s: %v", region, err)
			continue
		}
		cfg := AggregateConfig{
			Namespace:  fb.namespace,
			MetricName: fb.metricName,
			NormFactor: 1.0 / elapsed,
			FlushTime:  flushTime,
			Logger:     fb.logger,
		}
		batch, err := fb.aggregator.Aggregate(ctx, raw, cfg)
		if err != nil {
			fb.logger.Error("aggregate region %s: %v", region, err)
			continue
		}
		if err := fb.emfFlusher.Flush(ctx, region, batch); err != nil {
			fb.logger.Error("flush region %s: %v", region, err)
		}
		fb.resetRegion(region)
	}

	// 3) Persist this flush as the next “lastFlush”
	if err := os.WriteFile(fb.lastFlushFilePath,
		[]byte(flushTime.Format(time.RFC3339Nano)),
		0o644,
	); err != nil {
		fb.logger.Error("write lastFlushTimestamp %s: %v", fb.lastFlushFilePath, err)
	}

	return nil
}

// snapshotRegions returns a slice of all regions currently tracked.
func (fb *CTFileBatcher) snapshotRegions() []string {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	regions := make([]string, 0, len(fb.counts))
	for r := range fb.counts {
		regions = append(regions, r)
	}
	return regions
}

// readRegionFile reads the NDJSON file for one region and reconstructs EMFRecords,
// extracting dimensions and timestamps from the JSON payload.
func (fb *CTFileBatcher) readRegionFile(region string) ([]builder.EMFRecord, error) {
	path := filepath.Join(fb.baseDir, fmt.Sprintf("emf_%s.ndjson", region))
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []builder.EMFRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()

		// parse into a generic map
		var doc map[string]any
		if err := json.Unmarshal(line, &doc); err != nil {
			continue
		}

		// pull out the _aws block
		awsBlock, ok := doc["_aws"].(map[string]any)
		if !ok {
			continue
		}

		// extract timestamp (ms)
		tsVal, ok := awsBlock["Timestamp"].(float64)
		if !ok {
			continue
		}
		ts := time.UnixMilli(int64(tsVal))

		// extract the dimension names array
		cwMetrics, ok := awsBlock["CloudWatchMetrics"].([]any)
		if !ok || len(cwMetrics) == 0 {
			continue
		}
		metricDef, ok := cwMetrics[0].(map[string]any)
		if !ok {
			continue
		}
		dimsAny, ok := metricDef["Dimensions"].([]any)
		if !ok || len(dimsAny) == 0 {
			continue
		}
		nameList, ok := dimsAny[0].([]any)
		if !ok {
			continue
		}

		// rebuild [][]string from names + doc[name] values
		var dims [][]string
		for _, n := range nameList {
			name, ok := n.(string)
			if !ok {
				continue
			}
			val := fmt.Sprint(doc[name])
			dims = append(dims, []string{name, val})
		}

		records = append(records, builder.EMFRecord{
			Payload:    append([]byte(nil), line...), // clone
			TimeStamp:  ts,
			Dimensions: dims,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// resetRegion truncates the region file and zeroes out counts/sizes.
func (fb *CTFileBatcher) resetRegion(region string) {
	path := filepath.Join(fb.baseDir, fmt.Sprintf("emf_%s.ndjson", region))
	if err := os.Truncate(path, 0); err != nil {
		fb.logger.Error("resetRegion: truncate %s: %v", path, err)
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.counts[region] = 0
	fb.sizes[region] = 0
}

// ExtractInvoker returns a string of the form "<Type>:<Invoker>"
// where Invoker is the minimal identifier for the caller, chosen
// according to AWS docs. If nothing can be found, returns "<Type>:unknownInvoker".
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
		// userName isn’t in our struct, so fall back to last ARN segment
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

// lastSegment returns everything after the final '/' in arn.
func lastSegment(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractAfterPrefix finds "<prefix>/<name>" in arn and returns <name>.
// E.g. prefix="assumed-role", arn="arn:...:assumed-role/MyRole/..." → "MyRole".
func extractAfterPrefix(arn, prefix string) string {
	parts := strings.Split(arn, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == prefix {
			return parts[i+1]
		}
	}
	return ""
}
