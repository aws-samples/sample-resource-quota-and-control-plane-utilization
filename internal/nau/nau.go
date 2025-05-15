// Package nau provides functionality to calculate Network Address Utilization (NAU) for AWS VPCs.
// NAU is a metric used by AWS to measure the utilization of networking resources within a VPC.
package nau

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2Types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"golang.org/x/sync/errgroup"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/efsclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/elbv2client"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// paginationLimit defines the maximum number of results to return per page for all paginators
const paginationLimit = 1000

// NAUCalculator is the public interface for calculating Network Address Utilization.
type NAUCalculator interface {
	// CalculateVPCNAU returns the total NAU units for every VPC in the region.
	CalculateVPCNAU(ctx context.Context) (map[string]int64, error)
	// GetRegion returns the AWS region this calculator is operating in.
	GetRegion() string
}

// ResourceKey distinguishes NAU resource types for weight calculation.
type ResourceKey string

const (
	// IPv4IPv6Address represents an IPv4 or IPv6 address resource.
	IPv4IPv6Address ResourceKey = "ipv4-ipv6-address"
	// ENI represents an Elastic Network Interface resource.
	ENI ResourceKey = "eni"
	// PrefixAssignedToENI represents a CIDR prefix assigned to an ENI.
	PrefixAssignedToENI ResourceKey = "prefix-assigned-to-eni"
	// NetworkLoadBalancerPerAZ represents a Network Load Balancer in an AZ.
	NetworkLoadBalancerPerAZ ResourceKey = "network-load-balancer-per-az"
	// GatewayLoadBalancerPerAZ represents a Gateway Load Balancer in an AZ.
	GatewayLoadBalancerPerAZ ResourceKey = "gateway-load-balancer-per-az"
	// VPCEndpointPerAZ represents a VPC Endpoint in an AZ.
	VPCEndpointPerAZ ResourceKey = "vpc-endpoint-per-az"
	// TransitGatewayAttachment represents a Transit Gateway attachment.
	TransitGatewayAttachment ResourceKey = "transit-gateway-attachment"
	// LambdaFunction represents a Lambda function with VPC access.
	LambdaFunction ResourceKey = "lambda-function"
	// NATGateway represents a NAT Gateway.
	NATGateway ResourceKey = "nat-gateway"
	// EFSMountTarget represents an EFS Mount Target.
	EFSMountTarget ResourceKey = "efs-mount-target"
	// EFAInterface represents an Elastic Fabric Adapter interface.
	EFAInterface ResourceKey = "efa-interface"
	// EKSPod represents an EKS Pod using VPC networking.
	EKSPod ResourceKey = "eks-pod"
)

// WeightTable maps ResourceKey to its NAU weight value.
type WeightTable struct{ table map[ResourceKey]int }

// NewWeightTable returns the AWS-documented weights for NAU resources.
func NewWeightTable() *WeightTable {
	return &WeightTable{table: map[ResourceKey]int{
		IPv4IPv6Address:          1,
		ENI:                      1,
		PrefixAssignedToENI:      1,
		NetworkLoadBalancerPerAZ: 6,
		GatewayLoadBalancerPerAZ: 6,
		VPCEndpointPerAZ:         6,
		TransitGatewayAttachment: 6,
		LambdaFunction:           6,
		NATGateway:               6,
		EFSMountTarget:           6,
		EFAInterface:             1,
		EKSPod:                   1,
	}}
}

// Get returns the weight for the specified resource key (zero if missing).
func (w *WeightTable) Get(key ResourceKey) int { return w.table[key] }

// calculator implements the NAUCalculator interface to calculate NAU values.
type calculator struct {
	ec2    ec2client.Ec2Client
	efs    efsclient.EFSClient
	elb    elbv2client.ElbV2Client
	logger logger.Logger
	wt     *WeightTable
	region string
}

// NewCalculator creates a new NAUCalculator with the provided AWS clients and logger.
func NewCalculator(
	ec2Client ec2client.Ec2Client,
	efsClient efsclient.EFSClient,
	elbClient elbv2client.ElbV2Client,
	log logger.Logger,
) NAUCalculator {
	if log == nil {
		log = &logger.NoopLogger{}
	}
	return &calculator{
		ec2:    ec2Client,
		efs:    efsClient,
		elb:    elbClient,
		logger: log,
		wt:     NewWeightTable(),
		region: ec2Client.GetRegion(),
	}
}

