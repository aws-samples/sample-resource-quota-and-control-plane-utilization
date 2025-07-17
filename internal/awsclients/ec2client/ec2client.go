// Package ec2client provides a wrapper around the AWS EC2 SDK client
// with region validation and interface abstraction for testing and modularity.
package ec2client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants for EC2 client operations.
	errInvalidRegion = "ec2client creation failed. invalid region"
)

// Ec2Client defines an interface for AWS EC2 service operations
// with methods commonly used for resource monitoring and NAU calculations.
type Ec2Client interface {
	// GetRegion returns the AWS region this client is configured for.
	GetRegion() string
	// DescribeVpcs retrieves information about VPCs in the AWS account.
	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	// DescribeNetworkInterfaces retrieves information about network interfaces.
	DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	// DescribeNatGateways retrieves information about NAT gateways.
	DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)
	// DescribeVpcEndpoints retrieves information about VPC endpoints.
	DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error)
	// DescribeSubnets retrieves information about subnets.
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	// DescribeTransitGatewayVpcAttachments retrieves Transit Gateway VPC attachment information.
	DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error)
	// DescribeAvailabilityZones retrieves information about Availability Zones.
	DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
	// DescribeVolumes retrieves information about EBS volumes.
	DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
}

// ec2ClientImpl implements the Ec2Client interface using the AWS SDK.
type ec2ClientImpl struct {
	client *ec2.Client // Underlying AWS EC2 SDK client
	region string      // AWS region this client is configured for
}

// DescribeNetworkInterfaces retrieves network interface information from AWS EC2.
func (c *ec2ClientImpl) DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	return c.client.DescribeNetworkInterfaces(ctx, params, optFns...)
}

// NewEc2Client creates a new EC2 client wrapper with region validation.
func NewEc2Client(client *ec2.Client, region string) (Ec2Client, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &ec2ClientImpl{
		client: client,
		region: region,
	}, nil
}

// DescribeNatGateways retrieves information about NAT gateways in the AWS account
func (c *ec2ClientImpl) DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	return c.client.DescribeNatGateways(ctx, params, optFns...)
}

// DescribeVpcEndpoints retrieves information about VPC endpoints in the AWS account
func (c *ec2ClientImpl) DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error) {
	return c.client.DescribeVpcEndpoints(ctx, params, optFns...)
}

// DescribeSubnets retrieves information about subnets in the AWS account
func (c *ec2ClientImpl) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return c.client.DescribeSubnets(ctx, params, optFns...)
}

// GetRegion returns the AWS region this client is configured for
func (c *ec2ClientImpl) GetRegion() string {
	return c.region
}

// DescribeVpcs retrieves information about VPCs in the AWS account
func (c *ec2ClientImpl) DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return c.client.DescribeVpcs(ctx, params, optFns...)
}

// DescribeTransitGatewayVpcAttachments retrieves information about Transit Gateway VPC attachments
func (c *ec2ClientImpl) DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	return c.client.DescribeTransitGatewayVpcAttachments(ctx, params, optFns...)
}

// DescribeAvailabilityZones retrieves information about Availability Zones
func (c *ec2ClientImpl) DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return c.client.DescribeAvailabilityZones(ctx, params, optFns...)
}

// DescribeVolumes retrieves information about EBS volumes
func (c *ec2ClientImpl) DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return c.client.DescribeVolumes(ctx, params, optFns...)
}
