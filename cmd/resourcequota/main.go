package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	metricemfbatcher "github.com/outofoffice3/aws-samples/geras/internal/emfbatcher/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/handlers"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	"github.com/outofoffice3/aws-samples/geras/internal/serviceconfig"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"

	"github.com/outofoffice3/aws-samples/geras/internal/job/customjobs/ec2/networkinterfaces"
	"github.com/outofoffice3/aws-samples/geras/internal/job/customjobs/eks/listcluster"
	"github.com/outofoffice3/aws-samples/geras/internal/job/customjobs/iam/oidcproviders"
	"github.com/outofoffice3/aws-samples/geras/internal/job/customjobs/support/gp3storage"
	"github.com/outofoffice3/aws-samples/geras/internal/job/customjobs/support/iamroles"
	vpcnau "github.com/outofoffice3/aws-samples/geras/internal/job/customjobs/vpc/nau"
)

const (
	// Environment variables
	lambdaLayerPathEnv    = "LAMBDA_LAYER_PATH"
	logLevelEnv           = "LOG_LEVEL"
	cloudwatchLogGroupEnv = "CLOUDWATCH_LOG_GROUP"
	metricNamespaceEnv    = "METRIC_NAMESPACE"

	// Known service variables
	maxEvents          = 10000
	maxBytes           = 1 << 20
	flushInterval      = 45 * time.Second
	overhead           = 26
	defaultWorkerCount = 4
	defaultJobTimeout  = 120 * time.Second

	// Init errors
	ErrMsgCannotLoadEnvVar          = "cannot load env var"
	ErrMsgLoadConfig                = "error loading config"
	ErrMsgInvalidConfig             = "invalid config"
	ErrMsgLoadAWSConfig             = "error loading AWS config"
	ErrMsgEnsureLogGroup            = "error ensuring log group/stream"
	ErrMsgCreateCWLClientForMetrics = "error creating CWL client for metrics"
	ErrMsgInitMetricBatcher         = "error initializing metric batcher"
	ErrMsgCreateClientFactory       = "error creating client factory"

	// Create job errors
	ErrMsgCreateNetworkInterfacesJob = "error creating EC2 job"
	ErrMsgCreateListEKSClustersJob   = "error creating list EKS clusters job"
	ErrMsgCreateIAMOIDCJob           = "error creating IAM OIDC job"
	ErrMsgCreateIAMRolesJob          = "error creating IAM Roles job"
	ErrMsgCreateGP3StorageJob        = "error creating GP3 job"
	ErrMsgCreateVPCNAUJob            = "error creating VPC NAU job"

	// Create handler error
	ErrMsgCreateResourceQuotaHandler = "error creating resource quota handler"

	// Client creation errors
	ErrMsgCreateEC2Client          = "error creating EC2 client"
	ErrMsgCreateEKSClient          = "error creating EKS client"
	ErrMsgCreateIAMClient          = "error creating IAM client"
	ErrMsgCreateSupportClient      = "error creating Support client"
	ErrMsgCreateEFSClient          = "error creating EFS client"
	ErrMsgCreateELBClient          = "error creating ELB client"
	ErrMsgCreateServiceQuotaClient = "error creating Service Quota client"
)