// nauResult holds the result of a NAU calculation with its value and potential error
type nauResult struct {
	value int64
	err   error
	name  string
}

// calculateVPCTotalNAU calculates the total NAU for a specific VPC.
// It sums the NAU values from all resource types within the VPC using concurrent goroutines.
func (c *calculator) calculateVPCTotalNAU(ctx context.Context, vpcID string) (int64, error) {
	c.logger.Debug("calculating VPC %s nau's", vpcID)

	g, ctx := errgroup.WithContext(ctx)
	results := make(chan nauResult, 6) // Buffer for all calculation results

	// Launch ENI calculation
	g.Go(func() error {
		v, err := c.calculateENINau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "ENI"}
		return err
	})

	// Launch NAT Gateway calculation
	g.Go(func() error {
		v, err := c.calculateNATGatewayNau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "NAT"}
		return err
	})

	// Launch VPC Endpoints calculation
	g.Go(func() error {
		v, err := c.calculateVPCEndpointsNau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "VPC Endpoint"}
		return err
	})

	// Launch Load Balancers calculation
	g.Go(func() error {
		v, err := c.calculateLoadBalancersNau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "LB"}
		return err
	})

	// Launch Transit Gateway attachments calculation
	g.Go(func() error {
		v, err := c.calculateTransitGatewayVpcAttachmentsNau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "TGW-VPC Attach"}
		return err
	})

	// Launch EFS Mount Targets calculation
	g.Go(func() error {
		v, err := c.calculateEFSMountTargetsInVpcNau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "EFS-in-VPC"}
		return err
	})

	// Close results channel when all goroutines complete
	go func() {
		g.Wait()
		close(results)
	}()

	// Wait for all goroutines to complete or for an error
	if err := g.Wait(); err != nil {
		return 0, err
	}

	// Sum up all results
	var total int64
	for result := range results {
		c.logger.Debug("vpcId=%s %s NAU=%d", vpcID, result.name, result.value)
		total += result.value
	}

	c.logger.Info("vpcId %s total NAU=%d", vpcID, total)
	return total, nil
}

// CalculateVPCNAU paginates through every VPC then sums each resource's NAU units.
// Returns a map of VPC IDs to their total NAU values.
func (c *calculator) CalculateVPCNAU(ctx context.Context) (map[string]int64, error) {
	out := make(map[string]int64)
	c.logger.Info("starting VPC discovery for vpc nau's")
	pv := ec2.NewDescribeVpcsPaginator(c.ec2, &ec2.DescribeVpcsInput{}, func(o *ec2.DescribeVpcsPaginatorOptions) {
		o.Limit = paginationLimit
	})
	for pv.HasMorePages() {
		page, err := pv.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing VPCs: %w", err)
		}
		for _, v := range page.Vpcs {
			id := aws.ToString(v.VpcId)
			total, err := c.calculateVPCTotalNAU(ctx, id)
			if err != nil {
				return nil, err
			}
			out[id] = total
		}
	}
	return out, nil
}

