package cloudtrail

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"


	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockFileSystem for testing
type MockFileSystem struct {
	files   map[string][]byte
	errors  map[string]error
	removed map[string]bool
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files:   make(map[string][]byte),
		errors:  make(map[string]error),
		removed: make(map[string]bool),
	}
}

func (m *MockFileSystem) ReadFile(filename string) ([]byte, error) {
	if err, exists := m.errors[filename]; exists {
		return nil, err
	}
	if data, exists := m.files[filename]; exists {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	if err, exists := m.errors[filename]; exists {
		return err
	}
	m.files[filename] = data
	return nil
}

func (m *MockFileSystem) Remove(filename string) error {
	if err, exists := m.errors[filename]; exists {
		return err
	}
	delete(m.files, filename)
	m.removed[filename] = true
	return nil
}

func (m *MockFileSystem) Rename(oldpath, newpath string) error {
	if err, exists := m.errors[oldpath]; exists {
		return err
	}
	if data, exists := m.files[oldpath]; exists {
		m.files[newpath] = data
		delete(m.files, oldpath)
	}
	return nil
}

func (m *MockFileSystem) Glob(pattern string) ([]string, error) {
	if err, exists := m.errors[pattern]; exists {
		return nil, err
	}
	var matches []string
	for filename := range m.files {
		if matched, _ := filepath.Match(pattern, filename); matched {
			matches = append(matches, filename)
		}
	}
	return matches, nil
}

// MockTimeProvider for testing
type MockTimeProvider struct {
	fixedTime time.Time
}

func (m MockTimeProvider) Now() time.Time {
	return m.fixedTime
}

func (m MockTimeProvider) Parse(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}

// MockEMFFlusher for testing
type MockEMFFlusher struct {
	flushCalls []FlushCall
	flushError error
}

type FlushCall struct {
	Region  string
	Records []builder.EMFRecord
}

func (m *MockEMFFlusher) Flush(ctx context.Context, region string, batch []builder.EMFRecord) error {
	m.flushCalls = append(m.flushCalls, FlushCall{
		Region:  region,
		Records: batch,
	})
	return m.flushError
}

// Test fixtures
var (
	testTime = time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	mockTimeProvider = MockTimeProvider{fixedTime: testTime}
)

func TestIsValidRegion(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		expected bool
	}{
		{"valid us-east-1", "us-east-1", true},
		{"valid eu-west-2", "eu-west-2", true},
		{"valid ap-southeast-1", "ap-southeast-1", true},
		{"empty string", "", false},
		{"invalid region", "invalid-region", false},
		{"uppercase", "US-EAST-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.IsValidRegion(tt.region)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCTFileBatcherConfig_Validate(t *testing.T) {
	mockFlusher := &MockEMFFlusher{}
	
	tests := []struct {
		name        string
		config      CTFileBatcherConfig
		expectError bool
		errorType   error
	}{
		{
			name: "valid config",
			config: CTFileBatcherConfig{
				BaseDir:    "/tmp",
				Namespace:  "TestNamespace",
				MetricName: "TestMetric",
				EmfFlusher: mockFlusher,
			},
			expectError: false,
		},
		{
			name: "empty base dir",
			config: CTFileBatcherConfig{
				Namespace:  "TestNamespace",
				MetricName: "TestMetric",
				EmfFlusher: mockFlusher,
			},
			expectError: true,
			errorType:   ErrBaseDirEmpty,
		},
		{
			name: "empty namespace",
			config: CTFileBatcherConfig{
				BaseDir:    "/tmp",
				MetricName: "TestMetric",
				EmfFlusher: mockFlusher,
			},
			expectError: true,
			errorType:   ErrNamespaceEmpty,
		},
		{
			name: "empty metric name",
			config: CTFileBatcherConfig{
				BaseDir:   "/tmp",
				Namespace: "TestNamespace",
				EmfFlusher: mockFlusher,
			},
			expectError: true,
			errorType:   ErrMetricNameEmpty,
		},
		{
			name: "nil EMF flusher",
			config: CTFileBatcherConfig{
				BaseDir:    "/tmp",
				Namespace:  "TestNamespace",
				MetricName: "TestMetric",
			},
			expectError: true,
			errorType:   ErrEMFFlusherNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.errorType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewCTFileBatcher(t *testing.T) {
	mockFlusher := &MockEMFFlusher{}
	
	t.Run("valid config", func(t *testing.T) {
		config := CTFileBatcherConfig{
			BaseDir:    "/tmp",
			Namespace:  "TestNamespace",
			MetricName: "TestMetric",
			EmfFlusher: mockFlusher,
		}
		
		batcher, err := NewCTFileBatcher(config)
		require.NoError(t, err)
		assert.NotNil(t, batcher)
	})
	
	t.Run("invalid config", func(t *testing.T) {
		config := CTFileBatcherConfig{
			BaseDir: "/tmp",
			// Missing required fields
		}
		
		_, err := NewCTFileBatcher(config)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrNamespaceEmpty)
	})
	
	t.Run("with custom dependencies", func(t *testing.T) {
		mockFS := NewMockFileSystem()
		mockTime := MockTimeProvider{fixedTime: testTime}
		
		config := CTFileBatcherConfig{
			BaseDir:      "/tmp",
			Namespace:    "TestNamespace",
			MetricName:   "TestMetric",
			EmfFlusher:   mockFlusher,
			FileSystem:   mockFS,
			TimeProvider: mockTime,
		}
		
		batcher, err := NewCTFileBatcher(config)
		require.NoError(t, err)
		assert.NotNil(t, batcher)
	})
}

func TestCounterManager_ReadCounters(t *testing.T) {
	mockFS := NewMockFileSystem()
	cm := NewCounterManager(mockFS, "/tmp", &logger.NoopLogger{})
	
	t.Run("non-existent file", func(t *testing.T) {
		counters, err := cm.ReadCounters("us-east-1")
		require.NoError(t, err)
		assert.Empty(t, counters)
	})
	
	t.Run("existing file", func(t *testing.T) {
		testCounters := map[string]int{"event1": 5, "event2": 10}
		data, _ := json.Marshal(testCounters)
		mockFS.files["/tmp/counters_us-east-1.json"] = data
		
		counters, err := cm.ReadCounters("us-east-1")
		require.NoError(t, err)
		assert.Equal(t, testCounters, counters)
	})
	
	t.Run("invalid JSON", func(t *testing.T) {
		mockFS.files["/tmp/counters_invalid.json"] = []byte("invalid json")
		
		_, err := cm.ReadCounters("invalid")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCounterFileRead)
	})
	
	t.Run("read error", func(t *testing.T) {
		mockFS.errors["/tmp/counters_error.json"] = errors.New("read error")
		
		_, err := cm.ReadCounters("error")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCounterFileRead)
	})
}

func TestCounterManager_WriteCounters(t *testing.T) {
	mockFS := NewMockFileSystem()
	cm := NewCounterManager(mockFS, "/tmp", &logger.NoopLogger{})
	
	t.Run("successful write", func(t *testing.T) {
		testCounters := map[string]int{"event1": 5, "event2": 10}
		
		err := cm.WriteCounters("us-east-1", testCounters)
		require.NoError(t, err)
		
		// Verify file was written
		data, exists := mockFS.files["/tmp/counters_us-east-1.json"]
		assert.True(t, exists)
		
		var savedCounters map[string]int
		err = json.Unmarshal(data, &savedCounters)
		require.NoError(t, err)
		assert.Equal(t, testCounters, savedCounters)
	})
	
	t.Run("write error", func(t *testing.T) {
		mockFS.errors["/tmp/counters_error.json.tmp"] = errors.New("write error")
		
		err := cm.WriteCounters("error", map[string]int{"test": 1})
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCounterFileWrite)
	})
}

func TestCounterManager_IncrementCounter(t *testing.T) {
	mockFS := NewMockFileSystem()
	cm := NewCounterManager(mockFS, "/tmp", &logger.NoopLogger{})
	
	t.Run("new counter", func(t *testing.T) {
		err := cm.IncrementCounter("us-east-1", "event1")
		require.NoError(t, err)
		
		counters, err := cm.ReadCounters("us-east-1")
		require.NoError(t, err)
		assert.Equal(t, 1, counters["event1"])
	})
	
	t.Run("existing counter", func(t *testing.T) {
		// Set up existing counter
		testCounters := map[string]int{"event1": 5}
		data, _ := json.Marshal(testCounters)
		mockFS.files["/tmp/counters_us-east-1.json"] = data
		
		err := cm.IncrementCounter("us-east-1", "event1")
		require.NoError(t, err)
		
		counters, err := cm.ReadCounters("us-east-1")
		require.NoError(t, err)
		assert.Equal(t, 6, counters["event1"])
	})
}

func TestCounterManager_GetRegions(t *testing.T) {
	mockFS := NewMockFileSystem()
	cm := NewCounterManager(mockFS, "/tmp", &logger.NoopLogger{})
	
	t.Run("no files", func(t *testing.T) {
		regions, err := cm.GetRegions()
		require.NoError(t, err)
		assert.Empty(t, regions)
	})
	
	t.Run("multiple regions", func(t *testing.T) {
		mockFS.files["/tmp/counters_us-east-1.json"] = []byte("{}")
		mockFS.files["/tmp/counters_eu-west-2.json"] = []byte("{}")
		mockFS.files["/tmp/other_file.txt"] = []byte("ignored")
		
		regions, err := cm.GetRegions()
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"us-east-1", "eu-west-2"}, regions)
	})
	
	t.Run("glob error", func(t *testing.T) {
		mockFS.errors["/tmp/counters_*.json"] = errors.New("glob error")
		
		_, err := cm.GetRegions()
		assert.Error(t, err)
	})
}