// LambdaResponse represents the response returned by the Lambda function.
// It includes a status and message to indicate the result of processing.
type LambdaResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleRequest is the entrypoint for Lambda invocations.
// It processes CloudWatch events and coordinates resource quota monitoring.
func HandleRequest(ctx context.Context, event events.CloudWatchEvent) (LambdaResponse, error) {
	log := initLogger()

	// Load remaining env variables
	cloudwatchLogGroup := os.Getenv(cloudwatchLogGroupEnv)
	if cloudwatchLogGroup == "" {
		fatal(FatalInput{
			Logger: log,
			Msg:    ErrMsgCannotLoadEnvVar,
			Err:    fmt.Errorf("cloudwatch log group is not set"),
		})
	}
	log.Info("loaded cloudwatch log group env var: %s", cloudwatchLogGroup)

	namespace := os.Getenv(metricNamespaceEnv)
	if namespace == "" {
		fatal(FatalInput{
			Logger: log,
			Msg:    ErrMsgCannotLoadEnvVar,
			Err:    fmt.Errorf("metric namespace is not set"),
		})
	}
	log.Info("loaded metric namespace env var: %s", namespace)

	// Load service configuration from lambda layer
	svcCfg := loadServiceConfig(LoadServiceConfigInput{
		Logger: log,
	})
	if svcCfg == nil {
		log.Error("failed to load service config")
	} else {
		log.Info("loaded service config from lambda layer: %+v", *svcCfg)
	}

	// Initialize client factory
	clientFactory := initClientFactory(ctx, log)
	log.Info("initialized client factory")

	regions := svcCfg.Regions

	// Ensure cloudwatch log group and streams are created across all regions
	cloudWatchLogStream := utils.MakeStreamName()
	ensureLogGroup(EnsureLogGroupInput{
		Ctx:                 ctx,
		Regions:             regions,
		CloudwatchLogGroup:  cloudwatchLogGroup,
		CloudwatchLogStream: cloudWatchLogStream,
		ClientFactory:       clientFactory,
		Logger:              log,
	})
	log.Info("cloudwatch log group %s, log stream %s successfully created in all regions", cloudwatchLogGroup, cloudWatchLogStream)

	// Create metric emf metric batchers
	// It will convert cloudwatch metrics to EMF
	// and send them to cloudwatch logs
	regionalBatchers, regionalChans := initMetricBatchers(InitMetricBatchersInput{
		Ctx:           ctx,
		ClientFactory: clientFactory,
		Regions:       regions,
		LogGroup:      cloudwatchLogGroup,
		LogStream:     cloudWatchLogStream,
		Namespace:     namespace,
		Logger:        log,
	})
	log.Info("initialized cloudwatch metric batchers")

	// Build job manager
	// This will start the go routine worker pool which will process jobs in parallel
	jobMgr := buildJobManager(BuildJobManagerInput{
		Ctx:           ctx,
		ClientFactory: clientFactory,
		Regions:       regions,
		RegionalChans: regionalChans,
		Services:      svcCfg.Services,
		Logger:        log,
	})
	log.Info("built job manager")

	// Initialize handler
	handler := initResourceQuotaHandler(InitResourceQuotaHandlerInput{
		ClientFactory:                    clientFactory,
		LogGroup:                         cloudwatchLogGroup,
		LogStream:                        cloudWatchLogStream,
		Namespace:                        namespace,
		RegionalCloudwatchMetricBatchers: regionalBatchers,
		JobManager:                       jobMgr,
		ServiceConfig:                    svcCfg,
		Logger:                           log,
	})
	log.Info("initialized resource quota handler")

	// Handle event
	if err := handler.HandleEvent(ctx, event); err != nil {
		return LambdaResponse{"error", err.Error()}, err
	}
	return LambdaResponse{"success", "Processed event successfully"}, nil
}

// main is the entry point for the Lambda function.
func main() {
	lambda.Start(HandleRequest)
}

// initLogger sets up the package logger based on environment configuration.
func initLogger() logger.Logger {
	// Load log level from env var
	logLevel := os.Getenv(logLevelEnv)
	// If not set, default to INFO
	switch logLevel {
	case "info":
		logger.Init(logger.INFO, os.Stdout)
	case "debug":
		logger.Init(logger.DEBUG, os.Stdout)
	default:
		logger.Init(logger.INFO, os.Stdout)
	}
	return logger.Get()
}

// initClientFactory initializes the client factory for AWS service clients.
func initClientFactory(ctx context.Context, log logger.Logger) factory.ClientFactory {
	clientFactory, err := factory.NewFactory(ctx, log)
	if err != nil {
		fatal(FatalInput{
			Logger: log,
			Msg:    ErrMsgCreateClientFactory,
			Err:    err,
		})
	}
	return clientFactory
}

// LoadServiceConfigInput contains parameters for loading service configuration.
type LoadServiceConfigInput struct {
	Logger logger.Logger
}

// loadServiceConfig reads and validates the service config file.
func loadServiceConfig(input LoadServiceConfigInput) *serviceconfig.TopLevelServiceConfig {
	path := os.Getenv(lambdaLayerPathEnv)
	cfg, err := serviceconfig.LoadConfigFromFile(path, input.Logger)
	if err != nil {
		fatal(FatalInput{
			Logger: input.Logger,
			Msg:    ErrMsgLoadConfig,
			Err:    err,
		})
	}
	if err = serviceconfig.ValidateQuotaMetricConfig(*cfg, input.Logger); err != nil {
		fatal(FatalInput{
			Logger: input.Logger,
			Msg:    ErrMsgInvalidConfig,
			Err:    err,
		})
	}
	return cfg
}

// EnsureLogGroupInput contains parameters for ensuring log groups exist.
type EnsureLogGroupInput struct {
	Ctx                 context.Context
	Regions             []string
	CloudwatchLogGroup  string
	CloudwatchLogStream string
	ClientFactory       factory.ClientFactory
	Logger              logger.Logger
}

// ensureLogGroup creates the CloudWatch Logs group and stream across regions.
func ensureLogGroup(input EnsureLogGroupInput) {
	log := input.Logger
	if err := cwlclient.EnsureGroupAndStreamAcrossRegions(input.Ctx, input.Regions,
		input.CloudwatchLogGroup,
		input.CloudwatchLogStream,
		input.ClientFactory); err != nil {
		fatal(FatalInput{
			Logger: log,
			Msg:    ErrMsgEnsureLogGroup,
			Err:    err,
		})
	}
	log.Info("log group and stream ready in regions: %v", input.Regions)
}

