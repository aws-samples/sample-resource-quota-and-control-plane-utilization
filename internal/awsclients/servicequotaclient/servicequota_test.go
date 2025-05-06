package servicequotaclient_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/stretchr/testify/assert"
)

// TestNewServiceQuotaClient tests the creation of a new service quota client successfully
func TestNewServiceQuotaClient(t *testing.T) {
	// Create a new service quota client
	client, err := servicequotaclient.NewServiceQuotaClient(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "error creating service quota client")
	assert.NotNil(t, client, "service quota client is nil")
	assert.IsType(t, &servicequotaclient.ServiceQuotasImpl{}, client, "client is not of type ServiceQuotaClient")
}

// TestNewServiceQuotaClientError tests the creation of a new service quota client with an invalid region
func TestNewServiceQuotaClientError(t *testing.T) {
	// Create a new service quota client
	client, err := servicequotaclient.NewServiceQuotaClient(aws.Config{}, "invalid-region")
	assert.Error(t, err, "error creating service quota client")
	assert.Nil(t, client, "service quota client is not nil")
}

// TestGetServiceQuota tests GetServiceQuota interface method
func TestGetServiceQuotaInterface(t *testing.T) {
	// Create a new service quota client
	client, err := servicequotaclient.NewServiceQuotaClient(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "error creating service quota client")

	// Get a service quota
	quota, err := client.GetServiceQuota(context.Background(), &servicequotas.GetServiceQuotaInput{})
	assert.Error(t, err, "should error getting service quota")
	assert.IsType(t, &servicequotas.GetServiceQuotaOutput{}, quota, "quota is not of type ServiceQuota")
	assert.Equal(t, "us-east-1", client.GetRegion(), "region is not us-east-1")
}
