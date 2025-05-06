package elbv2client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// Elbv2Client defines an interface for using elbv2 client
type ElbV2Client interface {
	GetRegion() string
	DescribeLoadBalancers(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)
}

// ElbV2ClientImpl implements ElbV2Client interface
type ElbV2ClientImpl struct {
	region string
	client *elasticloadbalancingv2.Client
}

// NewElbV2Client creates a new ElbV2Client
func NewElbV2Client(cfg aws.Config, region string) (ElbV2Client, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("elbv2client creation failed. invalid region")
	}

	client := elasticloadbalancingv2.NewFromConfig(cfg, func(o *elasticloadbalancingv2.Options) {
		o.Region = region
	})
	return &ElbV2ClientImpl{
		client: client,
		region: region,
	}, nil
}

func (c *ElbV2ClientImpl) DescribeLoadBalancers(ctx context.Context, params *elasticloadbalancingv2.DescribeLoadBalancersInput, optFns ...func(*elasticloadbalancingv2.Options)) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	return c.client.DescribeLoadBalancers(ctx, params, optFns...)
}

// get region
func (c *ElbV2ClientImpl) GetRegion() string {
	return c.region
}
