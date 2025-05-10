// cmd/cloudtrail-extension/main.go
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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/extension"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	logLevelEnvVar        = "LOG_LEVEL"
	regionsEnvVar         = "REGIONS"
	cloudwatchGroupEnvVar = "CLOUDWATCH_LOG_GROUP"

	ErrMsgCannotLoadEnvVar = "cannot load environment variable"
)

var (
	extensionName   = filepath.Base(os.Args[0])
	extensionClient = extension.NewClient(os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	printPrefix     = fmt.Sprintf("[%s]", extensionName)
)

func main() {
	// read log level from env vars
	// if not set, default to INFO
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

	ctx, cancel := context.WithCancel(context.Background())
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Error("%s load AWS config failed: %v", printPrefix, err)
		os.Exit(1)
	}

	// read the environment variables to get the regions
	rawRegions := os.Getenv(regionsEnvVar)
	if rawRegions == "" {
		HandleInitError(log, errors.New(ErrMsgCannotLoadEnvVar))
	}
	regions := strings.Split(rawRegions, ",")
	log.Info("%s regions %s", printPrefix, regions)
	logGroup := os.Getenv(cloudwatchGroupEnvVar)

	if logGroup == "" {
		log.Error("%s cloudwatch log group not set", printPrefix)
		os.Exit(1)
	}

	logStreamName := utils.MakeStreamName()
	err = cwlclient.EnsureGroupAndStreamAcrossRegions(
		ctx,
		regions,
		logGroup,
		logStreamName,
		makeFactory(awsCfg),
	)
	if err != nil {
		HandleInitError(log, err)
	}
	log.Info("%s log group and stream created across all regions %s", printPrefix, regions)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()

	// ── 0) ONE-TIME INIT BEFORE REGISTER ─────────────────────────────────

	// a) list all stash files
	stashDir := os.TempDir()
	files, err := filepath.Glob(filepath.Join(stashDir, "emf_*.ndjson"))
	if err != nil {
		log.Error("%s glob failed: %v", printPrefix, err)
		os.Exit(1)
	}
	log.Debug("%s found %d files in %s", printPrefix, len(files), stashDir)

	// b) build per-region CWL client map
	cwlMap := &safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}
	for _, f := range files {
		region := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "emf_"), ".ndjson")
		client, err := cwlclient.NewCloudWatchLogsClient(awsCfg, region)
		if err != nil {
			log.Error("%s CWL client creation failed for region %s: %v", printPrefix, region, err)
			continue
		}
		cwlMap.Store(region, client)
		log.Debug("%s CWL client created for region %s", printPrefix, region)
	}

	// c) create EMF flusher

	log.Debug("%s cloudwatch log group set to %s", printPrefix, logGroup)
	flusher := emf.NewEMFFlusher(emf.EMFFlusherConfig{
		CwlClientMap:  cwlMap,
		LogStreamName: utils.MakeStreamName(),
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
				log.Error(printPrefix+" open %s: %v", path, err)
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
					log.Error(printPrefix+" meta unmarshal %s: %v", path, err)
					continue
				}
				batch = append(batch, emf.EMFRecord{
					Payload:   slices.Clone(line),
					TimeStamp: time.UnixMilli(meta.AWS.Timestamp),
				})
				log.Debug(printPrefix+" read %s: %s", path, string(line))
			}
			if len(batch) > 0 {
				flusher.Flush(context.Background(), filepath.Base(path), batch)
				log.Info(printPrefix+" flushed %s", path)
			}
			os.Truncate(path, 0)
		}(path)
	}
	wg.Wait()
	log.Info(printPrefix + " flush complete, exiting")
}

func processEvents(ctx context.Context, log logger.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Debug(printPrefix + " Waiting for event...")
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				log.Error(printPrefix, "Error:", err)
				return
			}
			log.Debug(printPrefix+" Received event: %s", prettyPrint(res))
			if res.EventType == extension.Shutdown {
				log.Info(printPrefix + " Received SHUTDOWN event")
				return
			}
		}
	}
}

func prettyPrint(v any) string {
	data, _ := json.MarshalIndent(v, "", "\t")
	return string(data)
}

// Handle Init Error
func HandleInitError(logger logger.Logger, err error) {
	logger.Error(printPrefix+" error initializing service: %v", err)
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
