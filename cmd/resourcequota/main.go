package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	awsclientsFactory "github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/flusher"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/handlers"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	"github.com/outofoffice3/aws-samples/geras/internal/serviceconfig"
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
	s3BucketNameEnv       = "S3_BUCKET"
	homeRegionEnv         = "HOME_REGION"

	// Known service variables
	maxEvents                 = 10000
	maxBytes                  = 1 << 20
	overhead                  = 26
	defaultWorkerCount        = 4
	defaultJobTimeout         = 120 * time.Second
	defaultManifestBufferSize = 100
	vpcNauManifestFilePrefix  = "vpc-network-address-usage-manifest"
	vpcNauManifestFileName    = "naureport"
)

// Initialization errors
var (
	ErrCannotLoadEnvVar          = errors.New("cannot load environment variable")
	ErrLoadConfig                = errors.New("error loading config")
	ErrInvalidConfig             = errors.New("invalid config")
	ErrLoadAWSConfig             = errors.New("error loading AWS config")
	ErrEnsureLogGroup            = errors.New("error ensuring log group/stream")
	ErrCreateCWLClientForMetrics = errors.New("error creating CloudWatch Logs client for metrics")
	ErrInitMetricBatcher         = errors.New("error initializing metric batcher")
	ErrCreateClientFactory       = errors.New("error creating client factory")
)

// Job‐creation errors
var (
	ErrCreateNetworkInterfacesJob = errors.New("error creating EC2 network interfaces job")
	ErrCreateListEKSClustersJob   = errors.New("error creating EKS list clusters job")
	ErrCreateIAMOIDCJob           = errors.New("error creating IAM OIDC providers job")
	ErrCreateIAMRolesJob          = errors.New("error creating IAM roles job")
	ErrCreateGP3StorageJob        = errors.New("error creating GP3 storage job")
	ErrCreateVPCNAUJob            = errors.New("error creating VPC NAU job")
)

// Handler‐creation errors
var (
	ErrCreateResourceQuotaHandler = errors.New("error creating resource quota handler")
)

// Client‐creation errors
var (
	ErrCreateEC2Client          = errors.New("error creating EC2 client")
	ErrCreateEKSClient          = errors.New("error creating EKS client")
	ErrCreateIAMClient          = errors.New("error creating IAM client")
	ErrCreateSupportClient      = errors.New("error creating Support client")
	ErrCreateEFSClient          = errors.New("error creating EFS client")
	ErrCreateELBClient          = errors.New("error creating ELB client")
	ErrCreateServiceQuotaClient = errors.New("error creating Service Quotas client")
	ErrCreateS3Client           = errors.New("error creating S3 client")
)

