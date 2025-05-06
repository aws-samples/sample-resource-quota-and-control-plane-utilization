package ec2client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// Ec2Client defines an interface for using AWS ec2 client
type Ec2Client interface {
	GetRegion() string
	DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)
	DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error)
	DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error)
}

// Ec2ClientImpl implements Ec2Client interface
type Ec2ClientImpl struct {
	client *ec2.Client
	region string
}

// DescribeNetworkInterfaces calls ec2 client's DescribeNetworkInterfaces method
func (c *Ec2ClientImpl) DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	return c.client.DescribeNetworkInterfaces(ctx, params, optFns...)
}

// NewEc2Client returns a new Ec2Client
func NewEc2Client(cfg aws.Config, region string) (Ec2Client, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("ec2client creation failed. invalid region")
	}

	client := ec2.NewFromConfig(cfg, func(o *ec2.Options) {
		o.Region = region
	})
	return &Ec2ClientImpl{
		client: client,
		region: region,
	}, nil
}

// DescribeNatGateways calls ec2 client's DescribeNatGateways method
func (c *Ec2ClientImpl) DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	return c.client.DescribeNatGateways(ctx, params, optFns...)
}

// DescribeVpcEndpoints calls ec2 client's DescribeVpcEndpoints method
func (c *Ec2ClientImpl) DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error) {
	return c.client.DescribeVpcEndpoints(ctx, params, optFns...)
}

// DescribeSubnets calls ec2 client's DescribeSubnets method
func (c *Ec2ClientImpl) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return c.client.DescribeSubnets(ctx, params, optFns...)
}

// GetRegion returns the region of the client
func (c *Ec2ClientImpl) GetRegion() string {
	return c.region
}

func (c *Ec2ClientImpl) DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return c.client.DescribeVpcs(ctx, params, optFns...)
}

// describe transit gateway vpc attachments
func (c *Ec2ClientImpl) DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	return c.client.DescribeTransitGatewayVpcAttachments(ctx, params, optFns...)
}
