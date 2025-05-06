package nau

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	efsTypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2Types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/efsclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/elbv2client"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// helper to build a base calculator
func buildCalc(ec2c *ec2client.FakeEC2Client, efsc *efsclient.FakeEFSClient, elbc *elbv2client.FakeELBV2Client) *calculator {
	return &calculator{
		ec2:    ec2c,
		efs:    efsc,
		elb:    elbc,
		logger: &logger.NoopLogger{},
		wt:     NewWeightTable(),
	}
}

func TestCalculateENINau(t *testing.T) {
	ctx := context.Background()
	// ENI: default weight 1 + 1 private IP + 1 public IP + 1 IPv6 + 2 prefixes = 6
	ec2c := &ec2client.FakeEC2Client{
		DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{{
			NetworkInterfaces: []ec2Types.NetworkInterface{{
				NetworkInterfaceId: aws.String("eni-1"),
				InterfaceType:      ec2Types.NetworkInterfaceTypeInterface,
				PrivateIpAddresses: []ec2Types.NetworkInterfacePrivateIpAddress{{
					Association: &ec2Types.NetworkInterfaceAssociation{PublicIp: aws.String("1.2.3.4")},
				}},
				Ipv6Addresses: []ec2Types.NetworkInterfaceIpv6Address{{Ipv6Address: aws.String("::1")}},
				Ipv6Prefixes:  []ec2Types.Ipv6PrefixSpecification{{Ipv6Prefix: aws.String("fd00::/64")}},
				Ipv4Prefixes:  []ec2Types.Ipv4PrefixSpecification{{Ipv4Prefix: aws.String("10.0.0.0/24")}},
			}},
		}},
		ErrOnDescribeENICall: -1,
	}
	efsc := &efsclient.FakeEFSClient{}
	elbc := &elbv2client.FakeELBV2Client{}
	calc := buildCalc(ec2c, efsc, elbc)
	units, err := calc.calculateENINau(ctx, "vpc-1")
	assert.NoError(t, err, "unexpected error")
	assert.Equal(t, int64(6), units, "ENI nau should equal 6")
}

