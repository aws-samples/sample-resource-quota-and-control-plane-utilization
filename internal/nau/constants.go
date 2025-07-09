// Package nau provides Network Address Usage (NAU) calculation functionality
// for AWS VPC resources, including weight tables, storage, and manifest generation.
package nau

// paginationLimit defines the maximum number of results to return per page for all paginators.
const paginationLimit = 1000

// ResourceKey identifies different types of NAU resources for weight calculation.
type ResourceKey string

// ServiceName represents AWS service names used in NAU calculations.
type ServiceName string

// NetworkInterfaceType represents different types of network interfaces in AWS.
type NetworkInterfaceType string

// NAU resource type constants for different AWS resources that consume network addresses.
const (
	// IPv4IPv6Address represents an IPv4 or IPv6 address resource (weight: 1).
	IPv4IPv6Address ResourceKey = "ipv4-ipv6-address"
	// ENI represents an Elastic Network Interface resource (weight: 1).
	ENI ResourceKey = "eni"
	// PrefixAssignedToENI represents a CIDR prefix assigned to an ENI (weight: 1).
	PrefixAssignedToENI ResourceKey = "prefix-assigned-to-eni"
	// NetworkLoadBalancerPerAZ represents a Network Load Balancer in an AZ (weight: 6).
	NetworkLoadBalancerPerAZ ResourceKey = "network-load-balancer-per-az"
	// GatewayLoadBalancerPerAZ represents a Gateway Load Balancer in an AZ (weight: 6).
	GatewayLoadBalancerPerAZ ResourceKey = "gateway-load-balancer-per-az"
	// VPCEndpointPerAZ represents a VPC Endpoint in an AZ (weight: 6).
	VPCEndpointPerAZ ResourceKey = "vpc-endpoint-per-az"
	// TransitGatewayAttachment represents a Transit Gateway attachment (weight: 6).
	TransitGatewayAttachment ResourceKey = "transit-gateway-attachment"
	// LambdaFunction represents a Lambda function with VPC access (weight: 6).
	LambdaFunction ResourceKey = "lambda-function"
	// NATGateway represents a NAT Gateway (weight: 6).
	NATGateway ResourceKey = "nat-gateway"
	// EFSMountTarget represents an EFS Mount Target (weight: 6).
	EFSMountTarget ResourceKey = "efs-mount-target"
	// EFAInterface represents an Elastic Fabric Adapter interface (weight: 1).
	EFAInterface ResourceKey = "efa-interface"
	// EKSPod represents an EKS Pod using VPC networking (weight: 1).
	EKSPod ResourceKey = "eks-pod"

	// AWS service name constants.
	ElasticComputeCloud ServiceName = "ec2"
	ElasticFileSystem   ServiceName = "efs"

	// Network interface types that extend beyond the standard EC2 API definitions.
	// These types are used for comprehensive NAU calculation across AWS services.
	NetworkInterfaceTypeApiGatewayManaged             NetworkInterfaceType = "api_gateway_managed"
	NetworkInterfaceTypeAwsCodestarConnectionsManaged NetworkInterfaceType = "aws_codestar_connections_managed"
	NetworkInterfaceTypeBranch                        NetworkInterfaceType = "branch"
	NetworkInterfaceTypeEc2InstanceConnect            NetworkInterfaceType = "ec2_instance_connect_endpoint"
	NetworkInterfaceTypeEfa                           NetworkInterfaceType = "efa"
	NetworkInterfaceTypeEfaOnly                       NetworkInterfaceType = "efa-only"
	NetworkInterfaceTypeEfs                           NetworkInterfaceType = "efs"
	NetworkInterfaceTypeEvs                           NetworkInterfaceType = "evs"
	NetworkInterfaceTypeGatewayLoadBalancer           NetworkInterfaceType = "gateway_load_balancer"
	NetworkInterfaceTypeGatewayLoadBalancerEndpoint   NetworkInterfaceType = "gateway_load_balancer_endpoint"
	NetworkInterfaceTypeGlobalAcceleratorManaged      NetworkInterfaceType = "global_accelerator_managed"
	NetworkInterfaceTypeInterface                     NetworkInterfaceType = "interface"
	NetworkInterfaceTypeIotRulesManaged               NetworkInterfaceType = "iot_rules_managed"
	NetworkInterfaceTypeLambda                        NetworkInterfaceType = "lambda"
	NetworkInterfaceTypeLoadBalancer                  NetworkInterfaceType = "load_balancer"
	NetworkInterfaceTypeNatGateway                    NetworkInterfaceType = "nat_gateway"
	NetworkInterfaceTypeNetworkLoadBalancer           NetworkInterfaceType = "network_load_balancer"
	NetworkInterfaceTypeQuicksight                    NetworkInterfaceType = "quicksight"
	NetworkInterfaceTypeTransitGateway                NetworkInterfaceType = "transit_gateway"
	NetworkInterfaceTypeTrunk                         NetworkInterfaceType = "trunk"
	NetworkInterfaceTypeVpcEndpoint                   NetworkInterfaceType = "vpc_endpoint"
)
