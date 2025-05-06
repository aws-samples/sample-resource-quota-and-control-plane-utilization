package iamclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// IamClient defines as interface for using AWS iam client
type IamClient interface {
	GetRegion() string
	// Implement ListOpenIDConnectProviders
	ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error)
	// Implement ListRoles
	ListRoles(ctx context.Context, params *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error)
}

// IamClientImpl implements IamClient interface
type IamClientImpl struct {
	client *iam.Client
	region string
}

// NewiacmClient returns a new IamClient
func NewIamClient(cfg aws.Config, region string) (IamClient, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("iamclient creation failed. invalid reigion")
	}

	client := iam.NewFromConfig(cfg, func(o *iam.Options) {
		o.Region = region
	})
	return &IamClientImpl{
		client: client,
		region: region,
	}, nil
}

// Implement ListOpenIDConnectProviders
func (c *IamClientImpl) ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error) {
	return c.client.ListOpenIDConnectProviders(ctx, params, optFns...)
}

// Implement ListRoles
func (c *IamClientImpl) ListRoles(ctx context.Context, params *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	return c.client.ListRoles(ctx, params, optFns...)
}

// GetRegion returns the region of the client
func (c *IamClientImpl) GetRegion() string {
	return c.region
}
