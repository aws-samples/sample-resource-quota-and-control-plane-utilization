package serviceconfig

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ---- MOCKS ----

type mockS3Client struct {
	GetObjectFunc func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return m.GetObjectFunc(ctx, params, optFns...)
}

// put object
func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetRegion() string {
	return ""
}

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

func TestLoadConfigFromS3(t *testing.T) {
	validData := []byte(`{"services":{"ec2":{"quotaMetrics":[{"name":"networkInterfaces"}]}},"regions":["us-east-1"]}`)

	t.Run("successful load", func(t *testing.T) {
		client := &mockS3Client{
			GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader(validData)),
				}, nil
			},
		}

		cfg, err := LoadConfigFromS3("bucket", "key", client)
		if err != nil || cfg == nil {
			t.Errorf("expected success, got error: %v", err)
		}
	})

	t.Run("GetObject error", func(t *testing.T) {
		client := &mockS3Client{
			GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return nil, errors.New("s3 error")
			},
		}

		_, err := LoadConfigFromS3("bucket", "key", client)
		if err == nil {
			t.Errorf("expected error on GetObject failure")
		}
	})

	t.Run("Body Read error", func(t *testing.T) {
		client := &mockS3Client{
			GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: &errorReader{},
				}, nil
			},
		}

		_, err := LoadConfigFromS3("bucket", "key", client)
		if err == nil {
			t.Errorf("expected error on body read failure")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		client := &mockS3Client{
			GetObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(bytes.NewReader([]byte("{invalid json"))),
				}, nil
			},
		}

		_, err := LoadConfigFromS3("bucket", "key", client)
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
			name:      "valid IAM",
			validate:  ValidateIAMQuotaMetrics,
			input:     ServiceConfig{QuotaMetrics: []QuotaMetric{{Name: "iamRoles"}}},
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
		{
			name:      "valid STS",
			validate:  ValidateSTSRateLimitApis,
			input:     ServiceConfig{RateLimitAPIs: []RateLimitAPIs{{Name: "assumeRole"}}},
			wantError: false,
		},
		{
			name:      "invalid STS",
			validate:  ValidateSTSRateLimitApis,
			input:     ServiceConfig{RateLimitAPIs: []RateLimitAPIs{{Name: "wrong"}}},
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

func TestValidateRateLimitConfig(t *testing.T) {
	t.Run("valid STS", func(t *testing.T) {
		cfg := TopLevelServiceConfig{
			Services: map[string]ServiceConfig{
				"sts": {RateLimitAPIs: []RateLimitAPIs{{Name: "assumeRole"}}},
			},
		}
		err := ValidateRateLimitConfig(cfg, nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("invalid STS", func(t *testing.T) {
		cfg := TopLevelServiceConfig{
			Services: map[string]ServiceConfig{
				"sts": {RateLimitAPIs: []RateLimitAPIs{{Name: "invalid"}}},
			},
		}
		err := ValidateRateLimitConfig(cfg, nil)
		if err == nil {
			t.Errorf("expected error")
		}
	})

	t.Run("ignore unknown service", func(t *testing.T) {
		cfg := TopLevelServiceConfig{
			Services: map[string]ServiceConfig{
				"unknown": {},
			},
		}
		err := ValidateRateLimitConfig(cfg, nil)
		if err != nil {
			t.Errorf("expected no error for unknown service")
		}
	})
}

func TestValidateQuotaMetricConfig(t *testing.T) {
	t.Run("valid ec2 config", func(t *testing.T) {
		cfg := TopLevelServiceConfig{
			Services: map[string]ServiceConfig{
				"ec2": {QuotaMetrics: []QuotaMetric{{Name: "networkInterfaces"}}},
			},
		}
		err := ValidateQuotaMetricConfig(cfg, nil)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("invalid ec2 config", func(t *testing.T) {
		cfg := TopLevelServiceConfig{
			Services: map[string]ServiceConfig{
				"ec2": {QuotaMetrics: []QuotaMetric{{Name: "wrong"}}},
			},
		}
		err := ValidateQuotaMetricConfig(cfg, nil)
		if err == nil {
			t.Errorf("expected error")
		}
	})

	t.Run("ignore unknown service", func(t *testing.T) {
		cfg := TopLevelServiceConfig{
			Services: map[string]ServiceConfig{
				"unknown": {},
			},
		}
		err := ValidateQuotaMetricConfig(cfg, nil)
		if err != nil {
			t.Errorf("expected no error for unknown service")
		}
	})
}
