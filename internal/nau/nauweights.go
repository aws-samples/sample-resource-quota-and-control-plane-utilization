package nau

// NauWeightTable provides weight values for different NAU resource types.
type NauWeightTable interface {
	// Get returns the NAU weight for the specified resource key.
	Get(key ResourceKey) int64
}

// nauWeightsImpl implements NauWeightTable with AWS-documented NAU weights.
type nauWeightsImpl struct{ table map[ResourceKey]int64 }

// NewNauWeightTable creates a weight table with AWS-documented NAU values.
// Weights range from 1 (for basic resources) to 6 (for complex services).
func NewNauWeightTable() *nauWeightsImpl {
	return &nauWeightsImpl{table: map[ResourceKey]int64{
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

// Get returns the NAU weight for the specified resource key.
// Returns zero if the resource key is not found in the table.
func (wt *nauWeightsImpl) Get(key ResourceKey) int64 { return wt.table[key] }
