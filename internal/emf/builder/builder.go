// Package builder provides functionality for creating Embedded Metric Format (EMF) documents.
// It converts metric data into properly formatted JSON documents for CloudWatch ingestion.
package builder

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (
	MetricUnitCount string = "Count"
)

// EMFInput contains all parameters needed to build an EMF document including
// metric name, value, dimensions, and timing information.
type EMFInput struct {
	Namespace  string     // CloudWatch namespace for the metric
	MetricName string     // Name of the metric
	Value      float64    // Metric value
	Unit       string     // Metric unit (e.g., "Count", "Percent")
	Dimensions [][]string // Metric dimensions as key-value pairs
	Timestamp  time.Time  // Timestamp for the metric
}

// EMFRecord represents a complete EMF document with JSON payload,
// timestamp, and dimensions ready for CloudWatch ingestion.
type EMFRecord struct {
	Payload    []byte     // JSON-encoded EMF document
	TimeStamp  time.Time  // Timestamp of the metric event
	Dimensions [][]string // Original dimensions for grouping/aggregation
}

// Build creates a properly formatted EMF JSON document from input parameters,
// including metric data, dimensions, and CloudWatch metadata structure.
func Build(input EMFInput, logger logger.Logger) (EMFRecord, error) {
	ts := input.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	// start the document with metric name and value
	doc := map[string]any{
		input.MetricName: input.Value,
	}

	// process dimensions once: add to doc, collect names, and build clean dimensions
	dimNames := make([]string, 0, len(input.Dimensions))
	cleanDims := make([][]string, 0, len(input.Dimensions))
	for _, dim := range input.Dimensions {
		if len(dim) >= 2 {
			name, value := dim[0], dim[1]
			doc[name] = value
			dimNames = append(dimNames, name)
			cleanDims = append(cleanDims, dim)
		}
	}

	doc["_aws"] = map[string]any{
		"Timestamp": ts.UnixMilli(),
		"CloudWatchMetrics": []any{
			map[string]any{
				"Namespace":  input.Namespace,
				"Dimensions": [][]string{dimNames},
				"Metrics":    []map[string]string{{"Name": input.MetricName, "Unit": input.Unit}},
			},
		},
	}

	data, err := json.Marshal(doc)
	if err != nil {
		logger.Error("Error marshaling EMF payload: %v", err.Error())
		return EMFRecord{}, err
	}

	return EMFRecord{
		Payload:    data,
		TimeStamp:  ts,
		Dimensions: cleanDims,
	}, nil
}

// ConvertSQSMessageToEMF parses a CloudTrail event from an SQS message body
// and converts it into an EMF record with specified dimensions and metrics.
func ConvertSQSMessageToEMF(
	ctx context.Context,
	msg events.SQSMessage,
	namespace, metricName, unit string,
	dimensions [][]string,
	applogger logger.Logger,
) (EMFRecord, error) {
	var ctEvent sharedTypes.CloudTrailEvent
	if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
		applogger.Error("Error unmarshaling CloudTrail event: %v", err.Error())
		return EMFRecord{}, err
	}

	timestamp := ctEvent.EventTime
	// normalize unit to Count if needed
	if !strings.EqualFold(unit, "Count") {
		applogger.Warn("Unknown metric unit %s, defaulting to Count", unit)
		unit = "Count"
	}

	in := EMFInput{
		Namespace:  namespace,
		MetricName: metricName,
		Value:      1,
		Unit:       unit,
		Dimensions: dimensions,
		Timestamp:  timestamp,
	}
	return Build(in, applogger)
}
