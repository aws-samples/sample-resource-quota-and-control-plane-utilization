package main

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	cloudtrailemfbatcher "github.com/outofoffice3/aws-samples/geras/internal/emfbatcher/cloudtrail"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"

	"github.com/outofoffice3/aws-samples/geras/internal/handlers"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

var (
	RateLimitHandler    *handlers.RateLimitHandler
	logStreamName       = utils.MakeStreamName()
	flusherInterval     time.Duration
	metricNameCallCount = "CallCount"
)

const (

	// known service variables
	maxEvents = 10000
	maxBytes  = 1 << 20
	overhead  = 26

	// environment variables
	regionsEnv            = "REGIONS"
	logLevelEnv           = "LOG_LEVEL"
	cloudwatchLogGroupEnv = "CLOUDWATCH_LOG_GROUP"
	metricNamespaceEnv    = "METRIC_NAMESPACE"
	flushIntervalEnv      = "FLUSH_INTERVAL"

	// error messages
	ErrMsgCannotLoadEnvVar  = "cannot load env var"
	ErrMsgServiceInitFailed = "failed to initialize service"
)

func HandleRequest(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	appLogger := logger.Get()
	failedItems, err := RateLimitHandler.HandleEvent(ctx, event)
	if err != nil {
		appLogger.Error("failed to handle event", err)
		return events.SQSEventResponse{}, err
	}
	// if we have failed items, we need to return them
	if len(failedItems) > 0 {
		appLogger.Error("failed to handle some items, %v", failedItems)
		return events.SQSEventResponse{
			BatchItemFailures: failedItems,
		}, nil
	}
	appLogger.Info("successfully handled event, %v records", len(event.Records))
	return events.SQSEventResponse{}, nil
}

func main() {

	// Here is where we will initialize the function when it cold starts
	// 1. Intialize the logger
	// 2. Make sure the log group and stream is set up in all regions
	// 3. Load a map of cloudwachlog clients per region

	// Intialize the logger
	logLevelValue := strings.ToLower(os.Getenv(logLevelEnv))
	var logLevel logger.LogLevel
	switch logLevelValue {
	case "debug":
		logLevel = logger.DEBUG
	case "info":
		logLevel = logger.INFO
	case "warn":
		logLevel = logger.WARN
	case "error":
		logLevel = logger.ERROR
	default:
		logLevel = logger.INFO
	}
	logger.Init(logLevel, os.Stdout)
	appLogger := logger.Get()
	appLogger.Info("Initializing the function")
	appLogger.Info("log value is %v", logLevelValue)

	// read the environment variables
	cloudwatchLogGroup := os.Getenv(cloudwatchLogGroupEnv)
	// if the environment variable is not set, handle error
	if cloudwatchLogGroup == "" {
		HandleInitError(appLogger, errors.New(ErrMsgCannotLoadEnvVar))
	}
	appLogger.Info("loaded cloudwatch log group env var %v", cloudwatchLogGroup)
	namespace := os.Getenv(metricNamespaceEnv)
	// if the environment variable is not set, handle error
	if namespace == "" {
		HandleInitError(appLogger, errors.New(ErrMsgCannotLoadEnvVar))
	}
	appLogger.Info("loaded metric namespace env var %v", namespace)

	// read the environment variables to get the regions
	rawRegions := os.Getenv(regionsEnv)
	if rawRegions == "" {
		HandleInitError(appLogger, errors.New(ErrMsgCannotLoadEnvVar))
	}
	regions := strings.Split(rawRegions, ",")
	appLogger.Info("loaded regions %v", regions)

	// readn environment variable to get flush interval
	rawInterval := os.Getenv(flushIntervalEnv)
	if rawInterval == "" {
		rawInterval = "45"
		appLogger.Warn("flush interval not set, defaulting to 45 seconds")
	}
	secs, err := strconv.Atoi(rawInterval)
	if err != nil {
		HandleInitError(appLogger, err)
	}
	flusherInterval = time.Duration(secs) * time.Second
	appLogger.Info("loaded flush interval %v", flusherInterval)

	ctx := context.Background()

	// load aws config
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		HandleInitError(appLogger, err)
	}

	// we need to ensure the log groups and streams exists in all regions
	err = cwlclient.EnsureGroupAndStreamAcrossRegions(
		ctx,
		regions,
		cloudwatchLogGroup,
		logStreamName,
		makeFactory(cfg),
	)
	if err != nil {
		HandleInitError(appLogger, err)
	}
	appLogger.Info("log group %s and stream %s created successfully in all regions", cloudwatchLogGroup, logStreamName)

	// load a safemap of cloudwatch log clients for each region
	cwlClientMap := &safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}
	for _, region := range regions {
		client, err := cwlclient.NewCloudWatchLogsClient(cfg, region)
		if err != nil {
			HandleInitError(appLogger, err)
		}

		// add client to map
		cwlClientMap.Store(region, client)
		appLogger.Info("loaded cloudwatch client for region %s", region)
	}

	// create emf flusher
	// this is used to read from the /tmp and push EMF's to cloudtrail logs
	flusher := emf.NewEMFFlusher(emf.EMFFlusherConfig{
		CwlClientMap:  cwlClientMap,
		LogStreamName: logStreamName,
		LogGroupName:  cloudwatchLogGroup,
		Logger:        appLogger,
	})

	// create cloud trail flie batchers
	// it will ingest cloudtrail records, convert them to emf
	// and store them in /tmp until the batch flush conditions are met
	cloudtrailFileBatcher := cloudtrailemfbatcher.NewCTFileBatcher(cloudtrailemfbatcher.CTFileBatcherConfig{
		ParentCtx:     ctx,
		Namespace:     namespace,
		MetricName:    metricNameCallCount,
		BaseDir:       os.TempDir(),
		MaxCount:      maxEvents,
		MaxBytes:      maxBytes,
		FlushInterval: flusherInterval,
		EmfFlusher:    flusher,
		Logger:        appLogger,
	})

	// initialize handler
	RateLimitHandler, err = handlers.NewRateLimitHandler(handlers.RateLimitHandlerConfig{
		CloudTrailEmfFileBatcher: cloudtrailFileBatcher,
		Namespace:                namespace,
		Logger:                   appLogger,
	})
	if err != nil {
		HandleInitError(appLogger, err)
	}

	appLogger.Info("initialization complete")
	lambda.Start(HandleRequest)
}

// Handle Init Error
func HandleInitError(logger logger.Logger, err error) {
	logger.Error("error initializing service: %v", err)
	os.Exit(1)
}

// re-useable function to create cloudwatch logs client
func makeFactory(cfg aws.Config) cwlclient.ClientFactory {
	return func(region string) (cwlclient.CloudWatchLogsClient, error) {
		cfg.Region = region
		client, err := cwlclient.NewCloudWatchLogsClient(cfg, region)
		if err != nil {
			return nil, err
		}
		return client, nil
	}
}
