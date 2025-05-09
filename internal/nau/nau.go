// nau/calculator.go
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

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/efsclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/elbv2client"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// NAUCalculator is the public interface.
type NAUCalculator interface {
	// CalculateVPCNAU returns the total NAU units for every VPC in the region.
	CalculateVPCNAU(ctx context.Context) (map[string]int64, error)
	// Get Region
	GetRegion() string
}

// ResourceKey distinguishes NAU resource types
type ResourceKey string

const (
	IPv4IPv6Address          ResourceKey = "ipv4-ipv6-address"
	ENI                      ResourceKey = "eni"
	PrefixAssignedToENI      ResourceKey = "prefix-assigned-to-eni"
	NetworkLoadBalancerPerAZ ResourceKey = "network-load-balancer-per-az"
	GatewayLoadBalancerPerAZ ResourceKey = "gateway-load-balancer-per-az"
	VPCEndpointPerAZ         ResourceKey = "vpc-endpoint-per-az"
	TransitGatewayAttachment ResourceKey = "transit-gateway-attachment"
	LambdaFunction           ResourceKey = "lambda-function"
	NATGateway               ResourceKey = "nat-gateway"
	EFSMountTarget           ResourceKey = "efs-mount-target"
	EFAInterface             ResourceKey = "efa-interface"
	EKSPod                   ResourceKey = "eks-pod"
)

// WeightTable maps ResourceKey→weight
type WeightTable struct{ table map[ResourceKey]int }

// NewWeightTable returns the AWS-documented weights.
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

// Get returns the weight for key (zero if missing)
func (w *WeightTable) Get(key ResourceKey) int { return w.table[key] }

// calculator does the work under the hood.
type calculator struct {
	ec2    ec2client.Ec2Client
	efs    efsclient.EFSClient
	elb    elbv2client.ElbV2Client
	logger logger.Logger
	wt     *WeightTable
	region string
}

// NewCalculator wires up your AWS clients + logger.
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

// CalculateVPCNAU paginates every VPC then sums each resource’s NAU units.
func (c *calculator) CalculateVPCNAU(ctx context.Context) (map[string]int64, error) {
	out := make(map[string]int64)
	c.logger.Info("starting VPC discovery for vpc nau's")
	pv := ec2.NewDescribeVpcsPaginator(c.ec2, &ec2.DescribeVpcsInput{})
	for pv.HasMorePages() {
		page, err := pv.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing VPCs: %w", err)
		}
		for _, v := range page.Vpcs {
			id := aws.ToString(v.VpcId)
			c.logger.Debug("calculating VPC %s nau's", id)

			var total int64
			if v, err := c.calculateENINau(ctx, id); err != nil {
				return nil, err
			} else {
				c.logger.Debug("vpcId=%s ENI NAU=%d", id, v)
				total += v
			}
			if v, err := c.calculateNATGatewayNau(ctx, id); err != nil {
				return nil, err
			} else {
				c.logger.Debug("vpcId=%s  NAT NAU=%d", id, v)
				total += v
			}
			if v, err := c.calculateVPCEndpointsNau(ctx, id); err != nil {
				return nil, err
			} else {
				c.logger.Debug("vpcId=%s  VPC Endpoint NAU=%d", id, v)
				total += v
			}
			if v, err := c.calculateLoadBalancersNau(ctx, id); err != nil {
				return nil, err
			} else {
				c.logger.Debug("vpcId=%s  LB NAU=%d", id, v)
				total += v
			}
			if v, err := c.calculateTransitGatewayVpcAttachmentsNau(ctx, id); err != nil {
				return nil, err
			} else {
				c.logger.Debug("vpcId=%s TGW-VPC Attach NAU=%d", id, v)
				total += v
			}
			if v, err := c.calculateEFSMountTargetsInVpcNau(ctx, id); err != nil {
				return nil, err
			} else {
				c.logger.Debug("vpcId=%s  EFS-in-VPC NAU=%d", id, v)
				total += v
			}

			c.logger.Info("vpdId %s total NAU=%d", id, total)
			out[id] = total
		}
	}
	return out, nil
}

//—— private helpers, each returning weighted NAU ——//

