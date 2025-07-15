package builder

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockTimeProvider for deterministic testing
type MockTimeProvider struct {
	fixedTime time.Time
}

func (m MockTimeProvider) Now() time.Time {
	return m.fixedTime
}

// Test fixtures
var (
	testTime     = time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	mockTimeProvider = MockTimeProvider{fixedTime: testTime}
)

func TestBuild_Success(t *testing.T) {
	tests := []struct {
		name     string
		input    EMFInput
		expected map[string]interface{}
	}{
		{
			name: "basic metric with dimensions",
			input: EMFInput{
				Namespace:  "TestNamespace",
				MetricName: "TestMetric",
				Value:      42.5,
				Unit:       "Count",
				Dimensions: [][]string{{"Region", "us-east-1"}, {"Service", "lambda"}},
				Timestamp:  testTime,
			},
			expected: map[string]interface{}{
				"TestMetric": 42.5,
				"Region":     "us-east-1",
				"Service":    "lambda",
				"_aws": map[string]interface{}{
					"Timestamp": float64(testTime.UnixMilli()),
					"CloudWatchMetrics": []interface{}{
						map[string]interface{}{
							"Namespace":  "TestNamespace",
							"Dimensions": []interface{}{[]interface{}{"Region", "Service"}},
							"Metrics":    []interface{}{map[string]interface{}{"Name": "TestMetric", "Unit": "Count"}},
						},
					},
				},
			},
		},
		{
			name: "metric without dimensions",
			input: EMFInput{
				Namespace:  "SimpleNamespace",
				MetricName: "SimpleMetric",
				Value:      1.0,
				Unit:       "Percent",
				Dimensions: [][]string{},
				Timestamp:  testTime,
			},
			expected: map[string]interface{}{
				"SimpleMetric": 1.0,
				"_aws": map[string]interface{}{
					"Timestamp": float64(testTime.UnixMilli()),
					"CloudWatchMetrics": []interface{}{
						map[string]interface{}{
							"Namespace":  "SimpleNamespace",
							"Dimensions": []interface{}{[]interface{}{}},
							"Metrics":    []interface{}{map[string]interface{}{"Name": "SimpleMetric", "Unit": "Percent"}},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &logger.NoopLogger{}
			
			result, err := BuildWithTimeProvider(tt.input, log, mockTimeProvider)
			
			require.NoError(t, err)
			assert.Equal(t, testTime, result.TimeStamp)
			
			var actualDoc map[string]interface{}
			err = json.Unmarshal(result.Payload, &actualDoc)
			require.NoError(t, err)
			
			assert.Equal(t, tt.expected, actualDoc)
		})
	}
}

func TestBuild_ZeroTimestamp(t *testing.T) {
	input := EMFInput{
		Namespace:  "TestNamespace",
		MetricName: "TestMetric",
		Value:      1.0,
		Unit:       "Count",
		Dimensions: [][]string{},
		Timestamp:  time.Time{}, // Zero timestamp
	}
	
	log := &logger.NoopLogger{}
	result, err := BuildWithTimeProvider(input, log, mockTimeProvider)
	
	require.NoError(t, err)
	assert.Equal(t, testTime, result.TimeStamp)
}

func TestBuild_ValidationErrors(t *testing.T) {
	tests := []struct {
		name          string
		input         EMFInput
		expectedError error
	}{
		{
			name: "empty metric name",
			input: EMFInput{
				Namespace:  "TestNamespace",
				MetricName: "",
				Value:      1.0,
				Unit:       "Count",
			},
			expectedError: ErrEmptyMetricName,
		},
		{
			name: "whitespace metric name",
			input: EMFInput{
				Namespace:  "TestNamespace",
				MetricName: "   ",
				Value:      1.0,
				Unit:       "Count",
			},
			expectedError: ErrEmptyMetricName,
		},
		{
			name: "empty namespace",
			input: EMFInput{
				Namespace:  "",
				MetricName: "TestMetric",
				Value:      1.0,
				Unit:       "Count",
			},
			expectedError: ErrEmptyNamespace,
		},
		{
			name: "whitespace namespace",
			input: EMFInput{
				Namespace:  "   ",
				MetricName: "TestMetric",
				Value:      1.0,
				Unit:       "Count",
			},
			expectedError: ErrEmptyNamespace,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &logger.NoopLogger{}
			
			_, err := BuildWithTimeProvider(tt.input, log, mockTimeProvider)
			
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestBuild_DimensionFiltering(t *testing.T) {
	input := EMFInput{
		Namespace:  "TestNamespace",
		MetricName: "TestMetric",
		Value:      1.0,
		Unit:       "Count",
		Dimensions: [][]string{
			{"ValidKey", "ValidValue"},
			{"OnlyKey"},                    // Invalid - only one element
			{"", "EmptyKey"},               // Invalid - empty key
			{"AnotherValid", "AnotherValue"},
		},
		Timestamp: testTime,
	}
	
	log := &logger.NoopLogger{}
	result, err := BuildWithTimeProvider(input, log, mockTimeProvider)
	
	require.NoError(t, err)
	
	// Should only have 2 valid dimensions
	assert.Len(t, result.Dimensions, 2)
	assert.Equal(t, [][]string{{"ValidKey", "ValidValue"}, {"AnotherValid", "AnotherValue"}}, result.Dimensions)
	
	var doc map[string]interface{}
	err = json.Unmarshal(result.Payload, &doc)
	require.NoError(t, err)
	
	// Check that only valid dimensions are in the document
	assert.Equal(t, "ValidValue", doc["ValidKey"])
	assert.Equal(t, "AnotherValue", doc["AnotherValid"])
	assert.NotContains(t, doc, "OnlyKey")
	assert.NotContains(t, doc, "")
}

func TestBuild_DefaultFunction(t *testing.T) {
	input := EMFInput{
		Namespace:  "TestNamespace",
		MetricName: "TestMetric",
		Value:      1.0,
		Unit:       "Count",
		Timestamp:  testTime,
	}
	
	log := &logger.NoopLogger{}
	result, err := Build(input, log)
	
	require.NoError(t, err)
	assert.Equal(t, testTime, result.TimeStamp)
}

func TestConvertSQSMessageToEMF_Success(t *testing.T) {
	cloudTrailEvent := sharedTypes.CloudTrailEvent{
		EventTime: testTime,
		EventName: "TestEvent",
	}
	
	eventJSON, err := json.Marshal(cloudTrailEvent)
	require.NoError(t, err)
	
	msg := events.SQSMessage{
		Body: string(eventJSON),
	}
	
	dimensions := [][]string{{"EventName", "TestEvent"}}
	
	log := &logger.NoopLogger{}
	result, err := ConvertSQSMessageToEMFWithTimeProvider(
		context.Background(),
		msg,
		"CloudTrail",
		"APICall",
		"Count",
		dimensions,
		log,
		mockTimeProvider,
	)
	
	require.NoError(t, err)
	assert.Equal(t, testTime, result.TimeStamp)
	
	var doc map[string]interface{}
	err = json.Unmarshal(result.Payload, &doc)
	require.NoError(t, err)
	
	assert.Equal(t, 1.0, doc["APICall"])
	assert.Equal(t, "TestEvent", doc["EventName"])
}

func TestConvertSQSMessageToEMF_InvalidJSON(t *testing.T) {
	msg := events.SQSMessage{
		Body: "invalid json",
	}
	
	log := &logger.NoopLogger{}
	_, err := ConvertSQSMessageToEMFWithTimeProvider(
		context.Background(),
		msg,
		"CloudTrail",
		"APICall",
		"Count",
		[][]string{},
		log,
		mockTimeProvider,
	)
	
	assert.Equal(t, ErrUnmarshalFailed, err)
}

func TestConvertSQSMessageToEMF_UnitNormalization(t *testing.T) {
	tests := []struct {
		name         string
		inputUnit    string
		expectedUnit string
	}{
		{"count lowercase", "count", "count"}, // EqualFold matches, so no change
		{"COUNT uppercase", "COUNT", "COUNT"}, // EqualFold matches, so no change
		{"Count mixed case", "Count", "Count"},
		{"invalid unit", "InvalidUnit", "Count"},
		{"empty unit", "", "Count"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudTrailEvent := sharedTypes.CloudTrailEvent{
				EventTime: testTime,
			}
			
			eventJSON, err := json.Marshal(cloudTrailEvent)
			require.NoError(t, err)
			
			msg := events.SQSMessage{
				Body: string(eventJSON),
			}
			
			log := &logger.NoopLogger{}
			result, err := ConvertSQSMessageToEMFWithTimeProvider(
				context.Background(),
				msg,
				"CloudTrail",
				"APICall",
				tt.inputUnit,
				[][]string{},
				log,
				mockTimeProvider,
			)
			
			require.NoError(t, err)
			
			var doc map[string]interface{}
			err = json.Unmarshal(result.Payload, &doc)
			require.NoError(t, err)
			
			awsSection := doc["_aws"].(map[string]interface{})
			metrics := awsSection["CloudWatchMetrics"].([]interface{})[0].(map[string]interface{})
			metricsList := metrics["Metrics"].([]interface{})[0].(map[string]interface{})
			
			assert.Equal(t, tt.expectedUnit, metricsList["Unit"])
		})
	}
}

func TestConvertSQSMessageToEMF_DefaultFunction(t *testing.T) {
	cloudTrailEvent := sharedTypes.CloudTrailEvent{
		EventTime: testTime,
	}
	
	eventJSON, err := json.Marshal(cloudTrailEvent)
	require.NoError(t, err)
	
	msg := events.SQSMessage{
		Body: string(eventJSON),
	}
	
	log := &logger.NoopLogger{}
	result, err := ConvertSQSMessageToEMF(
		context.Background(),
		msg,
		"CloudTrail",
		"APICall",
		"Count",
		[][]string{},
		log,
	)
	
	require.NoError(t, err)
	assert.Equal(t, testTime, result.TimeStamp)
}

func TestTimeProvider_DefaultTimeProvider(t *testing.T) {
	provider := DefaultTimeProvider{}
	now1 := provider.Now()
	time.Sleep(1 * time.Millisecond)
	now2 := provider.Now()
	
	assert.True(t, now2.After(now1))
}

func TestTimeProvider_MockTimeProvider(t *testing.T) {
	fixedTime := time.Date(2023, 5, 10, 12, 0, 0, 0, time.UTC)
	provider := MockTimeProvider{fixedTime: fixedTime}
	
	assert.Equal(t, fixedTime, provider.Now())
	assert.Equal(t, fixedTime, provider.Now()) // Should be consistent
}

func TestEMFRecord_Structure(t *testing.T) {
	input := EMFInput{
		Namespace:  "TestNamespace",
		MetricName: "TestMetric",
		Value:      123.45,
		Unit:       "Bytes",
		Dimensions: [][]string{{"Key1", "Value1"}, {"Key2", "Value2"}},
		Timestamp:  testTime,
	}
	
	log := &logger.NoopLogger{}
	result, err := BuildWithTimeProvider(input, log, mockTimeProvider)
	
	require.NoError(t, err)
	
	// Verify EMFRecord structure
	assert.NotEmpty(t, result.Payload)
	assert.Equal(t, testTime, result.TimeStamp)
	assert.Equal(t, [][]string{{"Key1", "Value1"}, {"Key2", "Value2"}}, result.Dimensions)
	
	// Verify JSON structure
	var doc map[string]interface{}
	err = json.Unmarshal(result.Payload, &doc)
	require.NoError(t, err)
	
	// Check metric value
	assert.Equal(t, 123.45, doc["TestMetric"])
	
	// Check dimensions are added to root
	assert.Equal(t, "Value1", doc["Key1"])
	assert.Equal(t, "Value2", doc["Key2"])
	
	// Check _aws section
	awsSection, exists := doc["_aws"]
	require.True(t, exists)
	
	awsMap := awsSection.(map[string]interface{})
	assert.Equal(t, float64(testTime.UnixMilli()), awsMap["Timestamp"])
	
	// Check CloudWatchMetrics structure
	cwMetrics := awsMap["CloudWatchMetrics"].([]interface{})
	require.Len(t, cwMetrics, 1)
	
	metric := cwMetrics[0].(map[string]interface{})
	assert.Equal(t, "TestNamespace", metric["Namespace"])
	
	dimensions := metric["Dimensions"].([]interface{})
	require.Len(t, dimensions, 1)
	dimArray := dimensions[0].([]interface{})
	assert.Contains(t, dimArray, "Key1")
	assert.Contains(t, dimArray, "Key2")
	
	metrics := metric["Metrics"].([]interface{})
	require.Len(t, metrics, 1)
	metricInfo := metrics[0].(map[string]interface{})
	assert.Equal(t, "TestMetric", metricInfo["Name"])
	assert.Equal(t, "Bytes", metricInfo["Unit"])
}

// Benchmark tests
func BenchmarkBuild(b *testing.B) {
	input := EMFInput{
		Namespace:  "BenchmarkNamespace",
		MetricName: "BenchmarkMetric",
		Value:      42.0,
		Unit:       "Count",
		Dimensions: [][]string{{"Region", "us-east-1"}, {"Service", "lambda"}},
		Timestamp:  testTime,
	}
	
	log := &logger.NoopLogger{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BuildWithTimeProvider(input, log, mockTimeProvider)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConvertSQSMessageToEMF(b *testing.B) {
	cloudTrailEvent := sharedTypes.CloudTrailEvent{
		EventTime: testTime,
		EventName: "BenchmarkEvent",
	}
	
	eventJSON, _ := json.Marshal(cloudTrailEvent)
	msg := events.SQSMessage{Body: string(eventJSON)}
	dimensions := [][]string{{"EventName", "BenchmarkEvent"}}
	log := &logger.NoopLogger{}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ConvertSQSMessageToEMFWithTimeProvider(
			context.Background(),
			msg,
			"CloudTrail",
			"APICall",
			"Count",
			dimensions,
			log,
			mockTimeProvider,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}