// Package factory provides a centralized factory for creating AWS service clients
// with consistent configuration, region validation, and error handling.
package factory

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/eksclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/iamclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/s3client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Error and success message templates for consistent logging.
	invalidRegionErr = "invalid region %s"
	clientCreatedMsg = "successfully created %s client for %s"
)

// ClientFactory provides a centralized interface for creating AWS service clients
// with consistent configuration and region validation across all services.
type ClientFactory interface {
	// CreateEC2 creates a new EC2 client for the specified region.
	CreateEC2(region string) (ec2client.Ec2Client, error)
	// CreateEKS creates a new EKS client for the specified region.
	CreateEKS(region string) (eksclient.EKSClient, error)
	// CreateIAM creates a new IAM client for the specified region.
	CreateIAM(region string) (iamclient.IamClient, error)
	// CreateServiceQuotas creates a new Service Quotas client for the specified region.
	CreateServiceQuotas(region string) (servicequotaclient.ServiceQuotasClient, error)
	// CreateCloudWatchLogs creates a new CloudWatch Logs client for the specified region.
	CreateCloudWatchLogs(region string) (cwlclient.CloudWatchLogsClient, error)
	// CreateS3 creates a new S3 client for the specified region.
	CreateS3(region string) (s3client.S3Client, error)
}

// factory implements the ClientFactory interface with shared AWS configuration.
type factory struct {
	baseCfg aws.Config    // Base AWS SDK configuration shared across all clients
	Logger  logger.Logger // Logger instance for factory operations
}

// logAndReturnError provides centralized error logging and formatting.
func (f *factory) logAndReturnError(format string, args ...interface{}) error {
	errMsg := fmt.Sprintf(format, args...)
	f.Logger.Error(errMsg)
	return fmt.Errorf(errMsg)
}

// validateRegion ensures the provided region is supported by the application.
func (f *factory) validateRegion(region string) error {
	if !utils.IsValidRegion(region) {
		return f.logAndReturnError(invalidRegionErr, region)
	}
	return nil
}

// NewFactory creates a new AWS client factory with shared configuration.
// It loads a base AWS config and allows customization through LoadOptions.
func NewFactory(ctx context.Context, log logger.Logger, opts ...func(*config.LoadOptions) error) (ClientFactory, error) {
	// if logger is nil get new copy
	if log == nil {
		logger.Init(logger.INFO, os.Stdout)
		log = logger.Get()
	}
	// load a base config
	base, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	log.Debug("client factory created successfully")

	return &factory{
		baseCfg: base,
		Logger:  log,
	}, nil
}

// CreateEC2 creates a new EC2 client for the specified region
func (f *factory) CreateEC2(region string) (ec2client.Ec2Client, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := ec2.NewFromConfig(f.baseCfg, func(o *ec2.Options) {
		o.Region = region
	})

	ec2Client, err := ec2client.NewEc2Client(client, region)
	if err != nil {
		return nil, err
	}

	f.Logger.Debug(clientCreatedMsg, "ec2", region)
	return ec2Client, nil
}

// CreateEKS creates a new EKS client for the specified region
func (f *factory) CreateEKS(region string) (eksclient.EKSClient, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := eks.NewFromConfig(f.baseCfg, func(o *eks.Options) {
		o.Region = region
	})

	eksClient, err := eksclient.NewEKSClient(client, region)
	if err != nil {
		return nil, err
	}

	f.Logger.Debug(clientCreatedMsg, "eks", region)
	return eksClient, nil
}

// CreateIAM creates a new IAM client for the specified region
func (f *factory) CreateIAM(region string) (iamclient.IamClient, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := iam.NewFromConfig(f.baseCfg, func(o *iam.Options) {
		o.Region = region
	})

	iamClient, err := iamclient.NewIamClient(client, region)
	if err != nil {
		return nil, err
	}

	f.Logger.Debug(clientCreatedMsg, "iam", region)
	return iamClient, nil
}

// CreateServiceQuotas creates a new Service Quotas client for the specified region
func (f *factory) CreateServiceQuotas(region string) (servicequotaclient.ServiceQuotasClient, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := servicequotas.NewFromConfig(f.baseCfg, func(o *servicequotas.Options) {
		o.Region = region
	})

	serviceQuotasClient, err := servicequotaclient.NewServiceQuotaClient(client, region)
	if err != nil {
		return nil, err
	}

	f.Logger.Debug(clientCreatedMsg, "service quotas", region)
	return serviceQuotasClient, nil
}

// CreateCloudWatchLogs creates a new CloudWatch Logs client for the specified region
func (f *factory) CreateCloudWatchLogs(region string) (cwlclient.CloudWatchLogsClient, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := cloudwatchlogs.NewFromConfig(f.baseCfg, func(o *cloudwatchlogs.Options) {
		o.Region = region
	})

	cwlClient, err := cwlclient.NewCloudWatchLogsClient(client, region)
	if err != nil {
		return nil, err
	}

	f.Logger.Debug(clientCreatedMsg, "cloudwatch logs", region)
	return cwlClient, nil
}

func (f *factory) CreateS3(region string) (s3client.S3Client, error) {
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(f.baseCfg, func(o *s3.Options) {
		o.Region = region
	})
	s3Client, err := s3client.NewS3Client(client, region)
	if err != nil {
		return nil, err
	}
	return s3Client, nil
}



// InitClientFactory is a convenience function that creates a new ClientFactory.
// It's an alias for NewFactory to maintain backward compatibility.
func InitClientFactory(ctx context.Context, log logger.Logger, opts ...func(*config.LoadOptions) error) (ClientFactory, error) {
	return NewFactory(ctx, log, opts...)
}
