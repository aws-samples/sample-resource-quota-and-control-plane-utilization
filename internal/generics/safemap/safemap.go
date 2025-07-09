// Package safemap provides a type-safe wrapper around sync.Map
// with generic type parameters for improved compile-time safety.
package safemap

import (
	"sync"
)

// TypedMap provides a thread-safe, type-safe map implementation using generics.
// It wraps sync.Map while eliminating the need for type assertions in user code.
type TypedMap[T any] struct {
	m sync.Map // Underlying concurrent map
}

// Store saves a typed value under the specified string key.
func (tm *TypedMap[T]) Store(key string, val T) {
	tm.m.Store(key, val)
}

// Load retrieves a typed value for the specified key.
// Returns the value and true if found, or zero value and false if not present.
func (tm *TypedMap[T]) Load(key string) (T, bool) {
	raw, ok := tm.m.Load(key)
	if !ok {
		var zero T
		return zero, false
	}
	// Type assertion is safe due to Store method constraints
	return raw.(T), true
}

// Delete removes the specified key and its value from the map.
func (tm *TypedMap[T]) Delete(key string) {
	tm.m.Delete(key)
}

// Range iterates over all key-value pairs in the map.
// Iteration stops early if the provided function returns false.
func (tm *TypedMap[T]) Range(fn func(key string, val T) bool) {
	tm.m.Range(func(rawKey, rawVal any) bool {
		k := rawKey.(string)
		v := rawVal.(T)
		return fn(k, v)
	})
}
