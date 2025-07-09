// Package handlers provides event handlers for rate limit monitoring.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aws/aws-lambda-go/events"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/cloudtrail"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (
	// Error message constants for rate limit handler validation.
	CloudTrailBatcherNilErrMsg = "cloudtrail batcher is nil"
	NamespaceNotSetErrMsg      = "namespace is not set"
)

// RateLimitHandler processes SQS events containing CloudTrail data
// and batches them into EMF records for CloudWatch metrics.
type RateLimitHandler struct {
	Batcher     cloudtrail.Batcher
	Logger      logger.Logger
	initialized bool
	Namespace   string
}

// RateLimitHandlerConfig contains configuration parameters for RateLimitHandler.
type RateLimitHandlerConfig struct {
	Batcher   cloudtrail.Batcher // EMF batcher for processing events
	Namespace string             // CloudWatch metrics namespace
	Logger    logger.Logger      // Logger for handler operations
}

// NewRateLimitHandler creates a new rate limit handler with validation.
// Returns an error if required configuration is missing or invalid.
func NewRateLimitHandler(config RateLimitHandlerConfig) (*RateLimitHandler, error) {
	// default logger
	if config.Logger == nil {
		config.Logger = logger.Get()
	}
	// validate batcher
	if config.Batcher == nil {
		return nil, LogAndReturnError(errors.New(CloudTrailBatcherNilErrMsg), config.Logger)
	}
	// validate namespace
	if config.Namespace == "" {
		return nil, LogAndReturnError(errors.New(NamespaceNotSetErrMsg), config.Logger)
	}
	// construct handler
	rlh := &RateLimitHandler{
		Batcher:     config.Batcher,
		Logger:      config.Logger,
		Namespace:   config.Namespace,
		initialized: true,
	}
	rlh.Logger.Info("RateLimitHandler initialized for namespace %s", config.Namespace)
	return rlh, nil
}

// HandleEvent processes SQS messages containing CloudTrail events or flush commands.
// Returns failed message IDs for partial batch failure handling in Lambda.
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
		// detect flush command
		var meta struct {
			Flush bool `json:"flush"`
		}
		if err := json.Unmarshal([]byte(msg.Body), &meta); err == nil && meta.Flush {
			rlh.Logger.Info("received FLUSH event from event bridge")
			err := rlh.Batcher.FlushAll(ctx, time.Now())
			if err != nil {
				rlh.Logger.Error("flush error: %v", err)
			}
			rlh.Logger.Info("flushed all CloudTrail EMF records due to flush message %s", msg.MessageId)
			continue
		}

		// normal CloudTrail event
		var ctEvent sharedtypes.CloudTrailEvent
		if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
			rlh.Logger.Error("failed to unmarshal SQS message %s: %v", msg.MessageId, err)
			failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: msg.MessageId})
			continue
		}

		// add to batcher
		rlh.Logger.Info("received cloudtrail event=%+v", ctEvent)
		rlh.Batcher.Add(ctx, ctEvent.AWSRegion, ctEvent)
	}

	return failures, nil
}

// LogAndReturnError provides centralized error logging for handler operations.
func LogAndReturnError(err error, applogger logger.Logger) error {
	applogger.Error("Handler error: %v", err)
	return err
}
