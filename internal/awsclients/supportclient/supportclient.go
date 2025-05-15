// Package supportclient provides a wrapper around AWS Support SDK client
package supportclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/support"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants
	errInvalidRegion = "supportclient creation failed. invalid region"
)

// SupportClient defines an interface for using AWS Trusted Advisor Client
type SupportClient interface {
	GetRegion() string // returns the region the client is created in
	RefreshTrustedAdvisorCheck(ctx context.Context, params *support.RefreshTrustedAdvisorCheckInput, optFns ...func(*support.Options)) (*support.RefreshTrustedAdvisorCheckOutput, error)
}

// supportClientImpl implements SupportClient
type supportClientImpl struct {
	region string          // region the client is created in
	client *support.Client // client for using support client
}

// NewSupportClient creates and returns a new Support client implementation
// It validates the provided region and returns an error if invalid
func NewSupportClient(client *support.Client, region string) (SupportClient, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &supportClientImpl{
		client: client,
		region: region,
	}, nil
}

// RefreshTrustedAdvisorCheck requests a refresh for a Trusted Advisor check
func (c *supportClientImpl) RefreshTrustedAdvisorCheck(ctx context.Context, params *support.RefreshTrustedAdvisorCheckInput, optFns ...func(*support.Options)) (*support.RefreshTrustedAdvisorCheckOutput, error) {
	return c.client.RefreshTrustedAdvisorCheck(ctx, params, optFns...)
}

// GetRegion returns the region the client is created in
func (c *supportClientImpl) GetRegion() string {
	return c.region
}
