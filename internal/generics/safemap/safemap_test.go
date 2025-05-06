package safemap_test

import (
	"testing"

	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
)

func TestStoreAndLoad(t *testing.T) {
	var m safemap.TypedMap[int]

	// Loading a missing key should return the zero value and ok==false
	if v, ok := m.Load("missing"); ok {
		t.Errorf("expected ok=false for missing key, got value=%v", v)
	}

	// Store and then load
	m.Store("foo", 42)
	if v, ok := m.Load("foo"); !ok {
		t.Fatalf("expected key 'foo' to be present")
	} else if v != 42 {
		t.Errorf("expected value 42 for 'foo', got %d", v)
	}
}

func TestDelete(t *testing.T) {
	var m safemap.TypedMap[string]
	m.Store("a", "apple")

	// Ensure it's there
	if _, ok := m.Load("a"); !ok {
		t.Fatal("expected key 'a' to exist before delete")
	}

	// Delete and then ensure it's gone
	m.Delete("a")
	if v, ok := m.Load("a"); ok {
		t.Errorf("expected 'a' to be deleted, but got %q", v)
	}
}

func TestRangeFullAndEarlyExit(t *testing.T) {
	var m safemap.TypedMap[int]
	// Insert three entries
	m.Store("k1", 1)
	m.Store("k2", 2)
	m.Store("k3", 3)

	// Full iteration
	seen := map[string]int{}
	m.Range(func(key string, val int) bool {
		seen[key] = val
		return true // keep going
	})
	if len(seen) != 3 {
		t.Fatalf("expected to see 3 entries, saw %d", len(seen))
	}
	for k, want := range map[string]int{"k1": 1, "k2": 2, "k3": 3} {
		if got, ok := seen[k]; !ok || got != want {
			t.Errorf("for key %q expected %d, got %d", k, want, got)
		}
	}

	// Early exit after first callback
	count := 0
	m.Range(func(key string, val int) bool {
		count++
		return false // stop immediately
	})
	if count != 1 {
		t.Errorf("expected early-exit count 1, got %d", count)
	}
}

func TestRangeEmpty(t *testing.T) {
	var m safemap.TypedMap[bool]

	// On an empty map, Range should never call the function
	calls := 0
	m.Range(func(key string, val bool) bool {
		calls++
		return true
	})
	if calls != 0 {
		t.Errorf("expected 0 calls on empty map, got %d", calls)
	}
}
