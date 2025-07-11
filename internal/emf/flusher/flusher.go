// Package flusher provides functionality for sending EMF records to CloudWatch Logs.
// It handles batching, sorting, and transmission of EMF documents for metric ingestion.
package flusher

import (
	"context"
	"fmt"
	"html"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// EMFFlusher defines the interface for sending batches of EMF records
// to CloudWatch Logs for automatic metric extraction.
type EMFFlusher interface {
	Flush(ctx context.Context, region string, batch []builder.EMFRecord) error
}

// EMFFlusherConfig holds all dependencies and configuration needed
// to create and configure an EMF flusher instance.
type EMFFlusherConfig struct {
	CwlClientMap  *safemap.TypedMap[cwlclient.CloudWatchLogsClient]
	LogGroupName  string
	LogStreamName string
	Logger        logger.Logger
}

// EMFFlusherImpl is the standard implementation of EMFFlusher that
// sends EMF records to CloudWatch Logs using the AWS SDK.
type EMFFlusherImpl struct {
	cfg EMFFlusherConfig
}

// NewEMFFlusher constructs a new EMFFlusher instance with the provided
// configuration and dependencies.
func NewEMFFlusher(cfg EMFFlusherConfig) EMFFlusher {
	return &EMFFlusherImpl{cfg: cfg}
}

// Flush sends a batch of EMF records to CloudWatch Logs for the specified region,
// sorting by timestamp and handling the log event creation and transmission.
func (f *EMFFlusherImpl) Flush(ctx context.Context, region string, batch []builder.EMFRecord) error {
	if len(batch) == 0 {
		f.cfg.Logger.Info("no records to flush for region %s", region)
		return nil
	}
	client, ok := f.cfg.CwlClientMap.Load(region)
	if !ok {
		return fmt.Errorf("no client for region %s", region)
	}

	// build CW Log events
	events := make([]cwlTypes.InputLogEvent, len(batch))
	for i, rec := range batch {
		events[i] = cwlTypes.InputLogEvent{
			Timestamp: aws.Int64(rec.TimeStamp.UnixMilli()),
			Message:   aws.String(string(rec.Payload)),
		}
	}

	// sort events by timestamp as required by CloudWatch Logs
	sort.Slice(events, func(i, j int) bool {
		return *events[i].Timestamp < *events[j].Timestamp
	})

	// send
	_, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(f.cfg.LogGroupName),
		LogStreamName: aws.String(f.cfg.LogStreamName),
		LogEvents:     events,
	})
	if err != nil {
		f.cfg.Logger.Error("flush error: %v", err)
		return err
	}
	f.cfg.Logger.Info("flushed %d records to %s/%s in %s", len(events), f.cfg.LogGroupName, f.cfg.LogStreamName, region)
	return nil
}

// MakeFlushFunc creates a generic flush function for any type T that can be converted
// to log events with custom payload and timestamp extraction functions.
func MakeFlushFunc[T any](
	client cwlclient.CloudWatchLogsClient,
	logGroup, logStream string,
	extractPayload func(T) []byte,
	extractTimestamp func(T) int64,
	logger logger.Logger,
) func(ctx context.Context, batch []T) error {
	return func(ctx context.Context, batch []T) error {
		if len(batch) == 0 {
			return nil
		}
		events := make([]cwlTypes.InputLogEvent, len(batch))
		for i, rec := range batch {
			events[i] = cwlTypes.InputLogEvent{
				Message:   aws.String(html.EscapeString(string(extractPayload(rec)))), // import "html"
				Timestamp: aws.Int64(extractTimestamp(rec)),
			}
		}
		sort.Slice(events, func(i, j int) bool {
			return *events[i].Timestamp < *events[j].Timestamp
		})
		_, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(logStream),
			LogEvents:     events,
		})
		if err != nil {
			logger.Error("emf flusher: error flushing batch: %v", err)
			return fmt.Errorf("emf flusher: %w", err)
		}
		logger.Debug("emf flusher: flushed batch of %d to %s/%s", len(batch), logGroup, logStream)
		return nil
	}
}