// InitMetricBatchersInput contains parameters for initializing metric batchers.
type InitMetricBatchersInput struct {
	Ctx           context.Context
	ClientFactory factory.ClientFactory
	Regions       []string
	LogGroup      string
	LogStream     string
	Namespace     string
	Logger        logger.Logger
}

// initMetricBatchers creates a metric batcher per region.
func initMetricBatchers(input InitMetricBatchersInput) (*safemap.TypedMap[metricemfbatcher.CloudWatchMetricBatcher], *safemap.TypedMap[chan sharedtypes.CloudWatchMetric]) {
	log := input.Logger
	cwlMap := &safemap.TypedMap[metricemfbatcher.CloudWatchMetricBatcher]{}
	chanMap := &safemap.TypedMap[chan sharedtypes.CloudWatchMetric]{}

	for _, region := range input.Regions {
		client, err := input.ClientFactory.CreateCloudWatchLogs(region)
		if err != nil {
			fatal(FatalInput{
				Logger: log,
				Msg:    ErrMsgCreateCWLClientForMetrics,
				Err:    err,
			})
		}
		mb, err := metricemfbatcher.NewCloudWatchMetricBatcher(input.Ctx, client,
			input.Namespace, input.LogGroup, input.LogStream,
			maxEvents, maxBytes, flushInterval, overhead, log)
		if err != nil {
			fatal(FatalInput{
				Logger: log,
				Msg:    ErrMsgInitMetricBatcher,
				Err:    err,
			})
		}
		cwlMap.Store(region, *mb)
		chanMap.Store(region, mb.Batcher.GetInputChannel())
		log.Info("metric batcher ready for region: %s", region)
	}
	return cwlMap, chanMap
}

// BuildJobManagerInput contains parameters for building the job manager.
type BuildJobManagerInput struct {
	Ctx           context.Context
	ClientFactory factory.ClientFactory
	Regions       []string
	RegionalChans *safemap.TypedMap[chan sharedtypes.CloudWatchMetric]
	Services      map[string]serviceconfig.ServiceConfig
	Logger        logger.Logger
}

