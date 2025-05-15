// Package servicequotaclient provides a wrapper around AWS Service Quotas SDK client
package servicequotaclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants
	errInvalidRegion = "servicequotaclient creation failed. invalid region"
)

// ServiceQuotasClient defines an interface for interacting with AWS Service Quotas service
type ServiceQuotasClient interface {
	// GetRegion returns the AWS region this client is configured for
	GetRegion() string
	// GetServiceQuota retrieves information about a specific service quota
	GetServiceQuota(ctx context.Context, params *servicequotas.GetServiceQuotaInput, optFns ...func(*servicequotas.Options)) (*servicequotas.GetServiceQuotaOutput, error)
}

// serviceQuotasImpl implements the ServiceQuotasClient interface
type serviceQuotasImpl struct {
	// region is the AWS region this client is configured for
	region string
	// client is the underlying AWS Service Quotas SDK client
	client *servicequotas.Client
}

// NewServiceQuotaClient creates and returns a new Service Quotas client implementation
// It validates the provided region and returns an error if invalid
func NewServiceQuotaClient(client *servicequotas.Client, region string) (ServiceQuotasClient, error) {
	// validate the region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &serviceQuotasImpl{
		client: client,
		region: region,
	}, nil
}

// GetServiceQuota retrieves information about a specific service quota
func (s *serviceQuotasImpl) GetServiceQuota(ctx context.Context, params *servicequotas.GetServiceQuotaInput, optFns ...func(*servicequotas.Options)) (*servicequotas.GetServiceQuotaOutput, error) {
	return s.client.GetServiceQuota(ctx, params, optFns...)
}

// GetRegion returns the AWS region this client is configured for
func (s *serviceQuotasImpl) GetRegion() string {
	return s.region
}
