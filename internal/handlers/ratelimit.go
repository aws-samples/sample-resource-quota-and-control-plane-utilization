package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aws/aws-lambda-go/events"

	cloudtrailemfbatcher "github.com/outofoffice3/aws-samples/geras/internal/emfbatcher/cloudtrail"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (

	// error msgs
	CloudTrailEmfFileBatcherNillErrMsg = "cloudtrail emf file batcher is nil"
	NamespaceNotSetErrMsg              = "namespace is not set"
	StashNilErrMsg                     = "stash is nil"
)

// RateLimitHandler handles scheduled events from EventBridge
// and batches EMF records to CloudWatch.
type RateLimitHandler struct {
	CloudTrailEmfFileBatcher cloudtrailemfbatcher.EMFFileBatcher
	Logger                   logger.Logger
	initialized              bool
	Namespace                string
}

type RateLimitHandlerConfig struct {
	CloudTrailEmfFileBatcher cloudtrailemfbatcher.EMFFileBatcher
	Namespace                string
	Logger                   logger.Logger
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
	if config.CloudTrailEmfFileBatcher == nil {
		return nil, LogAndReturnError(sharedtypes.ErrorRecord{
			Timestamp: time.Now(),
			Err:       errors.New(CloudTrailEmfFileBatcherNillErrMsg),
		}, config.Logger)
	}

	// if namespace is not set, throw error
	if config.Namespace == "" {
		return nil, LogAndReturnError(sharedtypes.ErrorRecord{
			Timestamp: time.Now(),
			Err:       errors.New(NamespaceNotSetErrMsg),
		}, config.Logger)
	}

	// construct handler
	rlh := &RateLimitHandler{
		CloudTrailEmfFileBatcher: config.CloudTrailEmfFileBatcher,
		Logger:                   config.Logger,
		Namespace:                config.Namespace,
		initialized:              true,
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
	if !rlh.initialized {
		return nil, errors.New("handler not initialized")
	}
	rlh.Logger.Info("Received %d records from SQS event", len(event.Records))

	var failures []events.SQSBatchItemFailure

	for _, msg := range event.Records {
		var ctEvent sharedtypes.CloudTrailEvent
		if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
			rlh.Logger.Error("failed to unmarshal SQS message %s: %v", msg.MessageId, err)
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: msg.MessageId})
			continue
		}

		rlh.CloudTrailEmfFileBatcher.Add(ctx, ctEvent.AWSRegion, ctEvent)
		rlh.Logger.Debug("added CloudTrail event to file batcher for region %s, message %s", ctEvent.AWSRegion, msg.MessageId)
	}

	return failures, nil

}

// LogAndReturnError centralizes error logging
func LogAndReturnError(er error, applogger logger.Logger) error {
	applogger.Error("Handler error: %v", er.Error())
	return er
}
