package s3client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

// defines an interface for interacting w/ s3 client
type S3Client interface {
	GetRegion() string
	// GetObject
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	// putObject
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

type S3ClientImpl struct {
	region string
	client *s3.Client
}

func NewS3Client(cfg aws.Config, region string) (S3Client, error) {
	// validate the region
	if !utils.IsValidRegion(region) {
		return nil, errors.New("failed to created s3client. invalid region")
	}

	cfg.Region = region
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.Region = region
	})
	return &S3ClientImpl{
		region: region,
		client: client,
	}, nil
}

// Get object from s3
func (s *S3ClientImpl) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return s.client.GetObject(ctx, params, optFns...)
}

// put object
func (s *S3ClientImpl) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return s.client.PutObject(ctx, params, optFns...)
}

// get region
func (s *S3ClientImpl) GetRegion() string {
	return s.region
}
