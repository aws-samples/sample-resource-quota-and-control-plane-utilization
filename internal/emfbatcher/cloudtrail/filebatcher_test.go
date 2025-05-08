// internal/emfbatcher/cloudtrail/ct_filebatcher_test.go
package cloudtrailemfbatcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// fakeFlusher records every Flush call.
type fakeFlusher struct {
	mu    sync.Mutex
	calls map[string][][]emf.EMFRecord
}

func newFakeFlusher() *fakeFlusher {
	return &fakeFlusher{calls: make(map[string][][]emf.EMFRecord)}
}

func (f *fakeFlusher) Flush(_ context.Context, region string, batch []emf.EMFRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[region] = append(f.calls[region], batch)
	return nil
}

func TestCTFileBatcher_CountThreshold(t *testing.T) {
	temp := t.TempDir()
	ff := newFakeFlusher()

	// maxCount=2, no byte limit, no periodic flush
	batcher := NewCTFileBatcher(CTFileBatcherConfig{
		ParentCtx:     context.Background(),
		Namespace:     "N",
		MetricName:    "M",
		BaseDir:       temp,
		MaxCount:      2,
		MaxBytes:      0,
		FlushInterval: time.Hour,
		EmfFlusher:    ff,
		Logger:        logger.Get(),
	})

	// 1st Add: below threshold → no flush
	batcher.Add(context.Background(), "us-west-2", sharedtypes.CloudTrailEvent{
		EventName: "E1", AWSRegion: "us-west-2", EventTime: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)
	assert.Empty(t, ff.calls["us-west-2"], "should not have flushed on first add")

	// 2nd Add: hits threshold → post-add flush
	batcher.Add(context.Background(), "us-west-2", sharedtypes.CloudTrailEvent{
		EventName: "E2", AWSRegion: "us-west-2", EventTime: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	ff.mu.Lock()
	defer ff.mu.Unlock()
	assert.Len(t, ff.calls["us-west-2"], 1, "should have flushed once on hitting count")
	assert.Len(t, ff.calls["us-west-2"][0], 2, "batch should contain exactly 2 records")
}

func TestCTFileBatcher_PeriodicFlush(t *testing.T) {
	temp := t.TempDir()
	ff := newFakeFlusher()

	// no count/byte limit, short flush interval
	batcher := NewCTFileBatcher(CTFileBatcherConfig{
		ParentCtx:     context.Background(),
		Namespace:     "N",
		MetricName:    "M",
		BaseDir:       temp,
		MaxCount:      0,
		MaxBytes:      0,
		FlushInterval: 50 * time.Millisecond,
		EmfFlusher:    ff,
		Logger:        logger.Get(),
	})

	// Add one event
	batcher.Add(context.Background(), "eu-central-1", sharedtypes.CloudTrailEvent{
		EventName: "E", AWSRegion: "eu-central-1", EventTime: time.Now(),
	})

	// wait twice the interval
	time.Sleep(120 * time.Millisecond)

	ff.mu.Lock()
	defer ff.mu.Unlock()
	assert.GreaterOrEqual(t, len(ff.calls["eu-central-1"]), 1, "should have at least one periodic flush")

	batcher.Stop()
}

func TestCTFileBatcher_StopFlushAll(t *testing.T) {
	temp := t.TempDir()
	ff1 := newFakeFlusher()

	batcher := NewCTFileBatcher(CTFileBatcherConfig{
		ParentCtx:     context.Background(),
		Namespace:     "N",
		MetricName:    "M",
		BaseDir:       temp,
		MaxCount:      100,       // high so no auto flush
		MaxBytes:      1 << 20,   // high
		FlushInterval: time.Hour, // no periodic
		EmfFlusher:    ff1,       // we’ll reuse the same flusher for both regions
		Logger:        logger.Get(),
	})

	// store both regions in the fake flusher’s map
	ff1.calls = map[string][][]emf.EMFRecord{
		"us-east-1":  nil,
		"ap-south-1": nil,
	}

	// Add events in two regions
	batcher.Add(context.Background(), "us-east-1", sharedtypes.CloudTrailEvent{EventName: "A", AWSRegion: "us-east-1", EventTime: time.Now()})
	batcher.Add(context.Background(), "ap-south-1", sharedtypes.CloudTrailEvent{EventName: "B", AWSRegion: "ap-south-1", EventTime: time.Now()})

	// Stop should flush both regions once
	batcher.Stop()

	ff1.mu.Lock()
	defer ff1.mu.Unlock()
	assert.Len(t, ff1.calls["us-east-1"], 1, "Stop should flush us-east-1")
	assert.Len(t, ff1.calls["ap-south-1"], 1, "Stop should flush ap-south-1")
}

func TestCTFileBatcher_FileWriteAndTruncate(t *testing.T) {
	temp := t.TempDir()
	ff := newFakeFlusher()

	batcher := NewCTFileBatcher(CTFileBatcherConfig{
		ParentCtx:     context.Background(),
		Namespace:     "N",
		MetricName:    "M",
		BaseDir:       temp,
		MaxCount:      1, // immediate post-add flush
		MaxBytes:      0,
		FlushInterval: time.Hour, // no periodic
		EmfFlusher:    ff,
		Logger:        logger.Get(),
	})

	// Add one event, triggers flush
	batcher.Add(context.Background(), "ca-central-1", sharedtypes.CloudTrailEvent{
		EventName: "X", AWSRegion: "ca-central-1", EventTime: time.Now(),
	})
	time.Sleep(10 * time.Millisecond)

	// After flush, file should be truncated to zero length
	path := filepath.Join(temp, "emf_ca-central-1.ndjson")
	info, err := os.Stat(path)
	assert.NoError(t, err)
	assert.Zero(t, info.Size(), "file must be truncated after flush")
}
