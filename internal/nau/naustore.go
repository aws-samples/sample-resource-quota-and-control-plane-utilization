package nau

import (
	"strings"
	"sync"
	"sync/atomic"
)

// NAUStore defines the operations you need.
type NAUStore interface {
	// Set overwrites the weight for this VPC/resource.
	Set(vpcID, resourceType string, weight int64)
	// Get loads the weight, returning (0,false) if not present.
	Get(vpcID, resourceType string) (int64, bool)
	// Add atomically increments (or decrements) the weight, returning the new value.
	Add(vpcID, resourceType string, delta int64) int64
	// Delete removes the entry entirely.
	Delete(vpcID, resourceType string)
	// Range iterates all entries; if f returns false, iteration stops.
	Range(f func(vpcID, resourceType string, weight int64) bool)
}

// syncNAUStore is our sync.Map–backed implementation.
type syncNAUStore struct {
	m sync.Map // map[string]*int64
}

// NewSyncNAUStore constructs a new NAUStore.
func NewSyncNAUStore() NAUStore {
	return &syncNAUStore{}
}

// compositeKey joins vpcID and resourceType into a single string key.
func compositeKey(vpcID, resourceType string) string {
	return vpcID + "|" + resourceType
}

// splitKey is the inverse of compositeKey.
func splitKey(key string) (vpcID, resourceType string) {
	parts := strings.SplitN(key, "|", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func (s *syncNAUStore) Set(vpcID, resourceType string, weight int64) {
	key := compositeKey(vpcID, resourceType)
	// Load or create a pointer to int64
	raw, _ := s.m.LoadOrStore(key, new(int64))
	ptr := raw.(*int64)
	atomic.StoreInt64(ptr, weight)
}

func (s *syncNAUStore) Get(vpcID, resourceType string) (int64, bool) {
	key := compositeKey(vpcID, resourceType)
	raw, ok := s.m.Load(key)
	if !ok {
		return 0, false
	}
	ptr := raw.(*int64)
	return atomic.LoadInt64(ptr), true
}

func (s *syncNAUStore) Add(vpcID, resourceType string, delta int64) int64 {
	key := compositeKey(vpcID, resourceType)
	raw, _ := s.m.LoadOrStore(key, new(int64))
	ptr := raw.(*int64)
	return atomic.AddInt64(ptr, delta)
}

func (s *syncNAUStore) Delete(vpcID, resourceType string) {
	key := compositeKey(vpcID, resourceType)
	s.m.Delete(key)
}

func (s *syncNAUStore) Range(f func(vpcID, resourceType string, weight int64) bool) {
	s.m.Range(func(k, v interface{}) bool {
		key := k.(string)
		ptr := v.(*int64)
		vpcID, resourceType := splitKey(key)
		weight := atomic.LoadInt64(ptr)
		return f(vpcID, resourceType, weight)
	})
}
