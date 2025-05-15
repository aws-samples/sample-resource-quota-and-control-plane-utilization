package cwlclient

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"
)

// testFactory is a helper type for testing
type testFactory struct {
	client CloudWatchLogsClient
	err    error
}

func (f *testFactory) CreateCloudWatchLogs(region string) (CloudWatchLogsClient, error) {
	return f.client, f.err
}

func TestNewCloudWatchLogsClient_ValidRegion(t *testing.T) {
	client, err := NewCloudWatchLogsClient(nil, "us-east-1")
	assert.NoError(t, err)
	assert.Equal(t, "us-east-1", client.GetRegion())
}

func TestNewCloudWatchLogsClient_InvalidRegion(t *testing.T) {
	_, err := NewCloudWatchLogsClient(nil, "invalid-region")
	assert.EqualError(t, err, errInvalidRegion)
}

func TestEnsureLogGroupExists_FoundInFirstPage(t *testing.T) {
	fake := &FakeCloudWatchLogsClient{
		Region: "us-west-2",
		DescribeLogGroupsPages: []*cloudwatchlogs.DescribeLogGroupsOutput{
			{
				LogGroups: []cwlTypes.LogGroup{
					{LogGroupName: aws.String("group1")},
				},
			},
		},
		ErrOnDescribeLogGroupsCall:  -1,
		ErrOnDescribeLogStreamsCall: -1,
	}
	err := EnsureLogGroupExists(context.Background(), fake, "group1")
	assert.NoError(t, err)
	assert.Equal(t, 1, fake.callDescribeLogGroupsCount)
	assert.Equal(t, 0, fake.callCreateLogGroupCount)
}

func TestEnsureLogGroupExists_FoundInLaterPage(t *testing.T) {
	fake := &FakeCloudWatchLogsClient{
		Region: "eu-central-1",
		DescribeLogGroupsPages: []*cloudwatchlogs.DescribeLogGroupsOutput{
			{
				LogGroups: []cwlTypes.LogGroup{}},
			{
				LogGroups: []cwlTypes.LogGroup{
					{
						LogGroupName: aws.String("my-group")},
				},
			},
		},
		ErrOnDescribeLogGroupsCall:  -1,
		ErrOnDescribeLogStreamsCall: -1,
	}
	err := EnsureLogGroupExists(context.Background(), fake, "my-group")
	assert.NoError(t, err)
	assert.Equal(t, 2, fake.callDescribeLogGroupsCount)
	assert.Equal(t, 0, fake.callCreateLogGroupCount)
}

func TestEnsureLogGroupExists_ErrorDuringDescribe(t *testing.T) {
	fake := &FakeCloudWatchLogsClient{
		Region:                     "ap-south-1",
		ErrOnDescribeLogGroupsCall: 0,
		DescribeLogGroupsPages:     []*cloudwatchlogs.DescribeLogGroupsOutput{{}},
	}
	err := EnsureLogGroupExists(context.Background(), fake, "any")
	assert.Error(t, err)
}

func TestEnsureLogGroupExists_CreateGroupWhenNotFound(t *testing.T) {
	fake := &FakeCloudWatchLogsClient{
		Region:                      "us-east-2",
		DescribeLogGroupsPages:      []*cloudwatchlogs.DescribeLogGroupsOutput{{}},
		ErrOnDescribeLogGroupsCall:  -1,
		ErrOnDescribeLogStreamsCall: -1,
		ErrCreateLogGroup:           false,
		ErrCreateLogStream:          false,
	}
	// simulate successful creation
	fake.ErrCreateLogGroup = false
	err := EnsureLogGroupExists(context.Background(), fake, "new-group")
	assert.NoError(t, err)
	assert.Equal(t, 1, fake.callCreateLogGroupCount)
}

func TestEnsureLogStreamExists_FoundInFirstPage(t *testing.T) {
	fake := &FakeCloudWatchLogsClient{
		Region: "us-west-1",
		DescribeLogStreamsPages: []*cloudwatchlogs.DescribeLogStreamsOutput{
			{
				LogStreams: []cwlTypes.LogStream{
					{
						LogStreamName: aws.String("stream1")},
				},
			},
		},
		ErrOnDescribeLogGroupsCall:  -1,
		ErrOnDescribeLogStreamsCall: -1,
	}
	err := EnsureLogStreamExists(context.Background(), fake, "grp", "stream1")
	assert.NoError(t, err)
	assert.Equal(t, 1, fake.callDescribeLogStreamsCount)
	assert.Equal(t, 0, fake.callCreateLogStreamCount)
}