func TestTimeCalculator_CalculateElapsedTime(t *testing.T) {
	mockFS := NewMockFileSystem()
	tc := NewTimeCalculator(mockFS, mockTimeProvider, "/tmp", "/tmp/lastFlush", "init.txt", &logger.NoopLogger{})
	
	t.Run("with last flush time", func(t *testing.T) {
		lastFlush := testTime.Add(-30 * time.Second)
		mockFS.files["/tmp/lastFlush"] = []byte(lastFlush.Format(time.RFC3339Nano))
		
		elapsed, err := tc.CalculateElapsedTime(testTime)
		require.NoError(t, err)
		assert.Equal(t, 30.0, elapsed)
	})
	
	t.Run("with init time fallback", func(t *testing.T) {
		// Clear any existing files first
		mockFS.files = make(map[string][]byte)
		initTime := testTime.Add(-45 * time.Second)
		mockFS.files["/tmp/init.txt"] = []byte(initTime.Format(time.RFC3339Nano))
		
		elapsed, err := tc.CalculateElapsedTime(testTime)
		require.NoError(t, err)
		assert.Equal(t, 45.0, elapsed)
	})
	
	t.Run("default fallback", func(t *testing.T) {
		// Clear any existing files first
		mockFS.files = make(map[string][]byte)
		elapsed, err := tc.CalculateElapsedTime(testTime)
		require.NoError(t, err)
		assert.Equal(t, 60.0, elapsed)
	})
	
	t.Run("negative elapsed time", func(t *testing.T) {
		futureTime := testTime.Add(30 * time.Second)
		mockFS.files["/tmp/lastFlush"] = []byte(futureTime.Format(time.RFC3339Nano))
		
		elapsed, err := tc.CalculateElapsedTime(testTime)
		require.NoError(t, err)
		assert.Equal(t, 60.0, elapsed) // Should default to 60
	})
}

