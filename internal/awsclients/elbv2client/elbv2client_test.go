package elbv2client

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/stretchr/testify/assert"
)

// TestNewElbv2Client tests the NewEc2Client function
func TestNewElbv2Client(t *testing.T) {
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
			client := elasticloadbalancingv2.NewFromConfig(aws.Config{})
			elbv2Client, err := NewElbV2Client(client, tt.region)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, elbv2Client)
				assert.Contains(t, err.Error(), "invalid region")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, elbv2Client)
				assert.Equal(t, tt.region, elbv2Client.GetRegion())
			}
		})
	}
}

// TestElbv2ClientImpl_GetRegion tests the GetRegion method
func TestElbv2ClientImpl_GetRegion(t *testing.T) {
	region := "us-west-2"
	client := &elbv2ClientImpl{
		region: region,
	}

	assert.Equal(t, region, client.GetRegion())
}

// TestElbv2ClientImpl_DescribeLoadBalancers tests the DescribeLoadBalancers method
func TestElbv2ClientImpl_DescribeLoadBalancers(t *testing.T) {
	elbv2Client := elasticloadbalancingv2.NewFromConfig(aws.Config{})
	client := elbv2ClientImpl{
		client: elbv2Client,
	}
	ctx := context.Background()
	input := &elasticloadbalancingv2.DescribeLoadBalancersInput{}

	output, err := client.DescribeLoadBalancers(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}
