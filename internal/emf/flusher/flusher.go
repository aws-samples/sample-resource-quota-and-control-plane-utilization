package flusher

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// EMFFlusher sends batches of EMFRecords to CloudWatch Logs.
type EMFFlusher interface {
	Flush(ctx context.Context, region string, batch []builder.EMFRecord) error
}

// EMFFlusherConfig holds dependencies for the flusher.
type EMFFlusherConfig struct {
	CwlClientMap  *safemap.TypedMap[cwlclient.CloudWatchLogsClient]
	LogGroupName  string
	LogStreamName string
	Logger        logger.Logger
}

// EMFFlusherImpl is the standard implementation.
type EMFFlusherImpl struct {
	cfg EMFFlusherConfig
}

// NewEMFFlusher constructs an EMFFlusher from config.
func NewEMFFlusher(cfg EMFFlusherConfig) EMFFlusher {
	return &EMFFlusherImpl{cfg: cfg}
}

// Flush implements the EMFFlusher interface.
func (e *EMFFlusherImpl) Flush(ctx context.Context, region string, batch []builder.EMFRecord) error {
	if len(batch) == 0 {
		e.cfg.Logger.Info("no records to flush for region %s", region)
		return nil
	}
	// sort by timestamp
	sort.Slice(batch, func(i, j int) bool {
		return batch[i].TimeStamp.Before(batch[j].TimeStamp)
	})

	client, ok := e.cfg.CwlClientMap.Load(region)
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

	// send
	_, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(e.cfg.LogGroupName),
		LogStreamName: aws.String(e.cfg.LogStreamName),
		LogEvents:     events,
	})
	if err != nil {
		e.cfg.Logger.Error("flush error: %v", err)
		return err
	}
	e.cfg.Logger.Info("flushed %d records to %s/%s in %s", len(events), e.cfg.LogGroupName, e.cfg.LogStreamName, region)
	return nil
}

// MakeFlushFunc is a generic helper for any T→[]byte, timestamp extractor.
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
				Message:   aws.String(string(extractPayload(rec))),
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
