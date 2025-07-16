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
	appLogger := initLogger()
	writeInitTimestamp(appLogger)
	config := loadConfig(appLogger)
	clientFactory := createClientFactory(appLogger)
	setupCloudWatchLogs(appLogger, config, clientFactory)
	initializeHandler(appLogger, config, clientFactory)
	appLogger.Info("initialization complete")
	lambda.Start(HandleRequest)
}

func initLogger() logger.Logger {
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
	return appLogger
}

func writeInitTimestamp(appLogger logger.Logger) {
	initTime := time.Now().UTC()
	initTs := initTime.Format(time.RFC3339Nano)
	initFile := filepath.Join(os.TempDir(), LambdaInitTimestampFileName)
	if err := os.WriteFile(initFile, []byte(initTs), 0o644); err != nil {
		appLogger.Error("failed to write init timestamp to file: %v", err)
	} else {
		appLogger.Info("wrote lambda init timestamp: %s", initTs)
	}
}

type Config struct {
	CloudwatchLogGroup string
	Namespace          string
	Regions            []string
	PropagateInvoker   bool
}

func loadConfig(appLogger logger.Logger) Config {
	cloudwatchLogGroup := os.Getenv(cloudwatchLogGroupEnv)
	if cloudwatchLogGroup == "" {
		HandleInitError(appLogger, ErrCannotLoadEnvVar)
	}
	namespace := os.Getenv(metricNamespaceEnv)
	if namespace == "" {
		HandleInitError(appLogger, ErrCannotLoadEnvVar)
	}
	rawRegions := os.Getenv(regionsEnv)
	if rawRegions == "" {
		HandleInitError(appLogger, ErrCannotLoadEnvVar)
	}
	regions := strings.Split(rawRegions, ",")
	propagateInvoker, _ := strconv.ParseBool(os.Getenv(propagateInvokerEnv))
	appLogger.Info("config loaded: logGroup=%s namespace=%s regions=%v propagateInvoker=%v", 
		cloudwatchLogGroup, namespace, regions, propagateInvoker)
	return Config{
		CloudwatchLogGroup: cloudwatchLogGroup,
		Namespace:          namespace,
		Regions:            regions,
		PropagateInvoker:   propagateInvoker,
	}
}

func createClientFactory(appLogger logger.Logger) factory.ClientFactory {
	ctx := context.Background()
	clientFactory, err := factory.NewFactory(ctx, appLogger)
	if err != nil {
		HandleInitError(appLogger, err)
	}
	return clientFactory
}

func setupCloudWatchLogs(appLogger logger.Logger, config Config, clientFactory factory.ClientFactory) {
	ctx := context.Background()
	if err := cwlclient.EnsureGroupAndStreamAcrossRegions(
		ctx, config.Regions, config.CloudwatchLogGroup, logStreamName, clientFactory); err != nil {
		HandleInitError(appLogger, err)
	}
}

func initializeHandler(appLogger logger.Logger, config Config, clientFactory factory.ClientFactory) {
	cwlClientMap := buildClientMap(appLogger, config.Regions, clientFactory)
	flusher := createEMFFlusher(appLogger, config.CloudwatchLogGroup, cwlClientMap)
	batcher := createCloudTrailBatcher(appLogger, config, flusher)
	createRateLimitHandler(appLogger, config.Namespace, batcher)
}

func buildClientMap(appLogger logger.Logger, regions []string, clientFactory factory.ClientFactory) safestore.Store[cwlclient.CloudWatchLogsClient] {
	cwlClientMap := safestore.NewSyncStore[cwlclient.CloudWatchLogsClient]()
	for _, r := range regions {
		client, err := clientFactory.CreateCloudWatchLogs(r)
		if err != nil {
			HandleInitError(appLogger, err)
		}
		cwlClientMap.Store(r, client)
	}
	return cwlClientMap
}

func createEMFFlusher(appLogger logger.Logger, logGroup string, cwlClientMap safestore.Store[cwlclient.CloudWatchLogsClient]) emf.EMFFlusher {
	flusher, err := emf.NewEMFFlusher(flusher.EMFFlusherConfig{
		CwlClientMap:  cwlClientMap,
		LogStreamName: logStreamName,
		LogGroupName:  logGroup,
		Logger:        appLogger,
	})
	if err != nil {
		HandleInitError(appLogger, err)
	}
	return flusher
}

func createCloudTrailBatcher(appLogger logger.Logger, config Config, flusher emf.EMFFlusher) cloudtrail.Batcher {
	lastFlushFile := filepath.Join(os.TempDir(), LastFlushTimestampFileName)
	batcher, err := cloudtrail.NewCTFileBatcher(cloudtrail.CTFileBatcherConfig{
		BaseDir:            os.TempDir(),
		Namespace:          config.Namespace,
		MetricName:         metricNameRequestsPerSecond,
		LastFlushFilePath:  lastFlushFile,
		LambdaInitFilePath: LambdaInitTimestampFileName,
		PropagateInvoker:   config.PropagateInvoker,
		EmfFlusher:         flusher,
		Logger:             appLogger,
	})
	if err != nil {
		HandleInitError(appLogger, err)
	}
	return batcher
}

func createRateLimitHandler(appLogger logger.Logger, namespace string, batcher cloudtrail.Batcher) {
	var err error
	RateLimitHandler, err = handlers.NewRateLimitHandler(handlers.RateLimitHandlerConfig{
		Batcher:   batcher,
		Namespace: namespace,
		Logger:    appLogger,
	})
	if err != nil {
		HandleInitError(appLogger, err)
	}
}

// HandleInitError logs initialization errors and exits
func HandleInitError(logger logger.Logger, err error) {
	logger.Error("error initializing service: %v", err)
	os.Exit(1)
}
