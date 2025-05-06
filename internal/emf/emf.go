package emf

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (
	metricUnitCount = "Count"

	// error messages
	noClientFoundForRegionErr = "no client found for region"
)

// EMFInput holds the minimal inputs needed to build your metric.
type EMFInput struct {
	Namespace  string
	MetricName string
	Value      float64
	Unit       string
	Dimensions [][]string
	Timestamp  time.Time
}

// EMFRecord contains the EMF document
type EMFRecord struct {
	Payload   []byte
	TimeStamp time.Time
}

// Build returns the JSON‐encoded EMF document.
func Build(input EMFInput, logger logger.Logger) (EMFRecord, error) {
	ts := input.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	// start the document with eventName
	doc := map[string]any{
		input.MetricName: input.Value,
	}

	// dynamically add add dimensions to top-level and collect their names
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
				"Metrics": []map[string]string{
					{"Name": input.MetricName, "Unit": input.Unit},
				},
			},
		},
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return EMFRecord{}, logAndReturnError(err, logger)
	}
	return EMFRecord{
		Payload:   data,
		TimeStamp: ts,
	}, nil
}

// ConvertSQSMessageToEMF take one SQS-wrapped CloudTrail event and returns
// the JSON EMF document (ready to ship to cloudwatchlogs)
func ConvertSQSMessageToEMF(ctx context.Context, msg events.SQSMessage,
	namespace, metricName, unit string,
	dimensions [][]string, applogger logger.Logger) (EMFRecord, error) {
	// Unmarshal the CloudTrail event
	var ctEvent sharedtypes.CloudTrailEvent
	if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
		return EMFRecord{}, logAndReturnError(err, applogger)
	}

	timestamp := ctEvent.EventTime

	lowerCaseUnit := strings.ToLower(unit)
	lowerCaseMetricNameCount := strings.ToLower(metricUnitCount)
	switch lowerCaseUnit {
	case lowerCaseMetricNameCount:
		{
			applogger.Debug("Metric unit is %s for %s metric ", unit, metricName)
			unit = metricUnitCount
		}
	default:
		{
			applogger.Warn("Unknown metric unit %s for %s metric", unit, metricName)
			applogger.Warn("Defaulting to Count")
			unit = metricUnitCount
		}
	}

	// Build the EMF envelope
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

// handle errors
func logAndReturnError(err error, applogger logger.Logger) error {
	applogger.Error("Error: %s", err.Error())
	return err
}

// EMFFusher knows how to send a batch of EMF records to Cloudwatch
type EMFFusher interface {
	Flush(ctx context.Context, region string, batch []EMFRecord) error
}

// EMFFlusherImpl uses a regional Cloudwatchclient map to push batchces
type EMFFlusherImpl struct {
	CwlClientMap  *safemap.TypedMap[cwlclient.CloudWatchLogsClient]
	LogStreamName string
	LogGroupName  string
	Logger        logger.Logger
}

type EMFFlusherConfig struct {
	CwlClientMap  *safemap.TypedMap[cwlclient.CloudWatchLogsClient]
	LogStreamName string
	LogGroupName  string
	Logger        logger.Logger
}

func NewEMFFlusher(config EMFFlusherConfig) EMFFusher {
	return &EMFFlusherImpl{
		CwlClientMap:  config.CwlClientMap,
		LogStreamName: config.LogStreamName,
		LogGroupName:  config.LogGroupName,
		Logger:        config.Logger,
	}
}

func (efi *EMFFlusherImpl) Flush(ctx context.Context, region string, batch []EMFRecord) error {
	if len(batch) == 0 {
		efi.Logger.Info("batch empty for %s region", region)
		return nil
	}
	client, ok := efi.CwlClientMap.Load(region)
	if !ok {
		return fmt.Errorf(noClientFoundForRegionErr+" %s", region)
	}
	// convert the batch to Input Log Event structure
	var logEvents []cwlTypes.InputLogEvent
	for _, records := range batch {
		logEvents = append(logEvents, cwlTypes.InputLogEvent{
			Timestamp: aws.Int64(records.TimeStamp.UnixMilli()),
			Message:   aws.String(string(records.Payload)),
		})
	}

	// sort them by timestamp ascending to satisfy cloudwatchlogs requiremment
	sort.Slice(logEvents, func(i, j int) bool {
		return *logEvents[i].Timestamp < *logEvents[j].Timestamp
	})

	// send entire batch to cloudwatch logs
	_, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(efi.LogGroupName),
		LogStreamName: aws.String(efi.LogStreamName),
		LogEvents:     logEvents,
	})
	if err != nil {
		return err
	}

	return nil
}
