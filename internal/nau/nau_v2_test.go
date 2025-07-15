package nau

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/stretchr/testify/assert"
)

func TestToNetworkInterfaceType(t *testing.T) {
	log := &logger.NoopLogger{}

	tests := []struct {
		name     string
		input    ec2Types.NetworkInterfaceType
		expected NetworkInterfaceType
	}{
		{"interface", ec2Types.NetworkInterfaceType("interface"), NetworkInterfaceTypeInterface},
		{"lambda", ec2Types.NetworkInterfaceType("lambda"), NetworkInterfaceTypeLambda},
		{"nat_gateway", ec2Types.NetworkInterfaceType("nat_gateway"), NetworkInterfaceTypeNatGateway},
		{"efs", ec2Types.NetworkInterfaceType("efs"), NetworkInterfaceTypeEfs},
		{"branch", ec2Types.NetworkInterfaceType("branch"), NetworkInterfaceTypeBranch},
		{"vpc_endpoint", ec2Types.NetworkInterfaceType("vpc_endpoint"), NetworkInterfaceTypeVpcEndpoint},
		{"transit_gateway", ec2Types.NetworkInterfaceType("transit_gateway"), NetworkInterfaceTypeTransitGateway},
		{"unknown_type", ec2Types.NetworkInterfaceType("unknown_type"), NetworkInterfaceType("unknown_type")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toNetworkInterfaceType(tt.input, log)
			assert.Equal(t, tt.expected, result, "Interface type conversion should match")
		})
	}
}

func TestCalculateNauForEni_BasicTypes(t *testing.T) {
	wt := NewNauWeightTable()
	region := "us-east-1"

	tests := []struct {
		name         string
		eniType      NetworkInterfaceType
		expectedKey  ResourceKey
		expectError  bool
	}{
		{"EFS mount target", NetworkInterfaceTypeEfs, EFSMountTarget, false},
		{"EKS pod", NetworkInterfaceTypeBranch, EKSPod, false},
		{"Lambda function", NetworkInterfaceTypeLambda, LambdaFunction, false},
		{"NAT gateway", NetworkInterfaceTypeNatGateway, NATGateway, false},
		{"VPC endpoint", NetworkInterfaceTypeVpcEndpoint, VPCEndpointPerAZ, false},
		{"Transit gateway", NetworkInterfaceTypeTransitGateway, TransitGatewayAttachment, false},
		{"EFA interface", NetworkInterfaceTypeEfa, EFAInterface, false},
		{"Network load balancer", NetworkInterfaceTypeNetworkLoadBalancer, NetworkLoadBalancerPerAZ, false},
		{"Gateway load balancer", NetworkInterfaceTypeGatewayLoadBalancer, GatewayLoadBalancerPerAZ, false},
		{"Unsupported type", NetworkInterfaceType("unsupported"), "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eni := ec2Types.NetworkInterface{
				NetworkInterfaceId: aws.String("eni-123"),
				VpcId:              aws.String("vpc-123"),
				InterfaceType:      ec2Types.NetworkInterfaceType(string(tt.eniType)),
			}

			records, err := calculateNauForEni(eni, tt.eniType, wt, region)

			if tt.expectError {
				assert.Error(t, err, "Should return error for unsupported type")
			} else {
				assert.NoError(t, err, "Should not return error")
				assert.Len(t, records, 1, "Should return one record")
				assert.Equal(t, tt.expectedKey, records[0].ResourceKey, "Resource key should match")
				assert.Equal(t, wt.Get(tt.expectedKey), records[0].Weight, "Weight should match")
			}
		})
	}
}

