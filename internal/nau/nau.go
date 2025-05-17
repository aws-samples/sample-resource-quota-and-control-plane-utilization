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
type WeightTable struct{ table map[ResourceKey]int64 }

// NewWeightTable returns the AWS-documented weights for NAU resources.
func NewWeightTable() *WeightTable {
	return &WeightTable{table: map[ResourceKey]int64{
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
func (w *WeightTable) Get(key ResourceKey) int64 { return w.table[key] }

// calculator implements the NAUCalculator interface to calculate NAU values.
type calculator struct {
	ec2       ec2client.Ec2Client
	efs       efsclient.EFSClient
	elb       elbv2client.ElbV2Client
	logger    logger.Logger
	wt        *WeightTable
	region    string
	nauValues NAUStore
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
	results := make(chan nauResult, 7) // Buffer for all calculation results

	// Launch Lambda calculation
	g.Go(func() error {
		v, err := c.calculateLambdaaNau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "Lambda"}
		return err
	})

	// Laumch efa calculation
	g.Go(func() error {
		v, err := c.calculateEfaNau(ctx, vpcID)
		results <- nauResult{value: v, err: err, name: "EFA"}
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

func (c *calculator) calculateEfaNau(ctx context.Context, vpcID string) (int64, error) {
	var sum int64
	c.logger.Debug("calculating efa naus for vpc %s", vpcID)
	p := ec2.NewDescribeNetworkInterfacesPaginator(c.ec2, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2Types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("interface-type"), Values: []string{string(ec2Types.NetworkInterfaceTypeEfa), string(ec2Types.NetworkInterfaceTypeEfaOnly)}},
		},
	}, func(o *ec2.DescribeNetworkInterfacesPaginatorOptions) {
		o.Limit = paginationLimit
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		sum += c.wt.Get(EFAInterface) * 1
		c.logger.Debug("vpcId [%s] found %d efa interfaces nau %d ", vpcID, len(page.NetworkInterfaces), sum)
		continue

	}
	efaTotal := c.nauValues.Add(vpcID, string(EFAInterface), sum)
	return efaTotal, nil
}

func (c *calculator) calculateLambdaaNau(ctx context.Context, vpcID string) (int64, error) {
	var sum int64
	c.logger.Debug("calculating lambda naus for vpc %s", vpcID)
	p := ec2.NewDescribeNetworkInterfacesPaginator(c.ec2, &ec2.DescribeNetworkInterfacesInput{
		Filters: []ec2Types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("interface-type"), Values: []string{string(ec2Types.NetworkInterfaceTypeLambda)}},
		},
	}, func(o *ec2.DescribeNetworkInterfacesPaginatorOptions) {
		o.Limit = paginationLimit
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		sum += c.wt.Get(EFAInterface) * 1
		c.logger.Debug("vpcId [%s] found %d lambda interfaces nau %d ", vpcID, len(page.NetworkInterfaces), sum)
		continue

	}
	lambdaTotal := c.nauValues.Add(vpcID, string(EFAInterface), sum)
	return lambdaTotal, nil
}

// calculateNATGatewayNau calculates the NAU units for NAT Gateways in a VPC.
func (c *calculator) calculateNATGatewayNau(ctx context.Context, vpcID string) (int64, error) {
	var sum int64
	paginator := ec2.NewDescribeNatGatewaysPaginator(c.ec2, &ec2.DescribeNatGatewaysInput{
		Filter: []ec2Types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	}, func(o *ec2.DescribeNatGatewaysPaginatorOptions) {
		o.Limit = paginationLimit
	})
	// get all nat gateways
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		sum += c.wt.Get(NATGateway) * int64(len(output.NatGateways))
		c.logger.Debug("vpcId [%s] found %d nat gateways nau %d ", vpcID, len(output.NatGateways), sum)
	}
	natgatewayNau := c.nauValues.Add(vpcID, string(NATGateway), sum)
	c.logger.Debug("vpcId [%s] nat gateways nau %d ", vpcID, sum)
	return natgatewayNau, nil
}

// calculateVPCEndpointsNau calculates the NAU units for VPC Endpoints in a VPC.
// This accounts for interface endpoints and gateway endpoints across AZs.
func (c *calculator) calculateVPCEndpointsNau(ctx context.Context, vpcID string) (int64, error) {
	azs, _ := c.ec2.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2Types.Filter{
			{Name: aws.String("state"), Values: []string{"available"}}},
	})
	azCount := int64(len(azs.AvailabilityZones))
	c.logger.Debug("vpcId [%s] found %d azs", vpcID, azCount)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		_, err := c.calculateInterfaceVPCEndpointsNau(ctx, vpcID)
		if err != nil {
			return err
		}
		return err
	})

	g.Go(func() error {
		_, err := c.calculateGatewayVPCEndpointsNau(ctx, vpcID, azCount)
		if err != nil {
			return err
		}
		return err
	})

	if err := g.Wait(); err != nil {
		return 0, err
	}

	total, ok := c.nauValues.Get(vpcID, string(VPCEndpointPerAZ))
	if !ok {
		return 0, nil
	}
	return total, nil
}

