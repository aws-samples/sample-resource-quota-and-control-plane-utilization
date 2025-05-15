package supportclient

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/support"
	"github.com/stretchr/testify/assert"
)

// TestSupportClient tests the NewSupportClient function
func TestSupportClient(t *testing.T) {
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
			client := support.NewFromConfig(aws.Config{})
			supportClient, err := NewSupportClient(client, tt.region)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, supportClient)
				assert.Contains(t, err.Error(), "invalid region")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, supportClient)
				assert.Equal(t, tt.region, supportClient.GetRegion())
			}
		})
	}
}

// TestSupportClient_GetRegion tests the GetRegion method
func TestSupportClient_GetRegion(t *testing.T) {
	region := "us-west-2"
	client := &supportClientImpl{
		region: region,
	}

	assert.Equal(t, region, client.GetRegion())
}

// TestSupportClient_RefreshTrustedAdvisorCheck tests the RefreshTrustedAdvisorCheck method
func TestSupportClient_RefreshTrustedAdvisorCheck(t *testing.T) {
	supportClient := support.NewFromConfig(aws.Config{})
	client := supportClientImpl{
		client: supportClient,
	}
	ctx := context.Background()
	input := &support.RefreshTrustedAdvisorCheckInput{}

	output, err := client.RefreshTrustedAdvisorCheck(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}
