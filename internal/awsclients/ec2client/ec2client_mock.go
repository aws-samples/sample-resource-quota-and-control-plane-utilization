package ec2client

import (
	"context"
	"errors"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// FakeEC2Client implements all the EC2Client methods you use, with AWS-style pagination.
type FakeEC2Client struct {
	Region string

	// pages for paginator calls:
	DescribeVpcsPages                         []*ec2.DescribeVpcsOutput
	DescribeNetworkInterfacesPages            []*ec2.DescribeNetworkInterfacesOutput
	DescribeSubnetsPages                      []*ec2.DescribeSubnetsOutput
	DescribeTransitGatewayVpcAttachmentsPages []*ec2.DescribeTransitGatewayVpcAttachmentsOutput

	// simple (non-paginated) responses:
	NatGateways  []ec2Types.NatGateway
	VpcEndpoints []ec2Types.VpcEndpoint

	// “throw on this call index” for each paginated method:
	ErrOnDescribeVpcsCall         int
	ErrOnDescribeENICall          int
	ErrOnDescribeSubnetCall       int
	ErrOnDescribeTGWVpcAttachCall int

	// simple error flags:
	ErrNat         bool
	ErrVPCEndpoint bool

	// internal counters:
	callVpcsCount             int
	callENICount              int
	callSubnetCount           int
	callTGWVpcAttachCount     int
	callDescribeVpcsNextCount int
}

// DescribeVpcs paginates DescribeVpcsPages, honoring NextToken and injected errors.
func (f *FakeEC2Client) DescribeVpcs(
	ctx context.Context,
	in *ec2.DescribeVpcsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeVpcsOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.callDescribeVpcsNextCount == f.ErrOnDescribeVpcsCall {
		return nil, errors.New("ec2 DescribeVpcs injected error")
	}

	idx := 0
	if in.NextToken != nil {
		i, err := strconv.Atoi(*in.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	var out *ec2.DescribeVpcsOutput
	if idx < len(f.DescribeVpcsPages) {
		page := f.DescribeVpcsPages[idx]
		out = &ec2.DescribeVpcsOutput{Vpcs: page.Vpcs}
	} else {
		out = &ec2.DescribeVpcsOutput{}
	}

	if idx+1 < len(f.DescribeVpcsPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}
	f.callDescribeVpcsNextCount++
	return out, nil
}

// DescribeNetworkInterfaces pages DescribeNetworkInterfacesPages.
func (f *FakeEC2Client) DescribeNetworkInterfaces(
	ctx context.Context,
	in *ec2.DescribeNetworkInterfacesInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeNetworkInterfacesOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.callENICount == f.ErrOnDescribeENICall {
		return nil, errors.New("ec2 DescribeNetworkInterfaces injected error")
	}

	idx := 0
	if in.NextToken != nil {
		i, err := strconv.Atoi(*in.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	var out *ec2.DescribeNetworkInterfacesOutput
	if idx < len(f.DescribeNetworkInterfacesPages) {
		page := f.DescribeNetworkInterfacesPages[idx]
		out = &ec2.DescribeNetworkInterfacesOutput{
			NetworkInterfaces: page.NetworkInterfaces,
		}
	} else {
		out = &ec2.DescribeNetworkInterfacesOutput{}
	}

	if idx+1 < len(f.DescribeNetworkInterfacesPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}
	f.callENICount++
	return out, nil
}

// DescribeSubnets pages DescribeSubnetsPages.
func (f *FakeEC2Client) DescribeSubnets(
	ctx context.Context,
	in *ec2.DescribeSubnetsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeSubnetsOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.callSubnetCount == f.ErrOnDescribeSubnetCall {
		return nil, errors.New("ec2 DescribeSubnets injected error")
	}

	idx := 0
	if in.NextToken != nil {
		i, err := strconv.Atoi(*in.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	var out *ec2.DescribeSubnetsOutput
	if idx < len(f.DescribeSubnetsPages) {
		page := f.DescribeSubnetsPages[idx]
		out = &ec2.DescribeSubnetsOutput{Subnets: page.Subnets}
	} else {
		out = &ec2.DescribeSubnetsOutput{}
	}

	if idx+1 < len(f.DescribeSubnetsPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}
	f.callSubnetCount++
	return out, nil
}

// DescribeTransitGatewayVpcAttachments pages DescribeTransitGatewayVpcAttachmentsPages.
func (f *FakeEC2Client) DescribeTransitGatewayVpcAttachments(
	ctx context.Context,
	in *ec2.DescribeTransitGatewayVpcAttachmentsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.callTGWVpcAttachCount == f.ErrOnDescribeTGWVpcAttachCall {
		return nil, errors.New("ec2 DescribeTransitGatewayVpcAttachments injected error")
	}

	idx := 0
	if in.NextToken != nil {
		i, err := strconv.Atoi(*in.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	var out *ec2.DescribeTransitGatewayVpcAttachmentsOutput
	if idx < len(f.DescribeTransitGatewayVpcAttachmentsPages) {
		page := f.DescribeTransitGatewayVpcAttachmentsPages[idx]
		out = &ec2.DescribeTransitGatewayVpcAttachmentsOutput{
			TransitGatewayVpcAttachments: page.TransitGatewayVpcAttachments,
		}
	} else {
		out = &ec2.DescribeTransitGatewayVpcAttachmentsOutput{}
	}

	if idx+1 < len(f.DescribeTransitGatewayVpcAttachmentsPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}
	f.callTGWVpcAttachCount++
	return out, nil
}

// DescribeNatGateways returns the static slice or an error.
func (f *FakeEC2Client) DescribeNatGateways(
	ctx context.Context,
	in *ec2.DescribeNatGatewaysInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeNatGatewaysOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.ErrNat {
		return nil, errors.New("ec2 DescribeNatGateways injected error")
	}
	return &ec2.DescribeNatGatewaysOutput{NatGateways: f.NatGateways}, nil
}

// DescribeVpcEndpoints returns the static slice or an error.
func (f *FakeEC2Client) DescribeVpcEndpoints(
	ctx context.Context,
	in *ec2.DescribeVpcEndpointsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeVpcEndpointsOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if f.ErrVPCEndpoint {
		return nil, errors.New("ec2 DescribeVpcEndpoints injected error")
	}
	return &ec2.DescribeVpcEndpointsOutput{VpcEndpoints: f.VpcEndpoints}, nil
}

// Reset clears all internal counters.
func (f *FakeEC2Client) Reset() {
	f.callVpcsCount = 0
	f.callENICount = 0
	f.callSubnetCount = 0
	f.callTGWVpcAttachCount = 0
}

// GetRegion returns the configured region.
func (f *FakeEC2Client) GetRegion() string {
	return f.Region
}
