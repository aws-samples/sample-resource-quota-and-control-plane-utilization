// Package factory provides a factory for creating AWS service clients
package factory

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/aws/aws-sdk-go-v2/service/support"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/efsclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/eksclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/elbv2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/iamclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/supportclient"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	invalidRegionErr = "invalid region %s"
	clientCreatedMsg = "successfully created %s client for %s"
)

// ClientFactory defines an interface for creating AWS service clients for any region
type ClientFactory interface {
	// CreateEC2 creates a new EC2 client for the specified region
	CreateEC2(region string) (ec2client.Ec2Client, error)
	// CreateEFS creates a new EFS client for the specified region
	CreateEFS(region string) (efsclient.EFSClient, error)
	// CreateEKS creates a new EKS client for the specified region
	CreateEKS(region string) (eksclient.EKSClient, error)
	// CreateELBV2 creates a new ELBv2 client for the specified region
	CreateELBV2(region string) (elbv2client.ElbV2Client, error)
	// CreateIAM creates a new IAM client for the specified region
	CreateIAM(region string) (iamclient.IamClient, error)
	// CreateServiceQuotas creates a new Service Quotas client for the specified region
	CreateServiceQuotas(region string) (servicequotaclient.ServiceQuotasClient, error)
	// CreateSupport creates a new Support client for the specified region
	CreateSupport(region string) (supportclient.SupportClient, error)
	// CreateCloudWatchLogs creates a new CloudWatch Logs client for the specified region
	CreateCloudWatchLogs(region string) (cwlclient.CloudWatchLogsClient, error)
}

// factory is the concrete implementation of the ClientFactory interface
type factory struct {
	// baseCfg is the base AWS SDK configuration
	baseCfg aws.Config
	// Logger is the logger instance for this factory
	Logger  logger.Logger
}

// logAndReturnError logs an error message and returns a formatted error
func (f *factory) logAndReturnError(format string, args ...interface{}) error {
	errMsg := fmt.Sprintf(format, args...)
	f.Logger.Error(errMsg)
	return fmt.Errorf(errMsg)
}

// validateRegion checks if the region is valid and returns an error if not
func (f *factory) validateRegion(region string) error {
	if !utils.IsValidRegion(region) {
		return f.logAndReturnError(invalidRegionErr, region)
	}
	return nil
}

// NewFactory creates a new ClientFactory with the provided logger and options
// It bootstraps with one aws.Config and allows passing additional config.LoadOptions
// to customize credentials, retries, endpoints, etc.
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

// CreateEFS creates a new EFS client for the specified region
func (f *factory) CreateEFS(region string) (efsclient.EFSClient, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := efs.NewFromConfig(f.baseCfg, func(o *efs.Options) {
		o.Region = region
	})

	efsClient, err := efsclient.NewEFSClient(client, region)
	if err != nil {
		return nil, err
	}
	
	f.Logger.Debug(clientCreatedMsg, "efs", region)
	return efsClient, nil
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

// CreateELBV2 creates a new ELBv2 client for the specified region
func (f *factory) CreateELBV2(region string) (elbv2client.ElbV2Client, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := elasticloadbalancingv2.NewFromConfig(f.baseCfg, func(o *elasticloadbalancingv2.Options) {
		o.Region = region
	})

	elbv2Client, err := elbv2client.NewElbV2Client(client, region)
	if err != nil {
		return nil, err
	}
	
	f.Logger.Debug(clientCreatedMsg, "elbv2", region)
	return elbv2Client, nil
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

// CreateSupport creates a new Support client for the specified region
func (f *factory) CreateSupport(region string) (supportclient.SupportClient, error) {
	// validate region is valid
	if err := f.validateRegion(region); err != nil {
		return nil, err
	}

	client := support.NewFromConfig(f.baseCfg, func(o *support.Options) {
		o.Region = region
	})

	supportClient, err := supportclient.NewSupportClient(client, region)
	if err != nil {
		return nil, err
	}
	
	f.Logger.Debug(clientCreatedMsg, "support", region)
	return supportClient, nil
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
