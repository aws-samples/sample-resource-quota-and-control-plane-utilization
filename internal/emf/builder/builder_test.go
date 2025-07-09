// internal/emf/builder/builder_test.go
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
)

func TestBuild_WithValidInputs(t *testing.T) {
	logger := &logger.NoopLogger{}
	t0 := time.Date(2025, 7, 7, 12, 34, 56, 0, time.UTC)

	tests := []struct {
		name          string
		input         EMFInput
		wantTimestamp time.Time
		wantDims      [][]string
	}{
		{
			name: "single dimension",
			input: EMFInput{
				Namespace:  "ns",
				MetricName: "m",
				Value:      1.0,
				Unit:       MetricUnitCount,
				Dimensions: [][]string{{"key", "val"}},
				Timestamp:  t0,
			},
			wantTimestamp: t0,
			wantDims:      [][]string{{"key", "val"}},
		},
		{
			name: "ignore invalid dims",
			input: EMFInput{
				Namespace:  "ns",
				MetricName: "m2",
				Value:      2.0,
				Unit:       MetricUnitCount,
				Dimensions: [][]string{{"bad"}, {"k2", "v2"}},
				Timestamp:  t0,
			},
			wantTimestamp: t0,
			wantDims:      [][]string{{"k2", "v2"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := Build(tt.input, logger)
			assert.NoError(t, err)
			// Timestamp preserved
			assert.Equal(t, tt.wantTimestamp.UnixMilli(), rec.TimeStamp.UnixMilli())
			// Dimensions preserved
			assert.Equal(t, tt.wantDims, rec.Dimensions)

			// Parse payload JSON
			var doc map[string]any
			err = json.Unmarshal(rec.Payload, &doc)
			assert.NoError(t, err)

			// Check metric value
			v, ok := doc[tt.input.MetricName].(float64)
			assert.True(t, ok)
			assert.Equal(t, tt.input.Value, v)

			// Check dimensions in top-level
			for _, dim := range tt.wantDims {
				val, ok := doc[dim[0]].(any)
				assert.True(t, ok)
				assert.Equal(t, dim[1], val)
			}

			// Check _aws block
			awsBlock, ok := doc["_aws"].(map[string]any)
			assert.True(t, ok)
			tsVal, ok := awsBlock["Timestamp"].(float64)
			assert.True(t, ok)
			assert.Equal(t, float64(tt.wantTimestamp.UnixMilli()), tsVal)

			cwMetrics, ok := awsBlock["CloudWatchMetrics"].([]any)
			assert.True(t, ok)
			assert.Len(t, cwMetrics, 1)
			metricDef := cwMetrics[0].(map[string]any)
			assert.Equal(t, tt.input.Namespace, metricDef["Namespace"])

			// Check nested Metrics array
			metricsArr := metricDef["Metrics"].([]any)
			assert.Len(t, metricsArr, 1)
			md := metricsArr[0].(map[string]any)
			assert.Equal(t, tt.input.MetricName, md["Name"])
			assert.Equal(t, tt.input.Unit, md["Unit"])
		})
	}
}

func TestBuild_ZeroTimestamp_UsesNow(t *testing.T) {
	logger := &logger.NoopLogger{}
	before := time.Now().Add(-time.Second)
	rec, err := Build(EMFInput{
		Namespace:  "ns",
		MetricName: "m",
		Value:      3.0,
		Unit:       MetricUnitCount,
		Dimensions: nil,
		Timestamp:  time.Time{},
	}, logger)
	after := time.Now().Add(time.Second)
	assert.NoError(t, err)
	assert.True(t, rec.TimeStamp.After(before))
	assert.True(t, rec.TimeStamp.Before(after))
}

func TestConvertSQSMessageToEMF_ValidJSONAndUnit(t *testing.T) {
	logger := &logger.NoopLogger{}
	t0 := time.Date(2025, 7, 7, 12, 0, 0, 0, time.UTC)
	ct := sharedTypes.CloudTrailEvent{EventName: "Evt", EventTime: t0}
	body, err := json.Marshal(ct)
	assert.NoError(t, err)
	msg := events.SQSMessage{Body: string(body)}

	rec, err := ConvertSQSMessageToEMF(
		context.Background(),
		msg,
		"ns",       // namespace
		"mymetric", // metricName
		"Count",    // unit
		[][]string{{"d", "v"}},
		logger,
	)
	assert.NoError(t, err)
	assert.Equal(t, t0.UnixMilli(), rec.TimeStamp.UnixMilli())
	assert.Equal(t, [][]string{{"d", "v"}}, rec.Dimensions)

	var doc map[string]any
	err = json.Unmarshal(rec.Payload, &doc)
	assert.NoError(t, err)

	awsBlock := doc["_aws"].(map[string]any)
	cwMetrics := awsBlock["CloudWatchMetrics"].([]any)
	metricDef := cwMetrics[0].(map[string]any)

	// Validate namespace
	assert.Equal(t, "ns", metricDef["Namespace"])

	metricsArr := metricDef["Metrics"].([]any)
	md := metricsArr[0].(map[string]any)

	// Validate metric name and unit
	assert.Equal(t, "mymetric", md["Name"])
	assert.Equal(t, "Count", md["Unit"])
}

func TestConvertSQSMessageToEMF_UnitNormalization(t *testing.T) {
	logger := &logger.NoopLogger{}
	t0 := time.Date(2025, 7, 7, 12, 0, 0, 0, time.UTC)
	ct := sharedTypes.CloudTrailEvent{EventName: "Evt", EventTime: t0}
	body, err := json.Marshal(ct)
	assert.NoError(t, err)
	msg := events.SQSMessage{Body: string(body)}

	rec, err := ConvertSQSMessageToEMF(
		context.Background(),
		msg,
		"ns",       // namespace
		"mymetric", // metricName
		"percent",
		nil,
		logger,
	)
	assert.NoError(t, err)

	var doc map[string]any
	err = json.Unmarshal(rec.Payload, &doc)
	assert.NoError(t, err)

	awsBlock := doc["_aws"].(map[string]any)
	cwMetrics := awsBlock["CloudWatchMetrics"].([]any)
	metricDef := cwMetrics[0].(map[string]any)

	// Validate namespace remains
	assert.Equal(t, "ns", metricDef["Namespace"])

	metricsArr := metricDef["Metrics"].([]any)
	md := metricsArr[0].(map[string]any)

	// Should normalize unit to Count
	assert.Equal(t, "mymetric", md["Name"])
	assert.Equal(t, "Count", md["Unit"])
}

func TestConvertSQSMessageToEMF_InvalidJSON(t *testing.T) {
	logger := &logger.NoopLogger{}
	msg := events.SQSMessage{Body: "invalid"}
	_, err := ConvertSQSMessageToEMF(
		context.Background(),
		msg,
		"ns",
		"mymetric",
		"Count",
		nil,
		logger,
	)
	assert.Error(t, err)
}