// LambdaResponse represents the response returned by the Lambda function.
// It includes a status and message to indicate the result of processing.
type LambdaResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// HandleRequest processes scheduled CloudWatch events to monitor resource quota utilization
func HandleRequest(ctx context.Context, event events.CloudWatchEvent) (LambdaResponse, error) {
	log := initLogger()

	// Load remaining env variables
	cloudwatchLogGroup := os.Getenv(cloudwatchLogGroupEnv)
	if cloudwatchLogGroup == "" {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCannotLoadEnvVar,
			Cause:   fmt.Errorf("cloudwatch log group is not set"),
		})
	}
	log.Info("loaded cloudwatch log group env var: %s", cloudwatchLogGroup)

	namespace := os.Getenv(metricNamespaceEnv)
	if namespace == "" {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCannotLoadEnvVar,
			Cause:   fmt.Errorf("metric namespace is not set"),
		})
	}
	log.Info("loaded metric namespace env var: %s", namespace)

	s3BucketName := os.Getenv(s3BucketNameEnv)
	if s3BucketName == "" {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCannotLoadEnvVar,
			Cause:   fmt.Errorf("s3 bucket name is not set"),
		})
	}

	rawHomeRegion := os.Getenv(homeRegionEnv)
	if rawHomeRegion == "" {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCannotLoadEnvVar,
			Cause:   fmt.Errorf("home region is not set"),
		})
	}
	validatedHomeRegion, err := utils.ParseAwsRegion(rawHomeRegion)
	if err != nil {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCannotLoadEnvVar,
			Cause:   fmt.Errorf("home region is not a valid AWS region"),
		})
	}

	// Load service configuration from lambda layer
	svcCfg := loadServiceConfig(LoadServiceConfigInput{Logger: log})
	if svcCfg == nil {
		log.Error("failed to load service config")
	} else {
		log.Info("loaded service config from lambda layer: %+v", *svcCfg)
	}

	// Initialize client factory
	clientFactory, err := awsclientsFactory.InitClientFactory(ctx, log)
	if err != nil {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCreateClientFactory,
			Cause:   err,
		})
	}
	log.Info("initialized client factory")

	regions := svcCfg.Regions

	// Ensure CloudWatch log group and streams exist in all regions
	cloudWatchLogStream := utils.MakeStreamName()
	ensureLogGroup(EnsureLogGroupInput{
		Ctx:                 ctx,
		Regions:             regions,
		CloudwatchLogGroup:  cloudwatchLogGroup,
		CloudwatchLogStream: cloudWatchLogStream,
		ClientFactory:       clientFactory,
		Logger:              log,
	})
	log.Info("cloudwatch log group %s, log stream %s successfully created in all regions",
		cloudwatchLogGroup, cloudWatchLogStream)

	// Create per-region MetricsBatcher map
	regionalBatchers := initMetricBatchers(InitMetricBatchersInput{
		Ctx:           ctx,
		ClientFactory: clientFactory,
		Regions:       regions,
		LogGroup:      cloudwatchLogGroup,
		LogStream:     cloudWatchLogStream,
		Namespace:     namespace,
		Logger:        log,
	})
	log.Info("initialized cloudwatch metric batchers")

	// Build NAU store
	key := nau.GenerateManifestKey(vpcNauManifestFilePrefix, vpcNauManifestFileName, time.Now())
	s3Client, err := clientFactory.CreateS3(validatedHomeRegion.String())
	if err != nil {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCreateS3Client,
			Cause:   err,
		})
	}
	nauManifest := nau.NewManifest(ctx, s3BucketName, key, s3Client,
		func(err error) { log.Error(err.Error()) }, log)
	nauStore := nau.NewAccountNauStore(nauManifest)

	// Build job manager, now using the batcher map
	jobMgr := buildJobManager(BuildJobManagerInput{
		Ctx:           ctx,
		ClientFactory: clientFactory,
		store:         nauStore,
		Regions:       regions,
		BatcherMap:    regionalBatchers,
		Services:      svcCfg.Services,
		Logger:        log,
		S3BucketName:  s3BucketName,
		HomeRegion:    validatedHomeRegion.String(),
	})
	log.Info("built job manager")

	// Initialize handler with the new batcher map
	handler := initResourceQuotaHandler(InitResourceQuotaHandlerInput{
		ClientFactory:    clientFactory,
		LogGroup:         cloudwatchLogGroup,
		LogStream:        cloudWatchLogStream,
		Namespace:        namespace,
		RegionalBatchers: regionalBatchers, // <— was RegionalCloudwatchMetricBatchers
		JobManager:       jobMgr,
		Store:            nauStore,
		ServiceConfig:    svcCfg,
		Logger:           log,
	})
	log.Info("initialized resource quota handler")

	// Handle the event
	if err := handler.HandleEvent(ctx, event); err != nil {
		return LambdaResponse{"error", err.Error()}, err
	}
	return LambdaResponse{"success", "Processed event successfully"}, nil
}

// main starts the Lambda function for resource quota monitoring
func main() {
	lambda.Start(HandleRequest)
}

// initLogger configures logging level from environment
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

// LoadServiceConfigInput contains parameters for loading service configuration.
type LoadServiceConfigInput struct {
	Logger logger.Logger
}

