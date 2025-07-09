package nau

import "strconv"

// Header returns the CSV header columns for ResourceMetadata export.
func (rh *ResourceMetadata) Header() []string {
	return []string{
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
}

// ResourceMetadata contains detailed information about a NAU resource
// for tracking and reporting purposes.
type ResourceMetadata struct {
	NauResourceType  ResourceKey // Type of NAU resource
	NauWeight        int64       // NAU weight value for this resource
	Id               string      // AWS resource identifier
	ResourceType     string      // AWS resource type
	VpcId            string      // VPC identifier
	Region           string      // AWS region
	AvailabilityZone string      // Availability zone
	SubnetId         string      // Subnet identifier
	Ipv4Address      string      // IPv4 address if applicable
	Ipv6Address      string      // IPv6 address if applicable
	Ipv4Prefix       string      // IPv4 CIDR prefix if applicable
	Ipv6Prefix       string      // IPv6 CIDR prefix if applicable
	Description      string      // Resource description
}

// Values returns the ResourceMetadata fields as a string slice for CSV export.
func (rm *ResourceMetadata) Values() []string {
	return []string{
		string(rm.NauResourceType),
		strconv.FormatInt(rm.NauWeight, 10),
		rm.Id,
		rm.ResourceType,
		rm.VpcId,
		rm.Region,
		rm.AvailabilityZone,
		rm.SubnetId,
		rm.Ipv4Address,
		rm.Ipv6Address,
		rm.Ipv4Prefix,
		rm.Ipv6Prefix,
		rm.Description,
	}
}

// NauRecord represents a single NAU calculation entry for a VPC resource,
// containing the resource type, weight, and detailed metadata.
type NauRecord struct {
	VpcID       string           // VPC identifier where this resource exists
	ResourceKey ResourceKey      // Type of NAU resource being recorded
	Weight      int64            // NAU weight units for this resource
	Metadata    ResourceMetadata // Detailed resource information
}
