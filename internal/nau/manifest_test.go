package nau

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/stretchr/testify/assert"
)

// Mock S3 client for testing
type mockS3Client struct {
	putObjectCalled bool
	putObjectError  error
	bucket          string
	key             string
}

func (m *mockS3Client) PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.putObjectCalled = true
	if input.Bucket != nil {
		m.bucket = *input.Bucket
	}
	if input.Key != nil {
		m.key = *input.Key
	}
	return &s3.PutObjectOutput{}, m.putObjectError
}

func (m *mockS3Client) GetRegion() string {
	return "us-east-1"
}

func TestNewManifest(t *testing.T) {
	ctx := context.Background()
	client := &mockS3Client{}
	log := &logger.NoopLogger{}

	manifest := NewManifest(ctx, "test-bucket", "test-key", client, nil, log)
	assert.NotNil(t, manifest, "Manifest should be created")
}

func TestManifest_WriteRecord(t *testing.T) {
	ctx := context.Background()
	client := &mockS3Client{}
	log := &logger.NoopLogger{}

	manifest := NewManifest(ctx, "test-bucket", "test-key", client, nil, log)

	// NewManifest automatically sets header, so WriteRecord should work
	record := &ResourceMetadata{Id: "test-id"}
	err := manifest.WriteRecord(record)
	assert.NoError(t, err, "Should succeed with auto-set header")

	// Write another record
	record2 := &ResourceMetadata{Id: "test-id-2", Region: "us-west-2"}
	err = manifest.WriteRecord(record2)
	assert.NoError(t, err, "WriteRecord should succeed for second record")
}

func TestManifest_Finalize(t *testing.T) {
	ctx := context.Background()
	client := &mockS3Client{}
	log := &logger.NoopLogger{}

	manifest := NewManifest(ctx, "test-bucket", "test-key", client, nil, log)

	// Write header and record
	// Header is automatically set in NewManifest
	record := &ResourceMetadata{Id: "test-id", Region: "us-east-1"}
	manifest.WriteRecord(record)

	// Finalize should upload to S3
	err := manifest.Finalize()
	assert.NoError(t, err, "Finalize should succeed")
	assert.True(t, client.putObjectCalled, "PutObject should be called")
	assert.Equal(t, "test-bucket", client.bucket, "Bucket should match")
	assert.Equal(t, "test-key", client.key, "Key should match")

	// Second finalize should be no-op
	client.putObjectCalled = false
	err = manifest.Finalize()
	assert.NoError(t, err, "Second finalize should succeed")
	assert.False(t, client.putObjectCalled, "PutObject should not be called again")
}

func TestManifest_FinalizeError(t *testing.T) {
	ctx := context.Background()
	client := &mockS3Client{putObjectError: errors.New("s3 error")}
	log := &logger.NoopLogger{}

	var capturedError error
	errorHandler := func(err error) {
		capturedError = err
	}

	manifest := NewManifest(ctx, "test-bucket", "test-key", client, errorHandler, log)
	// Header is automatically set in NewManifest

	err := manifest.Finalize()
	assert.Error(t, err, "Finalize should fail")
	assert.Equal(t, "s3 error", err.Error(), "Should return S3 error")
	assert.NotNil(t, capturedError, "Error handler should be called")
}

func TestManifest_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &mockS3Client{}
	log := &logger.NoopLogger{}

	manifest := NewManifest(ctx, "test-bucket", "test-key", client, nil, log)
	
	// Cancel context
	cancel()
	
	// WriteRecord should immediately return ErrManifestClosed due to fail-fast check
	record := &ResourceMetadata{Id: "test-id"}
	err := manifest.WriteRecord(record)
	assert.Equal(t, ErrManifestClosed, err, "Should return ErrManifestClosed when context cancelled")
}

func TestManifest_DrainRecords(t *testing.T) {
	ctx := context.Background()
	client := &mockS3Client{}
	log := &logger.NoopLogger{}

	manifest := NewManifest(ctx, "test-bucket", "test-key", client, nil, log)
	
	// Add multiple records quickly
	for i := 0; i < 5; i++ {
		record := &ResourceMetadata{Id: fmt.Sprintf("test-id-%d", i)}
		manifest.WriteRecord(record)
	}
	
	// Finalize should drain all records via drainRecords
	err := manifest.Finalize()
	assert.NoError(t, err, "Finalize should succeed")
	assert.True(t, client.putObjectCalled, "PutObject should be called")
}

func TestGenerateManifestKey(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC)

	tests := []struct {
		name     string
		prefix   string
		filename string
		time     time.Time
		expected string
	}{
		{
			name:     "standard key",
			prefix:   "nau-reports",
			filename: "vpc-nau",
			time:     testTime,
			expected: "nau-reports/2024-01-15/vpc-nau_2024-01-15_14-30-45.csv",
		},
		{
			name:     "empty filename uses default",
			prefix:   "reports",
			filename: "",
			time:     testTime,
			expected: "reports/2024-01-15/manifest_2024-01-15_14-30-45.csv",
		},
		{
			name:     "zero time uses current time",
			prefix:   "test",
			filename: "test",
			time:     time.Time{},
			expected: "", // We'll check the format separately since time.Now() varies
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateManifestKey(tt.prefix, tt.filename, tt.time)
			
			if tt.name == "zero time uses current time" {
				// Just check the format is correct
				assert.Contains(t, result, "test/", "Should contain prefix")
				assert.Contains(t, result, "/test_", "Should contain filename")
				assert.Contains(t, result, ".csv", "Should end with .csv")
			} else {
				assert.Equal(t, tt.expected, result, "Generated key should match expected")
			}
		})
	}
}

