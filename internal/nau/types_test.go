package nau

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceMetadata_Header(t *testing.T) {
	rm := &ResourceMetadata{}
	headers := rm.Header()

	expected := []string{
		"NauResourceType",
		"NauWeight",
		"Id",
		"ResourceType",
		"VpcId",
		"Region",
		"AvailabilityZone",
		"SubnetId",
		"Ipv4Address",
		"Ipv6Address",
		"Ipv4Prefix",
		"Ipv6Prefix",
		"Description",
	}

	assert.Equal(t, expected, headers, "Header should match expected columns")
	assert.Len(t, headers, 13, "Should have 13 columns")
}

func TestResourceMetadata_Values(t *testing.T) {
	rm := &ResourceMetadata{
		NauResourceType:  ENI,
		NauWeight:        1,
		Id:               "eni-123456",
		ResourceType:     "interface",
		VpcId:            "vpc-123456",
		Region:           "us-east-1",
		AvailabilityZone: "us-east-1a",
		SubnetId:         "subnet-123456",
		Ipv4Address:      "10.0.1.100",
		Ipv6Address:      "2001:db8::1",
		Ipv4Prefix:       "10.0.1.0/24",
		Ipv6Prefix:       "2001:db8::/64",
		Description:      "Test ENI",
	}

	values := rm.Values()
	expected := []string{
		"eni",
		"1",
		"eni-123456",
		"interface",
		"vpc-123456",
		"us-east-1",
		"us-east-1a",
		"subnet-123456",
		"10.0.1.100",
		"2001:db8::1",
		"10.0.1.0/24",
		"2001:db8::/64",
		"Test ENI",
	}

	assert.Equal(t, expected, values, "Values should match expected format")
	assert.Len(t, values, 13, "Should have 13 values")
}

func TestResourceMetadata_EmptyValues(t *testing.T) {
	rm := &ResourceMetadata{
		NauResourceType: IPv4IPv6Address,
		NauWeight:       1,
	}

	values := rm.Values()
	assert.Len(t, values, 13, "Should have 13 values even when empty")
	assert.Equal(t, "ipv4-ipv6-address", values[0], "First value should be resource type")
	assert.Equal(t, "1", values[1], "Second value should be weight")
	
	// Check that empty fields are empty strings
	for i := 2; i < len(values); i++ {
		assert.Equal(t, "", values[i], "Empty fields should be empty strings")
	}
}

func TestNauRecord_Structure(t *testing.T) {
	record := NauRecord{
		VpcID:       "vpc-test",
		ResourceKey: ENI,
		Weight:      1,
		Metadata: ResourceMetadata{
			Id:     "eni-test",
			Region: "us-west-2",
		},
	}

	assert.Equal(t, "vpc-test", record.VpcID, "VPC ID should match")
	assert.Equal(t, ENI, record.ResourceKey, "Resource key should match")
	assert.Equal(t, int64(1), record.Weight, "Weight should match")
	assert.Equal(t, "eni-test", record.Metadata.Id, "Metadata ID should match")
	assert.Equal(t, "us-west-2", record.Metadata.Region, "Metadata region should match")
}

func TestResourceKey_StringConversion(t *testing.T) {
	tests := []struct {
		name string
		key  ResourceKey
		want string
	}{
		{"ENI", ENI, "eni"},
		{"IPv4IPv6Address", IPv4IPv6Address, "ipv4-ipv6-address"},
		{"NATGateway", NATGateway, "nat-gateway"},
		{"EFSMountTarget", EFSMountTarget, "efs-mount-target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(tt.key), "String conversion should match")
		})
	}
}