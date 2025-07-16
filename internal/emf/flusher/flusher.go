// Package flusher provides functionality for sending EMF records to CloudWatch Logs.
// It handles batching, sorting, and transmission of EMF documents for metric ingestion.
package flusher

import (
	"context"
	"errors"
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
)

const (
	MaxLogEventsPerBatch = 10000
	MaxBatchSizeBytes    = 1048576 // 1MB
	MaxMessageSizeBytes  = 262144  // 256KB
	EventOverheadBytes   = 26      // Overhead per event
)

// Error variables for better error handling and testing
var (
	ErrEmptyBatch         = errors.New("batch is empty")
	ErrClientNotFound     = errors.New("client not found for region")
	ErrInvalidConfig      = errors.New("invalid flusher configuration")
	ErrClientMapNil       = errors.New("client map is nil")
	ErrLogGroupNameEmpty  = errors.New("log group name is empty")
	ErrLogStreamNameEmpty = errors.New("log stream name is empty")
	ErrBatchTooLarge      = errors.New("batch exceeds size limits")
	ErrMessageTooLarge    = errors.New("message exceeds size limit")
	ErrFlushFailed        = errors.New("failed to flush batch to CloudWatch Logs")
)

// EMFFlusher defines the interface for sending batches of EMF records
// to CloudWatch Logs for automatic metric extraction.
type EMFFlusher interface {
	Flush(ctx context.Context, region string, batch []builder.EMFRecord) error
}

// EMFFlusherConfig holds all dependencies and configuration needed
// to create and configure an EMF flusher instance.
type EMFFlusherConfig struct {
	CwlClientMap  safestore.Store[cwlclient.CloudWatchLogsClient]
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
func NewEMFFlusher(cfg EMFFlusherConfig) (EMFFlusher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &EMFFlusherImpl{cfg: cfg}, nil
}

// Validate checks if the EMFFlusherConfig is valid
func (cfg EMFFlusherConfig) Validate() error {
	if cfg.CwlClientMap == nil {
		return ErrClientMapNil
	}
	if strings.TrimSpace(cfg.LogGroupName) == "" {
		return ErrLogGroupNameEmpty
	}
	if strings.TrimSpace(cfg.LogStreamName) == "" {
		return ErrLogStreamNameEmpty
	}
	return nil
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
		return fmt.Errorf("%w: %s", ErrClientNotFound, region)
	}

	// build and validate CW Log events
	events := BuildLogEvents(batch)
	if err := ValidateBatchSize(events); err != nil {
		return err
	}

	// sort events by timestamp as required by CloudWatch Logs
	SortEventsByTimestamp(events)

	// send
	_, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(f.cfg.LogGroupName),
		LogStreamName: aws.String(f.cfg.LogStreamName),
		LogEvents:     events,
	})
	if err != nil {
		// amazonq-ignore-next-line
		f.cfg.Logger.Error("flush error: %v", err)
		// amazonq-ignore-next-line
		return fmt.Errorf("%w: %v", ErrFlushFailed, err)
	}

	f.cfg.Logger.Info("flushed %d records to %s/%s in %s", len(events), f.cfg.LogGroupName, f.cfg.LogStreamName, region)
	return nil
}

// BuildLogEvents converts EMF records to CloudWatch Log events
func BuildLogEvents(batch []builder.EMFRecord) []cwlTypes.InputLogEvent {
	events := make([]cwlTypes.InputLogEvent, len(batch))
	for i, rec := range batch {
		events[i] = cwlTypes.InputLogEvent{
			Timestamp: aws.Int64(rec.TimeStamp.UnixMilli()),
			Message:   aws.String(string(rec.Payload)),
		}
	}
	return events
}

// SortEventsByTimestamp sorts events by timestamp as required by CloudWatch Logs
func SortEventsByTimestamp(events []cwlTypes.InputLogEvent) {
	sort.Slice(events, func(i, j int) bool {
		return aws.ToInt64(events[i].Timestamp) < aws.ToInt64(events[j].Timestamp)
	})
}

// ValidateBatchSize checks if the batch meets CloudWatch Logs requirements
func ValidateBatchSize(events []cwlTypes.InputLogEvent) error {
	if len(events) > MaxLogEventsPerBatch {
		return fmt.Errorf("%w: batch size %d exceeds maximum %d", ErrBatchTooLarge, len(events), MaxLogEventsPerBatch)
	}

	totalSize := 0
	for _, event := range events {
		msgSize := len(aws.ToString(event.Message))
		if msgSize > MaxMessageSizeBytes {
			return fmt.Errorf("%w: message size %d exceeds maximum %d", ErrMessageTooLarge, msgSize, MaxMessageSizeBytes)
		}
		totalSize += msgSize + EventOverheadBytes
	}

	if totalSize > MaxBatchSizeBytes {
		return fmt.Errorf("%w: batch total size %d exceeds maximum %d", ErrBatchTooLarge, totalSize, MaxBatchSizeBytes)
	}

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

		events := BuildGenericLogEvents(batch, extractPayload, extractTimestamp, true)
		if err := ValidateBatchSize(events); err != nil {
			return err
		}

		SortEventsByTimestamp(events)

		_, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(logStream),
			LogEvents:     events,
		})
		if err != nil {
			logger.Error("emf flusher: error flushing batch: %v", err)
			// amazonq-ignore-next-line
			return fmt.Errorf("%w: %v", ErrFlushFailed, err)
		}

		logger.Debug("emf flusher: flushed batch of %d to %s/%s", len(batch), logGroup, logStream)
		return nil
	}
}

// BuildGenericLogEvents converts generic batch items to CloudWatch Log events
func BuildGenericLogEvents[T any](
	batch []T,
	extractPayload func(T) []byte,
	extractTimestamp func(T) int64,
	escapeHTML bool,
) []cwlTypes.InputLogEvent {
	events := make([]cwlTypes.InputLogEvent, len(batch))
	for i, rec := range batch {
		payload := extractPayload(rec)
		message := string(payload)
		if escapeHTML {
			message = html.EscapeString(message)
		}
		events[i] = cwlTypes.InputLogEvent{
			Message:   aws.String(message),
			Timestamp: aws.Int64(extractTimestamp(rec)),
		}
	}
	return events
}
