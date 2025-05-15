// Package efsclient provides a wrapper around AWS EFS SDK client
package efsclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants
	errInvalidRegion = "efsclient creation failed. invalid region"
)

// EFSClient defines an interface for interacting with AWS EFS service
type EFSClient interface {
	// GetRegion returns the AWS region this client is configured for
	GetRegion() string
	// DescribeFileSystems retrieves information about EFS file systems in the AWS account
	DescribeFileSystems(ctx context.Context, input *efs.DescribeFileSystemsInput, optFns ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error)
	// DescribeMountTargets retrieves information about mount targets for a file system
	DescribeMountTargets(ctx context.Context, input *efs.DescribeMountTargetsInput, optFns ...func(*efs.Options)) (*efs.DescribeMountTargetsOutput, error)
}

// efsClientImpl implements the EFSClient interface
type efsClientImpl struct {
	// client is the underlying AWS EFS SDK client
	client *efs.Client
	// region is the AWS region this client is configured for
	region string
}

// NewEFSClient creates a new EFSClient
func NewEFSClient(client *efs.Client, region string) (EFSClient, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &efsClientImpl{
		client: client,
		region: region,
	}, nil
}

// DescribeMountTargets calls the DescribeMountTargets API operation
func (c *efsClientImpl) DescribeMountTargets(ctx context.Context, input *efs.DescribeMountTargetsInput, optFns ...func(*efs.Options)) (*efs.DescribeMountTargetsOutput, error) {
	return c.client.DescribeMountTargets(ctx, input, optFns...)
}

// GetRegion returns the region of the client
func (c *efsClientImpl) GetRegion() string {
	return c.region
}

// DescribeFileSystems calls the DescribeFileSystems API operation
func (c *efsClientImpl) DescribeFileSystems(ctx context.Context, input *efs.DescribeFileSystemsInput, optFns ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error) {
	return c.client.DescribeFileSystems(ctx, input, optFns...)
}
