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
	RateLimitHandler *handlers.RateLimitHandler
	logStreamName    = utils.MakeStreamName()
	flusherInterval  time.Duration
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
	cannotLoadEnvVar = "cannot load env var"
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
	appLogger.Info("log level is %v", logLevel)

	// read the environment variables
	cloudwatchLogGroup := os.Getenv(cloudwatchLogGroupEnv)
	// if the environment variable is not set, panic
	if cloudwatchLogGroup == "" {
		HandleInitError(appLogger, errors.New(cannotLoadEnvVar))
	}
	appLogger.Info("loaded cloudwatch log group env var %v", cloudwatchLogGroup)
	namespace := os.Getenv(metricNamespaceEnv)
	// if the environment variable is not set, panic
	if namespace == "" {
		HandleInitError(appLogger, errors.New(cannotLoadEnvVar))
	}
	appLogger.Info("loaded metric namespace env var %v", namespace)

	ctx := context.Background()

	// read the environment variables to get the regions
	rawRegions := os.Getenv(regionsEnv)
	if rawRegions == "" {
		HandleInitError(appLogger, errors.New(cannotLoadEnvVar))
	}
	regions := strings.Split(rawRegions, ",")
	appLogger.Info("loaded regions %v", regions)

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
	appLogger.Info("flush interval %v", flusherInterval)

	// we need to make sure the log group and name are created
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		HandleInitError(appLogger, err)
	}

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
	appLogger.Info("log group and streams creating successfully in all regions")

	// load a safemap of cloudtrail emf batchers for each region
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
	flusher := emf.NewEMFFlusher(emf.EMFFlusherConfig{
		CwlClientMap:  cwlClientMap,
		LogStreamName: logStreamName,
		LogGroupName:  cloudwatchLogGroup,
		Logger:        appLogger,
	})

	// create cloud trail flie batcher
	cloudtrailFileBatcher := cloudtrailemfbatcher.NewCTFileBatcher(cloudtrailemfbatcher.CTFileBatcherConfig{
		ParentCtx:     ctx,
		Namespace:     namespace,
		MetricName:    "CallCount",
		BaseDir:       os.TempDir(),
		MaxCount:      maxEvents,
		MaxBytes:      maxBytes,
		FlushInterval: flusherInterval,
		EmfFlusher:    flusher,
		Logger:        appLogger,
	})

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
