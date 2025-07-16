package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Test rate limit error variables - core business logic
func TestRateLimitErrorVariables(t *testing.T) {
	rateLimitTests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrCloudTrailBatcherNil", ErrCloudTrailBatcherNil, "cloudtrail batcher is nil"},
		{"ErrNamespaceNotSet", ErrNamespaceNotSet, "namespace is not set"},
		{"ErrHandlerNotInitialized", ErrHandlerNotInitialized, "handler not initialized"},
	}

	for _, tt := range rateLimitTests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, tt.err)
			assert.Equal(t, tt.msg, tt.err.Error())
		})
	}
}

// Test rate limit interface definition exists
func TestRateLimitEventHandler_Interface(t *testing.T) {
	var rateLimitHandler RateLimitEventHandler
	assert.Nil(t, rateLimitHandler)
}

// Test basic validation without complex dependencies
func TestNewRateLimitHandler_NilValidation(t *testing.T) {
	rateLimitValidationTests := []struct {
		name        string
		config      RateLimitHandlerConfig
		expectError error
	}{
		{
			name: "nil batcher",
			config: RateLimitHandlerConfig{
				Batcher:   nil,
				Namespace: "test-namespace",
			},
			expectError: ErrCloudTrailBatcherNil,
		},
		{
			name: "empty namespace with nil batcher",
			config: RateLimitHandlerConfig{
				Batcher:   nil,
				Namespace: "",
			},
			expectError: ErrCloudTrailBatcherNil, // First validation error
		},
	}

	for _, tt := range rateLimitValidationTests {
		t.Run(tt.name, func(t *testing.T) {
			rateLimitHandler, err := NewRateLimitHandler(tt.config)

			assert.Error(t, err)
			assert.Equal(t, tt.expectError, err)
			assert.Nil(t, rateLimitHandler)
		})
	}
}

// Test isFlushCommand method logic
func TestRateLimitHandler_isFlushCommand(t *testing.T) {
	// Create minimal handler for testing
	rateLimitHandler := &RateLimitHandler{}

	flushCommandTests := []struct {
		name     string
		body     string
		expected bool
	}{
		{
			name:     "valid flush command true",
			body:     `{"flush": true}`,
			expected: true,
		},
		{
			name:     "valid flush command false",
			body:     `{"flush": false}`,
			expected: false,
		},
		{
			name:     "invalid json",
			body:     `{invalid json}`,
			expected: false,
		},
		{
			name:     "missing flush field",
			body:     `{"other": "value"}`,
			expected: false,
		},
		{
			name:     "empty body",
			body:     "",
			expected: false,
		},
		{
			name:     "null flush value",
			body:     `{"flush": null}`,
			expected: false,
		},
	}

	for _, tt := range flushCommandTests {
		t.Run(tt.name, func(t *testing.T) {
			result := rateLimitHandler.isFlushCommand(tt.body)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test FlushCommand struct marshaling/unmarshaling
func TestFlushCommand_JSON(t *testing.T) {
	flushCommandStructTests := []struct {
		name     string
		jsonStr  string
		expected FlushCommand
		hasError bool
	}{
		{
			name:     "flush true",
			jsonStr:  `{"flush": true}`,
			expected: FlushCommand{Flush: true},
			hasError: false,
		},
		{
			name:     "flush false",
			jsonStr:  `{"flush": false}`,
			expected: FlushCommand{Flush: false},
			hasError: false,
		},
		{
			name:     "empty object",
			jsonStr:  `{}`,
			expected: FlushCommand{Flush: false},
			hasError: false,
		},
		{
			name:     "invalid json",
			jsonStr:  `{invalid}`,
			expected: FlushCommand{},
			hasError: true,
		},
	}

	for _, tt := range flushCommandStructTests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd FlushCommand
			err := json.Unmarshal([]byte(tt.jsonStr), &cmd)

			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Flush, cmd.Flush)
			}
		})
	}
}

// Test FlushCommand struct field
func TestFlushCommand_Struct(t *testing.T) {
	flushStructTests := []struct {
		name     string
		command  FlushCommand
		expected bool
	}{
		{
			name:     "flush true",
			command:  FlushCommand{Flush: true},
			expected: true,
		},
		{
			name:     "flush false",
			command:  FlushCommand{Flush: false},
			expected: false,
		},
		{
			name:     "zero value",
			command:  FlushCommand{},
			expected: false,
		},
	}

	for _, tt := range flushStructTests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.command.Flush)
		})
	}
}

