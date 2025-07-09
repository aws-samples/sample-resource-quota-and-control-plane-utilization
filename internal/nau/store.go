package nau

import (
	"sync/atomic"

	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
)

// AccountNauStore provides storage and aggregation for NAU records across an AWS account.
type AccountNauStore interface {
	// AddRecord processes a NAU record and updates counters.
	AddRecord(rec NauRecord) error
	// RangeVPCs iterates over all VPC NAU tallies. Return false to stop iteration.
	RangeVPCs(fn func(vpcID string, vpc *VPCNAU) bool)
	// Close finalizes the store and uploads manifest data.
	Close() error
	// writeHeader initializes the CSV header for manifest export.
	writeHeader()
}

// accountNauStore implements AccountNauStore with thread-safe storage and manifest generation.
type accountNauStore struct {
	vpcs safestore.Store[*VPCNAU] // Thread-safe map of VPC ID to NAU tallies
	m    Manifest                  // Manifest for CSV export to S3
}

// NewAccountNauStore creates a new account-level NAU store with manifest support.
func NewAccountNauStore(m Manifest) AccountNauStore {
	store := &accountNauStore{
		vpcs: safestore.NewSyncStore[*VPCNAU](),
		m:    m,
	}

	store.writeHeader()
	return store
}

// AddRecord processes a NAU record by updating VPC counters and writing to manifest.
func (a *accountNauStore) AddRecord(rec NauRecord) error {
	// get or create per-VPC tally
	vpc, _ := a.vpcs.LoadOrStore(rec.VpcID, NewVPCNAU(rec.VpcID))
	// update the resource counter
	vpc.Add(rec.ResourceKey, rec.Weight)
	return a.m.WriteRecord(&rec.Metadata)

}

// RangeVPCs iterates over all stored VPC NAU data.
func (a *accountNauStore) RangeVPCs(fn func(vpcID string, vpc *VPCNAU) bool) {
	a.vpcs.Range(fn)
}

// Close finalizes the store by uploading the manifest to S3.
func (a *accountNauStore) Close() error {
	return a.m.Finalize()
}

// writeHeader initializes the CSV header for manifest export.
func (a *accountNauStore) writeHeader() {
	rm := ResourceMetadata{}
	a.m.WriteHeader(rm.Header())
}

// ResourceStats maintains thread-safe counters for NAU resource statistics.
type ResourceStats struct {
	Count  atomic.Int64 // Number of resources of this type
	Weight atomic.Int64 // Total NAU weight for this resource type
}

// VPCNAU aggregates NAU statistics for all resources within a single VPC.
type VPCNAU struct {
	VpcID   string                          // VPC identifier
	results safestore.Store[*ResourceStats] // Thread-safe map of resource type to statistics
}

// NewVPCNAU creates a new VPC-level NAU tracker.
func NewVPCNAU(vpcID string) *VPCNAU {
	return &VPCNAU{
		VpcID:   vpcID,
		results: safestore.NewSyncStore[*ResourceStats](),
	}
}

// Add increments the resource count and total weight for the specified resource type.
func (v *VPCNAU) Add(key ResourceKey, weight int64) {
	stats, _ := v.results.LoadOrStore(string(key), &ResourceStats{})
	stats.Count.Add(1)
	stats.Weight.Add(weight)
}

// Range iterates over all resource statistics in this VPC. Return false to stop iteration.
func (v *VPCNAU) Range(fn func(key ResourceKey, stats *ResourceStats) bool) {
	v.results.Range(func(k string, stats *ResourceStats) bool {
		return fn(ResourceKey(k), stats)
	})
}
