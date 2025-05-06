package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/handlers"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
)

// mockLogger implements logger.Logger with no-op methods
type mockLogger struct{}

func (l *mockLogger) Info(format string, args ...interface{})  {}
func (l *mockLogger) Debug(format string, args ...interface{}) {}
func (l *mockLogger) Error(format string, args ...interface{}) {}
func (l *mockLogger) Warn(format string, args ...interface{})  {}

// stubFusher implements emf.EMFFusher
type stubFusher struct {
	failRegions map[string]bool
}

func (s *stubFusher) Flush(ctx context.Context, region string, batch []emf.EMFRecord) error {
	if s.failRegions != nil && s.failRegions[region] {
		return errors.New("flush error")
	}
	return nil
}

// makeSQSEvent creates an SQSEvent with given regions and timestamps
func makeSQSEvent(regions []string, times []time.Time) events.SQSEvent {
	records := make([]events.SQSMessage, len(regions))
	for i, region := range regions {
		evt := sharedtypes.CloudTrailEvent{
			AWSRegion: region,
			EventTime: times[i],
		}
		body, _ := json.Marshal(evt)
		records[i] = events.SQSMessage{
			MessageId: region + "-id",
			Body:      string(body),
		}
	}
	return events.SQSEvent{Records: records}
}
func TestHandleEvent_NoRecords(t *testing.T) {
	handler := &handlers.RateLimitHandler{
		Logger:    &mockLogger{},
		EMFFusher: &stubFusher{},
		Namespace: "ns",
	}
	failures, err := handler.HandleEvent(context.Background(), events.SQSEvent{Records: []events.SQSMessage{}})
	assert.NoError(t, err, "No error expected for empty record set")
	assert.Nil(t, failures, "Failures should be nil when no records")
}
func TestHandleEvent_InvalidJSON(t *testing.T) {
	handler := &handlers.RateLimitHandler{
		Logger:    &mockLogger{},
		EMFFusher: &stubFusher{},
		Namespace: "ns",
	}
	now := time.Now()
	validEvt := sharedtypes.CloudTrailEvent{AWSRegion: "us-east-1", EventTime: now}
	validBody, _ := json.Marshal(validEvt)
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "bad-id", Body: "not-json"},
			{MessageId: "us-east-1-id", Body: string(validBody)},
		},
	}
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	assert.NoError(t, err, "No error expected when skipping invalid JSON")
	assert.Empty(t, failures, "All valid records should flush successfully")
}
func TestHandleEvent_FlushSuccessSingle(t *testing.T) {
	now := time.Now()
	sqsEvent := makeSQSEvent([]string{"us-west-2"}, []time.Time{now})
	handler := &handlers.RateLimitHandler{
		Logger:    &mockLogger{},
		EMFFusher: &stubFusher{},
		Namespace: "ns",
	}
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	assert.NoError(t, err, "No error expected when flush succeeds")
	assert.Empty(t, failures, "No failures expected on successful flush")
}
func TestHandleEvent_FlushFailureSingle(t *testing.T) {
	now := time.Now()
	sqsEvent := makeSQSEvent([]string{"us-west-2"}, []time.Time{now})
	handler := &handlers.RateLimitHandler{
		Logger:    &mockLogger{},
		EMFFusher: &stubFusher{failRegions: map[string]bool{"us-west-2": true}},
		Namespace: "ns",
	}
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	assert.NoError(t, err, "Handler should not return error, only failures list")
	assert.Len(t, failures, 1, "One failure expected for the single message")
	assert.Equal(t, "us-west-2-id", failures[0].ItemIdentifier)
}
func TestHandleEvent_MultipleRegionsMixed(t *testing.T) {
	now := time.Now()
	regions := []string{"us-east-1", "us-west-2"}
	times := []time.Time{now, now}
	sqsEvent := makeSQSEvent(regions, times)
	handler := &handlers.RateLimitHandler{
		Logger: &mockLogger{},
		EMFFusher: &stubFusher{failRegions: map[string]bool{
			"us-west-2": true,
		}},
		Namespace: "ns",
	}
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	assert.NoError(t, err, "Handler should not return error even when some flushes fail")
	// Only us-west-2-id should be retried
	ids := []string{failures[0].ItemIdentifier}
	assert.ElementsMatch(t, []string{"us-west-2-id"}, ids)
}
func TestHandleEvent_ConvertFailureSkipsAll(t *testing.T) {
	handler := &handlers.RateLimitHandler{
		Logger:    &mockLogger{},
		EMFFusher: &stubFusher{},
		Namespace: "ns",
	}
	// all invalid JSON => no conversions, byRegion empty => early exit
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "id1", Body: "not-json"},
			{MessageId: "id2", Body: "also-not-json"},
		},
	}
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	assert.NoError(t, err, "No error expected when all conversion fails")
	assert.Nil(t, failures, "Failures should be nil when no records are flushed")
}
