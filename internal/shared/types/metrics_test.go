package types

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestMetricUnit_String(t *testing.T) {
	tests := []struct {
		unit     MetricUnit
		expected string
	}{
		{UnitCount, "Count"},
		{UnitPercent, "Percent"},
		{"", "Count"}, // Empty should default to Count
	}
	
	for _, tt := range tests {
		if got := tt.unit.String(); got != tt.expected {
			t.Errorf("MetricUnit(%q).String() = %q, want %q", tt.unit, got, tt.expected)
		}
	}
}

func TestMetricUnit_Validate(t *testing.T) {
	tests := []struct {
		unit    MetricUnit
		wantErr bool
	}{
		{UnitCount, false},
		{UnitPercent, false},
		{"Invalid", true},
		{"", true},
	}
	
	for _, tt := range tests {
		err := tt.unit.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("MetricUnit(%q).Validate() error = %v, wantErr %v", tt.unit, err, tt.wantErr)
		}
		if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidMetricUnit) {
			t.Errorf("MetricUnit(%q).Validate() error = %v, want wrapped ErrInvalidMetricUnit", tt.unit, err)
		}
	}
}

func TestMetricUnit_Constants(t *testing.T) {
	if UnitCount != "Count" {
		t.Errorf("UnitCount = %q, want \"Count\"", UnitCount)
	}
	if UnitPercent != "Percent" {
		t.Errorf("UnitPercent = %q, want \"Percent\"", UnitPercent)
	}
}

func TestNewCloudWatchMetric(t *testing.T) {
	tests := []struct {
		name     string
		metName  JobName
		value    float64
		unit     MetricUnit
		wantErr  bool
	}{
		{"valid count", JobNetworkInterfaceUtilization, 42.0, UnitCount, false},
		{"valid percent", JobGP3StorageUtilization, 85.5, UnitPercent, false},
		{"invalid unit", JobIAMRoleUtilization, 10.0, "Invalid", true},
		{"invalid job name", "InvalidJobName", 10.0, UnitCount, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric, err := NewCloudWatchMetric(tt.metName, tt.value, tt.unit)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCloudWatchMetric() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if metric.Name != tt.metName {
					t.Errorf("NewCloudWatchMetric().Name = %q, want %q", metric.Name, tt.metName)
				}
				if metric.Value != tt.value {
					t.Errorf("NewCloudWatchMetric().Value = %f, want %f", metric.Value, tt.value)
				}
				if metric.Unit != tt.unit {
					t.Errorf("NewCloudWatchMetric().Unit = %q, want %q", metric.Unit, tt.unit)
				}
				if metric.Metadata == nil {
					t.Error("NewCloudWatchMetric().Metadata should be initialized")
				}
				if metric.Timestamp.IsZero() {
					t.Error("NewCloudWatchMetric().Timestamp should be set")
				}
			}
		})
	}
}

func TestNewCloudWatchMetric_InvalidUnit(t *testing.T) {
	_, err := NewCloudWatchMetric(JobNetworkInterfaceUtilization, 10.0, "InvalidUnit")
	if err == nil {
		t.Error("NewCloudWatchMetric with invalid unit should return error")
	}
	if !errors.Is(err, ErrInvalidMetricUnit) {
		t.Errorf("NewCloudWatchMetric error = %v, want wrapped ErrInvalidMetricUnit", err)
	}
}

func TestNewCloudWatchMetric_InvalidJobName(t *testing.T) {
	_, err := NewCloudWatchMetric("InvalidJobName", 10.0, UnitCount)
	if err == nil {
		t.Error("NewCloudWatchMetric with invalid job name should return error")
	}
	if !errors.Is(err, ErrInvalidJobName) {
		t.Errorf("NewCloudWatchMetric error = %v, want wrapped ErrInvalidJobName", err)
	}
}

func TestCloudWatchMetric_JSONSerialization(t *testing.T) {
	timestamp := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	metric := CloudWatchMetric{
		Name:      JobNetworkInterfaceUtilization,
		Value:     42.5,
		Unit:      UnitPercent,
		Timestamp: timestamp,
		Metadata:  map[string]string{"key": "value"},
	}
	
	// Marshal
	data, err := json.Marshal(metric)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Unmarshal
	var unmarshaled CloudWatchMetric
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Verify
	if unmarshaled.Name != metric.Name {
		t.Errorf("Unmarshaled Name = %q, want %q", unmarshaled.Name, metric.Name)
	}
	if unmarshaled.Value != metric.Value {
		t.Errorf("Unmarshaled Value = %f, want %f", unmarshaled.Value, metric.Value)
	}
	if unmarshaled.Unit != metric.Unit {
		t.Errorf("Unmarshaled Unit = %q, want %q", unmarshaled.Unit, metric.Unit)
	}
	if !unmarshaled.Timestamp.Equal(metric.Timestamp) {
		t.Errorf("Unmarshaled Timestamp = %v, want %v", unmarshaled.Timestamp, metric.Timestamp)
	}
	if unmarshaled.Metadata["key"] != "value" {
		t.Errorf("Unmarshaled Metadata[\"key\"] = %q, want \"value\"", unmarshaled.Metadata["key"])
	}
}

func TestCloudWatchMetric_DefaultTimestamp(t *testing.T) {
	before := time.Now()
	metric, err := NewCloudWatchMetric(JobNetworkInterfaceUtilization, 10.0, UnitCount)
	after := time.Now()
	
	if err != nil {
		t.Fatalf("NewCloudWatchMetric() error = %v", err)
	}
	
	if metric.Timestamp.Before(before) || metric.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", metric.Timestamp, before, after)
	}
}

func TestNewCloudWatchMetric_EmptyName(t *testing.T) {
	// Constructor should validate name
	_, err := NewCloudWatchMetric("", 10.0, UnitCount)
	if err == nil {
		t.Error("NewCloudWatchMetric with empty name should return error")
	}
	if !errors.Is(err, ErrInvalidJobName) {
		t.Errorf("NewCloudWatchMetric error = %v, want wrapped ErrInvalidJobName", err)
	}
}

func TestNewCloudWatchMetric_ZeroValue(t *testing.T) {
	metric, err := NewCloudWatchMetric(JobNetworkInterfaceUtilization, 0.0, UnitCount)
	if err != nil {
		t.Errorf("NewCloudWatchMetric with zero value should not error, got %v", err)
	}
	if metric.Value != 0.0 {
		t.Errorf("NewCloudWatchMetric().Value = %f, want 0.0", metric.Value)
	}
}

func TestNewCloudWatchMetric_NilMetadata(t *testing.T) {
	metric, err := NewCloudWatchMetric(JobNetworkInterfaceUtilization, 10.0, UnitCount)
	if err != nil {
		t.Fatalf("NewCloudWatchMetric() error = %v", err)
	}
	
	if metric.Metadata == nil {
		t.Error("NewCloudWatchMetric().Metadata should be initialized, got nil")
	}
	
	// Should be able to add to metadata without panic
	metric.Metadata["test"] = "value"
	if metric.Metadata["test"] != "value" {
		t.Error("Should be able to add to initialized metadata")
	}
}