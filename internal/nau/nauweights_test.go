package nau

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWeightTable_GetKnownAndUnknownKeys(t *testing.T) {
	wt := NewNauWeightTable()

	tests := []struct {
		name string
		key  ResourceKey
		want int64
	}{
		{"IPv4IPv6Address", IPv4IPv6Address, 1},
		{"ENI", ENI, 1},
		{"PrefixAssignedToENI", PrefixAssignedToENI, 1},
		{"NetworkLoadBalancerPerAZ", NetworkLoadBalancerPerAZ, 6},
		{"GatewayLoadBalancerPerAZ", GatewayLoadBalancerPerAZ, 6},
		{"VPCEndpointPerAZ", VPCEndpointPerAZ, 6},
		{"TransitGatewayAttachment", TransitGatewayAttachment, 6},
		{"LambdaFunction", LambdaFunction, 6},
		{"NATGateway", NATGateway, 6},
		{"EFSMountTarget", EFSMountTarget, 6},
		{"EFAInterface", EFAInterface, 1},
		{"EKSPod", EKSPod, 1},
		{"UnknownKey", ResourceKey("does_not_exist"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wt.Get(tt.key)
			assert.Equalf(t, tt.want, got, "WeightTable.Get(%q) = %d; want %d", tt.key, got, tt.want)
		})
	}
}

func TestNewWeightTable_IsIndependentInstance(t *testing.T) {
	wt1 := NewNauWeightTable()
	wt2 := NewNauWeightTable()

	// Mutate wt1's table and verify it doesn't affect wt2
	wt1.table[IPv4IPv6Address] = 42
	assert.Equal(t, int64(42), wt1.Get(IPv4IPv6Address), "wt1 should reflect the mutation")
	assert.Equal(t, int64(1), wt2.Get(IPv4IPv6Address), "wt2 should remain at the original value")
}
