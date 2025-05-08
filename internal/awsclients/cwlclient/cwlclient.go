package cwlclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// CloudWatchClient defines an interfafce for using AWS cloudwatch logs client
type CloudWatchLogsClient interface {
	GetRegion() string
	// PutLogEvents puts logs events into cloudwatch in batches
	PutLogEvents(ctx context.Context, params *cloudwatchlogs.PutLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error)
	// Create log group creates log group
	CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
	// DescribeLogGroups describes log groups
	DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	// Describe Log streams
	DescribeLogStreams(ctx context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error)
	// Create Log stream
	CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error)
}

// CloudWatchLogsClientImpl implements the CloudWatchLogsClient interface
type CloudWatchLogsClientImpl struct {
	region string
	client *cloudwatchlogs.Client
}

// NewCloudWatchLogsClient creates a new CloudWatchLogsClient
func NewCloudWatchLogsClient(cfg aws.Config, region string) (CloudWatchLogsClient, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("cloudwatchlogsclient creation failed. invalid region")
	}

	client := cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) {
		o.Region = region
	})
	return &CloudWatchLogsClientImpl{
		client: client,
		region: region}, nil
}

// PutLogEvents puts logs events into cloudwatch in batches
func (c *CloudWatchLogsClientImpl) PutLogEvents(ctx context.Context, params *cloudwatchlogs.PutLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	return c.client.PutLogEvents(ctx, params, optFns...)
}

// CreateLogGroup creates log group
func (c *CloudWatchLogsClientImpl) CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	return c.client.CreateLogGroup(ctx, params, optFns...)
}

// DescribeLogGroups describes log groups
func (c *CloudWatchLogsClientImpl) DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	return c.client.DescribeLogGroups(ctx, params, optFns...)
}

// get region
func (c *CloudWatchLogsClientImpl) GetRegion() string {
	return c.region
}

func (c *CloudWatchLogsClientImpl) DescribeLogStreams(ctx context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return c.client.DescribeLogStreams(ctx, params, optFns...)
}

func (c *CloudWatchLogsClientImpl) CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return c.client.CreateLogStream(ctx, params, optFns...)
}

// / EnsureLogGroupExists will page through DescribeLogGroups via
// the SDK‐provided paginator, and CreateLogGroup if no exact match.
func EnsureLogGroupExists(ctx context.Context, client CloudWatchLogsClient, groupName string) error {
	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(client, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: &groupName,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("[%s] describe log groups: %w", client.GetRegion(), err)
		}
		for _, g := range page.LogGroups {
			if *g.LogGroupName == groupName {
				return nil // found it
			}
		}
	}
	// not found—create it
	if _, err := client.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: &groupName,
	}); err != nil {
		var existsErr *cwlTypes.ResourceAlreadyExistsException
		var abortedErr *cwlTypes.OperationAbortedException
		if errors.As(err, &existsErr) || errors.As(err, &abortedErr) {
			return nil // race condition—group was created by XXXXXXX process
		}
		return fmt.Errorf("[%s] create log group %q: %w", client.GetRegion(), groupName, err)
	}
	return nil
}

// EnsureLogStreamExists will page through DescribeLogStreams via
// the SDK paginator, and CreateLogStream if no exact match.
func EnsureLogStreamExists(ctx context.Context, client CloudWatchLogsClient, groupName, streamName string) error {
	paginator := cloudwatchlogs.NewDescribeLogStreamsPaginator(client, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        &groupName,
		LogStreamNamePrefix: &streamName,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("[%s] describe log streams: %w", client.GetRegion(), err)
		}
		for _, s := range page.LogStreams {
			if *s.LogStreamName == streamName {
				return nil // found it
			}
		}
	}
	// not found—create it
	if _, err := client.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  &groupName,
		LogStreamName: &streamName,
	}); err != nil {
		var existsErr *cwlTypes.ResourceAlreadyExistsException
		var abortedErr *cwlTypes.OperationAbortedException
		if errors.As(err, &existsErr) || errors.As(err, &abortedErr) {
			return nil // race condition—group was created by XXXXXXX process
		}
		return fmt.Errorf("[%s] create log stream %q: %w", client.GetRegion(), streamName, err)
	}
	return nil
}

// ClientFactory produces a CloudWatchLogsClient for any region.
type ClientFactory func(region string) (CloudWatchLogsClient, error)

// EnsureGroupAndStreamAcrossRegions will, for each region, spin up
// a client via factory(), then EnsureLogGroupExists and EnsureLogStreamExists.
func EnsureGroupAndStreamAcrossRegions(
	ctx context.Context,
	regions []string,
	groupName, streamName string,
	factory ClientFactory,
) error {
	for _, region := range regions {
		client, err := factory(region)
		if err != nil {
			return fmt.Errorf("[%s] client init: %w", region, err)
		}
		if err := EnsureLogGroupExists(ctx, client, groupName); err != nil {
			return err
		}
		if err := EnsureLogStreamExists(ctx, client, groupName, streamName); err != nil {
			return err
		}
	}
	return nil
}