// MockBatcher for testing
type MockBatcher struct {
	mock.Mock
}

func (m *MockBatcher) Add(ctx context.Context, region string, ct types.CloudTrailEvent) {
	m.Called(ctx, region, ct)
}

func (m *MockBatcher) AddCounters(ctx context.Context, counters map[string]map[string]int) error {
	args := m.Called(ctx, counters)
	return args.Error(0)
}

func (m *MockBatcher) FlushAll(ctx context.Context, t time.Time) error {
	args := m.Called(ctx, t)
	return args.Error(0)
}

func (m *MockBatcher) PropagateInvoker() bool {
	args := m.Called()
	return args.Bool(0)
}

// Test handler event aggregation
func TestRateLimitHandler_HandleEvent_Aggregation(t *testing.T) {
	// Create mock batcher
	mockBatcher := new(MockBatcher)
	
	// Create handler
	handler := &RateLimitHandler{
		Batcher:   mockBatcher,
		Logger:    &logger.NoopLogger{},
		Namespace: "test-namespace",
	}
	
	// Create test events
	event1 := types.CloudTrailEvent{
		EventName: "CreateBucket",
		AWSRegion: "us-east-1",
		RequestID: "req-1",
		UserIdentity: types.UserIdentity{
			Type: "IAMUser",
			ARN:  "arn:aws:iam::123456789012:user/testuser",
		},
	}
	
	event2 := types.CloudTrailEvent{
		EventName: "CreateBucket",
		AWSRegion: "us-east-1",
		RequestID: "req-2",
		UserIdentity: types.UserIdentity{
			Type: "IAMUser",
			ARN:  "arn:aws:iam::123456789012:user/testuser",
		},
	}
	
	event3 := types.CloudTrailEvent{
		EventName: "ListBuckets",
		AWSRegion: "us-west-2",
		RequestID: "req-3",
		UserIdentity: types.UserIdentity{
			Type: "IAMUser",
			ARN:  "arn:aws:iam::123456789012:user/testuser",
		},
	}
	
	// Create SQS event with multiple CloudTrail events
	event1JSON, _ := json.Marshal(event1)
	event2JSON, _ := json.Marshal(event2)
	event3JSON, _ := json.Marshal(event3)
	
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "msg1", Body: string(event1JSON)},
			{MessageId: "msg2", Body: string(event2JSON)},
			{MessageId: "msg3", Body: string(event3JSON)},
		},
	}
	
	// Set up mock expectations
	mockBatcher.On("PropagateInvoker").Return(true)
	
	// Expect AddCounters to be called with aggregated counters
	mockBatcher.On("AddCounters", mock.Anything, mock.MatchedBy(func(counters map[string]map[string]int) bool {
		// Verify us-east-1 region counters
		if counters["us-east-1"]["CreateBucket"] != 2 {
			return false
		}
		
		// Verify us-west-2 region counters
		if counters["us-west-2"]["ListBuckets"] != 1 {
			return false
		}
		
		// Verify invoker counters
		if !strings.Contains(getInvokerKey(counters["us-east-1"]), "IAMUser:testuser") {
			return false
		}
		
		return true
	})).Return(nil)
	
	// Call handler
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	
	// Verify results
	require.NoError(t, err)
	assert.Empty(t, failures)
	
	// Verify mock expectations
	mockBatcher.AssertExpectations(t)
}

// Helper function to find invoker key
func getInvokerKey(counters map[string]int) string {
	for key := range counters {
		if strings.Contains(key, ":") {
			return key
		}
	}
	return ""
}

