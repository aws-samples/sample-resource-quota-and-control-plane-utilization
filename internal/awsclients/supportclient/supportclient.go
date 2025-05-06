package supportclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/support"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// SupportClient defines an interface for using AWS Trusted Advisor Client
type SupportClient interface {
	GetRegion() string // returns the region the client is created in
	RefreshTrustedAdvisorCheck(ctx context.Context, params *support.RefreshTrustedAdvisorCheckInput, optFns ...func(*support.Options)) (*support.RefreshTrustedAdvisorCheckOutput, error)
}

// SupportClientImpl implements SupportClient
type SupportClientImpl struct {
	region string          // region the client is created in
	client *support.Client // client for using support client
}

// New creates a new SupportClientImpl
func NewSupportClient(cfg aws.Config, region string) (SupportClient, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("supportclient creation failed. invalid region")
	}

	// create client w/ given region
	client := support.NewFromConfig(cfg, func(o *support.Options) {
		o.Region = region
	})

	return &SupportClientImpl{
		client: client,
		region: region,
	}, nil

}

// DescribeTrustedAdvisorChecks returns a list of Trusted Advisor checks
func (c *SupportClientImpl) RefreshTrustedAdvisorCheck(ctx context.Context, params *support.RefreshTrustedAdvisorCheckInput, optFns ...func(*support.Options)) (*support.RefreshTrustedAdvisorCheckOutput, error) {
	return c.client.RefreshTrustedAdvisorCheck(ctx, params, optFns...)
}

// GetRegion returns the region the client is created in
func (c *SupportClientImpl) GetRegion() string {
	return c.region
}
