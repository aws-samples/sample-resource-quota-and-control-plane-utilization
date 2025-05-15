// Package main implements the EMF extension for Lambda functions.
// It handles flushing EMF records to CloudWatch Logs.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"os/signal"
	"time"

	"slices"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/extension"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	// Environment variables
	logLevelEnvVar        = "LOG_LEVEL"
	regionsEnvVar         = "REGIONS"
	cloudwatchGroupEnvVar = "CLOUDWATCH_LOG_GROUP"

	// Error messages
	ErrMsgCannotLoadEnvVar      = "cannot load environment variable"
	ErrMsgLoadAWSConfigFailed   = "failed to load AWS config"
	ErrMsgCloudWatchGroupNotSet = "cloudwatch log group not set"
	ErrMsgGlobFailed            = "failed to glob stash files"
	ErrMsgClientCreationFailed  = "failed to create CloudWatch Logs client"
	ErrMsgFileOpenFailed        = "failed to open file"
	ErrMsgMetaUnmarshalFailed   = "failed to unmarshal metadata"
)

var (
	// extensionName is the name of this extension, derived from the executable name
	extensionName = filepath.Base(os.Args[0])

	// extensionClient is the client used to communicate with the Lambda Extensions API
	extensionClient = extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))

	// printPrefix is used as a prefix for log messages
	printPrefix = fmt.Sprintf("[%s]", extensionName)
)

// main is the entry point for the EMF extension.
// It initializes the extension, processes events, and flushes EMF records on shutdown.
func main() {
	// Initialize logger based on environment variable
	logLevelValue := strings.ToLower(os.Getenv(logLevelEnvVar))
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
	log := logger.Get()
	log.Debug("%s log level set to %s", printPrefix, logLevelValue)

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize client factory
	clientFactory, err := factory.NewFactory(ctx, log)
	if err != nil {
		log.Error("%s %s: %v", printPrefix, ErrMsgLoadAWSConfigFailed, err)
		os.Exit(1)
	}
	log.Debug("%s client factory initialized", printPrefix)

	// Read regions from environment variable
	rawRegions := os.Getenv(regionsEnvVar)
	if rawRegions == "" {
		handleInitError(log, errors.New(ErrMsgCannotLoadEnvVar))
	}
	regions := strings.Split(rawRegions, ",")
	log.Info("%s regions: %s", printPrefix, regions)

	// Read CloudWatch log group from environment variable
	logGroup := os.Getenv(cloudwatchGroupEnvVar)
	if logGroup == "" {
		log.Error("%s %s", printPrefix, ErrMsgCloudWatchGroupNotSet)
		os.Exit(1)
	}

	// Create log stream and ensure it exists in all regions
	logStreamName := utils.MakeStreamName()
	err = cwlclient.EnsureGroupAndStreamAcrossRegions(
		ctx,
		regions,
		logGroup,
		logStreamName,
		clientFactory,
	)
	if err != nil {
		handleInitError(log, err)
	}
	log.Info("%s log group and stream created across all regions: %s", printPrefix, regions)

	// Set up signal handling for graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()

	// ── 0) ONE-TIME INIT BEFORE REGISTER ─────────────────────────────────

	// List all stash files
	stashDir := os.TempDir()
	files, err := filepath.Glob(filepath.Join(stashDir, "emf_*.ndjson"))
	if err != nil {
		log.Error("%s %s: %v", printPrefix, ErrMsgGlobFailed, err)
		os.Exit(1)
	}
	log.Debug("%s found %d files in %s", printPrefix, len(files), stashDir)

	// Build per-region CloudWatch Logs client map
	cwlMap := &safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}
	for _, f := range files {
		region := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "emf_"), ".ndjson")
		client, err := clientFactory.CreateCloudWatchLogs(region)
		if err != nil {
			log.Error("%s %s for region %s: %v", printPrefix, ErrMsgClientCreationFailed, region, err)
			continue
		}
		cwlMap.Store(region, client)
		log.Debug("%s CloudWatch Logs client created for region: %s", printPrefix, region)
	}

	// Create EMF flusher
	log.Debug("%s cloudwatch log group set to: %s", printPrefix, logGroup)
	flusher := emf.NewEMFFlusher(emf.EMFFlusherConfig{
		CwlClientMap:  cwlMap,
		LogStreamName: logStreamName,
		LogGroupName:  logGroup,
		Logger:        log,
	})
	log.Debug("%s flusher created", printPrefix)

	// ── 1) REGISTER ────────────────────────────────────────────────────────
	res, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}
	log.Info("%s Register response: %v", printPrefix, prettyPrint(res))

	// ── 2) PROCESS EVENTS UNTIL SHUTDOWN ──────────────────────────────────
	processEvents(ctx, log)
	log.Info("%s processEvents() returned, SHUTDOWN", printPrefix)

	// ── 3) ON SHUTDOWN, FLUSH ALL STASH FILES ─────────────────────────────
	files, _ = filepath.Glob(filepath.Join(os.TempDir(), "emf_*.ndjson"))
	log.Info("%s flushing %d files", printPrefix, len(files))
	var wg sync.WaitGroup
	for _, path := range files {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			f, err := os.Open(path)
			if err != nil {
				log.Error("%s %s: %v", printPrefix, ErrMsgFileOpenFailed, err)
				return
			}
			defer f.Close()

			var batch []emf.EMFRecord
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				var meta struct {
					AWS struct{ Timestamp int64 } `json:"_aws"`
				}
				line := scanner.Bytes()
				if err := json.Unmarshal(line, &meta); err != nil {
					log.Error("%s %s for %s: %v", printPrefix, ErrMsgMetaUnmarshalFailed, path, err)
					continue
				}
				batch = append(batch, emf.EMFRecord{
					Payload:   slices.Clone(line),
					TimeStamp: time.UnixMilli(meta.AWS.Timestamp),
				})
				log.Debug("%s read from %s: %s", printPrefix, path, string(line))
			}
			if len(batch) > 0 {
				flusher.Flush(context.Background(), filepath.Base(path), batch)
				log.Info("%s flushed %s", printPrefix, path)
			}
			os.Truncate(path, 0)
		}(path)
	}
	wg.Wait()
	log.Info("%s flush complete, exiting", printPrefix)
}

// processEvents continuously processes events from the Lambda Extensions API
// until a shutdown event is received or the context is canceled.
func processEvents(ctx context.Context, log logger.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Debug("%s Waiting for event...", printPrefix)
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				log.Error("%s Error: %v", printPrefix, err)
				return
			}
			log.Debug("%s Received event: %s", printPrefix, prettyPrint(res))
			if res.EventType == extension.Shutdown {
				log.Info("%s Received SHUTDOWN event", printPrefix)
				return
			}
		}
	}
}

// prettyPrint formats a value as indented JSON for logging purposes.
func prettyPrint(v any) string {
	data, _ := json.MarshalIndent(v, "", "\t")
	return string(data)
}

// handleInitError logs an initialization error and exits the program.
func handleInitError(logger logger.Logger, err error) {
	logger.Error("%s error initializing service: %v", printPrefix, err)
	os.Exit(1)
}
