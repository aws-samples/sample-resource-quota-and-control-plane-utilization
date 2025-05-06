package eksclient

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// EKSClient will define an interface for the eks service
type EKSClient interface {
	GetRegion() string
	ListClusters(ctx context.Context, params *eks.ListClustersInput, optFns ...func(*eks.Options)) (*eks.ListClustersOutput, error)
}

type EKSClientImpl struct {
	client *eks.Client
	region string
}

// NewEKSClient will create a new EKSClient
func NewEKSClient(cfg aws.Config, region string) (EKSClient, error) {
	// validate the region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("eksclient creation failed. invalid region")
	}

	eksclient := eks.NewFromConfig(cfg, func(o *eks.Options) {
		o.Region = region
	})
	return &EKSClientImpl{
		client: eksclient,
		region: region,
	}, nil
}

func (e *EKSClientImpl) ListClusters(ctx context.Context, params *eks.ListClustersInput, optFns ...func(*eks.Options)) (*eks.ListClustersOutput, error) {
	return e.client.ListClusters(ctx, params, optFns...)
}

func (e *EKSClientImpl) GetRegion() string {
	return e.region
}
