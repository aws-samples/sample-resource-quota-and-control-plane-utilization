package efsclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// EFSClient defines an interface for using the aws efs client
type EFSClient interface {
	GetRegion() string
	// describe file systems
	DescribeFileSystems(ctx context.Context, input *efs.DescribeFileSystemsInput, optFns ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error)
	DescribeMountTargets(ctx context.Context, input *efs.DescribeMountTargetsInput, optFns ...func(*efs.Options)) (*efs.DescribeMountTargetsOutput, error)
}

// EFSClientImpl implements the EFSClient interface
type EFSClientImpl struct {
	client *efs.Client
	region string
}

// NewEFSClient creates a new EFSClient
func NewEFSClient(cfg aws.Config, region string) (*EFSClientImpl, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("efsclient creation failed. invalid region")
	}

	efsClient := efs.NewFromConfig(cfg, func(o *efs.Options) {
		o.Region = region
	})
	return &EFSClientImpl{
		client: efsClient,
	}, nil
}

// DescribeMountTargets calls the DescribeMountTargets API operation
func (c *EFSClientImpl) DescribeMountTargets(ctx context.Context, input *efs.DescribeMountTargetsInput, optFns ...func(*efs.Options)) (*efs.DescribeMountTargetsOutput, error) {
	return c.client.DescribeMountTargets(ctx, input, optFns...)
}

// GetRegion returns the region of the client
func (c *EFSClientImpl) GetRegion() string {
	return c.region
}

// DescribeFileSystems calls the DescribeFileSystems API operation
func (c *EFSClientImpl) DescribeFileSystems(ctx context.Context, input *efs.DescribeFileSystemsInput, optFns ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error) {
	return c.client.DescribeFileSystems(ctx, input, optFns...)
}
