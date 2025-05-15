package ec2client

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
)

// TestNewEc2Client tests the NewEc2Client function
func TestNewEc2Client(t *testing.T) {
	tests := []struct {
		name        string
		region      string
		expectError bool
	}{
		{
			name:        "Valid region",
			region:      "us-east-1",
			expectError: false,
		},
		{
			name:        "Invalid region",
			region:      "invalid-region",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := ec2.NewFromConfig(aws.Config{})
			ec2Client, err := NewEc2Client(client, tt.region)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, ec2Client)
				assert.Contains(t, err.Error(), "invalid region")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ec2Client)
				assert.Equal(t, tt.region, ec2Client.GetRegion())
			}
		})
	}
}

// TestEc2ClientImpl_GetRegion tests the GetRegion method
func TestEc2ClientImpl_GetRegion(t *testing.T) {
	region := "us-west-2"
	client := &ec2ClientImpl{
		region: region,
	}

	assert.Equal(t, region, client.GetRegion())
}

// TestEc2ClientImpl_DescribeVpcs tests the DescribeVpcs method
func TestEc2ClientImpl_DescribeVpcs(t *testing.T) {
	ec2Client := ec2.NewFromConfig(aws.Config{})
	client := ec2ClientImpl{
		client: ec2Client,
	}
	ctx := context.Background()
	input := &ec2.DescribeVpcsInput{}

	output, err := client.DescribeVpcs(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}

// TestEc2ClientImpl_DescribeNetworkInterfaces tests the DescribeNetworkInterfaces method
func TestEc2ClientImpl_DescribeNetworkInterfaces(t *testing.T) {
	ec2Client := ec2.NewFromConfig(aws.Config{})
	client := ec2ClientImpl{
		client: ec2Client,
	}
	ctx := context.Background()
	input := &ec2.DescribeNetworkInterfacesInput{}

	output, err := client.DescribeNetworkInterfaces(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}

// TestEc2ClientImpl_DescribeNatGateways tests the DescribeNatGateways method
func TestEc2ClientImpl_DescribeNatGateways(t *testing.T) {
	ec2Client := ec2.NewFromConfig(aws.Config{})
	client := ec2ClientImpl{
		client: ec2Client,
	}
	ctx := context.Background()
	input := &ec2.DescribeNatGatewaysInput{}

	output, err := client.DescribeNatGateways(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}

// TestEc2ClientImpl_DescribeVpcEndpoints tests the DescribeVpcEndpoints method
func TestEc2ClientImpl_DescribeVpcEndpoints(t *testing.T) {
	ec2Client := ec2.NewFromConfig(aws.Config{})
	client := ec2ClientImpl{
		client: ec2Client,
	}
	ctx := context.Background()
	input := &ec2.DescribeVpcEndpointsInput{}

	output, err := client.DescribeVpcEndpoints(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}

// TestEc2ClientImpl_DescribeSubnets tests the DescribeSubnets method
func TestEc2ClientImpl_DescribeSubnets(t *testing.T) {
	ec2Client := ec2.NewFromConfig(aws.Config{})
	client := ec2ClientImpl{
		client: ec2Client,
	}
	ctx := context.Background()
	input := &ec2.DescribeSubnetsInput{}

	output, err := client.DescribeSubnets(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}

// TestEc2ClientImpl_DescribeTransitGatewayVpcAttachments tests the DescribeTransitGatewayVpcAttachments method
func TestEc2ClientImpl_DescribeTransitGatewayVpcAttachments(t *testing.T) {
	ec2Client := ec2.NewFromConfig(aws.Config{})
	client := ec2ClientImpl{
		client: ec2Client,
	}
	ctx := context.Background()
	input := &ec2.DescribeTransitGatewayVpcAttachmentsInput{}

	output, err := client.DescribeTransitGatewayVpcAttachments(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}
