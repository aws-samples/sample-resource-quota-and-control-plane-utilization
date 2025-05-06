package iamclient_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/iamclient"
	"github.com/stretchr/testify/assert"
)

// Test the creation of an iam client succesfully
func TestIamClient_Success(t *testing.T) {
	iamc, err := iamclient.NewIamClient(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "should not be error creating iam client")
	assert.NotNil(t, iamc, "should not be nil")
	assert.IsType(t, &iamclient.IamClientImpl{}, iamc, "should be of type IamClientImpl")
	assert.Equal(t, "us-east-1", iamc.GetRegion(), "should be equal")
}

// Test the creation of an iam client with an invalid region
func TestIamClient_InvalidRegion(t *testing.T) {
	iamc, err := iamclient.NewIamClient(aws.Config{}, "invalid-region")
	assert.Error(t, err, "should be error creating iam client")
	assert.Nil(t, iamc, "should be nil")
}

// Test ListOpenIDConnectProviders
func TestListOpenIDConnectProviders(t *testing.T) {
	iamc, err := iamclient.NewIamClient(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "should not be error creating iam client")
	assert.NotNil(t, iamc, "should not be nil")

	_, err = iamc.ListOpenIDConnectProviders(context.Background(), &iam.ListOpenIDConnectProvidersInput{})
	assert.Error(t, err, "should be error listing open id connect providers")
}

// Test ListOpenIDConnectProviders
func TestListOpenIDConnectProviders_Success(t *testing.T) {
	iamc, err := iamclient.NewIamClient(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "should not be error creating iam client")
	assert.NotNil(t, iamc, "should not be nil")

	_, err = iamc.ListOpenIDConnectProviders(context.Background(), &iam.ListOpenIDConnectProvidersInput{})
	assert.Error(t, err, "should be error listing open id connect providers")
}

// Test ListRoles
func TestListRoles(t *testing.T) {
	iamc, err := iamclient.NewIamClient(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "should not be error creating iam client")
	assert.NotNil(t, iamc, "should not be nil")

	_, err = iamc.ListRoles(context.Background(), &iam.ListRolesInput{})
	assert.Error(t, err, "should be error listing roles")
}
