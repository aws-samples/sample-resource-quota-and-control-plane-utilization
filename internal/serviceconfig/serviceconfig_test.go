package serviceconfig

import (
	"errors"
	"os"
	"testing"
)

// ---- TESTS ----

func TestLoadConfigFromFile(t *testing.T) {
	validData := []byte(`{"services":{"ec2":{"quotaMetrics":[{"name":"networkInterfaces"}]}},"regions":["us-east-1"]}`)

	tmpFile, err := os.CreateTemp("", "test_config.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(validData); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	t.Run("successful load", func(t *testing.T) {
		cfg, err := LoadConfigFromFile(tmpFile.Name(), nil)
		if err != nil || cfg == nil {
			t.Errorf("expected success, got error: %v", err)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := LoadConfigFromFile("nonexistent.json", nil)
		if err == nil {
			t.Errorf("expected error for missing file")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		badFile, _ := os.CreateTemp("", "bad.json")
		defer os.Remove(badFile.Name())
		badFile.Write([]byte("{invalid json"))
		badFile.Close()

		_, err := LoadConfigFromFile(badFile.Name(), nil)
		if err == nil {
			t.Errorf("expected error for invalid JSON")
		}
	})
}

func TestValidateQuotaMetric(t *testing.T) {
	tests := []struct {
		name      string
		service   string
		metric    string
		wantError bool
	}{
		{"valid ec2 metric", "ec2", "networkInterfaces", false},
		{"invalid ec2 metric", "ec2", "wrong", true},
		{"valid eks metric", "eks", "listClusters", false},
		{"valid iam metric", "iam", "iamRoles", false},
		{"valid iam metric 2", "iam", "oidcProviders", false},
		{"valid ebs metric", "ebs", "gp3Storage", false},
		{"valid vpc metric", "vpc", "nau", false},
		{"unknown service", "unknown", "metric", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := TopLevelServiceConfig{
				Services: map[string]ServiceConfig{
					tt.service: {QuotaMetrics: []QuotaMetric{{Name: tt.metric}}},
				},
			}
			err := ValidateQuotaMetricConfig(cfg, nil)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateQuotaMetricConfig() error = %v, wantError %v", err, tt.wantError)
			}
			if err != nil && tt.wantError {
				if tt.service == "unknown" && !errors.Is(err, ErrUnsupportedService) {
					t.Errorf("Expected ErrUnsupportedService, got %v", err)
				} else if tt.service != "unknown" && !errors.Is(err, ErrInvalidQuotaMetric) {
					t.Errorf("Expected ErrInvalidQuotaMetric, got %v", err)
				}
			}
		})
	}
}

func TestValidateQuotaMetricConfig(t *testing.T) {
	tests := []struct {
		name      string
		services  map[string]ServiceConfig
		wantError bool
	}{
		{
			name: "valid ec2 config",
			services: map[string]ServiceConfig{
				"ec2": {QuotaMetrics: []QuotaMetric{{Name: "networkInterfaces"}}},
			},
			wantError: false,
		},
		{
			name: "valid eks config",
			services: map[string]ServiceConfig{
				"eks": {QuotaMetrics: []QuotaMetric{{Name: "listClusters"}}},
			},
			wantError: false,
		},
		{
			name: "valid iam config",
			services: map[string]ServiceConfig{
				"iam": {QuotaMetrics: []QuotaMetric{{Name: "iamRoles"}}},
			},
			wantError: false,
		},
		{
			name: "valid ebs config",
			services: map[string]ServiceConfig{
				"ebs": {QuotaMetrics: []QuotaMetric{{Name: "gp3storage"}}},
			},
			wantError: false,
		},
		{
			name: "valid vpc config",
			services: map[string]ServiceConfig{
				"vpc": {QuotaMetrics: []QuotaMetric{{Name: "nau"}}},
			},
			wantError: false,
		},
		{
			name: "invalid ec2 config",
			services: map[string]ServiceConfig{
				"ec2": {QuotaMetrics: []QuotaMetric{{Name: "wrong"}}},
			},
			wantError: true,
		},
		{
			name: "unknown service",
			services: map[string]ServiceConfig{
				"unknown": {},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := TopLevelServiceConfig{Services: tt.services}
			err := ValidateQuotaMetricConfig(cfg, nil)
			if (err != nil) != tt.wantError {
				t.Errorf("expected error=%v, got error=%v", tt.wantError, err != nil)
			}
			
			// Check for correct error type
			if err != nil {
				serviceName := ""
				for s := range tt.services {
					serviceName = s
					break
				}
				
				if serviceName == "unknown" && !errors.Is(err, ErrUnsupportedService) {
					t.Errorf("Expected ErrUnsupportedService, got %v", err)
				} else if serviceName != "unknown" && !errors.Is(err, ErrInvalidQuotaMetric) {
					t.Errorf("Expected ErrInvalidQuotaMetric, got %v", err)
				}
			}
		})
	}
}
