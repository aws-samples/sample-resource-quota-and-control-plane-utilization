// internal/handlers/ratelimit_test.go
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/assert"

	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

//––– Mocks –––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––

// fakeBatcher records calls to Add(...)
type fakeBatcher struct {
	mu    sync.Mutex
	calls []struct {
		region string
		event  sharedtypes.CloudTrailEvent
	}
}

func (f *fakeBatcher) Add(_ context.Context, region string, ct sharedtypes.CloudTrailEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		region string
		event  sharedtypes.CloudTrailEvent
	}{region, ct})
}

// testLogger captures Error(...) messages
type testLogger struct {
	mu      sync.Mutex
	errMsgs []string
}

func (l *testLogger) Error(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errMsgs = append(l.errMsgs, fmt.Sprintf(format, args...))
}
func (l *testLogger) Info(format string, args ...interface{})  {}
func (l *testLogger) Debug(format string, args ...interface{}) {}
func (l *testLogger) Warn(format string, args ...interface{})  {}

//––– Tests ––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––

func TestNewRateLimitHandler_Errors(t *testing.T) {
	fake := &fakeBatcher{}
	// nil batcher
	_, err := NewRateLimitHandler(RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: nil,
		Namespace:                "ns",
	})
	assert.Error(t, err, "nil batcher should error")

	// empty namespace
	_, err = NewRateLimitHandler(RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: fake,
		Namespace:                "",
	})
	assert.Error(t, err, "empty namespace should error")
}

func TestNewRateLimitHandler_Success(t *testing.T) {
	fake := &fakeBatcher{}
	h, err := NewRateLimitHandler(RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: fake,
		Namespace:                "ns",
		// omit Logger to exercise default
	})
	assert.NoError(t, err)
	assert.NotNil(t, h)
}

func TestHandleEvent_NotInitialized(t *testing.T) {
	// zero value handler: initialized=false
	h := &RateLimitHandler{}
	failures, err := h.HandleEvent(context.Background(), events.SQSEvent{})
	assert.Error(t, err)
	assert.Nil(t, failures)
}

func TestHandleEvent_EmptyRecords(t *testing.T) {
	fake := &fakeBatcher{}
	h, _ := NewRateLimitHandler(RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: fake,
		Namespace:                "ns",
	})
	failures, err := h.HandleEvent(context.Background(), events.SQSEvent{Records: nil})
	assert.NoError(t, err)
	assert.Empty(t, failures)
	assert.Empty(t, fake.calls)
}

func TestHandleEvent_InvalidJSON(t *testing.T) {
	fake := &fakeBatcher{}
	h, _ := NewRateLimitHandler(RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: fake,
		Namespace:                "ns",
	})
	input := events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "msg-1", Body: "not-json"},
	}}
	failures, err := h.HandleEvent(context.Background(), input)
	assert.NoError(t, err)
	assert.Len(t, failures, 1)
	assert.Equal(t, "msg-1", failures[0].ItemIdentifier)
	assert.Empty(t, fake.calls)
}

func TestHandleEvent_ValidJSON(t *testing.T) {
	fake := &fakeBatcher{}
	h, _ := NewRateLimitHandler(RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: fake,
		Namespace:                "ns",
	})

	now := time.Now().UTC().Truncate(time.Second)
	cte := sharedtypes.CloudTrailEvent{
		AWSRegion: "r1", EventName: "e1", EventTime: now,
	}
	payload, _ := json.Marshal(cte)

	input := events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "msg-2", Body: string(payload)},
	}}
	failures, err := h.HandleEvent(context.Background(), input)
	assert.NoError(t, err)
	assert.Empty(t, failures)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Len(t, fake.calls, 1)
	assert.Equal(t, "r1", fake.calls[0].region)
	assert.Equal(t, cte, fake.calls[0].event)
}

func TestHandleEvent_Mixed(t *testing.T) {
	fake := &fakeBatcher{}
	h, _ := NewRateLimitHandler(RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: fake,
		Namespace:                "ns",
	})

	now := time.Now().UTC().Truncate(time.Second)
	cte := sharedtypes.CloudTrailEvent{
		AWSRegion: "rX", EventName: "eX", EventTime: now,
	}
	validJSON, _ := json.Marshal(cte)

	input := events.SQSEvent{Records: []events.SQSMessage{
		{MessageId: "bad", Body: "xxx"},
		{MessageId: "good", Body: string(validJSON)},
	}}
	failures, err := h.HandleEvent(context.Background(), input)
	assert.NoError(t, err)
	assert.Len(t, failures, 1)
	assert.Equal(t, "bad", failures[0].ItemIdentifier)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	assert.Len(t, fake.calls, 1)
	assert.Equal(t, "rX", fake.calls[0].region)
}

func TestLogAndReturnError(t *testing.T) {
	tl := &testLogger{}
	errIn := errors.New("boom")
	errOut := LogAndReturnError(errIn, tl)
	assert.Same(t, errIn, errOut)
	tl.mu.Lock()
	defer tl.mu.Unlock()
	// should have logged "Handler error: boom"
	assert.Contains(t, tl.errMsgs[0], "Handler error: boom")
}