func (c *calculator) calculateENINau(ctx context.Context, vpcID string) (int64, error) {
	c.logger.Debug("calculating ENI NAU for vpc %s", vpcID)
	p := ec2.NewDescribeNetworkInterfacesPaginator(c.ec2, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
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
				c.logger.Debug("vpcId [%s] found lambda function %s, total eni naus %s", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
				continue
			case ec2Types.NetworkInterfaceTypeEfa, ec2Types.NetworkInterfaceTypeEfaOnly:
				sum += int64(c.wt.Get(EFAInterface))
				c.logger.Debug("vpcId [%s] found EFA interface %s, total eni naus %s", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
			case ec2Types.NetworkInterfaceTypeBranch:
				sum += int64(c.wt.Get(EKSPod))
				c.logger.Debug("vpcId [%s] found EKS pod %s, total eni naus %s", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
			default:
				sum += int64(c.wt.Get(ENI))
				c.logger.Debug("vpcId [%s] found eni %s, total eni naus %s", vpcID, aws.ToString(eni.NetworkInterfaceId), sum)
			}

			// IPv4/IPv6 addresses
			for _, ip := range eni.PrivateIpAddresses {
				sum += int64(c.wt.Get(IPv4IPv6Address))
				c.logger.Debug("vpcId [%s] found private ipv4 %s, total eni naus %s", vpcID, aws.ToString(ip.PrivateIpAddress), sum)
				if ip.Association != nil && ip.Association.PublicIp != nil {
					sum += int64(c.wt.Get(IPv4IPv6Address))
					c.logger.Debug("vpcId [%s] found public ipv4 %s, total eni naus %s", vpcID, aws.ToString(ip.Association.PublicIp), sum)
				}
			}
			sum += int64(len(eni.Ipv6Addresses)) * int64(c.wt.Get(IPv4IPv6Address))
			c.logger.Debug("vpcId [%s] found %d ipv6 addresses, total eni naus %s", vpcID, len(eni.Ipv6Addresses), sum)
			sum += int64(len(eni.Ipv6Prefixes)+len(eni.Ipv4Prefixes)) * int64(c.wt.Get(PrefixAssignedToENI))
			c.logger.Debug("vpcId [%s] found %d ipv6 prefixes, total eni naus %s", vpcID, len(eni.Ipv6Prefixes)+len(eni.Ipv4Prefixes), sum)
		}
	}
	return sum, nil
}

func (c *calculator) calculateNATGatewayNau(ctx context.Context, vpcID string) (int64, error) {
	out, err := c.ec2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{
		Filter: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	})
	if err != nil {
		return 0, err
	}
	// NAT gateways: one per subnet
	units := int64(c.wt.Get(NATGateway)) * int64(len(out.NatGateways))
	c.logger.Debug("vpcId [%s] found %d nat gateways nau %v ", vpcID, len(out.NatGateways), units)
	return units, nil
}

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
			c.logger.Debug("vpcId [%s] found %d vpc endpoint in %v az's", vpcID, ep.VpcEndpointType, azCount)

			// gateway endpoints: one route table ID per AZ
		} else if len(ep.RouteTableIds) > 0 {
			azCount = int64(len(ep.RouteTableIds))
			c.logger.Debug("vpcId [%s] found %d vpc endpoint %v az's", vpcID, ep.VpcEndpointType, azCount)
			// fallback if neither is set
		} else {
			azCount = 1
		}
		sum += azCount * int64(c.wt.Get(VPCEndpointPerAZ))
		c.logger.Debug("vpcId [%s] vpc endpoint nau %v", vpcID, sum)
	}
	return sum, nil
}

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
			c.logger.Debug("vpcId [%s] found load balancer %v, %v, total lb naus %s", vpcID, lb.Type, *lb.LoadBalancerArn, sum)
		}
	}
	return sum, nil
}

func (c *calculator) calculateTransitGatewayVpcAttachmentsNau(ctx context.Context, vpcID string) (int64, error) {
	p := ec2.NewDescribeTransitGatewayVpcAttachmentsPaginator(c.ec2, &ec2.DescribeTransitGatewayVpcAttachmentsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
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
	c.logger.Debug("vpcId [%s] total tgw-vpc attachments naus %s", vpcID, total)
	return total, nil
}

func (c *calculator) calculateEFSMountTargetsInVpcNau(ctx context.Context, vpcID string) (int64, error) {
	// 1) list subnets
	snOut, err := c.ec2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	})
	if err != nil {
		return 0, fmt.Errorf("listing subnets in %q: %w", vpcID, err)
	}
	subnets := make(map[string]struct{}, len(snOut.Subnets))
	for _, s := range snOut.Subnets {
		subnets[aws.ToString(s.SubnetId)] = struct{}{}
	}
	// 2) paginate filesystems → mount targets
	fsPager := efs.NewDescribeFileSystemsPaginator(c.efs, &efs.DescribeFileSystemsInput{})
	var total int64
	for fsPager.HasMorePages() {
		fsPage, err := fsPager.NextPage(ctx)
		if err != nil {
			return 0, fmt.Errorf("listing filesystems: %w", err)
		}
		for _, fs := range fsPage.FileSystems {
			mtPager := efs.NewDescribeMountTargetsPaginator(c.efs, &efs.DescribeMountTargetsInput{
				FileSystemId: fs.FileSystemId,
			})
			for mtPager.HasMorePages() {
				mtPage, err := mtPager.NextPage(ctx)
				if err != nil {
					return 0, fmt.Errorf("listing mount targets for %s: %w", aws.ToString(fs.FileSystemId), err)
				}
				for _, mt := range mtPage.MountTargets {
					if _, ok := subnets[aws.ToString(mt.SubnetId)]; ok {
						total += int64(c.wt.Get(EFSMountTarget))
						c.logger.Debug("vpcId [%s] found efs mount target %s, total efs naus %s", vpcID, aws.ToString(mt.MountTargetId), total)
					}
				}
			}
		}
	}
	c.logger.Debug("vpcId [%s] total efs mount targets naus %s", vpcID, total)
	return total, nil
}

func (c *calculator) GetRegion() string {
	return c.region
}
