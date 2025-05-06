package safemap

import (
	"sync"
)

// TypedMap is a thin, type-safe wrapper around sync.Map.
// Internally it still uses sync.Map, but the user only sees methods
// operating on T, not interface{}.
type TypedMap[T any] struct {
	m sync.Map
}

// Store saves a value of type T under the given key.
func (tm *TypedMap[T]) Store(key string, val T) {
	tm.m.Store(key, val)
}

// Load retrieves the value of type T for key.
// The ok return is false if no value was present.
func (tm *TypedMap[T]) Load(key string) (T, bool) {
	raw, ok := tm.m.Load(key)
	if !ok {
		var zero T
		return zero, false
	}
	// type assertion guaranteed by Store
	return raw.(T), true
}

// Delete removes the key from the map.
func (tm *TypedMap[T]) Delete(key string) {
	tm.m.Delete(key)
}

// Range calls the given function for every key/value.
// If fn returns false, iteration stops.
func (tm *TypedMap[T]) Range(fn func(key string, val T) bool) {
	tm.m.Range(func(rawKey, rawVal any) bool {
		k := rawKey.(string)
		v := rawVal.(T)
		return fn(k, v)
	})
}
