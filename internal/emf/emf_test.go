package emf

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
)

type mockLogger struct{}

func (l *mockLogger) Debug(format string, args ...interface{}) {}
func (l *mockLogger) Info(format string, args ...interface{})  {}
func (l *mockLogger) Warn(format string, args ...interface{})  {}
func (l *mockLogger) Error(format string, args ...interface{}) {}

type mockCWLClient struct {
	Fail bool
}

func (m *mockCWLClient) GetRegion() string { return "mock-region" }
func (m *mockCWLClient) PutLogEvents(ctx context.Context, params *cloudwatchlogs.PutLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	if m.Fail {
		return nil, errors.New("put log events failed")
	}
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}
func (m *mockCWLClient) CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return nil, nil
}
func (m *mockCWLClient) DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return nil, nil
}
func (m *mockCWLClient) DescribeLogStreams(ctx context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return nil, nil
}
func (m *mockCWLClient) CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return nil, nil
}

func TestBuild(t *testing.T) {
	logger := &mockLogger{}
	now := time.Now()

	t.Run("valid input", func(t *testing.T) {
		in := EMFInput{
			Namespace:  "test",
			MetricName: "myMetric",
			Value:      1,
			Unit:       "Count",
			Dimensions: [][]string{{"a"}},
			Timestamp:  now,
		}

		out, err := Build(in, logger)
		assert.NoError(t, err)
		assert.NotEmpty(t, out.Payload)
		assert.Equal(t, now, out.TimeStamp)
	})

	t.Run("zero timestamp fallback", func(t *testing.T) {
		in := EMFInput{
			Namespace:  "test",
			MetricName: "myMetric",
			Value:      1,
			Unit:       "Count",
			Dimensions: [][]string{{"a"}},
		}

		out, err := Build(in, logger)
		assert.NoError(t, err)
		assert.NotEmpty(t, out.Payload)
		assert.False(t, out.TimeStamp.IsZero())
	})

	t.Run("marshal failure", func(t *testing.T) {
		in := EMFInput{
			Namespace:  "test",
			MetricName: "badMetric",
			Value:      math.Inf(1),
			Unit:       "Count",
			Dimensions: [][]string{{"a"}},
		}

		out, err := Build(in, logger)
		assert.Error(t, err)
		assert.Empty(t, out.Payload)
	})
}

func TestConvertSQSMessageToEMF(t *testing.T) {
	logger := &mockLogger{}
	now := time.Now()

	ctEvent := sharedtypes.CloudTrailEvent{EventTime: now}
	body, _ := json.Marshal(ctEvent)

	t.Run("valid unit", func(t *testing.T) {
		msg := events.SQSMessage{Body: string(body)}
		emf, err := ConvertSQSMessageToEMF(context.TODO(), msg, "ns", "metric", "Count", [][]string{{"x"}}, logger)
		assert.NoError(t, err)
		assert.NotEmpty(t, emf.Payload)
		assert.WithinDuration(t, now, emf.TimeStamp, time.Millisecond)
	})

	t.Run("unknown unit", func(t *testing.T) {
		msg := events.SQSMessage{Body: string(body)}
		emf, err := ConvertSQSMessageToEMF(context.TODO(), msg, "ns", "metric", "Seconds", [][]string{{"x"}}, logger)
		assert.NoError(t, err)
		assert.NotEmpty(t, emf.Payload)
	})

	t.Run("unmarshal failure", func(t *testing.T) {
		msg := events.SQSMessage{Body: "not-json"}
		_, err := ConvertSQSMessageToEMF(context.TODO(), msg, "ns", "metric", "Count", [][]string{{"x"}}, logger)
		assert.Error(t, err)
	})
}

func TestFlush(t *testing.T) {
	logger := &mockLogger{}
	now := time.Now()
	var clientMap safemap.TypedMap[cwlclient.CloudWatchLogsClient]

	clientMap.Store("good", &mockCWLClient{})
	clientMap.Store("fail", &mockCWLClient{Fail: true})

	t.Run("empty batch", func(t *testing.T) {
		f := &EMFFlusherImpl{
			CwlClientMap:  &clientMap,
			LogGroupName:  "group",
			LogStreamName: "stream",
			Logger:        logger,
		}

		err := f.Flush(context.TODO(), "good", []EMFRecord{})
		assert.NoError(t, err)
	})

	t.Run("missing client", func(t *testing.T) {
		f := &EMFFlusherImpl{
			CwlClientMap:  &clientMap,
			LogGroupName:  "group",
			LogStreamName: "stream",
			Logger:        logger,
		}

		err := f.Flush(context.TODO(), "missing", []EMFRecord{{Payload: []byte("msg"), TimeStamp: now}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no client found for region")
	})

	t.Run("put log events failure", func(t *testing.T) {
		f := &EMFFlusherImpl{
			CwlClientMap:  &clientMap,
			LogGroupName:  "group",
			LogStreamName: "stream",
			Logger:        logger,
		}

		err := f.Flush(context.TODO(), "fail", []EMFRecord{{Payload: []byte("msg"), TimeStamp: now}})
		assert.Error(t, err)
	})

	t.Run("successful flush", func(t *testing.T) {
		f := &EMFFlusherImpl{
			CwlClientMap:  &clientMap,
			LogGroupName:  "group",
			LogStreamName: "stream",
			Logger:        logger,
		}

		err := f.Flush(context.TODO(), "good", []EMFRecord{{Payload: []byte("msg"), TimeStamp: now}})
		assert.NoError(t, err)
	})
}
