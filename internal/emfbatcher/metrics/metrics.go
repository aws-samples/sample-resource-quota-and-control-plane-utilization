package metricemfbatcher

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emfbatcher"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/batchprocessor"
	applogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// buildEMFRecord turns a CloudWatchMetric into an EMFRecord,
// embedding metadata as dimensions and fields.
func buildEMFRecord(m sharedtypes.CloudWatchMetric, namespace string) (sharedtypes.EMFRecord, error) {

	ts := m.Timestamp.UnixMilli()

	// sort metadata keys to produce deterministic dimension order
	var dimKeys []string
	for k := range m.Metadata {
		dimKeys = append(dimKeys, k)
	}
	sort.Strings(dimKeys)

	// build dimensions: a single dimension group listing all metadata keys
	dimensions := [][]string{dimKeys}

	// assemble payload map
	emf := map[string]any{
		"_aws": map[string]any{
			"Timestamp": ts,
			"CloudWatchMetrics": []map[string]any{{
				"Namespace":  namespace,
				"Dimensions": dimensions,
				"Metrics":    []map[string]any{{"Name": m.Name, "Unit": m.Unit}},
			}},
		},
		// metric value
		m.Name: m.Value,
	}
	// include metadata fields alongside metric
	for _, k := range dimKeys {
		emf[k] = m.Metadata[k]
	}

	payload, err := json.Marshal(emf)
	if err != nil {
		return sharedtypes.EMFRecord{}, err
	}
	return sharedtypes.EMFRecord{Payload: payload, Timestamp: ts}, nil
}

// CloudWatchMetricBatcher is a processor for CloudWatchMetric→EMFRecord.
type CloudWatchMetricBatcher struct {
	Batcher *batchprocessor.GenericBatchProcessor[sharedtypes.CloudWatchMetric, sharedtypes.EMFRecord]
}

// NewCloudWatchMetricBatcher sets up the batching, mapping & flushing.
func NewCloudWatchMetricBatcher(
	ctx context.Context,
	client cwlclient.CloudWatchLogsClient,
	namespace, logGroup, logStream string,
	maxEvents, maxBytes int,
	flushInterval time.Duration,
	overhead int,
	logger applogger.Logger,
) (*CloudWatchMetricBatcher, error) {
	if logger == nil {
		logger = &applogger.NoopLogger{}
	}

	// Map CloudWatchMetric → EMFRecord
	mapFn := func(m sharedtypes.CloudWatchMetric) (sharedtypes.EMFRecord, error) {
		return buildEMFRecord(m, namespace)
	}

	// Reuse shared flush logic (EMF→CloudWatch Logs)
	flushFn := emfbatcher.MakeFlushFunc(
		client, logGroup, logStream,
		func(r sharedtypes.EMFRecord) []byte { return r.Payload },
		func(r sharedtypes.EMFRecord) int64 { return r.Timestamp },
		logger,
	)

	// Size each EMFRecord as payload+overhead
	sizeFn := func(r sharedtypes.EMFRecord) int {
		return len(r.Payload) + overhead
	}

	// Wire up the generic batch processor
	return &CloudWatchMetricBatcher{
		Batcher: batchprocessor.NewGenericBatchProcessor(
			ctx,
			maxEvents,
			maxBytes,
			flushInterval,
			mapFn,
			flushFn,
			sizeFn,
			logger,
		),
	}, nil
}
