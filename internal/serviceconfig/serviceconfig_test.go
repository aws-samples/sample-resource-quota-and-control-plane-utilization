package serviceconfig

import (
	"errors"
	"os"
	"testing"
)

type errorReader struct{}

func (e *errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("read error")
}

func (e *errorReader) Close() error {
	return nil
}

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

func TestValidateFunctions(t *testing.T) {
	tests := []struct {
		name      string
		validate  func(ServiceConfig) error
		input     ServiceConfig
		wantError bool
	}{
		{
			name:      "valid EC2",
			validate:  ValidateEC2QuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "networkInterfaces"}}},
			wantError: false,
		},
		{
			name:      "invalid EC2",
			validate:  ValidateEC2QuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "wrong"}}},
			wantError: true,
		},
		{
			name:      "valid EKS",
			validate:  ValidateEKSQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "listClusters"}}},
			wantError: false,
		},
		{
			name:      "invalid EKS",
			validate:  ValidateEKSQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "wrong"}}},
			wantError: true,
		},
		{
			name:      "valid IAM iamRoles",
			validate:  ValidateIAMQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "iamRoles"}}},
			wantError: false,
		},
		{
			name:      "valid IAM oidcProviders",
			validate:  ValidateIAMQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "oidcProviders"}}},
			wantError: false,
		},
		{
			name:      "invalid IAM",
			validate:  ValidateIAMQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "wrong"}}},
			wantError: true,
		},
		{
			name:      "valid EBS",
			validate:  ValidateEBSQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "gp3storage"}}},
			wantError: false,
		},
		{
			name:      "invalid EBS",
			validate:  ValidateEBSQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "wrong"}}},
			wantError: true,
		},
		{
			name:      "valid VPC",
			validate:  ValidateVPCQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "nau"}}},
			wantError: false,
		},
		{
			name:      "invalid VPC",
			validate:  ValidateVPCQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "wrong"}}},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate(tt.input)
			if (err != nil) != tt.wantError {
				t.Errorf("expected error=%v, got error=%v", tt.wantError, err != nil)
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
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := TopLevelServiceConfig{Services: tt.services}
			err := ValidateQuotaMetricConfig(cfg, nil)
			if (err != nil) != tt.wantError {
				t.Errorf("expected error=%v, got error=%v", tt.wantError, err != nil)
			}
		})
	}
}
