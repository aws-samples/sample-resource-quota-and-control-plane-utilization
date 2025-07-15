package nau

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Mock manifest for testing
type mockManifest struct {
	headers []string
	records []CSVRecord
	closed  bool
}

func (m *mockManifest) WriteHeader(columns []string) error {
	m.headers = append([]string(nil), columns...)
	return nil
}

func (m *mockManifest) WriteRecord(rec CSVRecord) error {
	m.records = append(m.records, rec)
	return nil
}

func (m *mockManifest) Finalize() error {
	m.closed = true
	return nil
}

func TestAccountNauStore_AddRecord(t *testing.T) {
	mockM := &mockManifest{}
	store := NewAccountNauStore(mockM)

	record := NauRecord{
		VpcID:       "vpc-123",
		ResourceKey: ENI,
		Weight:      1,
		Metadata: ResourceMetadata{
			Id:     "eni-123",
			VpcId:  "vpc-123",
			Region: "us-east-1",
		},
	}

	err := store.AddRecord(record)
	assert.NoError(t, err, "AddRecord should succeed")
	assert.Len(t, mockM.records, 1, "Should write one record to manifest")
}

func TestAccountNauStore_RangeVPCs(t *testing.T) {
	mockM := &mockManifest{}
	store := NewAccountNauStore(mockM)

	// Add records for two VPCs
	records := []NauRecord{
		{VpcID: "vpc-123", ResourceKey: ENI, Weight: 1},
		{VpcID: "vpc-123", ResourceKey: IPv4IPv6Address, Weight: 1},
		{VpcID: "vpc-456", ResourceKey: NATGateway, Weight: 6},
	}

	for _, rec := range records {
		store.AddRecord(rec)
	}

	// Collect VPC data
	vpcs := make(map[string]int64)
	store.RangeVPCs(func(vpcID string, vpc *VPCNAU) bool {
		var total int64
		vpc.Range(func(_ ResourceKey, stats *ResourceStats) bool {
			total += stats.Weight.Load()
			return true
		})
		vpcs[vpcID] = total
		return true
	})

	assert.Len(t, vpcs, 2, "Should have two VPCs")
	assert.Equal(t, int64(2), vpcs["vpc-123"], "vpc-123 should have weight 2")
	assert.Equal(t, int64(6), vpcs["vpc-456"], "vpc-456 should have weight 6")
}

func TestAccountNauStore_Close(t *testing.T) {
	mockM := &mockManifest{}
	store := NewAccountNauStore(mockM)

	err := store.Close()
	assert.NoError(t, err, "Close should succeed")
	assert.True(t, mockM.closed, "Manifest should be finalized")
}

func TestVPCNAU_AddAndRange(t *testing.T) {
	vpc := NewVPCNAU("vpc-test")

	// Add different resource types
	vpc.Add(ENI, 1)
	vpc.Add(ENI, 1)
	vpc.Add(NATGateway, 6)

	// Collect statistics
	stats := make(map[ResourceKey]struct{ count, weight int64 })
	vpc.Range(func(key ResourceKey, s *ResourceStats) bool {
		stats[key] = struct{ count, weight int64 }{
			count:  s.Count.Load(),
			weight: s.Weight.Load(),
		}
		return true
	})

	assert.Len(t, stats, 2, "Should have two resource types")
	assert.Equal(t, int64(2), stats[ENI].count, "ENI count should be 2")
	assert.Equal(t, int64(2), stats[ENI].weight, "ENI weight should be 2")
	assert.Equal(t, int64(1), stats[NATGateway].count, "NAT Gateway count should be 1")
	assert.Equal(t, int64(6), stats[NATGateway].weight, "NAT Gateway weight should be 6")
}

func TestResourceStats_AtomicOperations(t *testing.T) {
	stats := &ResourceStats{}

	// Test concurrent safety (basic test)
	stats.Count.Add(5)
	stats.Weight.Add(10)

	assert.Equal(t, int64(5), stats.Count.Load(), "Count should be 5")
	assert.Equal(t, int64(10), stats.Weight.Load(), "Weight should be 10")
}