// loadServiceConfig loads service configuration from Lambda layer
func loadServiceConfig(input LoadServiceConfigInput) *serviceconfig.TopLevelServiceConfig {
	path := os.Getenv(lambdaLayerPathEnv)
	cfg, err := serviceconfig.LoadConfigFromFile(path, input.Logger)
	if err != nil {
		fatal(FatalInput{
			Logger:  input.Logger,
			ErrType: ErrLoadConfig,
			Cause:   err,
		})
	}
	if err = serviceconfig.ValidateQuotaMetricConfig(*cfg, input.Logger); err != nil {
		fatal(FatalInput{
			Logger:  input.Logger,
			ErrType: ErrInvalidConfig,
			Cause:   err,
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
	ClientFactory       awsclientsFactory.ClientFactory
	Logger              logger.Logger
}

// ensureLogGroup creates CloudWatch log groups/streams across all regions
func ensureLogGroup(input EnsureLogGroupInput) {
	log := input.Logger
	if err := cwlclient.EnsureGroupAndStreamAcrossRegions(input.Ctx, input.Regions,
		input.CloudwatchLogGroup,
		input.CloudwatchLogStream,
		input.ClientFactory); err != nil {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrEnsureLogGroup,
			Cause:   err,
		})
	}
	log.Info("log group and stream ready in regions: %v", input.Regions)
}

// InitMetricBatchersInput contains parameters for initializing metric batchers.
type InitMetricBatchersInput struct {
	Ctx           context.Context
	ClientFactory awsclientsFactory.ClientFactory
	Regions       []string
	Namespace     string
	LogGroup      string
	LogStream     string
	MaxCount      int
	MaxBytes      int64
	Logger        logger.Logger
}

// initMetricBatchers returns a map of region → metrics.Batcher.
func initMetricBatchers(input InitMetricBatchersInput) *safemap.TypedMap[metrics.Batcher] {
	log := input.Logger
	if log == nil {
		log = logger.Get()
	}

	// 1) Build a TypedMap of CloudWatch Logs clients
	cwlClients := safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}
	for _, region := range input.Regions {
		client, err := input.ClientFactory.CreateCloudWatchLogs(region)
		if err != nil {
			fatal(FatalInput{
				Logger:  log,
				ErrType: ErrCreateCWLClientForMetrics,
				Cause:   err,
			})
		}
		cwlClients.Store(region, client)
	}

	// 2) Create a single EMF flusher backed by that client map
	emfFlusher := emf.NewEMFFlusher(flusher.EMFFlusherConfig{
		CwlClientMap:  &cwlClients,
		LogGroupName:  input.LogGroup,
		LogStreamName: input.LogStream,
		Logger:        log,
	})

	// 3) Build one MetricsBatcher per region
	batchers := safemap.TypedMap[metrics.Batcher]{}
	for _, region := range input.Regions {
		batcher := metrics.NewMetricsBatcher(metrics.MetricsBatcherConfig{
			Namespace:  input.Namespace,
			LogGroup:   input.LogGroup,
			LogStream:  input.LogStream,
			Region:     region,
			MaxCount:   input.MaxCount,
			MaxBytes:   input.MaxBytes,
			EmfFlusher: emfFlusher,
			Logger:     log,
		})
		batchers.Store(region, batcher)
		log.Info("metrics batcher ready for region %s", region)
	}

	return &batchers
}

// BuildJobManagerInput contains parameters for building the job manager.
type BuildJobManagerInput struct {
	Ctx           context.Context
	ClientFactory awsclientsFactory.ClientFactory
	store         nau.AccountNauStore
	Regions       []string
	BatcherMap    *safemap.TypedMap[metrics.Batcher]
	Services      map[string]serviceconfig.ServiceConfig
	S3BucketName  string
	HomeRegion    string
	Logger        logger.Logger
}

