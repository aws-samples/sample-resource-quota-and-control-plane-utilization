package factory_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFactory(t *testing.T) {
	// Test cases for NewFactory
	tests := []struct {
		name    string
		log     logger.Logger
		wantErr bool
	}{
		{
			name:    "with nil logger",
			log:     nil,
			wantErr: false,
		},
		{
			name:    "with valid logger",
			log:     logger.Get(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f, err := factory.NewFactory(ctx, tt.log)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, f)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, f)
			}
		})
	}
}

func TestClientCreation(t *testing.T) {
	// Setup factory for all tests
	ctx := context.Background()
	f, err := factory.NewFactory(ctx, logger.Get())
	require.NoError(t, err)
	require.NotNil(t, f)

	// Valid and invalid regions for testing
	validRegion := "us-west-2"
	invalidRegion := "invalid-region"

	// Table-driven tests for all client creation methods
	tests := []struct {
		name       string
		region     string
		clientFunc func(string) (interface{}, error)
		wantErr    bool
	}{
		{
			name:   "CreateEC2 with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateEC2(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateEC2 with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateEC2(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
		{
			name:   "CreateEFS with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateEFS(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateEFS with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateEFS(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
		{
			name:   "CreateEKS with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateEKS(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateEKS with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateEKS(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
		{
			name:   "CreateELBV2 with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateELBV2(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateELBV2 with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateELBV2(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
		{
			name:   "CreateIAM with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateIAM(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateIAM with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateIAM(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
		{
			name:   "CreateServiceQuotas with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateServiceQuotas(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateServiceQuotas with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateServiceQuotas(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
		{
			name:   "CreateSupport with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateSupport(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateSupport with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateSupport(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
		{
			name:   "CreateCloudWatchLogs with valid region",
			region: validRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateCloudWatchLogs(region)
			},
			wantErr: false,
		},
		{
			name:   "CreateCloudWatchLogs with invalid region",
			region: invalidRegion,
			clientFunc: func(region string) (interface{}, error) {
				return f.CreateCloudWatchLogs(region)
			},
			wantErr: true, // Now returns error for invalid region
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := tt.clientFunc(tt.region)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

// TestFactoryWithCustomConfig tests factory creation with custom AWS config options
func TestFactoryWithCustomConfig(t *testing.T) {
	ctx := context.Background()

	// Create factory with custom option
	f, err := factory.NewFactory(ctx, logger.Get(), func(o *config.LoadOptions) error {
		o.Region = "us-east-1"
		return nil
	})

	assert.NoError(t, err)
	assert.NotNil(t, f)

	// Test that a client can be created with the factory
	ec2Client, err := f.CreateEC2("us-east-1")
	assert.NoError(t, err)
	assert.NotNil(t, ec2Client)
	assert.Equal(t, "us-east-1", ec2Client.GetRegion())
}

// TestErrorHandling tests the error handling functionality
func TestErrorHandling(t *testing.T) {
	ctx := context.Background()
	f, err := factory.NewFactory(ctx, logger.Get())
	require.NoError(t, err)
	require.NotNil(t, f)

	// Test with invalid region
	invalidRegion := "invalid-region"

	// Test error message format
	_, err = f.CreateEC2(invalidRegion)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid region")
	assert.Contains(t, err.Error(), invalidRegion)

	// Test that all client creation methods return errors for invalid regions
	_, err = f.CreateEFS(invalidRegion)
	assert.Error(t, err)

	_, err = f.CreateEKS(invalidRegion)
	assert.Error(t, err)

	_, err = f.CreateELBV2(invalidRegion)
	assert.Error(t, err)

	_, err = f.CreateIAM(invalidRegion)
	assert.Error(t, err)

	_, err = f.CreateServiceQuotas(invalidRegion)
	assert.Error(t, err)

	_, err = f.CreateSupport(invalidRegion)
	assert.Error(t, err)

	_, err = f.CreateCloudWatchLogs(invalidRegion)
	assert.Error(t, err)
}
