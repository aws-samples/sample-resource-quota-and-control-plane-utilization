// Package ec2client provides a wrapper around AWS EC2 SDK client
package ec2client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants
	errInvalidRegion = "ec2client creation failed. invalid region"
)

// Ec2Client defines an interface for interacting with AWS EC2 service
type Ec2Client interface {
	// GetRegion returns the AWS region the client is configured for
	GetRegion() string
	// DescribeVpcs retrieves information about VPCs in the AWS account
	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	// DescribeNetworkInterfaces retrieves information about network interfaces in the AWS account
	DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	// DescribeNatGateways retrieves information about NAT gateways in the AWS account
	DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)
	// DescribeVpcEndpoints retrieves information about VPC endpoints in the AWS account
	DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error)
	// DescribeSubnets retrieves information about subnets in the AWS account
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	// DescribeTransitGatewayVpcAttachments retrieves information about Transit Gateway VPC attachments
	DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error)
}

// ec2ClientImpl implements the Ec2Client interface
type ec2ClientImpl struct {
	// client is the underlying AWS EC2 SDK client
	client *ec2.Client
	// region is the AWS region this client is configured for
	region string
}

// DescribeNetworkInterfaces retrieves information about network interfaces in the AWS account
func (c *ec2ClientImpl) DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	return c.client.DescribeNetworkInterfaces(ctx, params, optFns...)
}

// NewEc2Client creates and returns a new EC2 client implementation
// It validates the provided region and returns an error if invalid
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
