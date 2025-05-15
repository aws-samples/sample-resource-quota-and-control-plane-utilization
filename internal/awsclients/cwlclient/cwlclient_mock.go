package cwlclient

import (
	"context"
	"errors"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
)

// FakeCloudWatchLogsClient implements CloudWatchLogsClient with AWS-style pagination
// and injectable errors, for use in unit tests.
type FakeCloudWatchLogsClient struct {
	Region string

	// pages for paginator calls:
	DescribeLogGroupsPages  []*cloudwatchlogs.DescribeLogGroupsOutput
	DescribeLogStreamsPages []*cloudwatchlogs.DescribeLogStreamsOutput

	// “throw on this call index” for each paginated method:
	ErrOnDescribeLogGroupsCall  int
	ErrOnDescribeLogStreamsCall int

	// simple error flags for non-paged calls:
	ErrPutLogEvents    bool
	ErrCreateLogGroup  bool
	ErrCreateLogStream bool

	// internal counters:
	callDescribeLogGroupsCount  int
	callDescribeLogStreamsCount int
	callPutLogEventsCount       int
	callCreateLogGroupCount     int
	callCreateLogStreamCount    int
}

// GetRegion returns the configured region.
func (f *FakeCloudWatchLogsClient) GetRegion() string {
	return f.Region
}

// DescribeLogGroups pages through DescribeLogGroupsPages, honoring NextToken
// and injecting an error when callDescribeLogGroupsCount == ErrOnDescribeLogGroupsCall.
func (f *FakeCloudWatchLogsClient) DescribeLogGroups(
	ctx context.Context,
	in *cloudwatchlogs.DescribeLogGroupsInput,
	optFns ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.callDescribeLogGroupsCount == f.ErrOnDescribeLogGroupsCall {
		return nil, errors.New("DescribeLogGroups injected error")
	}

	// parse NextToken as page index
	idx := 0
	if in.NextToken != nil {
		i, err := strconv.Atoi(*in.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	var out *cloudwatchlogs.DescribeLogGroupsOutput
	if idx < len(f.DescribeLogGroupsPages) {
		// clone page so tests can mutate safely
		page := f.DescribeLogGroupsPages[idx]
		out = &cloudwatchlogs.DescribeLogGroupsOutput{
			LogGroups: page.LogGroups,
		}
	} else {
		out = &cloudwatchlogs.DescribeLogGroupsOutput{}
	}
	// set NextToken if more pages remain
	if idx+1 < len(f.DescribeLogGroupsPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}

	f.callDescribeLogGroupsCount++
	return out, nil
}

// DescribeLogStreams pages through DescribeLogStreamsPages, honoring NextToken
// and injecting an error when callDescribeLogStreamsCount == ErrOnDescribeLogStreamsCall.
func (f *FakeCloudWatchLogsClient) DescribeLogStreams( // <— fix receiver name
	ctx context.Context,
	in *cloudwatchlogs.DescribeLogStreamsInput,
	optFns ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.callDescribeLogStreamsCount == f.ErrOnDescribeLogStreamsCall {
		return nil, errors.New("DescribeLogStreams injected error")
	}

	idx := 0
	if in.NextToken != nil {
		i, err := strconv.Atoi(*in.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	var out *cloudwatchlogs.DescribeLogStreamsOutput
	if idx < len(f.DescribeLogStreamsPages) {
		page := f.DescribeLogStreamsPages[idx]
		out = &cloudwatchlogs.DescribeLogStreamsOutput{
			LogStreams: page.LogStreams,
		}
	} else {
		out = &cloudwatchlogs.DescribeLogStreamsOutput{}
	}
	if idx+1 < len(f.DescribeLogStreamsPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}

	f.callDescribeLogStreamsCount++
	return out, nil
}

// PutLogEvents returns a stubbed output or an error if ErrPutLogEvents is set.
func (f *FakeCloudWatchLogsClient) PutLogEvents(
	ctx context.Context,
	in *cloudwatchlogs.PutLogEventsInput,
	optFns ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.PutLogEventsOutput, error) {
	f.callPutLogEventsCount++
	if f.ErrPutLogEvents {
		return nil, errors.New("PutLogEvents injected error")
	}
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}

// CreateLogGroup returns a stubbed output or an error if ErrCreateLogGroup is set.
func (f *FakeCloudWatchLogsClient) CreateLogGroup(
	ctx context.Context,
	in *cloudwatchlogs.CreateLogGroupInput,
	optFns ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	f.callCreateLogGroupCount++
	if f.ErrCreateLogGroup {
		return nil, errors.New("CreateLogGroup injected error")
	}
	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}

// CreateLogStream returns a stubbed output or an error if ErrCreateLogStream is set.
func (f *FakeCloudWatchLogsClient) CreateLogStream(
	ctx context.Context,
	in *cloudwatchlogs.CreateLogStreamInput,
	optFns ...func(*cloudwatchlogs.Options),
) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	f.callCreateLogStreamCount++
	if f.ErrCreateLogStream {
		return nil, errors.New("CreateLogStream injected error")
	}
	return &cloudwatchlogs.CreateLogStreamOutput{}, nil
}

// Reset clears all internal counters so you can reuse in multiple tests.
func (f *FakeCloudWatchLogsClient) Reset() {
	f.callDescribeLogGroupsCount = 0
	f.callDescribeLogStreamsCount = 0
	f.callPutLogEventsCount = 0
	f.callCreateLogGroupCount = 0
	f.callCreateLogStreamCount = 0
}
