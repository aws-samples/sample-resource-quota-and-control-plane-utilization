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
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
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
	// 1) logger
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

	// 2) ctx + shutdown signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()

	// 3) AWS factory
	clientFactory, err := factory.NewFactory(ctx, log)
	if err != nil {
		log.Error("%s %s: %v", printPrefix, ErrMsgLoadAWSConfigFailed, err)
		os.Exit(1)
	}

	// 4) env
	rawRegions := os.Getenv(regionsEnvVar)
	if rawRegions == "" {
		handleInitError(log, fmt.Errorf("%w, %s", ErrMsgCannotLoadEnvVar, regionsEnvVar))
	}
	regions := strings.Split(rawRegions, ",")

	logGroup := os.Getenv(cloudwatchGroupEnvVar)
	if logGroup == "" {
		handleInitError(log, fmt.Errorf("%s %w, %s", printPrefix, ErrMsgCannotLoadEnvVar, cloudwatchGroupEnvVar))
	}

	namespace := os.Getenv(metricNamespaceEnvVar)
	if namespace == "" {
		handleInitError(log, fmt.Errorf("%s %w, %s", printPrefix, ErrMsgCannotLoadEnvVar, metricNamespaceEnvVar))
	}

	// 5) ensure CWL
	stream := utils.MakeStreamName()
	if err := cwlclient.EnsureGroupAndStreamAcrossRegions(
		ctx, regions, logGroup, stream, clientFactory); err != nil {
		handleInitError(log, err)
	}

	// 6) build CWL client map
	cwlMap := &safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}
	for _, r := range regions {
		c, err := clientFactory.CreateCloudWatchLogs(r)
		if err != nil {
			handleInitError(log, err)
		}
		cwlMap.Store(r, c)
	}

	// 7) flusher
	ef := emf.NewEMFFlusher(flusher.EMFFlusherConfig{
		CwlClientMap:  cwlMap,
		LogGroupName:  logGroup,
		LogStreamName: stream,
		Logger:        log,
	})

	// 8) one‐time: write initial last‐flush timestamp so we have a baseline
	lastFlushFile := filepath.Join(os.TempDir(), "lastFlushTimestamp.txt")
	now := time.Now().UTC()
	if err := os.WriteFile(lastFlushFile, []byte(now.Format(time.RFC3339Nano)), 0644); err != nil {
		log.Error("%s failed to write initial lastFlushTimestamp: %v", printPrefix, err)
	}

	// 9) build a CTFileBatcher and register
	agg := cloudtrail.NewDefaultEMFAggregator()
	batcher := cloudtrail.NewCTFileBatcher(cloudtrail.CTFileBatcherConfig{
		BaseDir:          os.TempDir(),
		Namespace:        namespace,
		MetricName:       metricNameRequestsPerSecond,
		MaxCount:         0, // not used on shutdown flush
		MaxBytes:         0, // not used on shutdown flush
		PropagateInvoker: false,
		Aggregator:       agg,
		EmfFlusher:       ef,
		Logger:           log,
	})

	// 10) register with Lambda Extensions API
	res, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}
	log.Info("%s Registered: %v", printPrefix, prettyPrint(res))

	// 11) hang until shutdown
	processEvents(ctx, log)
	log.Info("%s Got SHUTDOWN event, flushing all", printPrefix)

	// 12) on shutdown, do one last aggregated flush using real now()
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
