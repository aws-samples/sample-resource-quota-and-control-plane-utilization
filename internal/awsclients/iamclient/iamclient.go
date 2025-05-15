// Package iamclient provides a wrapper around AWS IAM SDK client
package iamclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants
	errInvalidRegion = "iamclient creation failed. invalid region"
)

// IamClient defines an interface for interacting with AWS IAM service
type IamClient interface {
	// GetRegion returns the AWS region this client is configured for
	GetRegion() string
	// ListOpenIDConnectProviders retrieves information about the OpenID Connect providers in the AWS account
	ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error)
	// ListRoles retrieves information about IAM roles in the AWS account
	ListRoles(ctx context.Context, params *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error)
}

// iamClientImpl implements the IamClient interface
type iamClientImpl struct {
	// client is the underlying AWS IAM SDK client
	client *iam.Client
	// region is the AWS region this client is configured for
	region string
}

// NewIamClient creates and returns a new IAM client implementation
// It validates the provided region and returns an error if invalid
func NewIamClient(client *iam.Client, region string) (IamClient, error) {
	// validate region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &iamClientImpl{
		client: client,
		region: region,
	}, nil
}

// ListOpenIDConnectProviders retrieves information about the OpenID Connect providers in the AWS account
func (c *iamClientImpl) ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error) {
	return c.client.ListOpenIDConnectProviders(ctx, params, optFns...)
}

// ListRoles retrieves information about IAM roles in the AWS account
func (c *iamClientImpl) ListRoles(ctx context.Context, params *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	return c.client.ListRoles(ctx, params, optFns...)
}

// GetRegion returns the region of the client
func (c *iamClientImpl) GetRegion() string {
	return c.region
}
