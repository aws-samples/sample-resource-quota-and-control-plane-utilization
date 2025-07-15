package safestore

import (
	"sync"
	"testing"
)

func TestNewSyncStore(t *testing.T) {
	store := NewSyncStore[string]()
	if store == nil {
		t.Fatal("NewSyncStore returned nil")
	}
}

func TestStore_Load(t *testing.T) {
	store := NewSyncStore[string]()
	
	// Test loading non-existing key
	val, ok := store.Load("nonexistent")
	if ok || val != "" {
		t.Errorf("Load non-existing key: got (%q, %v), want (\"\", false)", val, ok)
	}
	
	// Store and load existing key
	store.Store("key1", "value1")
	val, ok = store.Load("key1")
	if !ok || val != "value1" {
		t.Errorf("Load existing key: got (%q, %v), want (\"value1\", true)", val, ok)
	}
}

func TestStore_Store(t *testing.T) {
	store := NewSyncStore[int]()
	
	store.Store("key1", 42)
	val, ok := store.Load("key1")
	if !ok || val != 42 {
		t.Errorf("Store/Load: got (%d, %v), want (42, true)", val, ok)
	}
	
	// Overwrite existing key
	store.Store("key1", 100)
	val, ok = store.Load("key1")
	if !ok || val != 100 {
		t.Errorf("Store overwrite: got (%d, %v), want (100, true)", val, ok)
	}
}

func TestStore_Delete(t *testing.T) {
	store := NewSyncStore[string]()
	
	// Delete non-existing key (should not panic)
	store.Delete("nonexistent")
	
	// Store, delete, and verify
	store.Store("key1", "value1")
	store.Delete("key1")
	val, ok := store.Load("key1")
	if ok || val != "" {
		t.Errorf("Delete: got (%q, %v), want (\"\", false)", val, ok)
	}
}

func TestStore_LoadOrStore(t *testing.T) {
	store := NewSyncStore[string]()
	
	// Load or store new key
	val, loaded := store.LoadOrStore("key1", "value1")
	if loaded || val != "value1" {
		t.Errorf("LoadOrStore new: got (%q, %v), want (\"value1\", false)", val, loaded)
	}
	
	// Load or store existing key
	val, loaded = store.LoadOrStore("key1", "value2")
	if !loaded || val != "value1" {
		t.Errorf("LoadOrStore existing: got (%q, %v), want (\"value1\", true)", val, loaded)
	}
}

func TestStore_ConcurrentReadWrite(t *testing.T) {
	store := NewSyncStore[int]()
	const numGoroutines = 100
	const numOperations = 1000
	
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)
	
	// Writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				store.Store("key", id*numOperations+j)
			}
		}(i)
	}
	
	// Readers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				store.Load("key")
			}
		}()
	}
	
	wg.Wait()
}

func TestStore_ConcurrentLoadOrStore(t *testing.T) {
	store := NewSyncStore[int]()
	const numGoroutines = 50
	
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	results := make([]bool, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_, loaded := store.LoadOrStore("key", id)
			results[id] = loaded
		}(i)
	}
	
	wg.Wait()
	
	// Exactly one goroutine should have loaded=false (first to store)
	falseCount := 0
	for _, loaded := range results {
		if !loaded {
			falseCount++
		}
	}
	if falseCount != 1 {
		t.Errorf("Expected exactly 1 false result, got %d", falseCount)
	}
}

func TestStore_ConcurrentRange(t *testing.T) {
	store := NewSyncStore[int]()
	
	// Pre-populate store
	for i := 0; i < 100; i++ {
		store.Store(string(rune('a'+i%26)), i)
	}
	
	var wg sync.WaitGroup
	wg.Add(3)
	
	// Range reader
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			store.Range(func(key string, val int) bool {
				return true
			})
		}
	}()
	
	// Writer during range
	go func() {
		defer wg.Done()
		for i := 100; i < 200; i++ {
			store.Store(string(rune('a'+i%26)), i)
		}
	}()
	
	// Deleter during range
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			store.Delete(string(rune('a' + i%26)))
		}
	}()
	
	wg.Wait()
}

func TestStore_GenericTypes(t *testing.T) {
	// Test with struct type
	type testStruct struct {
		Name string
		ID   int
	}
	
	store := NewSyncStore[testStruct]()
	expected := testStruct{Name: "test", ID: 42}
	
	store.Store("key1", expected)
	val, ok := store.Load("key1")
	if !ok || val != expected {
		t.Errorf("Generic struct: got (%+v, %v), want (%+v, true)", val, ok, expected)
	}
	
	// Test with pointer type
	ptrStore := NewSyncStore[*testStruct]()
	expectedPtr := &testStruct{Name: "ptr", ID: 100}
	
	ptrStore.Store("key1", expectedPtr)
	valPtr, ok := ptrStore.Load("key1")
	if !ok || valPtr != expectedPtr {
		t.Errorf("Generic pointer: got (%+v, %v), want (%+v, true)", valPtr, ok, expectedPtr)
	}
}

func TestStore_EmptyKey(t *testing.T) {
	store := NewSyncStore[string]()
	
	store.Store("", "empty_key_value")
	val, ok := store.Load("")
	if !ok || val != "empty_key_value" {
		t.Errorf("Empty key: got (%q, %v), want (\"empty_key_value\", true)", val, ok)
	}
}

func TestStore_NilValues(t *testing.T) {
	store := NewSyncStore[*string]()
	
	// Store nil pointer
	store.Store("key1", nil)
	val, ok := store.Load("key1")
	if !ok || val != nil {
		t.Errorf("Nil pointer: got (%v, %v), want (nil, true)", val, ok)
	}
}

func TestStore_RangeEarlyExit(t *testing.T) {
	store := NewSyncStore[int]()
	
	// Populate store
	for i := 0; i < 10; i++ {
		store.Store(string(rune('a'+i)), i)
	}
	
	count := 0
	store.Range(func(key string, val int) bool {
		count++
		return count < 5 // Stop after 5 iterations
	})
	
	if count != 5 {
		t.Errorf("Range early exit: got %d iterations, want 5", count)
	}
}