package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	cloudtrail "github.com/outofoffice3/aws-samples/geras/internal/emf/batch/cloudtrail"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/flusher"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
	"github.com/outofoffice3/aws-samples/geras/internal/handlers"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

var (
	// Global handler for processing SQS events containing CloudTrail records
	RateLimitHandler            *handlers.RateLimitHandler
	logStreamName               = utils.MakeStreamName()
	metricNameRequestsPerSecond = "RequestsPerSecond"
)

const (
	// known service variables
	LambdaInitTimestampFileName = "lambdaInitTimestamp.txt"
	LastFlushTimestampFileName  = "lastFlushTimestamp.txt"

	// environment variables
	regionsEnv            = "REGIONS"
	logLevelEnv           = "LOG_LEVEL"
	cloudwatchLogGroupEnv = "CLOUDWATCH_LOG_GROUP"
	metricNamespaceEnv    = "METRIC_NAMESPACE"
	flushIntervalEnv      = "FLUSH_INTERVAL"
	propagateInvokerEnv   = "PROPAGATE_INVOKER"
)

var (
	// error
	ErrCannotLoadEnvVar  = errors.New("cannot load env var")
	ErrServiceInitFailed = errors.New("failed to initialize service")
)

// HandleRequest processes SQS events containing CloudTrail records and publishes CallCount metrics
func HandleRequest(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	appLogger := logger.Get()
	failedItems, err := RateLimitHandler.HandleEvent(ctx, event)
	if err != nil {
		appLogger.Error("failed to handle event: %v", err)
		return events.SQSEventResponse{}, err
	}
	if len(failedItems) > 0 {
		appLogger.Error("failed to handle some items: %v", failedItems)
		return events.SQSEventResponse{
			BatchItemFailures: failedItems,
		}, nil
	}
	appLogger.Info("successfully handled event, %d records", len(event.Records))
	return events.SQSEventResponse{}, nil
}

func main() {
	// 1) Logger setup
	lvl := strings.ToLower(os.Getenv(logLevelEnv))
	var logLevel logger.LogLevel
	switch lvl {
	case "debug":
		logLevel = logger.DEBUG
	case "warn":
		logLevel = logger.WARN
	case "error":
		logLevel = logger.ERROR
	default:
		logLevel = logger.INFO
	}
	logger.Init(logLevel, os.Stdout)
	appLogger := logger.Get()
	appLogger.Info("Initializing the function, log level = %s", lvl)
	initTime := time.Now().UTC()
	initTs := initTime.Format(time.RFC3339Nano)
	initFile := filepath.Join(os.TempDir(), LambdaInitTimestampFileName)
	if err := os.WriteFile(initFile, []byte(initTs), 0o644); err != nil {
		appLogger.Error("failed to write init timestamp to file: %v", err)
	} else {
		appLogger.Info("wrote lambda init timestamp: %s", initTs)
	}

	// 2) Read required env vars
	cloudwatchLogGroup := os.Getenv(cloudwatchLogGroupEnv)
	if cloudwatchLogGroup == "" {
		HandleInitError(appLogger, ErrCannotLoadEnvVar)
	}
	appLogger.Info("cloudwatch log group env var value=%s", cloudwatchLogGroup)

	namespace := os.Getenv(metricNamespaceEnv)
	if namespace == "" {
		HandleInitError(appLogger, ErrCannotLoadEnvVar)
	}
	appLogger.Info("metric namespace env var value=%s", namespace)

	rawRegions := os.Getenv(regionsEnv)
	if rawRegions == "" {
		HandleInitError(appLogger, ErrCannotLoadEnvVar)
	}
	regions := strings.Split(rawRegions, ",")
	appLogger.Info("regions env var value=%v", regions)

	propagateInvoker, _ := strconv.ParseBool(os.Getenv(propagateInvokerEnv))
	appLogger.Info("propagate invoker env var value=%s", propagateInvoker)

	// 3) AWS client factory
	ctx := context.Background()
	clientFactory, err := factory.NewFactory(ctx, appLogger)
	if err != nil {
		HandleInitError(appLogger, err)
	}

	// 4) Ensure log group & stream across regions
	if err := cwlclient.EnsureGroupAndStreamAcrossRegions(
		ctx, regions, cloudwatchLogGroup, logStreamName, clientFactory); err != nil {
		HandleInitError(appLogger, err)
	}

	// 5) Build CWL client map
	cwlClientMap := safestore.NewSyncStore[cwlclient.CloudWatchLogsClient]()
	for _, r := range regions {
		client, err := clientFactory.CreateCloudWatchLogs(r)
		if err != nil {
			HandleInitError(appLogger, err)
		}
		cwlClientMap.Store(r, client)
	}

	// 6) Shared EMF flusher
	flusher, err := emf.NewEMFFlusher(flusher.EMFFlusherConfig{
		CwlClientMap:  cwlClientMap,
		LogStreamName: logStreamName,
		LogGroupName:  cloudwatchLogGroup,
		Logger:        appLogger,
	})
	if err != nil {
		HandleInitError(appLogger, err)
	}

	// 7) CloudTrail EMF batcher
	lastFlushFile := filepath.Join(os.TempDir(), LastFlushTimestampFileName)
	cloudtrailBatcher, err := cloudtrail.NewCTFileBatcher(cloudtrail.CTFileBatcherConfig{
		BaseDir:            os.TempDir(),
		Namespace:          namespace,
		MetricName:         metricNameRequestsPerSecond,
		LastFlushFilePath:  lastFlushFile,
		LambdaInitFilePath: LambdaInitTimestampFileName,
		PropagateInvoker:   propagateInvoker,
		EmfFlusher:         flusher,
		Logger:             appLogger,
	})
	if err != nil {
		HandleInitError(appLogger, err)
	}

	// 8) RateLimit handler
	RateLimitHandler, err = handlers.NewRateLimitHandler(handlers.RateLimitHandlerConfig{
		Batcher:   cloudtrailBatcher,
		Namespace: namespace,
		Logger:    appLogger,
	})
	if err != nil {
		HandleInitError(appLogger, err)
	}

	appLogger.Info("initialization complete")
	lambda.Start(HandleRequest)
}

// HandleInitError logs initialization errors and exits
func HandleInitError(logger logger.Logger, err error) {
	logger.Error("error initializing service: %v", err)
	os.Exit(1)
}