// calculateENINau calculates the NAU units for Elastic Network Interfaces in a VPC.
// This includes ENIs, Lambda functions, EFA interfaces, EKS pods, and IP addresses.
func (c *calculator) calculateENINau(ctx context.Context, vpcID string) (int64, error) {
	c.logger.Debug("calculating ENI NAU for vpc %s", vpcID)
	p := ec2.NewDescribeNetworkInterfacesPaginator(c.ec2, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	}, func(o *ec2.DescribeNetworkInterfacesPaginatorOptions) {
		o.Limit = paginationLimit
	})
	var sum int64
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, eni := range page.NetworkInterfaces {
			switch eni.InterfaceType {
			case ec2Types.NetworkInterfaceTypeLambda:
				sum += int64(c.wt.Get(LambdaFunction))
				c.logger.Debug("vpcId [%s] found lambda function %s, total eni naus %d", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
				continue
			case ec2Types.NetworkInterfaceTypeEfa, ec2Types.NetworkInterfaceTypeEfaOnly:
				sum += int64(c.wt.Get(EFAInterface))
				c.logger.Debug("vpcId [%s] found EFA interface %s, total eni naus %d", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
			case ec2Types.NetworkInterfaceTypeBranch:
				sum += int64(c.wt.Get(EKSPod))
				c.logger.Debug("vpcId [%s] found EKS pod %s, total eni naus %d", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
			default:
				sum += int64(c.wt.Get(ENI))
				c.logger.Debug("vpcId [%s] found eni %s, total eni naus %d", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
			}

			// IPv4/IPv6 addresses
			for _, ip := range eni.PrivateIpAddresses {
				sum += int64(c.wt.Get(IPv4IPv6Address))
				c.logger.Debug("vpcId [%s] found private ipv4 %s, total eni naus %d", vpcID, aws.ToString(ip.PrivateIpAddress), sum)
				if ip.Association != nil && ip.Association.PublicIp != nil {
					sum += int64(c.wt.Get(IPv4IPv6Address))
					c.logger.Debug("vpcId [%s] found public ipv4 %s, total eni naus %d", vpcID, aws.ToString(ip.Association.PublicIp), sum)
				}
			}
			sum += int64(len(eni.Ipv6Addresses)) * int64(c.wt.Get(IPv4IPv6Address))
			c.logger.Debug("vpcId [%s] found %d ipv6 addresses, total eni naus %d", vpcID, len(eni.Ipv6Addresses), sum)
			sum += int64(len(eni.Ipv6Prefixes)+len(eni.Ipv4Prefixes)) * int64(c.wt.Get(PrefixAssignedToENI))
			c.logger.Debug("vpcId [%s] found %d ipv6 prefixes, total eni naus %d", vpcID, len(eni.Ipv6Prefixes)+len(eni.Ipv4Prefixes), sum)
		}
	}
	return sum, nil
}

// calculateNATGatewayNau calculates the NAU units for NAT Gateways in a VPC.
func (c *calculator) calculateNATGatewayNau(ctx context.Context, vpcID string) (int64, error) {
	out, err := c.ec2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
		Filter: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	})
	if err != nil {
		return 0, err
	}
	// NAT gateways: one per subnet
	units := int64(c.wt.Get(NATGateway)) * int64(len(out.NatGateways))
	c.logger.Debug("vpcId [%s] found %d nat gateways nau %d ", vpcID, len(out.NatGateways), units)
	return units, nil
}

// calculateVPCEndpointsNau calculates the NAU units for VPC Endpoints in a VPC.
// This accounts for interface endpoints and gateway endpoints across AZs.
func (c *calculator) calculateVPCEndpointsNau(ctx context.Context, vpcID string) (int64, error) {
	out, err := c.ec2.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	})
	if err != nil {
		return 0, err
	}
	var sum int64
	for _, ep := range out.VpcEndpoints {
		var azCount int64
		// interface endpoints: one subnet ID for AZ
		if len(ep.SubnetIds) > 0 {
			azCount = int64(len(ep.SubnetIds))
			c.logger.Debug("vpcId [%s] found %v vpc endpoint in %d az's", vpcID, ep.VpcEndpointType, azCount)

			// gateway endpoints: one route table ID per AZ
		} else if len(ep.RouteTableIds) > 0 {
			azCount = int64(len(ep.RouteTableIds))
			c.logger.Debug("vpcId [%s] found %v vpc endpoint %d az's", vpcID, ep.VpcEndpointType, azCount)
			// fallback if neither is set
		} else {
			azCount = 1
		}
		sum += azCount * int64(c.wt.Get(VPCEndpointPerAZ))
		c.logger.Debug("vpcId [%s] vpc endpoint nau %d", vpcID, sum)
	}
	return sum, nil
}