func TestEnsureLogStreamExists_ErrorDuringDescribe(t *testing.T) {
	fake := &FakeCloudWatchLogsClient{
		Region:                      "eu-west-1",
		ErrOnDescribeLogStreamsCall: 0,
		DescribeLogStreamsPages:     []*cloudwatchlogs.DescribeLogStreamsOutput{{}},
		ErrOnDescribeLogGroupsCall:  -1,
	}
	err := EnsureLogStreamExists(context.Background(), fake, "g", "s")
	assert.Error(t, err)
}

func TestEnsureLogStreamExists_CreateStreamWhenNotFound(t *testing.T) {
	fake := &FakeCloudWatchLogsClient{
		Region:                      "ap-northeast-1",
		DescribeLogStreamsPages:     []*cloudwatchlogs.DescribeLogStreamsOutput{{}},
		ErrOnDescribeLogGroupsCall:  -1,
		ErrOnDescribeLogStreamsCall: -1,
		ErrCreateLogGroup:           false,
	}
	fake.ErrCreateLogStream = false
	err := EnsureLogStreamExists(context.Background(), fake, "g", "new-stream")
	assert.NoError(t, err)
	assert.Equal(t, 1, fake.callCreateLogStreamCount)
}

func TestEnsureGroupAndStreamAcrossRegions_FactoryInitError(t *testing.T) {
	factory := &testFactory{
		client: nil,
		err:    errors.New("init error"),
	}
	err := EnsureGroupAndStreamAcrossRegions(context.Background(), []string{"r1"}, "g", "s", factory)
	assert.Error(t, err, "should return error")
}

func TestEnsureGroupAndStreamAcrossRegions_Success(t *testing.T) {
	// set up fake where group & stream both exist
	fake := &FakeCloudWatchLogsClient{
		Region: "r2",
		DescribeLogGroupsPages: []*cloudwatchlogs.DescribeLogGroupsOutput{
			{LogGroups: []cwlTypes.LogGroup{{LogGroupName: aws.String("grp")}}},
		},
		DescribeLogStreamsPages: []*cloudwatchlogs.DescribeLogStreamsOutput{
			{LogStreams: []cwlTypes.LogStream{{LogStreamName: aws.String("str")}}},
		},
		ErrOnDescribeLogGroupsCall:  -1,
		ErrOnDescribeLogStreamsCall: -1,
		ErrCreateLogGroup:           false,
		ErrCreateLogStream:          false,
	}

	factory := &testFactory{
		client: fake,
		err:    nil,
	}

	err := EnsureGroupAndStreamAcrossRegions(context.Background(), []string{"r2"}, "grp", "str", factory)
	assert.NoError(t, err)
	// no creations
	assert.Equal(t, 1, fake.callDescribeLogGroupsCount)
	assert.Equal(t, 1, fake.callDescribeLogStreamsCount)
	assert.Equal(t, 0, fake.callCreateLogGroupCount)
	assert.Equal(t, 0, fake.callCreateLogStreamCount)
}

func TestEnsureGroupAndStreamAcrossRegions_CreateStream(t *testing.T) {
	// group exists, stream does not => stream create
	fake := &FakeCloudWatchLogsClient{
		Region: "r3",
		DescribeLogGroupsPages: []*cloudwatchlogs.DescribeLogGroupsOutput{
			{LogGroups: []cwlTypes.LogGroup{{LogGroupName: aws.String("g3")}}},
		},
		DescribeLogStreamsPages: []*cloudwatchlogs.DescribeLogStreamsOutput{
			{}, // no stream on first page
		},
		ErrOnDescribeLogGroupsCall:  -1,
		ErrOnDescribeLogStreamsCall: -1,
		ErrCreateLogGroup:           false,
		ErrCreateLogStream:          false,
	}

	factory := &testFactory{
		client: fake,
		err:    nil,
	}

	err := EnsureGroupAndStreamAcrossRegions(context.Background(), []string{"r3"}, "g3", "s3", factory)
	assert.NoError(t, err)
	assert.Equal(t, 1, fake.callCreateLogStreamCount)
}
