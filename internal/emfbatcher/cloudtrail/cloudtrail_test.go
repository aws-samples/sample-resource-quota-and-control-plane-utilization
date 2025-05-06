package cloudtrailemfbatcher_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/stretchr/testify/assert"

	"github.com/outofoffice3/aws-samples/geras/internal/emfbatcher"
	cloudtrailemfbatcher "github.com/outofoffice3/aws-samples/geras/internal/emfbatcher/cloudtrail"
	applogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
	shared "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// --- Fake CloudWatchLogsClient ------------------------------------------------

type fakeCWClient struct {
	calls     []*cloudwatchlogs.PutLogEventsInput
	returnErr error
}

func (f *fakeCWClient) GetRegion() string { return "us-test-1" }
func (f *fakeCWClient) PutLogEvents(
	ctx context.Context,
	input *cloudwatchlogs.PutLogEventsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.PutLogEventsOutput, error) {
	f.calls = append(f.calls, input)
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}
func (f *fakeCWClient) CreateLogGroup(ctx context.Context, _ *cloudwatchlogs.CreateLogGroupInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}
func (f *fakeCWClient) DescribeLogGroups(ctx context.Context, _ *cloudwatchlogs.DescribeLogGroupsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return &cloudwatchlogs.DescribeLogGroupsOutput{}, nil
}

func (f *fakeCWClient) CreateLogStream(ctx context.Context, _ *cloudwatchlogs.CreateLogStreamInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return &cloudwatchlogs.CreateLogStreamOutput{}, nil
}

func (f *fakeCWClient) DescribeLogStreams(ctx context.Context, _ *cloudwatchlogs.DescribeLogStreamsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return &cloudwatchlogs.DescribeLogStreamsOutput{}, nil
}

// helper to unmarshal the first event's JSON
func firstPayload(t *testing.T, call *cloudwatchlogs.PutLogEventsInput) map[string]any {
	assert.NotEmpty(t, call.LogEvents)
	var m map[string]any
	err := json.Unmarshal([]byte(*call.LogEvents[0].Message), &m)
	assert.NoError(t, err)
	return m
}

// dummy CloudTrailEvent generator
func makeEvent(name string, ts time.Time) shared.CloudTrailEvent {
	return shared.CloudTrailEvent{
		EventName: name,
		EventTime: ts,
	}
}

// --- Tests ---------------------------------------------------------------------

func Test_NewCloudTrailEMFBatcher_NilLogger(t *testing.T) {
	client := &fakeCWClient{}
	// pass nil logger
	wrapper, err := cloudtrailemfbatcher.NewCloudTrailEMFBatcher(
		context.Background(),
		1024, 10,
		time.Hour,
		0,
		client,
		"ns", "lg", "ls",
		nil,
	)
	assert.NoError(t, err)
	// no events → no calls, no panic
	wrapper.Batcher.Wait()
	assert.Empty(t, client.calls)
}

func Test_NewCloudTrailEMFBatcher_SuccessfulFlush(t *testing.T) {
	client := &fakeCWClient{}
	wrapper, err := cloudtrailemfbatcher.NewCloudTrailEMFBatcher(
		context.Background(),
		1024, 10,
		time.Hour,
		0,
		client,
		"ns", "MyGroup", "MyStream",
		applogger.Get(),
	)
	assert.NoError(t, err)

	// Add one event and wait
	e := makeEvent("TestAPI", time.UnixMilli(123456))
	wrapper.Batcher.Add(e)
	wrapper.Batcher.Wait()

	// Exactly one PutLogEvents call
	assert.Len(t, client.calls, 1)
	payload := firstPayload(t, client.calls[0])
	// Verify EMF shape
	awsBlock := payload["_aws"].(map[string]any)
	metrics := awsBlock["CloudWatchMetrics"].([]any)[0].(map[string]any)
	assert.Equal(t, "ns", metrics["Namespace"])
	assert.Equal(t, float64(1), payload["CallCount"])
	assert.Equal(t, "TestAPI", payload["eventName"])
}

func Test_MakeFlushFunc_ZeroBatch(t *testing.T) {
	fn := emfbatcher.MakeFlushFunc(
		&fakeCWClient{}, "lg", "ls",
		func(ev shared.CloudTrailEvent) []byte { return []byte("{}") },
		func(ev shared.CloudTrailEvent) int64 { return 0 },
		nil, // nil logger
	)
	// zero-length batch → no call, no error
	err := fn(context.Background(), []shared.CloudTrailEvent{})
	assert.NoError(t, err)
}

func Test_MakeFlushFunc_Error(t *testing.T) {
	client := &fakeCWClient{returnErr: errors.New("put-fail")}
	fn := emfbatcher.MakeFlushFunc(
		client, "lg", "ls",
		func(i int) []byte { return []byte(fmt.Sprintf(`{"v":%d}`, i)) },
		func(i int) int64 { return int64(i) },
		applogger.Get(),
	)
	// batch of one → error propagate
	err := fn(context.Background(), []int{7})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error")
	assert.Len(t, client.calls, 1)
}

func Test_CountAndByteThresholds(t *testing.T) {
	client := &fakeCWClient{}
	wrapper, _ := cloudtrailemfbatcher.NewCloudTrailEMFBatcher(
		context.Background(),
		1, // maxBytes so that first event flushes on byte threshold
		2, // maxEvents threshold
		0, // no timer
		0,
		client,
		"ns", "lg", "ls",
		applogger.Get(),
	)
	// MaxBytes=1 → any payload >1 triggers flush immediately
	wrapper.Batcher.Add(makeEvent("A", time.Now()))
	wrapper.Batcher.Add(makeEvent("B", time.Now()))
	wrapper.Batcher.Add(makeEvent("C", time.Now()))
	wrapper.Batcher.Wait()
	// Expect 3 flush calls (immediate-on-byte for each) or ≥2
	assert.True(t, len(client.calls) >= 2)
}

func Test_InvalidMapFn(t *testing.T) {
	client := &fakeCWClient{}
	wrapper, _ := cloudtrailemfbatcher.NewCloudTrailEMFBatcher(
		context.Background(),
		1024, 10,
		time.Hour,
		0,
		client,
		"ns", "lg", "ls",
		applogger.Get(),
	)
	// override MapFunc to always error
	wrapper.Batcher.MapFunc = func(_ shared.CloudTrailEvent) (shared.EMFRecord, error) {
		return shared.EMFRecord{}, errors.New("bad map")
	}
	wrapper.Batcher.Add(makeEvent("X", time.Now()))
	wrapper.Batcher.Wait()

	// Should have logged map-error via errCh but no PutLogEvents
	assert.Len(t, client.calls, 0)
}
