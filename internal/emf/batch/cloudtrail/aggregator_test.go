// internal/emf/batch/cloudtrail/aggregator_test.go
package cloudtrail

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/stretchr/testify/assert"
)

func TestDimensionKey(t *testing.T) {
	tests := []struct {
		name string
		dims [][]string
		want string
	}{
		{
			name: "single dimension",
			dims: [][]string{{"key", "value"}},
			want: "key=value",
		},
		{
			name: "multiple dimensions",
			dims: [][]string{{"a", "1"}, {"b", "2"}, {"c", "3"}},
			want: "a=1|b=2|c=3",
		},
		{
			name: "empty dims",
			dims: [][]string{},
			want: "",
		},
		{
			name: "keys with pipes",
			dims: [][]string{{"x|y", "z"}},
			want: "x|y=z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dimensionKey(tt.dims)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultEMFAggregator_Aggregate(t *testing.T) {
	logger := logger.Get()
	agg := NewDefaultEMFAggregator()
	flushTime := time.Now().Truncate(time.Second)

	// prepare raw EMFRecords: two groups
	raw := []builder.EMFRecord{
		{Dimensions: [][]string{{"d", "v1"}}, TimeStamp: time.Time{}},
		{Dimensions: [][]string{{"d", "v1"}}, TimeStamp: time.Time{}},
		{Dimensions: [][]string{{"d", "v2"}}, TimeStamp: time.Time{}},
	}

	cfg := AggregateConfig{
		Namespace:  "ns",
		MetricName: "m",
		NormFactor: 2.0,
		FlushTime:  flushTime,
		Logger:     logger,
	}

	ctx := context.Background()
	out, err := agg.Aggregate(ctx, raw, cfg)
	assert.NoError(t, err)
	// Expect two aggregated outputs
	assert.Len(t, out, 2)

	// Map output by dimension value
	m := make(map[string]builder.EMFRecord)
	for _, rec := range out {
		key := rec.Dimensions[0][1] // "v1" or "v2"
		m[key] = rec
	}

	// Check for group "v1": count 2 * NormFactor
	rec1, ok := m["v1"]
	assert.True(t, ok, "missing group v1")
	// timestamp matches
	assert.True(t, rec1.TimeStamp.Equal(flushTime), "timestamp mismatch")
	// payload contains correct value
	var doc1 map[string]interface{}
	err = json.Unmarshal(rec1.Payload, &doc1)
	assert.NoError(t, err)
	// metricName field
	val1, ok := doc1["m"].(float64)
	assert.True(t, ok)
	assert.Equal(t, 2.0*cfg.NormFactor, val1)

	// Check for group "v2": count 1 * NormFactor
	rec2, ok := m["v2"]
	assert.True(t, ok, "missing group v2")
	var doc2 map[string]interface{}
	err = json.Unmarshal(rec2.Payload, &doc2)
	assert.NoError(t, err)
	val2, ok := doc2["m"].(float64)
	assert.True(t, ok)
	assert.Equal(t, 1.0*cfg.NormFactor, val2)
}

func TestDefaultEMFAggregator_CancelledContext(t *testing.T) {
	agg := NewDefaultEMFAggregator()
	flushTime := time.Now()
	cfg := AggregateConfig{
		Namespace:  "ns",
		MetricName: "m",
		NormFactor: 1.0,
		FlushTime:  flushTime,
		Logger:     logger.Get(),
	}
	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	raw := []builder.EMFRecord{{Dimensions: [][]string{{"a", "b"}}, TimeStamp: flushTime}}
	out, err := agg.Aggregate(ctx, raw, cfg)
	assert.Nil(t, out)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}
