package eksclient

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/stretchr/testify/assert"
)

// TestNewEksClient tests the NewEksClient function
func TestNewEksClient(t *testing.T) {
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
			client := eks.NewFromConfig(aws.Config{})
			eksClient, err := NewEKSClient(client, tt.region)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, eksClient)
				assert.Contains(t, err.Error(), "invalid region")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, eksClient)
				assert.Equal(t, tt.region, eksClient.GetRegion())
			}
		})
	}
}

// TestEksClientImpl_GetRegion tests the GetRegion method
func TestEksClientImpl_GetRegion(t *testing.T) {
	region := "us-west-2"
	client := &eksClientimpl{
		region: region,
	}

	assert.Equal(t, region, client.GetRegion())
}

// TestEkslientImpl_ListClusters tests the ListClusters method
func TestEkslientImpl_ListClusters(t *testing.T) {
	eksClient := eks.NewFromConfig(aws.Config{})
	client := eksClientimpl{
		client: eksClient,
	}
	ctx := context.Background()
	input := &eks.ListClustersInput{}

	output, err := client.ListClusters(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}
