// internal/emfbatcher/cloudtrail/ct_filebatcher.go
package cloudtrailemfbatcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"slices"

	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	appLogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

type EMFFileBatcher interface {
	Add(ctx context.Context, region string, event sharedtypes.CloudTrailEvent)
}

// CTFileBatcher handles CloudTrail events by:
// 1) converting each event to an EMFRecord,
// 2) immediately appending the EMF JSON to per-region files,
// 3) tracking in-memory counts and byte-sizes of EMFRecords per region,
// 4) flushing (reading and pushing EMFRecords) when:
//      - record count >= maxCount,
//      - total bytes >= maxBytes,
//      - flushInterval elapses,
// 5) gracefully stopping and flushing all regions on context cancellation.

type CTFileBatcher struct {
	namespace  string
	metricName string

	baseDir       string
	maxCount      int
	maxBytes      int64
	flushInterval time.Duration

	// multi region emf flusher
	emfFlusher emf.EMFFlusher

	logger appLogger.Logger

	mu     sync.Mutex
	counts map[string]int   // region -> EMF record count
	sizes  map[string]int64 // region -> EMF byte-size

	ticker *time.Ticker
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCTFileBatcher creates a new CTFileBatcher that writes EMFRecords to disk
// and flushes via the provided EMF flushers per region.

type CTFileBatcherConfig struct {
	ParentCtx     context.Context
	Namespace     string
	MetricName    string
	BaseDir       string
	MaxCount      int
	MaxBytes      int64
	FlushInterval time.Duration
	EmfFlusher    emf.EMFFlusher
	Logger        appLogger.Logger
}

func NewCTFileBatcher(
	config CTFileBatcherConfig) *CTFileBatcher {
	if config.Logger == nil {
		config.Logger = appLogger.Get()
	}
	ctx, cancel := context.WithCancel(config.ParentCtx)
	fb := &CTFileBatcher{
		namespace:     config.Namespace,
		metricName:    config.MetricName,
		baseDir:       config.BaseDir,
		maxCount:      config.MaxCount,
		maxBytes:      config.MaxBytes,
		flushInterval: config.FlushInterval,
		emfFlusher:    config.EmfFlusher,
		logger:        config.Logger,
		counts:        make(map[string]int),
		sizes:         make(map[string]int64),
		ticker:        time.NewTicker(config.FlushInterval),
		ctx:           ctx,
		cancel:        cancel,
	}
	// Start periodic flush
	go fb.startTicker()
	return fb
}

// Add converts a CloudTrailEvent to EMFRecord, writes its JSON to disk,
// updates counters, and triggers flushes BEFORE and AFTER writing if thresholds
// would be or are exceeded.
func (fb *CTFileBatcher) Add(ctx context.Context, region string, ct sharedtypes.CloudTrailEvent) {
	// 1) convert to EMF
	emfRec, err := emf.Build(emf.EMFInput{
		Namespace:  fb.namespace,
		MetricName: fb.metricName,
		Value:      1,
		Unit:       emf.MetricUnitCount,
		Dimensions: [][]string{{"eventName", ct.EventName}},
		Timestamp:  ct.EventTime,
	}, fb.logger)
	if err != nil {
		fb.logger.Error("EMF build failed: %v", err)
		return
	}
	data := emfRec.Payload
	recSize := int64(len(data) + 1) // include newline

	// 2) pre-add threshold check: if adding would overflow, flush now
	fb.mu.Lock()
	prevCount := fb.counts[region]
	prevSize := fb.sizes[region]
	fb.mu.Unlock()

	if (fb.maxCount > 0 && prevCount+1 > fb.maxCount) ||
		(fb.maxBytes > 0 && prevSize+recSize > fb.maxBytes) {
		fb.logger.Info("threshold reached for region %s before add; flushing", region)
		fb.flushRegion(region)
	}

	// 3) write new EMF JSON to file
	path := filepath.Join(fb.baseDir, fmt.Sprintf("emf_%s.ndjson", region))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fb.logger.Error("unable to open file for region %s: %v", region, err)
		return
	}
	f.Write(data)
	f.Write([]byte("\n"))
	f.Close()

	// 4) update counters
	fb.mu.Lock()
	fb.counts[region]++
	fb.sizes[region] += recSize
	newCount := fb.counts[region]
	newSize := fb.sizes[region]
	fb.mu.Unlock()

	// 5) post-add threshold flush if exactly hit
	if (fb.maxCount > 0 && newCount >= fb.maxCount) ||
		(fb.maxBytes > 0 && newSize >= fb.maxBytes) {
		fb.logger.Info("threshold reached for region %s after add; flushing asynchronously", region)
		fb.flushRegion(region)
	}
}

// Stop cancels periodic flushes and flushes all regions once.
func (fb *CTFileBatcher) Stop() {
	fb.ticker.Stop()

	// snapshot regions
	fb.mu.Lock()
	regions := make([]string, 0, len(fb.counts))
	for r := range fb.counts {
		regions = append(regions, r)
	}
	fb.mu.Unlock()

	// flush in parallel
	var wg sync.WaitGroup
	for _, region := range regions {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			fb.flushRegion(r)
		}(region)
	}
	wg.Wait()
	fb.cancel()
}

// startTicker periodically flushes all regions until context is canceled.
func (fb *CTFileBatcher) startTicker() {
	for {
		select {
		case <-fb.ctx.Done():
			return
		case <-fb.ticker.C:
			// snapshot regions
			fb.mu.Lock()
			regions := make([]string, 0, len(fb.counts))
			for r := range fb.counts {
				regions = append(regions, r)
			}
			fb.mu.Unlock()

			// flush in parallel
			var wg sync.WaitGroup
			for _, region := range regions {
				wg.Add(1)
				go func(r string) {
					defer wg.Done()
					fb.flushRegion(r)
				}(region)
			}
			wg.Wait()
		}
	}
}

// flushRegion reads EMF JSON lines, parses timestamps, invokes the EMF flusher,
// truncates the file, and resets counters.
func (fb *CTFileBatcher) flushRegion(region string) {
	// respect cancellation
	select {
	case <-fb.ctx.Done():
		return
	default:
	}

	path := filepath.Join(fb.baseDir, fmt.Sprintf("emf_%s.ndjson", region))
	f, err := os.Open(path)
	if err != nil {
		fb.logger.Error("cannot open file for flush for region %s: %v", region, err)
		return
	}
	scanner := bufio.NewScanner(f)
	var batch []emf.EMFRecord
	for scanner.Scan() {
		line := scanner.Bytes()
		fb.logger.Debug("flushing line: %s", string(line))
		var env struct {
			AWS struct {
				Timestamp int64 `json:"Timestamp"`
			} `json:"_aws"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		rec := emf.EMFRecord{Payload: slices.Clone(line), TimeStamp: time.UnixMilli(env.AWS.Timestamp)}
		batch = append(batch, rec)
	}
	f.Close()
	if err := scanner.Err(); err != nil {
		fb.logger.Error("error reading file for region %s: %v", region, err)
	}

	if len(batch) > 0 {
		fb.emfFlusher.Flush(fb.ctx, region, batch)
	}

	// truncate & reset
	if err := os.Truncate(path, 0); err != nil {
		fb.logger.Error("failed to truncate file for region %s: %v", region, err)
	}
	fb.mu.Lock()
	fb.counts[region] = 0
	fb.sizes[region] = 0
	fb.mu.Unlock()
}