// calculateLoadBalancersNau calculates the NAU units for Load Balancers in a VPC.
// This includes both Network Load Balancers and Gateway Load Balancers across AZs.
func (c *calculator) calculateLoadBalancersNau(ctx context.Context, vpcID string) (int64, error) {
	p := elbv2.NewDescribeLoadBalancersPaginator(c.elb, &elbv2.DescribeLoadBalancersInput{})
	var sum int64
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, lb := range page.LoadBalancers {
			if *lb.VpcId != vpcID {
				c.logger.Debug("vpcId [%s] found lb %s in %s, skipping", vpcID, aws.ToString(lb.LoadBalancerArn), *lb.VpcId)
				continue
			}
			weight := c.wt.Get(NetworkLoadBalancerPerAZ)
			if lb.Type == elbv2Types.LoadBalancerTypeEnumGateway {
				weight = c.wt.Get(GatewayLoadBalancerPerAZ)
				c.logger.Debug("vpcId [%s] found load balancer type %s , %s", vpcID, lb.Type, *lb.LoadBalancerArn)
			}
			sum += int64(len(lb.AvailabilityZones)) * int64(weight)
			c.logger.Debug("vpcId [%s] found load balancer %v, %s, total lb naus %d", vpcID, lb.Type, *lb.LoadBalancerArn, sum)
		}
	}
	return sum, nil
}

// calculateTransitGatewayVpcAttachmentsNau calculates the NAU units for Transit Gateway VPC attachments.
func (c *calculator) calculateTransitGatewayVpcAttachmentsNau(ctx context.Context, vpcID string) (int64, error) {
	p := ec2.NewDescribeTransitGatewayVpcAttachmentsPaginator(c.ec2, &ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	}, func(o *ec2.DescribeTransitGatewayVpcAttachmentsPaginatorOptions) {
		o.Limit = paginationLimit
	})
	var total int64
	weight := int64(c.wt.Get(TransitGatewayAttachment))
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("count TGW-VPC attachments for %s: %w", vpcID, err)
		}
		total += int64(len(page.TransitGatewayVpcAttachments)) * weight
	}
	c.logger.Debug("vpcId [%s] total tgw-vpc attachments naus %d", vpcID, total)
	return total, nil
}

// calculateEFSMountTargetsInVpcNau calculates the NAU units for EFS Mount Targets in a VPC.
// This involves finding all subnets in the VPC and then checking if any EFS mount targets
// are in those subnets.
func (c *calculator) calculateEFSMountTargetsInVpcNau(ctx context.Context, vpcID string) (int64, error) {
	// 1) list subnets
	snOut, err := c.ec2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	})
	if err != nil {
		return 0, fmt.Errorf("listing subnets in %s: %w", vpcID, err)
	}
	subnets := make(map[string]struct{}, len(snOut.Subnets))
	for _, s := range snOut.Subnets {
		subnets[aws.ToString(s.SubnetId)] = struct{}{}
	}
	// 2) paginate filesystems → mount targets
	fsPager := efs.NewDescribeFileSystemsPaginator(c.efs, &efs.DescribeFileSystemsInput{}, func(o *efs.DescribeFileSystemsPaginatorOptions) {
		o.Limit = paginationLimit
	})
	var total int64
	for fsPager.HasMorePages() {
		fsPage, err := fsPager.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("listing filesystems: %w", err)
		}
		for _, fs := range fsPage.FileSystems {
			mtPager := efs.NewDescribeMountTargetsPaginator(c.efs, &efs.DescribeMountTargetsInput{
				FileSystemId: fs.FileSystemId,
			}, func(o *efs.DescribeMountTargetsPaginatorOptions) {
				o.Limit = paginationLimit
			})
			for mtPager.HasMorePages() {
				mtPage, err := mtPager.NextPage(ctx)
				if err != nil {
					return 0, fmt.Errorf("listing mount targets for %s: %w", aws.ToString(fs.FileSystemId), err)
				}
				for _, mt := range mtPage.MountTargets {
					if _, ok := subnets[aws.ToString(mt.SubnetId)]; ok {
						total += int64(c.wt.Get(EFSMountTarget))
						c.logger.Debug("vpcId [%s] found efs mount target %s, total efs naus %v", vpcID, aws.ToString(mt.MountTargetId), total)
					}
				}
			}
		}
	}
	c.logger.Debug("vpcId [%s] total efs mount targets naus %v", vpcID, total)
	return total, nil
}

// GetRegion returns the AWS region this calculator is operating in.
func (c *calculator) GetRegion() string {
	return c.region
}
