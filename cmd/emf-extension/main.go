// Package main implements the EMF extension for Lambda functions.
// It handles flushing EMF records to CloudWatch Logs.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/cloudtrail"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/flusher"
	"github.com/outofoffice3/aws-samples/geras/internal/extension"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Environment variables
	logLevelEnvVar              = "LOG_LEVEL"
	regionsEnvVar               = "REGIONS"
	cloudwatchGroupEnvVar       = "CLOUDWATCH_LOG_GROUP"
	metricNamespaceEnvVar       = "METRIC_NAMESPACE"
	metricNameRequestsPerSecond = "RequestsPerSecond"
)

var (
	extensionName   = filepath.Base(os.Args[0])
	extensionClient = extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	printPrefix     = fmt.Sprintf("[%s]", extensionName)

	// Error messages
	ErrMsgCannotLoadEnvVar      error = errors.New("cannot load environment variable")
	ErrMsgLoadAWSConfigFailed   error = errors.New("failed to load AWS config")
	ErrMsgCloudWatchGroupNotSet error = errors.New("cloudwatch log group not set")
)

func main() {
	log := setupLogger()
	ctx := setupContext()
	clientFactory := setupAWSFactory(ctx, log)
	config := loadEnvironmentConfig(log)
	batcher := setupBatcher(ctx, log, clientFactory, config)
	runExtension(ctx, log, batcher)
}

type envConfig struct {
	regions   []string
	logGroup  string
	namespace string
}

func setupLogger() logger.Logger {
	lvl := strings.ToLower(os.Getenv(logLevelEnvVar))
	var ll logger.LogLevel
	switch lvl {
	case "debug":
		ll = logger.DEBUG
	case "info":
		ll = logger.INFO
	case "warn":
		ll = logger.WARN
	case "error":
		ll = logger.ERROR
	default:
		ll = logger.INFO
	}
	logger.Init(ll, os.Stdout)
	log := logger.Get()
	log.Debug("%s log level %s", printPrefix, lvl)
	return log
}

func setupContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()
	return ctx
}

func setupAWSFactory(ctx context.Context, log logger.Logger) factory.ClientFactory {
	clientFactory, err := factory.NewFactory(ctx, log)
	if err != nil {
		log.Error("%s %s: %v", printPrefix, ErrMsgLoadAWSConfigFailed, err)
		os.Exit(1)
	}
	return clientFactory
}

func loadEnvironmentConfig(log logger.Logger) envConfig {
	rawRegions := os.Getenv(regionsEnvVar)
	if rawRegions == "" {
		handleInitError(log, fmt.Errorf("%w, %s", ErrMsgCannotLoadEnvVar, regionsEnvVar))
	}
	logGroup := os.Getenv(cloudwatchGroupEnvVar)
	if logGroup == "" {
		handleInitError(log, fmt.Errorf("%s %w, %s", printPrefix, ErrMsgCannotLoadEnvVar, cloudwatchGroupEnvVar))
	}
	namespace := os.Getenv(metricNamespaceEnvVar)
	if namespace == "" {
		handleInitError(log, fmt.Errorf("%s %w, %s", printPrefix, ErrMsgCannotLoadEnvVar, metricNamespaceEnvVar))
	}
	return envConfig{
		regions:   strings.Split(rawRegions, ","),
		logGroup:  logGroup,
		namespace: namespace,
	}
}

func setupBatcher(ctx context.Context, log logger.Logger, clientFactory factory.ClientFactory, config envConfig) cloudtrail.Batcher {
	stream := utils.MakeStreamName()
	if err := cwlclient.EnsureGroupAndStreamAcrossRegions(ctx, config.regions, config.logGroup, stream, clientFactory); err != nil {
		handleInitError(log, err)
	}

	cwlMap := safestore.NewSyncStore[cwlclient.CloudWatchLogsClient]()
	for _, r := range config.regions {
		c, err := clientFactory.CreateCloudWatchLogs(r)
		if err != nil {
			handleInitError(log, err)
		}
		cwlMap.Store(r, c)
	}

	ef, err := emf.NewEMFFlusher(flusher.EMFFlusherConfig{
		CwlClientMap:  cwlMap,
		LogGroupName:  config.logGroup,
		LogStreamName: stream,
		Logger:        log,
	})
	if err != nil {
		handleInitError(log, err)
	}

	lastFlushFile := filepath.Join(os.TempDir(), "lastFlushTimestamp.txt")
	lambdaInitFile := filepath.Join(os.TempDir(), "lambdaInitTimestamp.txt")
	now := time.Now().UTC()
	if err := os.WriteFile(lastFlushFile, []byte(now.Format(time.RFC3339Nano)), 0644); err != nil {
		log.Error("%s failed to write initial lastFlushTimestamp: %v", printPrefix, err)
	}
	if err := os.WriteFile(lambdaInitFile, []byte(now.Format(time.RFC3339Nano)), 0644); err != nil {
		log.Error("%s failed to write lambda init timestamp: %v", printPrefix, err)
	}

	batcher, err := cloudtrail.NewCTFileBatcher(cloudtrail.CTFileBatcherConfig{
		BaseDir:            os.TempDir(),
		Namespace:          config.namespace,
		MetricName:         metricNameRequestsPerSecond,
		LastFlushFilePath:  lastFlushFile,
		LambdaInitFilePath: "lambdaInitTimestamp.txt",
		PropagateInvoker:   false,
		EmfFlusher:         ef,
		Logger:             log,
	})
	if err != nil {
		handleInitError(log, err)
	}
	return batcher
}

func runExtension(ctx context.Context, log logger.Logger, batcher cloudtrail.Batcher) {
	res, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}
	log.Info("%s Registered: %v", printPrefix, prettyPrint(res))

	processEvents(ctx, log)
	log.Info("%s Got SHUTDOWN event, flushing all", printPrefix)

	if err := batcher.FlushAll(context.Background(), time.Now().UTC()); err != nil {
		log.Error("%s shutdown flush error: %v", printPrefix, err)
	}
	log.Info("%s flush complete, exiting", printPrefix)
}

// processEvents unchanged…

func processEvents(ctx context.Context, log logger.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				log.Error("%s NextEvent error: %v", printPrefix, err)
				return
			}
			if res.EventType == extension.Shutdown {
				return
			}
		}
	}
}

func prettyPrint(v any) string {
	data, _ := json.MarshalIndent(v, "", "\t")
	return string(data)
}

func handleInitError(log logger.Logger, err error) {
	log.Error("%s init error: %v", printPrefix, err)
	os.Exit(1)
}
