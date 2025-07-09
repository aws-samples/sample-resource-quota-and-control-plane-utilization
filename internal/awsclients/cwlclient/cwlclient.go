// Package cwlclient provides a wrapper around the AWS CloudWatch Logs SDK client
// with region validation, interface abstraction, and utility functions for log management.
package cwlclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message templates for CloudWatch Logs operations.
	errInvalidRegion        = "cloudwatchlogsclient creation failed. invalid region"
	errDescribeLogGroups    = "[%s] describe log groups: %w"
	errCreateLogGroup       = "[%s] create log group %q: %w"
	errDescribeLogStreams   = "[%s] describe log streams: %w"
	errCreateLogStream      = "[%s] create log stream %q: %w"
	errClientInit           = "[%s] client init: %w"
)

// CloudWatchLogsClient defines an interface for AWS CloudWatch Logs operations
// including log event publishing and log group/stream management.
type CloudWatchLogsClient interface {
	// GetRegion returns the AWS region this client is configured for.
	GetRegion() string
	// PutLogEvents sends log events to CloudWatch Logs in batches.
	PutLogEvents(ctx context.Context, params *cloudwatchlogs.PutLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error)
	// CreateLogGroup creates a new log group in CloudWatch Logs.
	CreateLogGroup(ctx context.Context, params *cloudwatchlogs.CreateLogGroupInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
	// DescribeLogGroups retrieves information about existing log groups.
	DescribeLogGroups(ctx context.Context, params *cloudwatchlogs.DescribeLogGroupsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	// DescribeLogStreams retrieves information about log streams in a log group.
	DescribeLogStreams(ctx context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error)
	// CreateLogStream creates a new log stream in a log group.
	CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error)
}

// CloudWatchLogsClientImpl implements the CloudWatchLogsClient interface using the AWS SDK.
type CloudWatchLogsClientImpl struct {
	region string                    // AWS region this client is configured for
	client *cloudwatchlogs.Client    // Underlying AWS CloudWatch Logs SDK client
}

// NewCloudWatchLogsClient creates a new CloudWatch Logs client wrapper with region validation.
func NewCloudWatchLogsClient(client *cloudwatchlogs.Client, region string) (CloudWatchLogsClient, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &CloudWatchLogsClientImpl{
		client: client,
		region: region,
	}, nil
}

// PutLogEvents sends log events to CloudWatch Logs in batches.
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

// GetRegion returns the AWS region this client is configured for
func (c *CloudWatchLogsClientImpl) GetRegion() string {
	return c.region
}

// DescribeLogStreams retrieves information about log streams in a log group
func (c *CloudWatchLogsClientImpl) DescribeLogStreams(ctx context.Context, params *cloudwatchlogs.DescribeLogStreamsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	return c.client.DescribeLogStreams(ctx, params, optFns...)
}

// CreateLogStream creates a new log stream in a log group
func (c *CloudWatchLogsClientImpl) CreateLogStream(ctx context.Context, params *cloudwatchlogs.CreateLogStreamInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	return c.client.CreateLogStream(ctx, params, optFns...)
}

// EnsureLogGroupExists checks if a log group exists and creates it if not found.
// It uses pagination to search through existing log groups and handles race conditions.
func EnsureLogGroupExists(ctx context.Context, client CloudWatchLogsClient, groupName string) error {
	paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(client, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: &groupName,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf(errDescribeLogGroups, client.GetRegion(), err)
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
			return nil // race condition—group was created by another process
		}
		return fmt.Errorf(errCreateLogGroup, client.GetRegion(), groupName, err)
	}
	return nil
}

// EnsureLogStreamExists checks if a log stream exists and creates it if not found.
// It uses pagination to search through existing log streams and handles race conditions.
func EnsureLogStreamExists(ctx context.Context, client CloudWatchLogsClient, groupName, streamName string) error {
	paginator := cloudwatchlogs.NewDescribeLogStreamsPaginator(client, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        &groupName,
		LogStreamNamePrefix: &streamName,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf(errDescribeLogStreams, client.GetRegion(), err)
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
			return nil // race condition—group was created by another process
		}
		return fmt.Errorf(errCreateLogStream, client.GetRegion(), streamName, err)
	}
	return nil
}

// EnsureGroupAndStreamAcrossRegions creates log groups and streams across multiple regions.
// It uses the provided factory to create regional clients and ensures consistent log infrastructure.
func EnsureGroupAndStreamAcrossRegions(
	ctx context.Context,
	regions []string,
	groupName, streamName string,
	factory interface {
		CreateCloudWatchLogs(region string) (CloudWatchLogsClient, error)
	},
) error {
	for _, region := range regions {
		client, err := factory.CreateCloudWatchLogs(region)
		if err != nil {
			return fmt.Errorf(errClientInit, region, err)
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
