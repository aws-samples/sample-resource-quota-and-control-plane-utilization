package s3client

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

type S3Client interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

type s3ClientImpl struct {
	client *s3.Client
	region string
}

func NewS3Client(client *s3.Client, region string) (S3Client, error) {
	if !utils.IsValidRegion(region) {
		return nil, errors.New("invalid region")
	}

	return s3ClientImpl{
		client: client,
		region: region,
	}, nil
}

func (c s3ClientImpl) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return c.client.PutObject(ctx, params, optFns...)
}
