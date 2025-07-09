// internal/emf/batch/cloudtrail/aggregator.go
package cloudtrail

import (
	"context"
	"strings"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/builder"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// AggregateConfig holds parameters for EMF aggregation.
type AggregateConfig struct {
	Namespace  string
	MetricName string
	NormFactor float64   // e.g., 1/flushInterval.Seconds()
	FlushTime  time.Time // timestamp for all aggregated EMFs
	Logger     logger.Logger
}

// EMFAggregator groups raw EMFRecords into ready-to-flush EMFRecords.
type EMFAggregator interface {
	Aggregate(ctx context.Context, raw []builder.EMFRecord, cfg AggregateConfig) ([]builder.EMFRecord, error)
}

// DefaultEMFAggregator is the default implementation of EMFAggregator.
type DefaultEMFAggregator struct{}

// NewDefaultEMFAggregator constructs a DefaultEMFAggregator.
func NewDefaultEMFAggregator() EMFAggregator {
	return &DefaultEMFAggregator{}
}

// Aggregate groups by dimensions, normalizes counts, and builds EMFRecords.
func (a *DefaultEMFAggregator) Aggregate(
	ctx context.Context,
	raw []builder.EMFRecord,
	cfg AggregateConfig,
) ([]builder.EMFRecord, error) {
	// group counts by dimension key
	log := cfg.Logger
	log.Info("starting EMF record aggregation")
	groups := make(map[string]struct {
		dims  [][]string
		count float64
	})
	for _, rec := range raw {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		log.Debug("emf record (pre-aggregated): %s", string(rec.Payload))
		key := dimensionKey(rec.Dimensions)
		g := groups[key]
		g.dims = rec.Dimensions
		g.count += 1
		groups[key] = g
		log.Debug("dimesion-set=%s count=%.2f", g.dims, g.count)
	}

	// build aggregated EMFRecords
	var out []builder.EMFRecord
	for key, g := range groups {
		normalized := g.count * cfg.NormFactor
		emfRec, err := builder.Build(builder.EMFInput{
			Namespace:  cfg.Namespace,
			MetricName: cfg.MetricName,
			Value:      normalized,
			Unit:       builder.MetricUnitCount,
			Dimensions: g.dims,
			Timestamp:  cfg.FlushTime,
		}, cfg.Logger)
		if err != nil {
			cfg.Logger.Error("aggregator: failed build for %s: %v", key, err)
			continue
		}
		cfg.Logger.Info("aggregator: built EMFRecord: dimension=%s, payload=%s", emfRec.Dimensions, string(emfRec.Payload))
		out = append(out, emfRec)
	}
	return out, nil
}

// dimensionKey returns a stable string key for the given dimension set.
func dimensionKey(dims [][]string) string {
	parts := make([]string, len(dims))
	for i, kv := range dims {
		parts[i] = kv[0] + "=" + kv[1]
	}
	// sort if needed for stability
	// sort.Strings(parts)
	return strings.Join(parts, "|")
}
