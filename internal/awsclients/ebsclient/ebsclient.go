// Package ebsclient provides a wrapper around AWS EC2 SDK client for EBS operations
package ebsclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	errInvalidRegion = "ebsclient creation failed. invalid region"
)

// EBSClient defines an interface for interacting with EBS volumes
type EBSClient interface {
	GetRegion() string
	DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
}

// ebsClientImpl implements EBSClient
type ebsClientImpl struct {
	client *ec2.Client
	region string
}

// NewEBSClient creates and returns a new EBS client implementation
func NewEBSClient(client *ec2.Client, region string) (EBSClient, error) {
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &ebsClientImpl{
		client: client,
		region: region,
	}, nil
}

// DescribeVolumes retrieves information about EBS volumes
func (c *ebsClientImpl) DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return c.client.DescribeVolumes(ctx, params, optFns...)
}

// GetRegion returns the region the client is created in
func (c *ebsClientImpl) GetRegion() string {
	return c.region
}