func TestCalculateLambdaENINau(t *testing.T) {
	ctx := context.Background()
	ec2c := &ec2client.FakeEC2Client{
		DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{{
			NetworkInterfaces: []ec2Types.NetworkInterface{{
				InterfaceType: ec2Types.NetworkInterfaceTypeLambda,
				// no IP loops
			}},
		}},
		ErrOnDescribeENICall: -1,
	}
	calc := buildCalc(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	units, _ := calc.calculateENINau(ctx, "vpc-1")
	assert.Equal(t, int64(calc.wt.Get(LambdaFunction)), units, "Lambda NAU should equal 0")
}

func TestCalculateNATGatewayNau(t *testing.T) {
	ctx := context.Background()
	ec2c := &ec2client.FakeEC2Client{
		NatGateways: []ec2Types.NatGateway{{}},
	}
	calc := buildCalc(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	units, _ := calc.calculateNATGatewayNau(ctx, "vpc-1")
	assert.Equal(t, int64(calc.wt.Get(NATGateway)), units, "NAT NAU should equal 1")
}

func TestCalculateVPCEndpointsNau(t *testing.T) {
	ctx := context.Background()
	ec2c := &ec2client.FakeEC2Client{
		VpcEndpoints: []ec2Types.VpcEndpoint{{}},
	}
	calc := buildCalc(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	units, _ := calc.calculateVPCEndpointsNau(ctx, "vpc-1")
	assert.Equal(t, int64(calc.wt.Get(VPCEndpointPerAZ)), units, "VPC Endpoint NAU should equal 1")
}

func TestCalculateLoadBalancersNau(t *testing.T) {
	ctx := context.Background()
	elbc := &elbv2client.FakeELBV2Client{
		DescribeOutputs: []*elbv2.DescribeLoadBalancersOutput{{
			LoadBalancers: []elbv2Types.LoadBalancer{
				{Type: elbv2Types.LoadBalancerTypeEnumNetwork, VpcId: aws.String("vpc-1"), AvailabilityZones: []elbv2Types.AvailabilityZone{{ZoneName: aws.String("us-west-2a")}}},
				{Type: elbv2Types.LoadBalancerTypeEnumGateway, VpcId: aws.String("vpc-1"), AvailabilityZones: []elbv2Types.AvailabilityZone{{ZoneName: aws.String("us-west-2b")}, {ZoneName: aws.String("us-west-2c")}}},
			},
		}},
		ErrorOnCall: -1,
	}
	calc := buildCalc(&ec2client.FakeEC2Client{}, &efsclient.FakeEFSClient{}, elbc)
	units, _ := calc.calculateLoadBalancersNau(ctx, "vpc-1")
	// network: 1 AZ *6 + gateway: 2 AZ*6 = 6 +12 =18
	assert.Equal(t, int64(18), units, "LoadBalancer NAU should equal 18")
}

func TestCalculateTransitGatewayVpcAttachmentsNau_Error(t *testing.T) {
	ctx := context.Background()
	ec2c := &ec2client.FakeEC2Client{ErrOnDescribeTGWVpcAttachCall: 0}
	calc := buildCalc(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	_, err := calc.calculateTransitGatewayVpcAttachmentsNau(ctx, "vpc-1")
	assert.Error(t, err, "expected error counting TGW-VPC attachments")
}

func TestCalculateEFSMountTargetsInVpcNau_Error(t *testing.T) {
	ctx := context.Background()
	ec2c := &ec2client.FakeEC2Client{ErrOnDescribeSubnetCall: 0}
	calc := buildCalc(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	_, err := calc.calculateEFSMountTargetsInVpcNau(ctx, "vpc-1")
	assert.Error(t, err, "expected error counting EFS mount targets")
}

func TestCalculateVPCNAU_Happy(t *testing.T) {
	ctx := context.Background()
	ec2c := &ec2client.FakeEC2Client{
		DescribeVpcsPages:              []*ec2.DescribeVpcsOutput{{Vpcs: []ec2Types.Vpc{{VpcId: aws.String("vpc-1")}}}},
		DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{{NetworkInterfaces: []ec2Types.NetworkInterface{{InterfaceType: ec2Types.NetworkInterfaceTypeInterface}}}},
		NatGateways:                    []ec2Types.NatGateway{{}},
		VpcEndpoints:                   []ec2Types.VpcEndpoint{{}},
		DescribeSubnetsPages:           []*ec2.DescribeSubnetsOutput{{Subnets: []ec2Types.Subnet{{SubnetId: aws.String("subnet-1")}}}},
		ErrOnDescribeVpcsCall:          -1,
		ErrOnDescribeENICall:           -1,
		ErrOnDescribeSubnetCall:        -1,
		ErrOnDescribeTGWVpcAttachCall:  -1,
		ErrNat:                         false,
		ErrVPCEndpoint:                 false,
	}
	selbc := &elbv2client.FakeELBV2Client{
		DescribeOutputs: []*elbv2.DescribeLoadBalancersOutput{
			{
				LoadBalancers: []elbv2Types.LoadBalancer{
					{
						Type:  elbv2Types.LoadBalancerTypeEnumNetwork,
						VpcId: aws.String("vpc-1"),
						AvailabilityZones: []elbv2Types.AvailabilityZone{
							{
								ZoneName: aws.String("a")}}}}}},
		ErrorOnCall: -1,
	}
	efsc := &efsclient.FakeEFSClient{
		ErrOnDescribeFileSystemsCall: -1,
	} // no FS => EFS=0
	calc := NewCalculator(ec2c, efsc, selbc, &logger.NoopLogger{})
	out, err := calc.CalculateVPCNAU(ctx)
	assert.NoError(t, err, "unexpected error calculating VPC NAU")
	assert.Equal(t, int64(19), out["vpc-1"], "ENI NAU should equal 1")
}

func TestCalculateVPCNAU_VPCError(t *testing.T) {
	ctx := context.Background()
	ec2c := &ec2client.FakeEC2Client{ErrOnDescribeVpcsCall: 0}
	calc := NewCalculator(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{}, &logger.NoopLogger{})
	_, err := calc.CalculateVPCNAU(ctx)
	assert.Error(t, err, "expected error calculating VPC NAU")
}

func Test_countEFSMountTargetsInVPC_ErrorCases(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name            string
		subnetPages     []*ec2.DescribeSubnetsOutput
		errOnSubnetCall int
		fsPages         []*efs.DescribeFileSystemsOutput
		errOnFSCall     int
		mtPages         []*efs.DescribeMountTargetsOutput
		errOnMTCall     int
		wantErrContains string
	}{
		{
			name:            "DescribeFileSystems error",
			subnetPages:     []*ec2.DescribeSubnetsOutput{{Subnets: []ec2Types.Subnet{{SubnetId: aws.String("subnet-1")}}}},
			errOnSubnetCall: -1,
			fsPages:         []*efs.DescribeFileSystemsOutput{{FileSystems: []efsTypes.FileSystemDescription{{FileSystemId: aws.String("fs-1")}}}},
			errOnFSCall:     0,
			mtPages:         []*efs.DescribeMountTargetsOutput{{MountTargets: []efsTypes.MountTargetDescription{{SubnetId: aws.String("subnet-1")}}}},
			errOnMTCall:     -1,
			wantErrContains: "listing filesystems: efs describefilesystems error",
		},
		{
			name:            "DescribeMountTargets error",
			subnetPages:     []*ec2.DescribeSubnetsOutput{{Subnets: []ec2Types.Subnet{{SubnetId: aws.String("subnet-1")}}}},
			errOnSubnetCall: -1,
			fsPages:         []*efs.DescribeFileSystemsOutput{{FileSystems: []efsTypes.FileSystemDescription{{FileSystemId: aws.String("fs-1")}}}},
			errOnFSCall:     -1,
			mtPages:         []*efs.DescribeMountTargetsOutput{{MountTargets: []efsTypes.MountTargetDescription{{SubnetId: aws.String("subnet-1")}}}},
			errOnMTCall:     0,
			wantErrContains: "listing mount targets for fs-1: efs describemounttargets error",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// EC2 stub for subnets
			ec2c := &ec2client.FakeEC2Client{
				Region:                  "r1",
				DescribeSubnetsPages:    tc.subnetPages,
				ErrOnDescribeSubnetCall: tc.errOnSubnetCall,
			}
			// EFS stub with paginated FS + MT
			efsc := &efsclient.FakeEFSClient{
				Region:                        "r1",
				DescribeFileSystemsPages:      tc.fsPages,
				ErrOnDescribeFileSystemsCall:  tc.errOnFSCall,
				DescribeMountTargetsPages:     tc.mtPages,
				ErrOnDescribeMountTargetsCall: tc.errOnMTCall,
			}
			// build a calculator that only exercises the EFS‐in‐VPC path
			calc := &calculator{
				ec2:    ec2c,
				efs:    efsc,
				elb:    nil, // unused here
				logger: &logger.NoopLogger{},
				wt:     NewWeightTable(),
			}
			_, err := calc.calculateEFSMountTargetsInVpcNau(ctx, "vpc-1")
			assert.Error(t, err, "expected an error in %q", tc.name)
			assert.Contains(t, err.Error(), tc.wantErrContains,
				"wrong error message for %q: got %q", tc.name, err.Error())
		})
	}
}

func Test_countEFSMountTargetsInVPC_withFakeEFSClient(t *testing.T) {
	ctx := context.Background()
	weight := NewWeightTable().Get(EFSMountTarget)
	tests := []struct {
		name             string
		subnets          []string
		fsPages          []*efs.DescribeFileSystemsOutput
		mtPages          []*efs.DescribeMountTargetsOutput
		errOnFS, errOnMT int
		wantUnits        int64
		wantErr          bool
	}{
		{
			name:    "one FS, one matching MT -> weight*1",
			subnets: []string{"s1"},
			fsPages: []*efs.DescribeFileSystemsOutput{
				{FileSystems: []efsTypes.FileSystemDescription{
					{FileSystemId: aws.String("fs-1")},
				}},
			},
			mtPages: []*efs.DescribeMountTargetsOutput{
				{MountTargets: []efsTypes.MountTargetDescription{
					{FileSystemId: aws.String("fs-1"), SubnetId: aws.String("s1")},
				}},
			},
			errOnFS:   -1,
			errOnMT:   -1,
			wantUnits: int64(weight * 1),
		},
		{
			name:    "one FS, two pages, two matching + one non -> weight*2",
			subnets: []string{"s1", "s2"},
			fsPages: []*efs.DescribeFileSystemsOutput{
				{FileSystems: []efsTypes.FileSystemDescription{
					{FileSystemId: aws.String("fs-1")},
				}},
			},
			mtPages: []*efs.DescribeMountTargetsOutput{
				{MountTargets: []efsTypes.MountTargetDescription{
					{FileSystemId: aws.String("fs-1"), SubnetId: aws.String("s1")},
					{FileSystemId: aws.String("fs-1"), SubnetId: aws.String("x")},
				}},
				{MountTargets: []efsTypes.MountTargetDescription{
					{FileSystemId: aws.String("fs-1"), SubnetId: aws.String("s2")},
				}},
			},
			errOnFS:   -1,
			errOnMT:   -1,
			wantUnits: int64(weight * 2),
		},
		{
			name:    "DescribeFileSystems error",
			subnets: []string{"s1"},
			fsPages: nil,
			mtPages: nil,
			errOnFS: 0, // first FS call errors
			errOnMT: -1,
			wantErr: true,
		},
		{
			name:    "DescribeMountTargets error",
			subnets: []string{"s1"},
			fsPages: []*efs.DescribeFileSystemsOutput{
				{FileSystems: []efsTypes.FileSystemDescription{
					{FileSystemId: aws.String("fs-1")},
				}},
			},
			mtPages: []*efs.DescribeMountTargetsOutput{
				{MountTargets: []efsTypes.MountTargetDescription{{SubnetId: aws.String("s1")}}},
			},
			errOnFS: -1,
			errOnMT: 0, // first MT call errors
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// build EC2 fake with exactly the subnets we care about
			ec2c := &ec2client.FakeEC2Client{
				Region: "r1",
				DescribeSubnetsPages: []*ec2.DescribeSubnetsOutput{{
					Subnets: func() []ec2Types.Subnet {
						out := make([]ec2Types.Subnet, len(tc.subnets))
						for i, id := range tc.subnets {
							out[i] = ec2Types.Subnet{SubnetId: aws.String(id)}
						}
						return out
					}(),
				}},
				ErrOnDescribeSubnetCall:       -1,
				ErrOnDescribeVpcsCall:         -1,
				ErrOnDescribeENICall:          -1,
				ErrOnDescribeTGWVpcAttachCall: -1,
				ErrNat:                        false,
				ErrVPCEndpoint:                false,
			}
			// build EFS fake with paged FS and MT calls
			efsc := &efsclient.FakeEFSClient{
				Region:                        "r1",
				DescribeFileSystemsPages:      tc.fsPages,
				DescribeMountTargetsPages:     tc.mtPages,
				ErrOnDescribeFileSystemsCall:  tc.errOnFS,
				ErrOnDescribeMountTargetsCall: tc.errOnMT,
			}
			calc := NewCalculator(
				ec2c,
				efsc,
				&elbv2client.FakeELBV2Client{Region: "r1"},
				&logger.NoopLogger{},
			).(*calculator)
			units, err := calc.calculateEFSMountTargetsInVpcNau(ctx, "vpc-1")
			if tc.wantErr {
				assert.Error(t, err, "expected error in %q", tc.name)
				return
			}
			assert.NoError(t, err, "unexpected error in %q", tc.name)
			assert.Equalf(t, tc.wantUnits, units,
				"%q: expected %d NAU units, got %d", tc.name, tc.wantUnits, units)
		})
	}
}

func TestCalculateENINau_InterfaceTypeWeights(t *testing.T) {
	ctx := context.Background()
	wt := NewWeightTable()
	cases := []struct {
		name          string
		ifaceType     ec2Types.NetworkInterfaceType
		expectedUnits int64
	}{
		{"Lambda", ec2Types.NetworkInterfaceTypeLambda, int64(wt.Get(LambdaFunction))},
		{"EFA", ec2Types.NetworkInterfaceTypeEfa, int64(wt.Get(EFAInterface))},
		{"EFAOnly", ec2Types.NetworkInterfaceTypeEfaOnly, int64(wt.Get(EFAInterface))},
		{"Branch", ec2Types.NetworkInterfaceTypeBranch, int64(wt.Get(EKSPod))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// EC2 fake with one page containing a single ENI of the given type
			ec2c := &ec2client.FakeEC2Client{
				Region: "r1",
				DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{{
					NetworkInterfaces: []ec2Types.NetworkInterface{{
						InterfaceType: tc.ifaceType,
						// No IPs or prefixes so only the type weight counts
					}},
				}},
				ErrOnDescribeENICall: -1,
			}
			// build calculator
			calc := &calculator{
				ec2:    ec2c,
				efs:    &efsclient.FakeEFSClient{Region: "r1"},
				elb:    &elbv2client.FakeELBV2Client{Region: "r1"},
				logger: &logger.NoopLogger{},
				wt:     wt,
			}
			sum, err := calc.calculateENINau(ctx, "vpc-1")
			assert.NoError(t, err, "unexpected error for %s", tc.name)
			assert.Equalf(t, tc.expectedUnits, sum,
				"%s: expected %d units, got %d", tc.name, tc.expectedUnits, sum)
		})
	}
}

func buildCalcForErrors(
	ec2c *ec2client.FakeEC2Client,
	efsc *efsclient.FakeEFSClient,
	elbc *elbv2client.FakeELBV2Client,
) *calculator {
	return &calculator{
		ec2:    ec2c,
		efs:    efsc,
		elb:    elbc,
		logger: &logger.NoopLogger{},
		wt:     NewWeightTable(),
	}
}
func TestCalculateENINau_Error(t *testing.T) {
	ec2c := &ec2client.FakeEC2Client{
		// error on first DescribeNetworkInterfaces call
		ErrOnDescribeENICall: 0,
	}
	calc := buildCalcForErrors(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	_, err := calc.calculateENINau(context.Background(), "vpc-1")
	assert.Error(t, err, "expected error from DescribeNetworkInterfaces")
}
func TestCalculateNATGatewayNau_Error(t *testing.T) {
	ec2c := &ec2client.FakeEC2Client{
		ErrNat: true,
	}
	calc := buildCalcForErrors(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	_, err := calc.calculateNATGatewayNau(context.Background(), "vpc-1")
	assert.Error(t, err, "expected error from DescribeNatGateways")
}
func TestCalculateVPCEndpointsNau_Error(t *testing.T) {
	ec2c := &ec2client.FakeEC2Client{
		ErrVPCEndpoint: true,
	}
	calc := buildCalcForErrors(ec2c, &efsclient.FakeEFSClient{}, &elbv2client.FakeELBV2Client{})
	_, err := calc.calculateVPCEndpointsNau(context.Background(), "vpc-1")
	assert.Error(t, err, "expected error from DescribeVpcEndpoints")
}
func TestCalculateLoadBalancersNau_Error(t *testing.T) {
	elbc := &elbv2client.FakeELBV2Client{
		ErrorOnCall: 0,
	}
	calc := buildCalcForErrors(&ec2client.FakeEC2Client{}, &efsclient.FakeEFSClient{}, elbc)
	_, err := calc.calculateLoadBalancersNau(context.Background(), "vpc-1")
	assert.Error(t, err, "expected error from DescribeLoadBalancers")
}

func TestCalculateEFSMountTargetsInVpcNau_Error_PaginateFS(t *testing.T) {
	ec2c := &ec2client.FakeEC2Client{
		DescribeSubnetsPages:    []*ec2.DescribeSubnetsOutput{{Subnets: []ec2Types.Subnet{{SubnetId: aws.String("s1")}}}},
		ErrOnDescribeSubnetCall: -1,
	}
	efsc := &efsclient.FakeEFSClient{
		// error on first DescribeFileSystems
		ErrOnDescribeFileSystemsCall: 0,
	}
	calc := buildCalcForErrors(ec2c, efsc, &elbv2client.FakeELBV2Client{})
	_, err := calc.calculateEFSMountTargetsInVpcNau(context.Background(), "vpc-1")
	assert.Error(t, err, "expected error from DescribeFileSystems")
}
func TestCalculateEFSMountTargetsInVpcNau_Error_PaginateMT(t *testing.T) {
	ec2c := &ec2client.FakeEC2Client{
		DescribeSubnetsPages:    []*ec2.DescribeSubnetsOutput{{Subnets: []ec2Types.Subnet{{SubnetId: aws.String("s1")}}}},
		ErrOnDescribeSubnetCall: -1,
	}
	efsc := &efsclient.FakeEFSClient{
		DescribeFileSystemsPages:      []*efs.DescribeFileSystemsOutput{{FileSystems: []efsTypes.FileSystemDescription{{FileSystemId: aws.String("fs-1")}}}},
		ErrOnDescribeFileSystemsCall:  -1,
		ErrOnDescribeMountTargetsCall: 0,
	}
	calc := buildCalcForErrors(ec2c, efsc, &elbv2client.FakeELBV2Client{})
	_, err := calc.calculateEFSMountTargetsInVpcNau(context.Background(), "vpc-1")
	assert.Error(t, err, "expected error from DescribeMountTargets")
}

func TestCalculateVPCNAU_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	// a minimal happy‐path setup we can tweak per‐case:
	baseVPCs := []*ec2.DescribeVpcsOutput{
		{Vpcs: []ec2Types.Vpc{{VpcId: aws.String("vpc-1")}}},
	}
	baseENI := []*ec2.DescribeNetworkInterfacesOutput{
		{NetworkInterfaces: []ec2Types.NetworkInterface{}},
	}
	baseNat := []ec2Types.NatGateway{{}}
	baseEP := []ec2Types.VpcEndpoint{{}}
	baseSubnets := []*ec2.DescribeSubnetsOutput{
		{Subnets: []ec2Types.Subnet{{SubnetId: aws.String("subnet-1")}}}, // for EFS
	}
	baseFS := []*efs.DescribeFileSystemsOutput{
		{FileSystems: []efsTypes.FileSystemDescription{{FileSystemId: aws.String("fs-1")}}}, // for EFS
	}
	baseMT := []*efs.DescribeMountTargetsOutput{
		{MountTargets: []efsTypes.MountTargetDescription{{SubnetId: aws.String("subnet-1")}}},
	}
	baseLB := []*elbv2.DescribeLoadBalancersOutput{
		{LoadBalancers: []elbv2Types.LoadBalancer{
			{
				Type:              elbv2Types.LoadBalancerTypeEnumNetwork,
				VpcId:             aws.String("vpc-1"),
				AvailabilityZones: []elbv2Types.AvailabilityZone{{ZoneName: aws.String("a")}},
			},
		}},
	}
	type tc struct {
		name    string
		ec2Opts func(*ec2client.FakeEC2Client)
		efsOpts func(*efsclient.FakeEFSClient)
		elbOpts func(*elbv2client.FakeELBV2Client)
	}
	tests := []tc{
		{
			name: "error listing VPCs",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.ErrOnDescribeVpcsCall = 0
			},
		},
		{
			name: "error listing ENIs",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.DescribeVpcsPages = baseVPCs
				f.ErrOnDescribeENICall = 0
			},
		},
		{
			name: "error listing NAT gateways",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.ErrOnDescribeENICall = -1
				f.DescribeVpcsPages = baseVPCs
				f.DescribeNetworkInterfacesPages = baseENI
				f.ErrNat = true
			},
		},
		{
			name: "error listing VPC endpoints",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.DescribeVpcsPages = baseVPCs
				f.DescribeNetworkInterfacesPages = baseENI
				f.NatGateways = baseNat
				f.ErrVPCEndpoint = true
				f.ErrNat = false
			},
		},
		{
			name: "error listing load balancers",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.DescribeVpcsPages = baseVPCs
				f.DescribeNetworkInterfacesPages = baseENI
				f.NatGateways = baseNat
				f.VpcEndpoints = baseEP
			},
			elbOpts: func(f *elbv2client.FakeELBV2Client) {
				f.DescribeOutputs = baseLB
				f.ErrorOnCall = 0
			},
		},
		{
			name: "error listing TGW-VPC attachments",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.DescribeVpcsPages = baseVPCs
				f.DescribeNetworkInterfacesPages = baseENI
				f.NatGateways = baseNat
				f.VpcEndpoints = baseEP
				f.ErrOnDescribeTGWVpcAttachCall = 0
			},
		},
		{
			name: "error listing file systems (EFS‐in‐VPC)",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.DescribeVpcsPages = baseVPCs
				f.DescribeNetworkInterfacesPages = baseENI
				f.NatGateways = baseNat
				f.VpcEndpoints = baseEP
				f.DescribeSubnetsPages = baseSubnets
				// ensure TGW works so we get to EFS
				f.ErrOnDescribeTGWVpcAttachCall = -1
			},
			efsOpts: func(f *efsclient.FakeEFSClient) {
				f.DescribeFileSystemsPages = baseFS
				f.ErrOnDescribeFileSystemsCall = 0
			},
		},
		{
			name: "error listing mount targets (EFS‐in‐VPC)",
			ec2Opts: func(f *ec2client.FakeEC2Client) {
				f.DescribeVpcsPages = baseVPCs
				f.DescribeNetworkInterfacesPages = baseENI
				f.NatGateways = baseNat
				f.VpcEndpoints = baseEP
				f.DescribeSubnetsPages = baseSubnets
				f.ErrOnDescribeTGWVpcAttachCall = -1
			},
			efsOpts: func(f *efsclient.FakeEFSClient) {
				f.DescribeFileSystemsPages = baseFS
				f.ErrOnDescribeFileSystemsCall = -1
				f.DescribeMountTargetsPages = baseMT
				f.ErrOnDescribeMountTargetsCall = 0
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// build fakes with default happy pages
			ec2c := &ec2client.FakeEC2Client{
				Region:                         "r1",
				DescribeNetworkInterfacesPages: baseENI,
				NatGateways:                    baseNat,
				VpcEndpoints:                   baseEP,
				DescribeSubnetsPages:           baseSubnets,
				DescribeTransitGatewayVpcAttachmentsPages: []*ec2.DescribeTransitGatewayVpcAttachmentsOutput{
					// at least one page so that pagination is exercised
					{TransitGatewayVpcAttachments: []ec2Types.TransitGatewayVpcAttachment{{}}},
				},
				ErrOnDescribeVpcsCall: -1,
			}
			// always provide at least one VPC
			ec2c.DescribeVpcsPages = baseVPCs
			// ELB happy by default
			elbc := &elbv2client.FakeELBV2Client{
				Region:          "r1",
				DescribeOutputs: baseLB,
				ErrorOnCall:     -1,
			}
			// EFS happy by default
			efsc := &efsclient.FakeEFSClient{
				Region:                        "r1",
				DescribeFileSystemsPages:      baseFS,
				DescribeMountTargetsPages:     baseMT,
				ErrOnDescribeFileSystemsCall:  -1,
				ErrOnDescribeMountTargetsCall: -1,
			}
			// apply per‐case overrides
			if tc.ec2Opts != nil {
				tc.ec2Opts(ec2c)
			}
			if tc.elbOpts != nil {
				tc.elbOpts(elbc)
			}
			if tc.efsOpts != nil {
				tc.efsOpts(efsc)
			}
			calc := NewCalculator(ec2c, efsc, elbc, &logger.NoopLogger{})
			_, err := calc.CalculateVPCNAU(ctx)
			assert.Error(t, err, "%s: expected an error", tc.name)
		})
	}
}
