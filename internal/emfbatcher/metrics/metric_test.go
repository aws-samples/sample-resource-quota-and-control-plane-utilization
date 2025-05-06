package metricemfbatcher

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/stretchr/testify/assert"

	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// mockCWL implements the CloudWatchLogsClient interface.
type mockCWL struct {
	region   string
	putInput *cloudwatchlogs.PutLogEventsInput
	putErr   error
}

func (m *mockCWL) GetRegion() string {
	return m.region
}
func (m *mockCWL) PutLogEvents(
	ctx context.Context,
	params *cloudwatchlogs.PutLogEventsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.PutLogEventsOutput, error) {
	m.putInput = params
	return &cloudwatchlogs.PutLogEventsOutput{}, m.putErr
}
func (m *mockCWL) CreateLogGroup(
	ctx context.Context,
	params *cloudwatchlogs.CreateLogGroupInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}
func (m *mockCWL) DescribeLogGroups(
	ctx context.Context,
	params *cloudwatchlogs.DescribeLogGroupsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return &cloudwatchlogs.DescribeLogGroupsOutput{}, nil
}

func (m *mockCWL) CreateLogStream(
	ctx context.Context,
	params *cloudwatchlogs.CreateLogStreamInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return &cloudwatchlogs.CreateLogStreamOutput{}, nil
}

func (m *mockCWL) DescribeLogStreams(
	ctx context.Context,
	params *cloudwatchlogs.DescribeLogStreamsInput,
	_ ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return &cloudwatchlogs.DescribeLogStreamsOutput{}, nil
}

func TestMetricBatcher_Success(t *testing.T) {
	ctx := context.Background()
	mock := &mockCWL{region: "us-west-2"}

	// Create a CloudWatchMetricBatcher with small batch sizes
	b, err := NewCloudWatchMetricBatcher(
		ctx,
		mock,
		"MyNS",     // namespace
		"myGroup",  // logGroup
		"myStream", // logStream
		1,          // maxEvents = 1 to flush immediately
		0,          // maxBytes disabled
		1*time.Hour,
		0,   // overhead
		nil, // logger
	)
	assert.NoError(t, err)

	// Send one metric
	m := sharedtypes.CloudWatchMetric{
		Name:      "foo",
		Value:     9.99,
		Unit:      cwTypes.StandardUnitCount,
		Timestamp: time.Unix(42, 0),
		Metadata:  map[string]string{"vpc": "vpc-1"},
	}
	// push to input channel
	b.Batcher.GetInputChannel() <- m

	// Wait will close channel and flush remaining
	b.Batcher.Wait()

	// Verify that PutLogEvents was called exactly once
	inp := mock.putInput
	assert.NotNil(t, inp, "PutLogEvents never called")

	// The log event message should contain our metric name and metadata
	msg := *inp.LogEvents[0].Message
	assert.True(t, strings.Contains(msg, `"foo":9.99`), "metric value missing in EMF payload")
	assert.True(t, strings.Contains(msg, `"vpc":"vpc-1"`), "metadata missing in EMF payload")

	// Verify group/stream are correct
	assert.Equal(t, "myGroup", *inp.LogGroupName)
	assert.Equal(t, "myStream", *inp.LogStreamName)
}

func TestMetricBatcher_FlushError(t *testing.T) {
	ctx := context.Background()
	mock := &mockCWL{region: "us-west-2", putErr: errors.New("fail")}

	b, err := NewCloudWatchMetricBatcher(
		ctx,
		mock,
		"NS",        // namespace
		"grp",       // logGroup
		"stream",    // logStream
		1,           // flush on every item
		0,           // maxBytes disabled
		time.Minute, // flush interval
		0,           // overhead
		nil,
	)
	assert.NoError(t, err)

	// Send a metric
	m := sharedtypes.CloudWatchMetric{
		Name:      "bar",
		Value:     1,
		Unit:      cwTypes.StandardUnitCount,
		Timestamp: time.Unix(100, 0),
		Metadata:  map[string]string{"x": "y"},
	}
	b.Batcher.GetInputChannel() <- m

	// Waiting will trigger the flush, which our mock returns error for
	b.Batcher.Wait()

}