func TestGenerateCounterKeys(t *testing.T) {
	event := sharedTypes.CloudTrailEvent{
		EventName: "TestEvent",
		UserIdentity: sharedTypes.UserIdentity{
			Type: "IAMUser",
			ARN:  "arn:aws:iam::123456789012:user/testuser",
		},
	}
	
	t.Run("without propagate invoker", func(t *testing.T) {
		keys := GenerateCounterKeys(event, false)
		assert.Equal(t, []string{"TestEvent"}, keys)
	})
	
	t.Run("with propagate invoker", func(t *testing.T) {
		keys := GenerateCounterKeys(event, true)
		assert.Len(t, keys, 2)
		assert.Equal(t, "TestEvent", keys[0])
		assert.Contains(t, keys[1], "TestEvent:IAMUser:")
	})
}

func TestParseCounterKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected [][]string
	}{
		{
			name:     "simple key",
			key:      "TestEvent",
			expected: [][]string{{"eventName", "TestEvent"}},
		},
		{
			name:     "complex key",
			key:      "TestEvent:IAMUser:testuser",
			expected: [][]string{{"eventName", "TestEvent"}, {"invoker", "IAMUser:testuser"}},
		},
		{
			name:     "malformed key",
			key:      "TestEvent:IAMUser",
			expected: [][]string{{"eventName", "TestEvent:IAMUser"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCounterKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractInvoker(t *testing.T) {
	tests := []struct {
		name     string
		event    sharedTypes.CloudTrailEvent
		expected string
	}{
		{
			name: "IAM User",
			event: sharedTypes.CloudTrailEvent{
				UserIdentity: sharedTypes.UserIdentity{
					Type: "IAMUser",
					ARN:  "arn:aws:iam::123456789012:user/testuser",
				},
			},
			expected: "IAMUser:testuser",
		},
		{
			name: "Root",
			event: sharedTypes.CloudTrailEvent{
				UserIdentity: sharedTypes.UserIdentity{
					Type:      "Root",
					ARN:       "arn:aws:iam::123456789012:root",
					AccountId: "123456789012",
				},
			},
			expected: "Root:123456789012",
		},
		{
			name: "AWS Service",
			event: sharedTypes.CloudTrailEvent{
				UserIdentity: sharedTypes.UserIdentity{
					Type:      "AWSService",
					InvokedBy: "lambda.amazonaws.com",
				},
			},
			expected: "AWSService:lambda.amazonaws.com",
		},
		{
			name: "Unknown type",
			event: sharedTypes.CloudTrailEvent{
				UserIdentity: sharedTypes.UserIdentity{
					Type: "UnknownType",
					ARN:  "arn:aws:iam::123456789012:unknown/test",
				},
			},
			expected: "UnknownType:test",
		},
		{
			name: "Empty ARN fallback",
			event: sharedTypes.CloudTrailEvent{
				UserIdentity: sharedTypes.UserIdentity{
					Type: "UnknownType",
				},
			},
			expected: "UnknownType:unknownInvoker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractInvoker(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCTFileBatcher_Add(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFlusher := &MockEMFFlusher{}
	
	config := CTFileBatcherConfig{
		BaseDir:          "/tmp",
		Namespace:        "TestNamespace",
		MetricName:       "TestMetric",
		PropagateInvoker: true,
		EmfFlusher:       mockFlusher,
		FileSystem:       mockFS,
		TimeProvider:     mockTimeProvider,
		Logger:           &logger.NoopLogger{},
	}
	
	batcher, err := NewCTFileBatcher(config)
	require.NoError(t, err)
	
	event := sharedTypes.CloudTrailEvent{
		EventName: "TestEvent",
		UserIdentity: sharedTypes.UserIdentity{
			Type: "IAMUser",
			ARN:  "arn:aws:iam::123456789012:user/testuser",
		},
	}
	
	t.Run("valid region and event", func(t *testing.T) {
		batcher.Add(context.Background(), "us-east-1", event)
		
		// Verify counters were incremented
		cm := batcher.(*CTFileBatcher).counterMgr
		counters, err := cm.ReadCounters("us-east-1")
		require.NoError(t, err)
		assert.Equal(t, 1, counters["TestEvent"])
		assert.Equal(t, 1, counters["TestEvent:IAMUser:testuser"])
	})
	
	t.Run("invalid region", func(t *testing.T) {
		// Clear existing files first
		mockFS.files = make(map[string][]byte)
		// Should not panic or create files
		batcher.Add(context.Background(), "invalid/region", event)
		
		// Verify no files were created
		assert.Empty(t, mockFS.files)
	})
}

func TestCTFileBatcher_FlushAll(t *testing.T) {
	mockFS := NewMockFileSystem()
	mockFlusher := &MockEMFFlusher{}
	
	config := CTFileBatcherConfig{
		BaseDir:           "/tmp",
		Namespace:         "TestNamespace",
		MetricName:        "TestMetric",
		LastFlushFilePath: "/tmp/lastFlush",
		EmfFlusher:        mockFlusher,
		FileSystem:        mockFS,
		TimeProvider:      mockTimeProvider,
		Logger:            &logger.NoopLogger{},
	}
	
	batcher, err := NewCTFileBatcher(config)
	require.NoError(t, err)
	
	t.Run("successful flush", func(t *testing.T) {
		// Set up counter data
		counters := map[string]int{"TestEvent": 10, "AnotherEvent": 5}
		data, _ := json.Marshal(counters)
		mockFS.files["/tmp/counters_us-east-1.json"] = data
		
		// Set up last flush time (30 seconds ago)
		lastFlush := testTime.Add(-30 * time.Second)
		mockFS.files["/tmp/lastFlush"] = []byte(lastFlush.Format(time.RFC3339Nano))
		
		err := batcher.FlushAll(context.Background(), testTime)
		require.NoError(t, err)
		
		// Verify EMF flusher was called
		assert.Len(t, mockFlusher.flushCalls, 1)
		assert.Equal(t, "us-east-1", mockFlusher.flushCalls[0].Region)
		assert.Len(t, mockFlusher.flushCalls[0].Records, 2)
		
		// Verify counter file was cleared
		_, exists := mockFS.files["/tmp/counters_us-east-1.json"]
		assert.False(t, exists)
		
		// Verify flush time was saved
		savedTime, exists := mockFS.files["/tmp/lastFlush"]
		assert.True(t, exists)
		assert.Equal(t, testTime.Format(time.RFC3339Nano), string(savedTime))
	})
	
	t.Run("flush error", func(t *testing.T) {
		mockFlusher.flushError = errors.New("flush failed")
		
		// Set up counter data
		counters := map[string]int{"TestEvent": 1}
		data, _ := json.Marshal(counters)
		mockFS.files["/tmp/counters_us-east-1.json"] = data
		
		err := batcher.FlushAll(context.Background(), testTime)
		require.NoError(t, err) // FlushAll doesn't return flush errors
		
		// Verify counter file was NOT cleared due to flush error
		_, exists := mockFS.files["/tmp/counters_us-east-1.json"]
		assert.True(t, exists)
	})
}

// Benchmark tests
func BenchmarkExtractInvoker(b *testing.B) {
	event := sharedTypes.CloudTrailEvent{
		UserIdentity: sharedTypes.UserIdentity{
			Type: "IAMUser",
			ARN:  "arn:aws:iam::123456789012:user/testuser",
		},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ExtractInvoker(event)
	}
}

func BenchmarkCounterIncrement(b *testing.B) {
	mockFS := NewMockFileSystem()
	cm := NewCounterManager(mockFS, "/tmp", &logger.NoopLogger{})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cm.IncrementCounter("us-east-1", "TestEvent")
	}
}