package efsclient

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/stretchr/testify/assert"
)

// TestNewEFSClient tests the NewEFSClient function
func TestNewEFSClient(t *testing.T) {
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
			client := efs.NewFromConfig(aws.Config{})
			efsClient, err := NewEFSClient(client, tt.region)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, efsClient)
				assert.Contains(t, err.Error(), "invalid region")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, efsClient)
				assert.Equal(t, tt.region, efsClient.GetRegion())
			}
		})
	}
}

// TestEfsClientImpl_GetRegion tests the GetRegion method
func TestEfsClientImpl_GetRegion(t *testing.T) {
	region := "us-west-2"
	client := &efsClientImpl{
		region: region,
	}

	assert.Equal(t, region, client.GetRegion())
}

// TestEfsClientImpl_DescribeFileSystems tests the DescribeFileSystems method
func TestEfsClientImpl_DescribeFileSystems(t *testing.T) {
	efsClient := efs.NewFromConfig(aws.Config{})
	client := efsClientImpl{
		client: efsClient,
	}
	ctx := context.Background()
	input := &efs.DescribeFileSystemsInput{}

	output, err := client.DescribeFileSystems(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}

// TestEfsClientImpl_DescribeMountTargets tests the DescribeMountTargets method
func TestEfsClientImpl_DescribeMountTargets(t *testing.T) {
	efsClient := efs.NewFromConfig(aws.Config{})
	client := efsClientImpl{
		client: efsClient,
	}
	ctx := context.Background()
	input := &efs.DescribeMountTargetsInput{}

	output, err := client.DescribeMountTargets(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}
