package servicequotaclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// ServiceQuotaClient will define an interface for the aws service quota client
type ServiceQuotasClient interface {
	GetRegion() string
	GetServiceQuota(ctx context.Context, params *servicequotas.GetServiceQuotaInput, optFns ...func(*servicequotas.Options)) (*servicequotas.GetServiceQuotaOutput, error)
}

type ServiceQuotasImpl struct {
	region string
	client *servicequotas.Client
}

// NewServiceQuotaClient will create a new service quota client
func NewServiceQuotaClient(cfg aws.Config, region string) (ServiceQuotasClient, error) {
	// validate the region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("servicequotaclient creation failed. invalid region ")
	}

	serviceQuotaClient := servicequotas.NewFromConfig(cfg, func(o *servicequotas.Options) {
		o.Region = region
	})
	return &ServiceQuotasImpl{
		client: serviceQuotaClient,
		region: region,
	}, nil
}

func (s *ServiceQuotasImpl) GetServiceQuota(ctx context.Context, params *servicequotas.GetServiceQuotaInput, optFns ...func(*servicequotas.Options)) (*servicequotas.GetServiceQuotaOutput, error) {
	return s.client.GetServiceQuota(ctx, params, optFns...)
}

func (s *ServiceQuotasImpl) GetRegion() string {
	return s.region
}