func TestCalculateNauForEni_InterfaceType(t *testing.T) {
	wt := NewNauWeightTable()
	region := "us-east-1"

	tests := []struct {
		name        string
		deviceIndex *int32
		attachment  *ec2Types.NetworkInterfaceAttachment
		expectError bool
		expectedKey ResourceKey
	}{
		{
			name:        "primary interface (device index 0)",
			deviceIndex: aws.Int32(0),
			attachment:  &ec2Types.NetworkInterfaceAttachment{DeviceIndex: aws.Int32(0)},
			expectError: false,
			expectedKey: IPv4IPv6Address,
		},
		{
			name:        "secondary interface",
			deviceIndex: aws.Int32(1),
			attachment:  &ec2Types.NetworkInterfaceAttachment{DeviceIndex: aws.Int32(1)},
			expectError: false,
			expectedKey: ENI,
		},
		{
			name:        "no attachment",
			attachment:  nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eni := ec2Types.NetworkInterface{
				NetworkInterfaceId: aws.String("eni-123"),
				VpcId:              aws.String("vpc-123"),
				InterfaceType:      ec2Types.NetworkInterfaceTypeInterface,
				Attachment:         tt.attachment,
				PrivateIpAddresses: []ec2Types.NetworkInterfacePrivateIpAddress{
					{PrivateIpAddress: aws.String("10.0.1.100")},
				},
			}

			records, err := calculateNauForEni(eni, NetworkInterfaceTypeInterface, wt, region)

			if tt.expectError {
				assert.Error(t, err, "Should return error")
			} else {
				assert.NoError(t, err, "Should not return error")
				assert.NotEmpty(t, records, "Should return records")
				if tt.expectedKey == IPv4IPv6Address {
					// Primary interface returns IP records
					assert.Equal(t, IPv4IPv6Address, records[0].ResourceKey)
				} else {
					// Secondary interface returns ENI records
					found := false
					for _, record := range records {
						if record.ResourceKey == ENI {
							found = true
							break
						}
					}
					assert.True(t, found, "Should contain ENI record")
				}
			}
		})
	}
}

func TestMakeNauRecords_IpAddresses(t *testing.T) {
	wt := NewNauWeightTable()
	region := "us-east-1"

	eni := ec2Types.NetworkInterface{
		NetworkInterfaceId: aws.String("eni-123"),
		VpcId:              aws.String("vpc-123"),
		PrivateIpAddresses: []ec2Types.NetworkInterfacePrivateIpAddress{
			{
				PrivateIpAddress: aws.String("10.0.1.100"),
				Association: &ec2Types.NetworkInterfaceAssociation{
					PublicIp: aws.String("1.2.3.4"),
				},
			},
			{PrivateIpAddress: aws.String("10.0.1.101")},
		},
		Ipv6Addresses: []ec2Types.NetworkInterfaceIpv6Address{
			{Ipv6Address: aws.String("2001:db8::1")},
		},
	}

	records := makeNauRecords(eni, IPv4IPv6Address, wt, region)

	// Should have 4 records: 2 private IPs + 1 public IP + 1 IPv6
	assert.Len(t, records, 4, "Should have 4 IP address records")

	// Check that all records have correct resource key and weight
	for _, record := range records {
		assert.Equal(t, IPv4IPv6Address, record.ResourceKey)
		assert.Equal(t, wt.Get(IPv4IPv6Address), record.Weight)
		assert.Equal(t, "vpc-123", record.VpcID)
	}
}

func TestMakeNauRecords_Prefixes(t *testing.T) {
	wt := NewNauWeightTable()
	region := "us-east-1"

	eni := ec2Types.NetworkInterface{
		NetworkInterfaceId: aws.String("eni-123"),
		VpcId:              aws.String("vpc-123"),
		Ipv4Prefixes: []ec2Types.Ipv4PrefixSpecification{
			{Ipv4Prefix: aws.String("10.0.1.0/24")},
		},
		Ipv6Prefixes: []ec2Types.Ipv6PrefixSpecification{
			{Ipv6Prefix: aws.String("2001:db8::/64")},
		},
	}

	records := makeNauRecords(eni, PrefixAssignedToENI, wt, region)

	// Should have 2 records: 1 IPv4 prefix + 1 IPv6 prefix
	assert.Len(t, records, 2, "Should have 2 prefix records")

	// Check that all records have correct resource key and weight
	for _, record := range records {
		assert.Equal(t, PrefixAssignedToENI, record.ResourceKey)
		assert.Equal(t, wt.Get(PrefixAssignedToENI), record.Weight)
		assert.Equal(t, "vpc-123", record.VpcID)
	}
}

