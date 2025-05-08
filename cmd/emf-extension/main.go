// cmd/cloudtrail-extension/main.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"os/signal"
	"time"

	"slices"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/extension"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/utils"
)

const (
	logLevelEnvVar = "LOG_LEVEL"
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
	log.Debug(printPrefix+" log level set to %s", logLevelValue)

	ctx, cancel := context.WithCancel(context.Background())
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
		log.Error("glob failed: %v", err)
		os.Exit(1)
	}
	log.Debug(printPrefix+" found %d files in %s", len(files), stashDir)

	// b) build per-region CWL client map
	awsCfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Error(printPrefix+" load AWS config failed: %v", err)
		os.Exit(1)
	}
	cwlMap := &safemap.TypedMap[cwlclient.CloudWatchLogsClient]{}
	for _, f := range files {
		region := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "emf_"), ".ndjson")
		client, err := cwlclient.NewCloudWatchLogsClient(awsCfg, region)
		if err != nil {
			log.Error(printPrefix+" CWL client creation failed for region %s: %v", region, err)
			continue
		}
		cwlMap.Store(region, client)
		log.Debug(printPrefix+" CWL client created for region %s", region)
	}

	// c) create EMF flusher
	logGroup := os.Getenv("CLOUDWATCH_LOG_GROUP")
	if logGroup == "" {
		log.Error(printPrefix + " CLOUDWATCH_LOG_GROUP not set")
		os.Exit(1)
	}
	log.Debug(printPrefix+" cloudwatch log group set to %s", logGroup)
	flusher := emf.NewEMFFlusher(emf.EMFFlusherConfig{
		CwlClientMap:  cwlMap,
		LogStreamName: utils.MakeStreamName(),
		LogGroupName:  logGroup,
		Logger:        log,
	})
	log.Debug(printPrefix + " flusher created")

	// ── 1) REGISTER ────────────────────────────────────────────────────────
	res, err := extensionClient.Register(ctx, extensionName)
	if err != nil {
		panic(err)
	}
	println(printPrefix, "Register response:", prettyPrint(res))

	// ── 2) PROCESS EVENTS UNTIL SHUTDOWN ──────────────────────────────────
	processEvents(ctx)
	log.Debug(printPrefix + " processEvents() returned, SHUTDOWN")

	// ── 3) ON SHUTDOWN, FLUSH ALL STASH FILES ─────────────────────────────
	files, _ = filepath.Glob(filepath.Join(os.TempDir(), "emf_*.ndjson"))
	log.Info(printPrefix+" flushing %d files", len(files))
	var wg sync.WaitGroup
	for _, path := range files {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			f, err := os.Open(path)
			if err != nil {
				log.Error(printPrefix+"open %s: %v", path, err)
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
	println(printPrefix, "Flush complete, exiting")
}

func processEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			println(printPrefix, "Waiting for event...")
			res, err := extensionClient.NextEvent(ctx)
			if err != nil {
				println(printPrefix, "Error:", err)
				return
			}
			println(printPrefix, "Received event:", prettyPrint(res))
			if res.EventType == extension.Shutdown {
				println(printPrefix, "Received SHUTDOWN event")
				return
			}
		}
	}
}

func prettyPrint(v any) string {
	data, _ := json.MarshalIndent(v, "", "\t")
	return string(data)
}
