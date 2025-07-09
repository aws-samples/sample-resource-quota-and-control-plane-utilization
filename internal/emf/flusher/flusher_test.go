package flusher

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/stretchr/testify/assert"
)

// getCallCount uses reflection to read the private callPutLogEventsCount
func getCallCount(f *cwlclient.FakeCloudWatchLogsClient) int {
	v := reflect.ValueOf(f).Elem().FieldByName("callPutLogEventsCount")
	return int(v.Int())
}

func TestEMFFlusher_FlushScenarios(t *testing.T) {
	ctx := context.Background()
	lg, ls := "lg", "ls"
	clientMap := safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}
	// 1) Empty batch: no calls
	fake1 := &cwlclient.FakeCloudWatchLogsClient{Region: "r1"}
	clientMap.Store("r1", fake1)
	fl1 := NewEMFFlusher(EMFFlusherConfig{CwlClientMap: &clientMap, LogGroupName: lg, LogStreamName: ls, Logger: &logger.NoopLogger{}})
	err := fl1.Flush(ctx, "r1", []builder.EMFRecord{})
	assert.NoError(t, err)
	assert.Equal(t, 0, getCallCount(fake1), "empty batch should not call PutLogEvents")

	// 2) Missing client: returns error
	fl2 := NewEMFFlusher(EMFFlusherConfig{CwlClientMap: &safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}, LogGroupName: lg, LogStreamName: ls, Logger: &logger.NoopLogger{}})
	err = fl2.Flush(ctx, "missing", []builder.EMFRecord{{Payload: []byte("x"), TimeStamp: time.Now()}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no client for region missing")

	// 3) Successful flush: increments call count
	fake3 := &cwlclient.FakeCloudWatchLogsClient{Region: "r3"}
	clientMap.Store("r3", fake3)
	fl3 := NewEMFFlusher(EMFFlusherConfig{CwlClientMap: &clientMap, LogGroupName: lg, LogStreamName: ls, Logger: &logger.NoopLogger{}})
	recOld := builder.EMFRecord{Payload: []byte("old"), TimeStamp: time.Now().Add(-time.Second)}
	recNew := builder.EMFRecord{Payload: []byte("new"), TimeStamp: time.Now()}
	err = fl3.Flush(ctx, "r3", []builder.EMFRecord{recNew, recOld})
	assert.NoError(t, err)
	assert.Equal(t, 1, getCallCount(fake3), "successful flush should call PutLogEvents once")

	// 4) Client error propagates
	fake4 := &cwlclient.FakeCloudWatchLogsClient{Region: "r4", ErrPutLogEvents: true}
	clientMap.Store("r4", fake4)
	fl4 := NewEMFFlusher(EMFFlusherConfig{CwlClientMap: &clientMap, LogGroupName: lg, LogStreamName: ls, Logger: &logger.NoopLogger{}})
	err = fl4.Flush(ctx, "r4", []builder.EMFRecord{{Payload: []byte("p"), TimeStamp: time.Now()}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PutLogEvents injected error")
	assert.Equal(t, 1, getCallCount(fake4), "even on error, PutLogEvents should be called")
}

func TestMakeFlushFuncScenarios(t *testing.T) {
	ctx := context.Background()
	lg, ls := "LG", "LS"
	// Use FakeCloudWatchLogsClient directly
	fake := &cwlclient.FakeCloudWatchLogsClient{Region: "rX"}
	flushFn := MakeFlushFunc[builder.EMFRecord](
		fake,
		lg,
		ls,
		func(r builder.EMFRecord) []byte { return r.Payload },
		func(r builder.EMFRecord) int64 { return r.TimeStamp.UnixMilli() },
		&logger.NoopLogger{},
	)

	// Empty batch: no call
	err := flushFn(ctx, []builder.EMFRecord{})
	assert.NoError(t, err)
	assert.Equal(t, 0, getCallCount(fake), "empty batch should not call PutLogEvents")

	// Non-empty batch: one call
	t0 := time.Now()
	recs := []builder.EMFRecord{
		{Payload: []byte("a"), TimeStamp: t0},
	}
	err = flushFn(ctx, recs)
	assert.NoError(t, err)
	assert.Equal(t, 1, getCallCount(fake), "flushFn should call PutLogEvents once for batch")

	// Order doesn't matter for this mock, but count should increment
	fake.Reset()
	recs2 := []builder.EMFRecord{
		{Payload: []byte("x"), TimeStamp: t0.Add(time.Second)},
		{Payload: []byte("y"), TimeStamp: t0.Add(-time.Second)},
	}
	err = flushFn(ctx, recs2)
	assert.NoError(t, err)
	assert.Equal(t, 1, getCallCount(fake), "flushFn should call PutLogEvents once per batch")

	// Error propagation
	fake.ErrPutLogEvents = true
	fake.Reset()
	err = flushFn(ctx, recs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "emf flusher: PutLogEvents injected error")
	assert.Equal(t, 1, getCallCount(fake), "call count even on error")
}
