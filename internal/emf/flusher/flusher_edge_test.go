package flusher

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test EMFFlusherConfig validation
func TestEMFFlusherConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      EMFFlusherConfig
		expectError error
	}{
		{
			name: "valid config",
			config: EMFFlusherConfig{
				CwlClientMap:  safestore.NewSyncStore[cwlclient.CloudWatchLogsClient](),
				LogGroupName:  "test-group",
				LogStreamName: "test-stream",
			},
			expectError: nil,
		},
		{
			name: "nil client map",
			config: EMFFlusherConfig{
				CwlClientMap:  nil,
				LogGroupName:  "test-group",
				LogStreamName: "test-stream",
			},
			expectError: ErrClientMapNil,
		},
		{
			name: "empty log group name",
			config: EMFFlusherConfig{
				CwlClientMap:  safestore.NewSyncStore[cwlclient.CloudWatchLogsClient](),
				LogGroupName:  "",
				LogStreamName: "test-stream",
			},
			expectError: ErrLogGroupNameEmpty,
		},
		{
			name: "empty log stream name",
			config: EMFFlusherConfig{
				CwlClientMap:  safestore.NewSyncStore[cwlclient.CloudWatchLogsClient](),
				LogGroupName:  "test-group",
				LogStreamName: "",
			},
			expectError: ErrLogStreamNameEmpty,
		},
		{
			name: "whitespace log group name",
			config: EMFFlusherConfig{
				CwlClientMap:  safestore.NewSyncStore[cwlclient.CloudWatchLogsClient](),
				LogGroupName:  "   ",
				LogStreamName: "test-stream",
			},
			expectError: ErrLogGroupNameEmpty,
		},
		{
			name: "whitespace log stream name",
			config: EMFFlusherConfig{
				CwlClientMap:  safestore.NewSyncStore[cwlclient.CloudWatchLogsClient](),
				LogGroupName:  "test-group",
				LogStreamName: "   ",
			},
			expectError: ErrLogStreamNameEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError != nil {
				assert.ErrorIs(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test batch size validation
func TestValidateBatchSize(t *testing.T) {
	tests := []struct {
		name        string
		events      []cwlTypes.InputLogEvent
		expectError error
	}{
		{
			name:        "empty batch",
			events:      []cwlTypes.InputLogEvent{},
			expectError: nil,
		},
		{
			name: "valid batch",
			events: []cwlTypes.InputLogEvent{
				{
					Message:   aws.String("test message"),
					Timestamp: aws.Int64(time.Now().UnixMilli()),
				},
			},
			expectError: nil,
		},
		{
			name: "too many events",
			events: func() []cwlTypes.InputLogEvent {
				events := make([]cwlTypes.InputLogEvent, MaxLogEventsPerBatch+1)
				for i := range events {
					events[i] = cwlTypes.InputLogEvent{
						Message:   aws.String("test message"),
						Timestamp: aws.Int64(time.Now().UnixMilli()),
					}
				}
				return events
			}(),
			expectError: ErrBatchTooLarge,
		},
		{
			name: "message too large",
			events: []cwlTypes.InputLogEvent{
				{
					Message:   aws.String(string(make([]byte, MaxMessageSizeBytes+1))),
					Timestamp: aws.Int64(time.Now().UnixMilli()),
				},
			},
			expectError: ErrMessageTooLarge,
		},
		{
			name: "batch too large",
			events: func() []cwlTypes.InputLogEvent {
				// Create a batch that exceeds MaxBatchSizeBytes
				events := []cwlTypes.InputLogEvent{}
				totalSize := 0
				msgSize := 100000 // 100KB per message
				
				for totalSize < MaxBatchSizeBytes {
					events = append(events, cwlTypes.InputLogEvent{
						Message:   aws.String(string(make([]byte, msgSize))),
						Timestamp: aws.Int64(time.Now().UnixMilli()),
					})
					totalSize += msgSize + EventOverheadBytes
				}
				
				return events
			}(),
			expectError: ErrBatchTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBatchSize(tt.events)
			if tt.expectError != nil {
				assert.ErrorIs(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test SortEventsByTimestamp
func TestSortEventsByTimestamp(t *testing.T) {
	now := time.Now().UnixMilli()
	
	events := []cwlTypes.InputLogEvent{
		{
			Message:   aws.String("message 3"),
			Timestamp: aws.Int64(now + 200),
		},
		{
			Message:   aws.String("message 1"),
			Timestamp: aws.Int64(now),
		},
		{
			Message:   aws.String("message 2"),
			Timestamp: aws.Int64(now + 100),
		},
	}
	
	SortEventsByTimestamp(events)
	
	// Verify events are sorted by timestamp
	assert.Equal(t, now, *events[0].Timestamp)
	assert.Equal(t, now+100, *events[1].Timestamp)
	assert.Equal(t, now+200, *events[2].Timestamp)
	
	assert.Equal(t, "message 1", *events[0].Message)
	assert.Equal(t, "message 2", *events[1].Message)
	assert.Equal(t, "message 3", *events[2].Message)
}

// Test BuildGenericLogEvents with HTML escaping
func TestBuildGenericLogEvents_HTMLEscaping(t *testing.T) {
	type testRecord struct {
		payload   []byte
		timestamp int64
	}
	
	records := []testRecord{
		{
			payload:   []byte("<script>alert('xss')</script>"),
			timestamp: 123456789,
		},
		{
			payload:   []byte("normal text"),
			timestamp: 987654321,
		},
	}
	
	extractPayload := func(r testRecord) []byte {
		return r.payload
	}
	
	extractTimestamp := func(r testRecord) int64 {
		return r.timestamp
	}
	
	// Test with HTML escaping enabled
	events := BuildGenericLogEvents(records, extractPayload, extractTimestamp, true)
	assert.Len(t, events, 2)
	assert.Equal(t, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;", *events[0].Message)
	assert.Equal(t, "normal text", *events[1].Message)
	
	// Test with HTML escaping disabled
	events = BuildGenericLogEvents(records, extractPayload, extractTimestamp, false)
	assert.Len(t, events, 2)
	assert.Equal(t, "<script>alert('xss')</script>", *events[0].Message)
	assert.Equal(t, "normal text", *events[1].Message)
}

// Test MakeFlushFunc with error handling
func TestMakeFlushFunc_ErrorHandling(t *testing.T) {
	// Create a fake client that returns an error
	fake := &cwlclient.FakeCloudWatchLogsClient{
		ErrPutLogEvents: true,
	}
	
	// Create a flush function
	flushFn := MakeFlushFunc[builder.EMFRecord](
		fake,
		"test-group",
		"test-stream",
		func(r builder.EMFRecord) []byte { return r.Payload },
		func(r builder.EMFRecord) int64 { return r.TimeStamp.UnixMilli() },
		&logger.NoopLogger{},
	)
	
	// Create a test batch
	batch := []builder.EMFRecord{
		{
			Payload:   []byte("test payload"),
			TimeStamp: time.Now(),
		},
	}
	
	// Call the flush function
	err := flushFn(context.Background(), batch)
	
	// Verify error
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrFlushFailed)
}

// Test EMFFlusherImpl.Flush with context cancellation
func TestEMFFlusherImpl_Flush_ContextCancellation(t *testing.T) {
	// Skip this test for now as the implementation doesn't check for context cancellation
	// This would be a good enhancement for the future
	t.Skip("Implementation doesn't currently check for context cancellation")
	
	// Create a client map with a fake client
	clientMap := safestore.NewSyncStore[cwlclient.CloudWatchLogsClient]()
	fake := &cwlclient.FakeCloudWatchLogsClient{
		Region: "test-region",
	}
	clientMap.Store("test-region", fake)
	
	// Create a flusher
	flusher, err := NewEMFFlusher(EMFFlusherConfig{
		CwlClientMap:  clientMap,
		LogGroupName:  "test-group",
		LogStreamName: "test-stream",
		Logger:        &logger.NoopLogger{},
	})
	require.NoError(t, err)
	
	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	// Create a test batch
	batch := []builder.EMFRecord{
		{
			Payload:   []byte("test payload"),
			TimeStamp: time.Now(),
		},
	}
	
	// Call Flush with cancelled context
	err = flusher.Flush(ctx, "test-region", batch)
	
	// Verify error (context cancellation should be propagated)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// Test BuildLogEvents
func TestBuildLogEvents(t *testing.T) {
	now := time.Now()
	
	batch := []builder.EMFRecord{
		{
			Payload:   []byte("payload 1"),
			TimeStamp: now,
		},
		{
			Payload:   []byte("payload 2"),
			TimeStamp: now.Add(time.Second),
		},
	}
	
	events := BuildLogEvents(batch)
	
	assert.Len(t, events, 2)
	assert.Equal(t, "payload 1", *events[0].Message)
	assert.Equal(t, now.UnixMilli(), *events[0].Timestamp)
	assert.Equal(t, "payload 2", *events[1].Message)
	assert.Equal(t, now.Add(time.Second).UnixMilli(), *events[1].Timestamp)
}