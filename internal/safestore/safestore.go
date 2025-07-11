// Package safestore provides a thread-safe generic key-value store interface
// and implementation using sync.Map for concurrent access patterns.
package safestore

import "sync"

// Store is a thread-safe key→value map interface.
type Store[V any] interface {
	// Load returns the value stored under key, or (zero, false) if none.
	Load(key string) (V, bool)
	// LoadOrStore returns the existing value for key if present.
	// Otherwise it stores and returns newVal, with loaded=false.
	LoadOrStore(key string, newVal V) (actual V, loaded bool)
	// Store unconditionally sets key=newVal.
	Store(key string, newVal V)
	// Delete removes key from the store.
	Delete(key string)
	// Range calls fn for every key/value in the store.
	// If fn returns false, iteration stops.
	Range(fn func(key string, val V) bool)
}

// syncStore is a sync.Map–backed Store implementation that provides
// thread-safe operations for concurrent read/write access.
type syncStore[V any] struct {
	m sync.Map // underlying sync.Map for thread-safe storage
}

// NewSyncStore creates a new thread-safe store backed by sync.Map.
// The store can hold values of any type V specified by the generic parameter.
func NewSyncStore[V any]() Store[V] {
	return &syncStore[V]{}
}

// Load retrieves the value associated with the given key.
// Returns the value and true if found, or zero value and false if not found.
func (s *syncStore[V]) Load(key string) (V, bool) {
	raw, ok := s.m.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	val, ok := raw.(V)
	if !ok {
		var zero V
		return zero, false
	}
	return val, true
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value, with loaded=false.
// This operation is atomic.
func (s *syncStore[V]) LoadOrStore(key string, newVal V) (V, bool) {
	raw, loaded := s.m.LoadOrStore(key, newVal)
	val, ok := raw.(V)
	if !ok {
		var zero V
		return zero, false
	}
	return val, loaded
}

// Store unconditionally sets the value for the given key.
// If the key already exists, its value is overwritten.
func (s *syncStore[V]) Store(key string, newVal V) {
	s.m.Store(key, newVal)
}

// Delete removes the key and its associated value from the store.
// No-op if the key doesn't exist.
func (s *syncStore[V]) Delete(key string) {
	s.m.Delete(key)
}

// Range calls the provided function for each key-value pair in the store.
// Iteration stops early if the function returns false.
// The iteration order is not guaranteed to be consistent.
func (s *syncStore[V]) Range(fn func(key string, val V) bool) {
	s.m.Range(func(rawKey, rawVal any) bool {
		key, keyOk := rawKey.(string)
		val, valOk := rawVal.(V)
		if !keyOk || !valOk {
			return true // skip invalid entries
		}
		return fn(key, val)
	})
}