func TestFetchIpFunctions(t *testing.T) {
	t.Run("fetchIpv4Ips", func(t *testing.T) {
		ips := []ec2Types.NetworkInterfacePrivateIpAddress{
			{
				PrivateIpAddress: aws.String("10.0.1.100"),
				Association: &ec2Types.NetworkInterfaceAssociation{
					PublicIp: aws.String("1.2.3.4"),
				},
			},
			{PrivateIpAddress: aws.String("10.0.1.101")},
		}

		result := fetchIpv4Ips(ips)
		expected := []string{"10.0.1.100", "1.2.3.4", "10.0.1.101"}
		assert.Equal(t, expected, result, "Should extract all IPv4 addresses")
	})

	t.Run("fetchIpv6Ips", func(t *testing.T) {
		ips := []ec2Types.NetworkInterfaceIpv6Address{
			{Ipv6Address: aws.String("2001:db8::1")},
			{Ipv6Address: aws.String("2001:db8::2")},
		}

		result := fetchIpv6Ips(ips)
		expected := []string{"2001:db8::1", "2001:db8::2"}
		assert.Equal(t, expected, result, "Should extract all IPv6 addresses")
	})

	t.Run("fetchIpv4Prefixes", func(t *testing.T) {
		prefixes := []ec2Types.Ipv4PrefixSpecification{
			{Ipv4Prefix: aws.String("10.0.1.0/24")},
			{Ipv4Prefix: aws.String("10.0.2.0/24")},
		}

		result := fetchIpv4Prefixes(prefixes)
		expected := []string{"10.0.1.0/24", "10.0.2.0/24"}
		assert.Equal(t, expected, result, "Should extract all IPv4 prefixes")
	})

	t.Run("fetchIpv6Prefixes", func(t *testing.T) {
		prefixes := []ec2Types.Ipv6PrefixSpecification{
			{Ipv6Prefix: aws.String("2001:db8::/64")},
		}

		result := fetchIpv6Prefixes(prefixes)
		expected := []string{"2001:db8::/64"}
		assert.Equal(t, expected, result, "Should extract all IPv6 prefixes")
	})
}

func TestCopyMetadata(t *testing.T) {
	original := ResourceMetadata{
		NauResourceType:  ENI,
		NauWeight:        1,
		Id:               "eni-123",
		VpcId:            "vpc-123",
		Region:           "us-east-1",
		Ipv4Address:      "10.0.1.100",
	}

	copy := copyMetadata(original)

	// Verify all fields are copied
	assert.Equal(t, original, copy, "Copy should match original")

	// Verify it's a deep copy by modifying the copy
	copy.Ipv4Address = "10.0.1.200"
	assert.NotEqual(t, original.Ipv4Address, copy.Ipv4Address, "Should be independent copies")
}

func TestCreateResourceMetadataForEni(t *testing.T) {
	eni := ec2Types.NetworkInterface{
		NetworkInterfaceId: aws.String("eni-123456"),
		InterfaceType:      ec2Types.NetworkInterfaceTypeInterface,
		VpcId:              aws.String("vpc-123456"),
		AvailabilityZone:   aws.String("us-east-1a"),
		SubnetId:           aws.String("subnet-123456"),
		Description:        aws.String("Test ENI"),
	}

	meta := createResourceMetadataForEni(eni, ENI, 1, "us-east-1")

	assert.Equal(t, ENI, meta.NauResourceType)
	assert.Equal(t, int64(1), meta.NauWeight)
	assert.Equal(t, "eni-123456", meta.Id)
	assert.Equal(t, "interface", meta.ResourceType)
	assert.Equal(t, "vpc-123456", meta.VpcId)
	assert.Equal(t, "us-east-1", meta.Region)
	assert.Equal(t, "us-east-1a", meta.AvailabilityZone)
	assert.Equal(t, "subnet-123456", meta.SubnetId)
	assert.Equal(t, "Test ENI", meta.Description)
}