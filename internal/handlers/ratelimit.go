package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (
	metricNameCallCount = "CallCount"
	metricUnitCount     = "Count"

	// error msgs
	FlusherNilErrMsg       = "flusher is nil"
	CwlClientMapNilErrMsg  = "cwl client map is nil"
	NamspaceNotSetErrMsg   = "namespace is not set"
	HomeRegionNotSetErrMsg = "home region is not set"
)

// RateLimitHandler handles scheduled events from EventBridge
// and batches EMF records to CloudWatch.
type RateLimitHandler struct {
	CwlClientMap *safemap.TypedMap[cwlclient.CloudWatchLogsClient]
	Logger       logger.Logger
	EMFFusher    emf.EMFFusher
	initialized  bool
	Namespace    string
	HomeRegion   string // region where the Lambda runs
}

type RateLimitHandlerConfig struct {
	CwlMap     *safemap.TypedMap[cwlclient.CloudWatchLogsClient]
	Flusher    emf.EMFFusher
	Namespace  string
	HomeRegion string
	Logger     logger.Logger
}

// NewRateLimitHandler constructs a fully-initialized RateLimitHandler.
func NewRateLimitHandler(
	config RateLimitHandlerConfig,
) (*RateLimitHandler, error) {
	// if logger is nil set to stub logger to prevent panic
	if config.Logger == nil {
		config.Logger = logger.Get()
	}

	// if client mpa is nil, throw error
	if config.CwlMap == nil {
		return nil, LogAndReturnError(sharedtypes.ErrorRecord{
			Timestamp: time.Now(),
			Err:       errors.New(CwlClientMapNilErrMsg),
		}, config.Logger)
	}

	// if flusher is nil. throw error
	if config.Flusher == nil {
		return nil, LogAndReturnError(sharedtypes.ErrorRecord{
			Timestamp: time.Now(),
			Err:       errors.New(FlusherNilErrMsg),
		}, config.Logger)
	}

	// if namespace is not set, throw error
	if config.Namespace == "" {
		return nil, LogAndReturnError(sharedtypes.ErrorRecord{
			Timestamp: time.Now(),
			Err:       errors.New(NamspaceNotSetErrMsg),
		}, config.Logger)
	}

	// if home region is not set, throw error
	if config.HomeRegion == "" {
		return nil, LogAndReturnError(sharedtypes.ErrorRecord{
			Timestamp: time.Now(),
			Err:       errors.New(HomeRegionNotSetErrMsg),
		}, config.Logger)
	}
	// construct handler
	rlh := &RateLimitHandler{
		CwlClientMap: config.CwlMap,
		Logger:       config.Logger,
		EMFFusher:    config.Flusher,
		Namespace:    config.Namespace,
		HomeRegion:   config.HomeRegion,
		initialized:  true,
	}
	rlh.Logger.Info("RateLimitHandler initialized")
	return rlh, nil
}

// HandleEvent processes an SQS event, batching and flushing EMF records.
// Returns a slice of failed message IDs for partial-batch retry.
func (rlh *RateLimitHandler) HandleEvent(
	ctx context.Context,
	event events.SQSEvent,
) ([]events.SQSBatchItemFailure, error) {
	rlh.Logger.Info("Received %d records from SQS event", len(event.Records))

	// entry pairs an EMFRecord with its original SQS MessageId
	type entry struct {
		msgID string
		rec   emf.EMFRecord
	}
	byRegion := make(map[string][]entry)

	// Convert and group by region
	for _, msg := range event.Records {
		rlh.Logger.Debug("Processing messageID=%s, message=%s", msg.MessageId, msg.Body)
		var ctEvent sharedtypes.CloudTrailEvent
		if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
			LogAndReturnError(sharedtypes.ErrorRecord{
				Timestamp: time.Now(),
				Err:       err,
			}, rlh.Logger)
			continue
		}
		if ctEvent.AWSRegion == "" {
			rlh.Logger.Warn("Message %s has no AWSRegion; skipping", msg.MessageId)
			continue
		}
		emfRecord, err := emf.ConvertSQSMessageToEMF(ctx, msg,
			rlh.Namespace, metricNameCallCount, metricUnitCount, [][]string{{"eventName", ctEvent.EventName}}, rlh.Logger)
		if err != nil {
			LogAndReturnError(sharedtypes.ErrorRecord{
				Timestamp: time.Now(),
				Err:       err,
			}, rlh.Logger)
			continue
		}
		byRegion[ctEvent.AWSRegion] = append(byRegion[ctEvent.AWSRegion], entry{msgID: msg.MessageId, rec: emfRecord})
		rlh.Logger.Debug("Prepared EMFRecord for region=%s messageID=%s", ctEvent.AWSRegion, msg.MessageId)
	}

	if len(byRegion) == 0 {
		rlh.Logger.Info("No valid EMFRecords to flush; exiting")
		return nil, nil
	}

	// Channel to collect failed MessageIds (buffered to total records)
	failCh := make(chan string, len(event.Records))

	// Flush each region’s batch in parallel
	var wg sync.WaitGroup
	for region, entries := range byRegion {
		wg.Add(1)
		go func(region string, entries []entry) {
			rlh.Logger.Info("Flushing %d records to region %s", len(entries), region)
			defer wg.Done()
			batch := make([]emf.EMFRecord, len(entries))
			for i, e := range entries {
				rlh.Logger.Debug("Flushing messageID=%s, message=%s", e.msgID, string(e.rec.Payload))
				batch[i] = e.rec
			}
			if err := rlh.EMFFusher.Flush(ctx, region, batch); err != nil {
				rlh.Logger.Error("Error flushing to region %s: %v", region, err)
				// report per-record failures
				for _, e := range entries {
					failCh <- e.msgID
				}
			} else {
				rlh.Logger.Info("Successfully flushed %d records to region %s", len(entries), region)
				for _, e := range entries {
					rlh.Logger.Debug("Successfully flushed messageID=%s", e.msgID)
				}
			}
		}(region, entries)
	}

	wg.Wait()
	close(failCh)

	// Collect failures
	var failures []events.SQSBatchItemFailure
	for id := range failCh {
		failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: id})
	}

	if len(failures) > 0 {
		rlh.Logger.Info("Reporting %d failed message(s) for retry", len(failures))
		// loop through and a log all errors so they appear in cloudwatch logs
		for _, f := range failures {
			rlh.Logger.Error("Failed messageID=%s", f.ItemIdentifier)
		}
	} else {
		rlh.Logger.Info("All messages flushed successfully, no failures")
	}

	return failures, nil
}

// LogAndReturnError centralizes error logging
func LogAndReturnError(er error, applogger logger.Logger) error {
	applogger.Error("Handler error: %v", er.Error())
	return er
}
