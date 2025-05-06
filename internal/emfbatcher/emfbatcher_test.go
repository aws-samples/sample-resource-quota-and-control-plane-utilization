package emfbatcher_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/stretchr/testify/assert"

	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/outofoffice3/aws-samples/geras/internal/emfbatcher"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// fakeCWClient captures PutLogEvents inputs and can simulate errors.
type fakeCWClient struct {
	calls     [][]cwlTypes.InputLogEvent
	returnErr error
}

func (f *fakeCWClient) GetRegion() string { return "us-test-1" }
func (f *fakeCWClient) PutLogEvents(
	ctx context.Context,
	in *cloudwatchlogs.PutLogEventsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.PutLogEventsOutput, error) {
	f.calls = append(f.calls, in.LogEvents)
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}
func (f *fakeCWClient) CreateLogGroup(context.Context, *cloudwatchlogs.CreateLogGroupInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}
func (f *fakeCWClient) DescribeLogGroups(context.Context, *cloudwatchlogs.DescribeLogGroupsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return &cloudwatchlogs.DescribeLogGroupsOutput{}, nil
}
func (f *fakeCWClient) CreateLogStream(context.Context, *cloudwatchlogs.CreateLogStreamInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return &cloudwatchlogs.CreateLogStreamOutput{}, nil
}
func (f *fakeCWClient) DescribeLogStreams(context.Context, *cloudwatchlogs.DescribeLogStreamsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return &cloudwatchlogs.DescribeLogStreamsOutput{}, nil
}

// fakeLogger satisfies logger.Logger but does nothing.
type fakeLogger struct{}

func (fakeLogger) Debug(string, ...any) {}
func (fakeLogger) Info(string, ...any)  {}
func (fakeLogger) Warn(string, ...any)  {}
func (fakeLogger) Error(string, ...any) {}

// Test zero-length batch returns nil and no client calls.
func TestMakeFlushFunc_ZeroBatch(t *testing.T) {
	client := &fakeCWClient{}
	fn := emfbatcher.MakeFlushFunc(
		client,
		"my-group", "my-stream",
		func(i int) []byte { return []byte(fmt.Sprintf("%d", i)) },
		func(i int) int64 { return int64(i) },
		fakeLogger{},
	)

	err := fn(context.Background(), []int{})
	assert.NoError(t, err)
	assert.Empty(t, client.calls)
}

// Test successful flush with one record.
func TestMakeFlushFunc_Success(t *testing.T) {
	client := &fakeCWClient{}
	fn := emfbatcher.MakeFlushFunc(
		client,
		"g", "s",
		func(s string) []byte { return []byte(s) },
		func(s string) int64 { return int64(len(s)) },
		fakeLogger{},
	)

	err := fn(context.Background(), []string{"hello"})
	assert.NoError(t, err)
	// one call, one event
	assert.Len(t, client.calls, 1)
	events := client.calls[0]
	assert.Len(t, events, 1)
	assert.Equal(t, "hello", *events[0].Message)
	assert.Equal(t, int64(5), *events[0].Timestamp)
}

// Test client error path: logger.Error and error returned.
func TestMakeFlushFunc_Error(t *testing.T) {
	client := &fakeCWClient{returnErr: errors.New("bang")}
	// capture Error calls by using real logger (prints to stdout) but we only care about return
	fn := emfbatcher.MakeFlushFunc(
		client,
		"group", "stream",
		func(b byte) []byte { return []byte{b} },
		func(b byte) int64 { return int64(b) },
		logger.Get(), // real logger
	)

	err := fn(context.Background(), []byte{0x01, 0x02})
	assert.Error(t, err)
	// Should have attempted exactly one call
	assert.Len(t, client.calls, 1)
}

// Test that multiple events are all forwarded correctly.
func TestMakeFlushFunc_Multiple(t *testing.T) {
	client := &fakeCWClient{}
	fn := emfbatcher.MakeFlushFunc(
		client,
		"G", "S",
		func(i int) []byte { return []byte(fmt.Sprintf("v=%d", i)) },
		func(i int) int64 { return int64(i * 10) },
		fakeLogger{},
	)

	batch := []int{1, 2, 3}
	err := fn(context.Background(), batch)
	assert.NoError(t, err)
	assert.Len(t, client.calls, 1)
	events := client.calls[0]
	assert.Len(t, events, 3)
	for idx, v := range batch {
		wantMsg := fmt.Sprintf("v=%d", v)
		assert.Equal(t, wantMsg, *events[idx].Message)
		assert.Equal(t, int64(v*10), *events[idx].Timestamp)
	}
}
