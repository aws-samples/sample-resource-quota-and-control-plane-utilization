package iamclient

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/stretchr/testify/assert"
)

// TestNewIamClient tests the NewIamClient function
func TestNewIamClient(t *testing.T) {
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
			client := iam.NewFromConfig(aws.Config{})
			iamClient, err := NewIamClient(client, tt.region)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, iamClient)
				assert.Contains(t, err.Error(), "invalid region")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, iamClient)
				assert.Equal(t, tt.region, iamClient.GetRegion())
			}
		})
	}
}

// TestIamClientImpl_GetRegion tests the GetRegion method
func TestIamClientImpl_GetRegion(t *testing.T) {
	region := "us-west-2"
	client := &iamClientImpl{
		region: region,
	}

	assert.Equal(t, region, client.GetRegion())
}

// TestIamClientImpl_ListRoles tests the ListRoles method
func TestIamClientImpl_ListRoles(t *testing.T) {
	iamClient := iam.NewFromConfig(aws.Config{})
	client := iamClientImpl{
		client: iamClient,
	}
	ctx := context.Background()
	input := &iam.ListRolesInput{}

	output, err := client.ListRoles(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}

// TestIamClientImpl_ListOpenIDConnectProviders tests the ListOpenIDConnectProviders method
func TestIamClientImpl_ListOpenIDConnectProviders(t *testing.T) {
	iamClient := iam.NewFromConfig(aws.Config{})
	client := iamClientImpl{
		client: iamClient,
	}
	ctx := context.Background()
	input := &iam.ListOpenIDConnectProvidersInput{}

	output, err := client.ListOpenIDConnectProviders(ctx, input)
	assert.Error(t, err, "client is nil, should return error")
	assert.Nil(t, output, "output should be nil")
}
