package emfbatcher

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

const (
	pkgPrefix = "emfbatcher: "
)

// MakeFlushFunc returns a FlushFunc for any record type T, given two extractors.
// - extractPayload: return the EMF JSON bytes.
// - extractTimestamp: return the event timestamp in ms.
// The resulting function will batch up to 10 000 events or 1 MiB total (enforced elsewhere).
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

		// 1) Build the slice of InputLogEvents
		events := make([]cwlTypes.InputLogEvent, len(batch))
		for i, rec := range batch {
			events[i] = cwlTypes.InputLogEvent{
				Message:   aws.String(string(extractPayload(rec))),
				Timestamp: aws.Int64(extractTimestamp(rec)),
			}
		}

		// 2) Sort them by Timestamp ascending to satisfy CloudWatch Logs requirement
		sort.Slice(events, func(i, j int) bool {
			return *events[i].Timestamp < *events[j].Timestamp
		})

		// 3) Ship them to CloudWatch Logs
		_, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(logStream),
			LogEvents:     events,
		})
		if err != nil {
			logger.Error(pkgPrefix+"error flushing batch: %v", err)
			return fmt.Errorf(pkgPrefix+"error flushing batch: %w", err)
		}

		logger.Debug(pkgPrefix+"flushed batch "+"batchSize : %v , logGroup : %v , logStream : %v", len(batch), logGroup, logStream)
		return nil
	}
}