// Test handler with mixed events (CloudTrail + flush command)
func TestRateLimitHandler_HandleEvent_MixedEvents(t *testing.T) {
	// Create mock batcher
	mockBatcher := new(MockBatcher)
	
	// Create handler
	handler := &RateLimitHandler{
		Batcher:   mockBatcher,
		Logger:    &logger.NoopLogger{},
		Namespace: "test-namespace",
	}
	
	// Create test event
	event1 := types.CloudTrailEvent{
		EventName: "CreateBucket",
		AWSRegion: "us-east-1",
		RequestID: "req-1",
	}
	
	// Create flush command
	flushCmd := FlushCommand{Flush: true}
	flushJSON, _ := json.Marshal(flushCmd)
	
	// Create SQS event with mixed messages
	event1JSON, _ := json.Marshal(event1)
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "msg1", Body: string(event1JSON)},
			{MessageId: "msg2", Body: string(flushJSON)},
		},
	}
	
	// Set up mock expectations
	mockBatcher.On("PropagateInvoker").Return(false)
	
	// Expect AddCounters to be called with aggregated counters
	mockBatcher.On("AddCounters", mock.Anything, mock.MatchedBy(func(counters map[string]map[string]int) bool {
		return counters["us-east-1"]["CreateBucket"] == 1
	})).Return(nil)
	
	// Expect FlushAll to be called for the flush command
	mockBatcher.On("FlushAll", mock.Anything, mock.Anything).Return(nil)
	
	// Call handler
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	
	// Verify results
	require.NoError(t, err)
	assert.Empty(t, failures)
	
	// Verify mock expectations
	mockBatcher.AssertExpectations(t)
}

// Test handler with batch failure
func TestRateLimitHandler_HandleEvent_BatchFailure(t *testing.T) {
	// Create mock batcher
	mockBatcher := new(MockBatcher)
	
	// Create handler
	handler := &RateLimitHandler{
		Batcher:   mockBatcher,
		Logger:    &logger.NoopLogger{},
		Namespace: "test-namespace",
	}
	
	// Create test events
	event1 := types.CloudTrailEvent{
		EventName: "CreateBucket",
		AWSRegion: "us-east-1",
		RequestID: "req-1",
	}
	
	event2 := types.CloudTrailEvent{
		EventName: "ListBuckets",
		AWSRegion: "us-west-2",
		RequestID: "req-2",
	}
	
	// Create SQS event
	event1JSON, _ := json.Marshal(event1)
	event2JSON, _ := json.Marshal(event2)
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "msg1", Body: string(event1JSON)},
			{MessageId: "msg2", Body: string(event2JSON)},
		},
	}
	
	// Set up mock expectations
	mockBatcher.On("PropagateInvoker").Return(false)
	
	// Expect AddCounters to fail
	mockBatcher.On("AddCounters", mock.Anything, mock.Anything).Return(errors.New("batch error"))
	
	// Call handler
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	
	// Verify results
	require.NoError(t, err)
	assert.Len(t, failures, 2) // Both messages should be marked as failed
	assert.Equal(t, "msg1", failures[0].ItemIdentifier)
	assert.Equal(t, "msg2", failures[1].ItemIdentifier)
	
	// Verify mock expectations
	mockBatcher.AssertExpectations(t)
}

// Test handler with malformed event
func TestRateLimitHandler_HandleEvent_MalformedEvent(t *testing.T) {
	// Create mock batcher
	mockBatcher := new(MockBatcher)
	
	// Create handler
	handler := &RateLimitHandler{
		Batcher:   mockBatcher,
		Logger:    &logger.NoopLogger{},
		Namespace: "test-namespace",
	}
	
	// Create valid event
	event1 := types.CloudTrailEvent{
		EventName: "CreateBucket",
		AWSRegion: "us-east-1",
		RequestID: "req-1",
	}
	event1JSON, _ := json.Marshal(event1)
	
	// Create SQS event with one valid and one invalid message
	sqsEvent := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "msg1", Body: string(event1JSON)},
			{MessageId: "msg2", Body: "invalid json"},
		},
	}
	
	// Set up mock expectations
	mockBatcher.On("PropagateInvoker").Return(false)
	
	// Expect AddCounters to be called with only the valid event
	mockBatcher.On("AddCounters", mock.Anything, mock.MatchedBy(func(counters map[string]map[string]int) bool {
		return counters["us-east-1"]["CreateBucket"] == 1
	})).Return(nil)
	
	// Call handler
	failures, err := handler.HandleEvent(context.Background(), sqsEvent)
	
	// Verify results
	require.NoError(t, err)
	assert.Len(t, failures, 1) // Only the invalid message should fail
	assert.Equal(t, "msg2", failures[0].ItemIdentifier)
	
	// Verify mock expectations
	mockBatcher.AssertExpectations(t)
}