// buildJobManager creates jobs for each service/region combination based on config
func buildJobManager(input BuildJobManagerInput) *job.JobManager {
	log := input.Logger
	clientFactory := input.ClientFactory

	jm := job.NewJobManager(job.JobManagerConfig{
		ParentCtx:  input.Ctx,
		Workers:    defaultWorkerCount,
		JobTimeout: defaultJobTimeout,
		BatcherMap: input.BatcherMap,
		Log:        log,
	})

	var (
		iamServiceQuotaRegion = utils.AwsRegionUSEast1.String()
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
								Logger:  log,
								ErrType: ErrCreateEC2Client,
								Cause:   err,
							})
						}
						sqClient, err := clientFactory.CreateServiceQuotas(region)
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateServiceQuotaClient,
								Cause:   err,
							})
						}
						job, err := networkinterfaces.NewNetworkInterfaceJob(networkinterfaces.NetworkInterfaceJobConfig{
							Ec2Client:           ec2Client,
							ServiceQuotasClient: sqClient,
							Logger:              log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateNetworkInterfacesJob,
								Cause:   err,
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
								Logger:  log,
								ErrType: ErrCreateEKSClient,
								Cause:   err,
							})
						}
						sqClient, err := clientFactory.CreateServiceQuotas(region)
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateServiceQuotaClient,
								Cause:   err,
							})
						}
						job, err := listcluster.NewListClusterJob(listcluster.ListClusterJobConfig{
							EksClient:           eksClient,
							ServiceQuotasClient: sqClient,
							Logger:              log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateListEKSClustersJob,
								Cause:   err,
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
								Logger:  log,
								ErrType: ErrCreateIAMClient,
								Cause:   err,
							})
						}
						sqClient, err := clientFactory.CreateServiceQuotas(iamServiceQuotaRegion)
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateServiceQuotaClient,
								Cause:   err,
							})
						}
						job, err := oidcproviders.NewOIDCProviderJob(oidcproviders.OIDCProviderJobConfig{
							IamClient:          iamClient,
							ServiceQuotasCliet: sqClient,
							Logger:             log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateIAMOIDCJob,
								Cause:   err,
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
								Logger:  log,
								ErrType: ErrCreateSupportClient,
								Cause:   err,
							})
						}
						job, err := iamroles.NewIamRoleJob(iamroles.IamRoleJobConfig{
							SupportClient: supportClient,
							Logger:        log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateIAMRolesJob,
								Cause:   err,
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
								Logger:  log,
								ErrType: ErrCreateSupportClient,
								Cause:   err,
							})
						}
						job, err := gp3storage.NewGp3StorageJob(gp3storage.Gp3StorageJobConfig{
							SupportClient: supportClient,
							Logger:        log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateGP3StorageJob,
								Cause:   err,
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
								Logger:  log,
								ErrType: ErrCreateEC2Client,
								Cause:   err,
							})
						}
						serviceQuotasClient, err := clientFactory.CreateServiceQuotas(region)
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateServiceQuotaClient,
								Cause:   err,
							})
						}
						nauCalc := nau.NewNauCalculatorV2(input.Ctx, ec2Client, input.store, log)
						job, err := vpcnau.NewVPCNAUJob(vpcnau.VPCNAUConfig{
							NauCalculator:       nauCalc,
							ServiceQuotasClient: serviceQuotasClient,
							Logger:              log,
						})
						if err != nil {
							fatal(FatalInput{
								Logger:  log,
								ErrType: ErrCreateVPCNAUJob,
								Cause:   err,
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
	ClientFactory    awsclientsFactory.ClientFactory    // AWS client factory
	LogGroup         string                             // CloudWatch log group name
	LogStream        string                             // CloudWatch log stream name
	Namespace        string                             // Metrics namespace
	RegionalBatchers *safemap.TypedMap[metrics.Batcher] // Region-specific metric batchers
	JobManager       *job.JobManager
	Store            nau.AccountNauStore                  // Job execution coordinator
	ServiceConfig    *serviceconfig.TopLevelServiceConfig // Service monitoring config
	Logger           logger.Logger                        // Logger instance
}

// initResourceQuotaHandler creates the main handler with all dependencies.
func initResourceQuotaHandler(input InitResourceQuotaHandlerInput) *handlers.ResourceQuotaHandler {
	log := input.Logger
	h, err := handlers.NewResourceQuotaHandler(handlers.ResourceQuotaHandlerConfig{
		ClientFactory:       input.ClientFactory,
		CloudwatchLogGroup:  input.LogGroup,
		CloudWatchLogStream: input.LogStream,
		Namespace:           input.Namespace,
		RegionalBatchers:    input.RegionalBatchers,
		JobManager:          input.JobManager,
		Store:               input.Store,
		ServiceConfig:       input.ServiceConfig,
		Logger:              log,
	})
	if err != nil {
		fatal(FatalInput{
			Logger:  log,
			ErrType: ErrCreateResourceQuotaHandler,
			Cause:   err,
		})
	}
	return h
}

// FatalInput contains parameters for the fatal function which logs an error and exits.
type FatalInput struct {
	Logger  logger.Logger
	ErrType error // e.g. ErrCannotLoadEnvVar
	Cause   error // the wrapped or underlying error, e.g. io.ErrNotExist
}

// fatal logs error and exits the process
func fatal(input FatalInput) {
	if input.Cause != nil {
		// prints "ErrCannotLoadEnvVar: open config.yaml: no such file or directory"
		input.Logger.Error("%v: %v", input.ErrType, input.Cause)
	} else {
		// just prints the primary error if no cause
		input.Logger.Error("%v", input.ErrType)
	}
	os.Exit(1)
}
