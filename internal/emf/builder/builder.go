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

// EMFInput contains parameters needed to build an EMF document.
type EMFInput struct {
	Namespace  string     // CloudWatch namespace for the metric
	MetricName string     // Name of the metric
	Value      float64    // Metric value
	Unit       string     // Metric unit (e.g., "Count", "Percent")
	Dimensions [][]string // Metric dimensions as key-value pairs
	Timestamp  time.Time  // Timestamp for the metric
}

// EMFRecord represents a complete EMF document ready for ingestion.
type EMFRecord struct {
	Payload    []byte     // JSON-encoded EMF document
	TimeStamp  time.Time  // Timestamp of the metric event
	Dimensions [][]string // Original dimensions for grouping/aggregation
}

// Build creates an EMF document from the provided input parameters.
// Returns an EMFRecord with payload, timestamp, and dimensions.
func Build(input EMFInput, logger logger.Logger) (EMFRecord, error) {
	ts := input.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	// start the document with metric name and value
	doc := map[string]any{
		input.MetricName: input.Value,
	}

	// dynamically add dimensions to top-level and collect their names
	dimNames := make([]string, 0, len(input.Dimensions))
	for _, dim := range input.Dimensions {
		if len(dim) >= 2 {
			name, value := dim[0], dim[1]
			doc[name] = value
			dimNames = append(dimNames, name)
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
		logger.Error("Error marshaling EMF payload: %v", err)
		return EMFRecord{}, err
	}

	var cleanDims [][]string
	for _, dim := range input.Dimensions {
		if len(dim) >= 2 {
			cleanDims = append(cleanDims, dim)
		}
	}

	return EMFRecord{
		Payload:    data,
		TimeStamp:  ts,
		Dimensions: cleanDims,
	}, nil
}

// ConvertSQSMessageToEMF converts an SQS message containing a CloudTrail event
// into an EMF record for CloudWatch metrics ingestion.
func ConvertSQSMessageToEMF(
	ctx context.Context,
	msg events.SQSMessage,
	namespace, metricName, unit string,
	dimensions [][]string,
	applogger logger.Logger,
) (EMFRecord, error) {
	var ctEvent sharedTypes.CloudTrailEvent
	if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
		applogger.Error("Error unmarshaling CloudTrail event: %v", err)
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
