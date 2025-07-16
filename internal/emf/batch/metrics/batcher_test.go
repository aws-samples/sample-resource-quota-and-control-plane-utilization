package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing
type MockEMFFlusher struct {
	mock.Mock
}

func (m *MockEMFFlusher) Flush(ctx context.Context, region string, records []builder.EMFRecord) error {
	args := m.Called(ctx, region, records)
	return args.Error(0)
}

type MockEMFBuilder struct {
	mock.Mock
}

func (m *MockEMFBuilder) Build(input builder.EMFInput, logger logger.Logger) (builder.EMFRecord, error) {
	args := m.Called(input, logger)
	return args.Get(0).(builder.EMFRecord), args.Error(1)
}

type MockThresholdChecker struct {
	mock.Mock
}

func (m *MockThresholdChecker) ShouldFlush(currentCount int, currentSize int64, newRecordSize int64, maxCount int, maxBytes int64) bool {
	args := m.Called(currentCount, currentSize, newRecordSize, maxCount, maxBytes)
	return args.Bool(0)
}

type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Info(format string, args ...interface{})  { m.Called(format, args) }
func (m *MockLogger) Debug(format string, args ...interface{}) { m.Called(format, args) }
func (m *MockLogger) Error(format string, args ...interface{}) { m.Called(format, args) }
func (m *MockLogger) Warn(format string, args ...interface{})  { m.Called(format, args) }

// Test data
var (
	testEMFRecord = builder.EMFRecord{
		Payload: []byte(`{"test": "payload"}`),
	}
	testMetric = sharedTypes.CloudWatchMetric{
		Name:      "TestMetric",
		Value:     42.0,
		Unit:      sharedTypes.UnitPercent,
		Timestamp: time.Now(),
		Metadata:  map[string]string{"region": "us-east-1"},
	}
)

func TestMetricsBatcherConfig_Validate(t *testing.T) {
	mockFlusher := &MockEMFFlusher{}

	tests := []struct {
		name    string
		config  MetricsBatcherConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: MetricsBatcherConfig{
				Namespace:  "TestNamespace",
				Region:     "us-east-1",
				EmfFlusher: mockFlusher,
			},
			wantErr: nil,
		},
		{
			name: "empty namespace",
			config: MetricsBatcherConfig{
				Region:     "us-east-1",
				EmfFlusher: mockFlusher,
			},
			wantErr: ErrEmptyNamespace,
		},
		{
			name: "empty region",
			config: MetricsBatcherConfig{
				Namespace:  "TestNamespace",
				EmfFlusher: mockFlusher,
			},
			wantErr: ErrEmptyRegion,
		},
		{
			name: "nil EMF flusher",
			config: MetricsBatcherConfig{
				Namespace: "TestNamespace",
				Region:    "us-east-1",
			},
			wantErr: ErrNilEMFFlusher,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewMetricsBatcher(t *testing.T) {
	mockFlusher := &MockEMFFlusher{}

	tests := []struct {
		name    string
		config  MetricsBatcherConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: MetricsBatcherConfig{
				Namespace:  "TestNamespace",
				Region:     "us-east-1",
				EmfFlusher: mockFlusher,
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: MetricsBatcherConfig{
				Namespace: "TestNamespace",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batcher, err := NewMetricsBatcher(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, batcher)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, batcher)
			}
		})
	}
}

func TestMetricsBatcher_Add(t *testing.T) {
	mockFlusher := &MockEMFFlusher{}
	mockBuilder := &MockEMFBuilder{}
	mockChecker := &MockThresholdChecker{}
	mockLogger := &MockLogger{}

	// Setup mocks
	mockBuilder.On("Build", mock.Anything, mock.Anything).Return(testEMFRecord, nil)
	mockChecker.On("ShouldFlush", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(false)
	mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Error", mock.Anything, mock.Anything).Maybe()

	batcher, err := NewMetricsBatcher(MetricsBatcherConfig{
		Namespace:        "TestNamespace",
		Region:           "us-east-1",
		EmfFlusher:       mockFlusher,
		Logger:           mockLogger,
		EMFBuilder:       mockBuilder,
		ThresholdChecker: mockChecker,
	})
	assert.NoError(t, err)

	ctx := context.Background()
	batcher.Add(ctx, testMetric)

	mockBuilder.AssertExpectations(t)
	mockChecker.AssertExpectations(t)
}

func TestMetricsBatcher_FlushAll(t *testing.T) {
	mockFlusher := &MockEMFFlusher{}
	mockLogger := &MockLogger{}

	mockFlusher.On("Flush", mock.Anything, "us-east-1", mock.Anything).Return(nil)
	mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Error", mock.Anything, mock.Anything).Maybe()

	batcher, err := NewMetricsBatcher(MetricsBatcherConfig{
		Namespace:  "TestNamespace",
		Region:     "us-east-1",
		EmfFlusher: mockFlusher,
		Logger:     mockLogger,
	})
	assert.NoError(t, err)

	// Add a record to the batch so FlushAll will actually call Flush
	mb := batcher.(*MetricsBatcher)
	mb.records = []builder.EMFRecord{testEMFRecord}
	mb.count = 1
	mb.size = int64(len(testEMFRecord.Payload))

	ctx := context.Background()
	batcher.FlushAll(ctx)

	// Verify batch is reset
	assert.Empty(t, mb.records)
	assert.Equal(t, 0, mb.count)
	assert.Equal(t, int64(0), mb.size)

	mockFlusher.AssertExpectations(t)
}

func TestDefaultThresholdChecker_ShouldFlush(t *testing.T) {
	checker := DefaultThresholdChecker{}

	tests := []struct {
		name           string
		currentCount   int
		currentSize    int64
		newRecordSize  int64
		maxCount       int
		maxBytes       int64
		expectedResult bool
	}{
		{
			name:           "no thresholds set",
			currentCount:   5,
			currentSize:    100,
			newRecordSize:  50,
			maxCount:       0,
			maxBytes:       0,
			expectedResult: false,
		},
		{
			name:           "count threshold reached",
			currentCount:   9,
			currentSize:    100,
			newRecordSize:  50,
			maxCount:       10,
			maxBytes:       0,
			expectedResult: true,
		},
		{
			name:           "bytes threshold exceeded",
			currentCount:   5,
			currentSize:    950,
			newRecordSize:  100,
			maxCount:       0,
			maxBytes:       1000,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.ShouldFlush(tt.currentCount, tt.currentSize, tt.newRecordSize, tt.maxCount, tt.maxBytes)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestBuildDimensions(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		expected map[string]string
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: nil,
		},
		{
			name: "single dimension",
			metadata: map[string]string{
				"region": "us-east-1",
			},
			expected: map[string]string{"region": "us-east-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildDimensions(tt.metadata)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Len(t, result, len(tt.expected))
				// Convert result back to map for comparison
				resultMap := make(map[string]string)
				for _, dim := range result {
					assert.Len(t, dim, 2, "each dimension should have exactly 2 elements")
					resultMap[dim[0]] = dim[1]
				}
				assert.Equal(t, tt.expected, resultMap)
			}
		})
	}
}