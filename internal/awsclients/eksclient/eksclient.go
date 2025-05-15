// Package eksclient provides a wrapper around AWS EKS SDK client
package eksclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error message constants
	errInvalidRegion = "eksclient creation failed. invalid region"
)

// EKSClient defines an interface for interacting with AWS EKS service
type EKSClient interface {
	// GetRegion returns the AWS region this client is configured for
	GetRegion() string
	// ListClusters retrieves a list of EKS clusters in the AWS account
	ListClusters(ctx context.Context, params *eks.ListClustersInput, optFns ...func(*eks.Options)) (*eks.ListClustersOutput, error)
}

// eksClientimpl implements the EKSClient interface
type eksClientimpl struct {
	// client is the underlying AWS EKS SDK client
	client *eks.Client
	// region is the AWS region this client is configured for
	region string
}

// NewEKSClient will create a new EKSClient
func NewEKSClient(client *eks.Client, region string) (EKSClient, error) {
	// validate the region
	if !utils.IsValidRegion(region) {
		return nil, errors.New(errInvalidRegion)
	}

	return &eksClientimpl{
		client: client,
		region: region,
	}, nil
}

// ListClusters retrieves a list of EKS clusters in the AWS account
func (e *eksClientimpl) ListClusters(ctx context.Context, params *eks.ListClustersInput, optFns ...func(*eks.Options)) (*eks.ListClustersOutput, error) {
	return e.client.ListClusters(ctx, params, optFns...)
}

// GetRegion returns the AWS region this client is configured for
func (e *eksClientimpl) GetRegion() string {
	return e.region
}
