package cloudtrailemfbatcher

import (
	"context"
	"encoding/json"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emfbatcher"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/batchprocessor"
	applogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (
	pkgPrefix = "cloudtrail: "
)

type CloudTrailEventEMFBatcher struct {
	Batcher *batchprocessor.GenericBatchProcessor[sharedtypes.CloudTrailEvent, sharedtypes.EMFRecord]
}

// NewEMFBatcher constructs a batch processor that:
// - maps CloudTrailEvent → EMFRecord,
// - batches by record count, byte size, or time interval,
// - flushes EMFRecords to CloudWatch Logs.
func NewCloudTrailEMFBatcher(
	ctx context.Context,
	maxBytes int,
	maxEvents int,
	flushInterval time.Duration,
	overhead int,
	client cwlclient.CloudWatchLogsClient,
	namespace,
	logGroup string,
	logStream string,
	logger applogger.Logger,
	opts ...batchprocessor.GenericOption[sharedtypes.CloudTrailEvent, sharedtypes.EMFRecord],
) (*CloudTrailEventEMFBatcher, error) {
	// check if logger is nil
	// if so add the noop logger
	if logger == nil {
		logger = &applogger.NoopLogger{}
	}
	// MapFunc: CloudTrailEvent → EMFRecord
	mapFn := func(ev sharedtypes.CloudTrailEvent) (sharedtypes.EMFRecord, error) {
		now := ev.EventTime.UnixMilli()
		emf := map[string]any{
			"_aws": map[string]any{
				"Timestamp": now,
				"CloudWatchMetrics": []map[string]any{{
					"Namespace":  namespace,
					"Dimensions": [][]string{{"eventName"}},
					"Metrics":    []map[string]any{{"Name": "CallCount", "Unit": "Count"}},
				}},
			},
			"eventName": ev.EventName,
			"CallCount": 1.0,
		}
		data, err := json.Marshal(emf)
		if err != nil {
			return sharedtypes.EMFRecord{}, err
		}
		return sharedtypes.EMFRecord{Payload: data, Timestamp: now}, nil
	}

	// FlushFunc: send a batch of EMFRecords to CloudWatch Logs
	flushFn := emfbatcher.MakeFlushFunc(client, logGroup, logStream,
		func(r sharedtypes.EMFRecord) []byte { return r.Payload },
		func(r sharedtypes.EMFRecord) int64 { return r.Timestamp },
		logger,
	)

	// ItemSizer: size of one EMFRecord = payload bytes + overhead
	sizeFn := func(r sharedtypes.EMFRecord) int {
		logger.Debug(pkgPrefix+"calculating EMF record size; EMFRecord=%v, size=%v",
			r, len(r.Payload)+overhead)
		return len(r.Payload) + overhead
	}

	// Construct generic processor
	b := batchprocessor.NewGenericBatchProcessor(
		ctx,
		maxEvents,
		maxBytes,
		flushInterval,
		mapFn,
		flushFn,
		sizeFn,
		logger,
		opts...,
	)
	logger.Info("%s new batch processor", pkgPrefix)
	// Return batcher
	return &CloudTrailEventEMFBatcher{
		Batcher: b,
	}, nil
}