// buildJobManager wires up all jobs from service configs.
func buildJobManager(input BuildJobManagerInput) *job.JobManager {
	log := input.Logger
	clientFactory := input.ClientFactory
	jm := job.NewJobManager(job.JobManagerConfig{
		ParentCtx:  input.Ctx,
		Workers:    defaultWorkerCount,
		JobTimeout: defaultJobTimeout,
		MetricMap:  input.RegionalChans,
		Log:        log,
	})

	var (
		iamServiceQuotaRegion = "us-east-1"
	)
	for _, region := range input.Regions {
		for serviceName, svcCfg := range input.Services {
			switch serviceName {
			case "ec2":
				for _, qm := range svcCfg.QuotaMetrics {
					if qm.Name == "networkInterfaces" {
						log.Info("creating EC2 job for region: %s", region)
						ec2Client, err := clientFactory.CreateEC2(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateEC2Client,
								Err:    err,
							})
						}
						sqClient, err := clientFactory.CreateServiceQuotas(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateServiceQuotaClient,
								Err:    err,
							})
						}
						job, err := networkinterfaces.NewNetworkInterfaceJob(networkinterfaces.NetworkInterfaceJobConfig{
							Ec2Client:           ec2Client,
							ServiceQuotasClient: sqClient,
							Logger:              log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateNetworkInterfacesJob,
								Err:    err,
							})
						}
						jm.AddJob(job)
						log.Info("added network interfaces job for region: %s to job manager", region)
					}
				}

			case "eks":
				for _, qm := range svcCfg.QuotaMetrics {
					if qm.Name == "listClusters" {
						log.Info("creating EKS job for region: %s", region)
						eksClient, err := clientFactory.CreateEKS(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateEKSClient,
								Err:    err,
							})
						}
						sqClient, err := clientFactory.CreateServiceQuotas(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateServiceQuotaClient,
								Err:    err,
							})
						}
						job, err := listcluster.NewListClusterJob(listcluster.ListClusterJobConfig{
							EksClient:           eksClient,
							ServiceQuotasClient: sqClient,
							Logger:              log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateListEKSClustersJob,
								Err:    err,
							})
						}
						jm.AddJob(job)
						log.Info("added list clusters job for region: %s to job manager", region)
					}
				}

			case "iam":
				for _, qm := range svcCfg.QuotaMetrics {
					if qm.Name == "oidcProviders" {
						log.Info("creating IAM OIDC job for region: %s", region)
						iamClient, err := clientFactory.CreateIAM(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateIAMClient,
								Err:    err,
							})
						}
						sqClient, err := clientFactory.CreateServiceQuotas(iamServiceQuotaRegion)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateServiceQuotaClient,
								Err:    err,
							})
						}
						job, err := oidcproviders.NewOIDCProviderJob(oidcproviders.OIDCProviderJobConfig{
							IamClient:          iamClient,
							ServiceQuotasCliet: sqClient,
							Logger:             log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateIAMOIDCJob,
								Err:    err,
							})
						}
						jm.AddJob(job)
						log.Info("added oidc providers job for region: %s to job manager", region)
					}
					if qm.Name == "iamRoles" {
						log.Info("creating IAM Roles job for region: %s", region)
						supportClient, err := clientFactory.CreateSupport(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateSupportClient,
								Err:    err,
							})
						}
						job, err := iamroles.NewIamRoleJob(iamroles.IamRoleJobConfig{
							SupportClient: supportClient,
							Logger:        log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateIAMRolesJob,
								Err:    err,
							})
						}
						jm.AddJob(job)
						log.Info("added iam Roles job for region: %s to job manager", region)
					}
				}

			case "ebs":
				for _, qm := range svcCfg.QuotaMetrics {
					if qm.Name == "gp3Storage" {
						log.Info("creating GP3 storage job for region: %s", region)
						supportClient, err := clientFactory.CreateSupport(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateSupportClient,
								Err:    err,
							})
						}
						job, err := gp3storage.NewGp3StorageJob(gp3storage.Gp3StorageJobConfig{
							SupportClient: supportClient,
							Logger:        log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateGP3StorageJob,
								Err:    err,
							})
						}
						jm.AddJob(job)
						log.Info("added gp3 storage job for region: %s to job manager", region)
					}
				}

			case "vpc":
				for _, qm := range svcCfg.QuotaMetrics {
					if qm.Name == "nau" {
						log.Info("creating vpc nau job for region: %s", region)
						ec2Client, err := clientFactory.CreateEC2(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateEC2Client,
								Err:    err,
							})
						}
						efsClient, err := clientFactory.CreateEFS(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateEFSClient,
								Err:    err,
							})
						}
						elbClient, err := clientFactory.CreateELBV2(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateELBClient,
								Err:    err,
							})
						}
						serviceQuotasClient, err := clientFactory.CreateServiceQuotas(region)
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateServiceQuotaClient,
								Err:    err,
							})
						}
						nauCalc := nau.NewCalculator(ec2Client, efsClient, elbClient, log)
						job, err := vpcnau.NewVPCNAUJob(vpcnau.VPCNAUConfig{
							NauCalculator:       nauCalc,
							ServiceQuotasClient: serviceQuotasClient,
							Logger:              log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger: log,
								Msg:    ErrMsgCreateVPCNAUJob,
								Err:    err,
							})
						}
						jm.AddJob(job)
						log.Info("added vpc nau job for region: %s to job manager", region)
					}
				}
			}
		}
	}
	log.Info("all jobs added to manager")
	return jm
}

// InitResourceQuotaHandlerInput contains parameters for initializing the resource quota handler.
type InitResourceQuotaHandlerInput struct {
	ClientFactory                    factory.ClientFactory
	LogGroup                         string
	LogStream                        string
	Namespace                        string
	RegionalCloudwatchMetricBatchers *safemap.TypedMap[metricemfbatcher.CloudWatchMetricBatcher]
	JobManager                       *job.JobManager
	ServiceConfig                    *serviceconfig.TopLevelServiceConfig
	Logger                           logger.Logger
}

// initResourceQuotaHandler builds the handler with dependencies.
func initResourceQuotaHandler(input InitResourceQuotaHandlerInput) *handlers.ResourceQuotaHandler {
	log := input.Logger
	h, err := handlers.NewResourceQuotaHandler(handlers.ResourceQuotaHandlerConfig{
		ClientFactory:                    input.ClientFactory,
		CloudwatchLogGroup:               input.LogGroup,
		CloudWatchLogGroupStream:         input.LogStream,
		Namespace:                        input.Namespace,
		RegionalCloudwatchMetricBatchers: input.RegionalCloudwatchMetricBatchers,
		JobManager:                       input.JobManager,
		ServiceConfig:                    input.ServiceConfig,
		Logger:                           log,
	})
	if err != nil {
		fatal(FatalInput{
			Logger: log,
			Msg:    ErrMsgCreateResourceQuotaHandler,
			Err:    err,
		})
	}
	return h
}

// FatalInput contains parameters for the fatal function which logs an error and exits.
type FatalInput struct {
	Logger logger.Logger
	Msg    string
	Err    error
}

// fatal logs an error message and exits the program with status code 1.
func fatal(input FatalInput) {
	log := input.Logger
	log.Error("%s: %v", input.Msg, input.Err)
	os.Exit(1)
}
