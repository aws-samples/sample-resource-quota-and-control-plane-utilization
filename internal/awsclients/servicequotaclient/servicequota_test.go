package servicequotaclient

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/stretchr/testify/assert"
)

// TestNewServiceQuotasClient tests the NewServiceQuotasClient function
func TestNewServiceQuotasClient(t *testing.T) {
	tests := []struct {
		name        string
		region      string
		expectError bool
	}{
		{
			name:        "Valid region",
			region:      "us-east-1",
			expectError: false,
		},
		{
			name:        "Invalid region",
			region:      "invalid-region",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := servicequotas.NewFromConfig(aws.Config{})
			serviceQuotaClient, err := NewServiceQuotaClient(client, tt.region)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, serviceQuotaClient)
				assert.Contains(t, err.Error(), "invalid region")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, serviceQuotaClient)
				assert.Equal(t, tt.region, serviceQuotaClient.GetRegion())
			}
		})
	}
}

// TestServiceQuotasImpl_GetRegion tests the GetRegion method
func TestServiceQuotasImpl_GetRegion(t *testing.T) {
	region := "us-west-2"
	client := &serviceQuotasImpl{
		region: region,
	}

	assert.Equal(t, region, client.GetRegion())
}

// TestServiceQuotasImpl_GetServiceQuota tests the GetServiceQuota method
func TestServiceQuotasImpl_GetServiceQuota(t *testing.T) {
	serviceQuotaClient := servicequotas.NewFromConfig(aws.Config{})
	client := serviceQuotasImpl{
		client: serviceQuotaClient,
	}
	ctx := context.Background()
	input := &servicequotas.GetServiceQuotaInput{}

	output, err := client.GetServiceQuota(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}
