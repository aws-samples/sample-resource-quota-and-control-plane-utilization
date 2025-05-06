package servicequotaclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	serviceQuotaTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
)

// FakeServiceQuotaClient implements the necessary Service Quota API for testing.
type FakeServiceQuotaClient struct {
	Region      string
	QuotaValue  float64
	ReturnError bool
}

func (f *FakeServiceQuotaClient) GetServiceQuota(ctx context.Context, input *servicequotas.GetServiceQuotaInput, optFns ...func(*servicequotas.Options)) (*servicequotas.GetServiceQuotaOutput, error) {
	if f.ReturnError {
		return nil, errors.New("service quota error")
	}
	return &servicequotas.GetServiceQuotaOutput{
		Quota: &serviceQuotaTypes.ServiceQuota{
			Value: aws.Float64(f.QuotaValue),
		},
	}, nil
}

// get region
func (f *FakeServiceQuotaClient) GetRegion() string {
	return f.Region
}
