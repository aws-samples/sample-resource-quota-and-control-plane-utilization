package elbv2client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// FakeELBV2Client implements the subset of ElbV2Client you need.
type FakeELBV2Client struct {
	Region string
	// Each successive DescribeLoadBalancers call returns the next element.
	DescribeOutputs []*elasticloadbalancingv2.DescribeLoadBalancersOutput
	// If >= 0, that call index returns an error.
	ErrorOnCall int
	callCount   int
}

func (f *FakeELBV2Client) DescribeLoadBalancers(
	ctx context.Context,
	in *elasticloadbalancingv2.DescribeLoadBalancersInput,
	optFns ...func(*elasticloadbalancingv2.Options),
) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	if f.callCount == f.ErrorOnCall {
		return nil, errors.New("elbv2 describeloadbalancers error")
	}
	if f.callCount < len(f.DescribeOutputs) {
		out := f.DescribeOutputs[f.callCount]
		f.callCount++
		return out, nil
	}
	return &elasticloadbalancingv2.DescribeLoadBalancersOutput{}, nil
}

// clear internal call count
func (f *FakeELBV2Client) Reset() {
	f.callCount = 0
}

// get region
func (f *FakeELBV2Client) GetRegion() string {
	return f.Region
}
