package types

import (
	"errors"
	"fmt"
	"time"
)

// MetricUnit represents CloudWatch metric units.
type MetricUnit string

const (
	UnitCount   MetricUnit = "Count"
	UnitPercent MetricUnit = "Percent"
)

var ErrInvalidMetricUnit = errors.New("invalid metric unit")

// String implements fmt.Stringer interface.
func (u MetricUnit) String() string {
	if u == "" {
		return string(UnitCount)
	}
	return string(u)
}

// Validate checks if the metric unit is valid.
func (u MetricUnit) Validate() error {
	switch u {
	case UnitCount, UnitPercent:
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidMetricUnit, u)
}

// CloudWatchMetric represents a metric data point to be sent to CloudWatch,
// including value, unit, timestamp, and associated metadata.
type CloudWatchMetric struct {
	Name      JobName           `json:"name"`      // Metric name (strongly typed)
	Value     float64           `json:"value"`     // Metric value
	Unit      MetricUnit        `json:"unit"`      // Metric unit
	Timestamp time.Time         `json:"timestamp"` // Metric timestamp
	Metadata  map[string]string `json:"metadata"`  // Additional metric dimensions and metadata
}

// NewCloudWatchMetric creates a new CloudWatch metric with validation.
func NewCloudWatchMetric(name JobName, value float64, unit MetricUnit) (*CloudWatchMetric, error) {
	if err := unit.Validate(); err != nil {
		return nil, err
	}
	if err := name.Validate(); err != nil {
		return nil, err
	}
	return &CloudWatchMetric{
		Name:      name,
		Value:     value,
		Unit:      unit,
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}, nil
}