func (c *calculator) calculateGatewayVPCEndpointsNau(ctx context.Context, vpcID string, azCount int64) (int64, error) {
	var sum int64
	paginator := ec2.NewDescribeVpcEndpointsPaginator(c.ec2, &ec2.DescribeVpcEndpointsInput{
		Filters: []ec2Types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("vpc-endpoint-type"), Values: []string{"Gateway"}},
		}}, func(o *ec2.DescribeVpcEndpointsPaginatorOptions) {
		o.Limit = paginationLimit
	})
	// get all gateway vpc endpoints
	var totalNau int64
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, ep := range output.VpcEndpoints {
			sum += int64(len(ep.SubnetIds)) * int64(c.wt.Get(VPCEndpointPerAZ))
			totalNau = c.nauValues.Add(vpcID, string(VPCEndpointPerAZ), sum)
			c.logger.Debug("vpcId [%s] found gateway vpc endpoint %s, total vpc endpoint naus %d", vpcID, aws.ToString(ep.VpcEndpointId), totalNau)
		}
	}
	return totalNau, nil
}

func (c *calculator) calculateInterfaceVPCEndpointsNau(ctx context.Context, vpcID string) (int64, error) {
	var sum int64
	paginator := ec2.NewDescribeVpcEndpointsPaginator(c.ec2, &ec2.DescribeVpcEndpointsInput{
		Filters: []ec2Types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("vpc-endpoint-type"), Values: []string{"Interface"}},
		}}, func(o *ec2.DescribeVpcEndpointsPaginatorOptions) {
		o.Limit = paginationLimit
	})
	// get all interface vpc endpoints
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, ep := range output.VpcEndpoints {
			sum += int64(c.wt.Get(VPCEndpointPerAZ)) * int64(len(ep.SubnetIds))
			c.logger.Debug("vpcId [%s] found interface vpc endpoint %s, total vpc endpoint naus %d", vpcID, aws.ToString(ep.VpcEndpointId), sum)
		}
	}
	interfaceNau := c.nauValues.Add(vpcID, string(VPCEndpointPerAZ), sum)
	c.logger.Debug("vpcId [%s] interface vpc endpoint naus %d", vpcID, sum)
	return interfaceNau, nil
}

// calculateLoadBalancersNau calculates the NAU units for Load Balancers in a VPC.
// This includes both Network Load Balancers and Gateway Load Balancers across AZs.
func (c *calculator) calculateLoadBalancersNau(ctx context.Context, vpcID string) (int64, error) {
	p := elbv2.NewDescribeLoadBalancersPaginator(c.elb, &elbv2.DescribeLoadBalancersInput{
		PageSize: aws.Int32(int32(paginationLimit)),
	})
	var sum, totalNetworkNau, totalGatewayNau int64
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return 0, err
		}
		for _, lb := range page.LoadBalancers {
			if *lb.VpcId != vpcID {
				continue
			}
			if lb.Type == elbv2Types.LoadBalancerTypeEnumNetwork {
				weight := c.wt.Get(NetworkLoadBalancerPerAZ)
				totalNetworkNau += weight * int64(len(lb.AvailabilityZones))
				c.logger.Debug("vpcId [%s] found load balancer type %s, %s, total network lb naus %d", vpcID, lb.Type, *lb.LoadBalancerArn, totalNetworkNau)
				continue
			}
			if lb.Type == elbv2Types.LoadBalancerTypeEnumGateway {
				weight := c.wt.Get(GatewayLoadBalancerPerAZ)
				totalGatewayNau += weight * int64(len(lb.AvailabilityZones))
				c.logger.Debug("vpcId [%s] found load balancer type %s, %s, total gateway lb naus %d", vpcID, lb.Type, *lb.LoadBalancerArn, totalGatewayNau)
				continue
			}
		}
	}
	sum = totalGatewayNau + totalNetworkNau
	c.logger.Debug("vpcId [%s] total load balancer naus %d (gateway nau : %s, network nau : %s )", vpcID, sum, totalGatewayNau, totalNetworkNau)
	loadBalancerNau := c.nauValues.Add(vpcID, string(GatewayLoadBalancerPerAZ), sum)
	c.logger.Debug("vpcId [%s] gateway load balancer naus %d", vpcID, loadBalancerNau)
	return loadBalancerNau, nil
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
		c.logger.Debug("vpcId [%s] found %d transit gateway vpc attachments, total transit gateway naus %d", vpcID, len(page.TransitGatewayVpcAttachments), total)
	}
	totalTransitGatewayNau := c.nauValues.Add(vpcID, string(TransitGatewayAttachment), total)
	c.logger.Debug("vpcId [%s] total transit gateway naus %d", vpcID, totalTransitGatewayNau)
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
						c.logger.Debug("vpcId [%s] found efs mount target %s, total efs naus %v", vpcID, aws.ToString(mt.MountTargetId), total)
					}
				}
			}
		}
	}
	efsTotal := c.nauValues.Add(vpcID, string(EFSMountTarget), total)
	c.logger.Debug("vpcId [%s] total efs naus %d", vpcID, efsTotal)
	return total, nil
}

// GetRegion returns the AWS region this calculator is operating in.
func (c *calculator) GetRegion() string {
	return c.region
}
