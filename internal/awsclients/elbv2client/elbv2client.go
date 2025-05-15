// Package elbv2client provides a wrapper around AWS Elastic Load Balancing v2 SDK client
package elbv2client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants
	errInvalidRegion = "elbv2client creation failed. invalid region"
)

// ElbV2Client defines an interface for interacting with AWS Elastic Load Balancing v2 service
type ElbV2Client interface {
	// GetRegion returns the AWS region this client is configured for
	GetRegion() string
	// DescribeLoadBalancers retrieves information about load balancers in the AWS account
	DescribeLoadBalancers(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
}

// elbv2ClientImpl implements the ElbV2Client interface
type elbv2ClientImpl struct {
	// region is the AWS region this client is configured for
	region string
	// client is the underlying AWS ELBv2 SDK client
	client *elasticloadbalancingv2.Client
}

// NewElbV2Client creates a new ElbV2Client
func NewElbV2Client(client *elasticloadbalancingv2.Client, region string) (ElbV2Client, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &elbv2ClientImpl{
		client: client,
		region: region,
	}, nil
}

// DescribeLoadBalancers retrieves information about load balancers in the AWS account
func (c *elbv2ClientImpl) DescribeLoadBalancers(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	return c.client.DescribeLoadBalancers(ctx, params, optFns...)
}

// GetRegion returns the AWS region this client is configured for
func (c *elbv2ClientImpl) GetRegion() string {
	return c.region
}
