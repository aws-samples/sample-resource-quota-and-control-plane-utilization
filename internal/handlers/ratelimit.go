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
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

var (
	// Error variables for rate limit handler validation.
	ErrCloudTrailBatcherNil  = errors.New("cloudtrail batcher is nil")
	ErrNamespaceNotSet       = errors.New("namespace is not set")
	ErrHandlerNotInitialized = errors.New("handler not initialized")
)

// RateLimitEventHandler defines the interface for handling SQS events.
type RateLimitEventHandler interface {
	HandleEvent(ctx context.Context, event events.SQSEvent) ([]events.SQSBatchItemFailure, error)
}

// RateLimitHandler processes SQS events containing CloudTrail data
// and batches them into EMF records for CloudWatch metrics.
type RateLimitHandler struct {
	Batcher   cloudtrail.Batcher
	Logger    logger.Logger
	Namespace string
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
		return nil, ErrCloudTrailBatcherNil
	}
	// validate namespace
	if config.Namespace == "" {
		return nil, ErrNamespaceNotSet
	}
	// construct handler
	rlh := &RateLimitHandler{
		Batcher:   config.Batcher,
		Logger:    config.Logger,
		Namespace: config.Namespace,
	}
	rlh.Logger.Info("RateLimitHandler initialized for namespace %s", config.Namespace)
	return rlh, nil
}

// FlushCommand represents a flush command message structure.
type FlushCommand struct {
	Flush bool `json:"flush"`
}

// HandleEvent processes SQS messages containing CloudTrail events or flush commands.
// Returns failed message IDs for partial batch failure handling in Lambda.
func (rlh *RateLimitHandler) HandleEvent(
	ctx context.Context,
	event events.SQSEvent,
) ([]events.SQSBatchItemFailure, error) {
	rlh.Logger.Info("Received %d records from SQS event", len(event.Records))

	// Process messages sequentially
	var failures []events.SQSBatchItemFailure
	for _, msg := range event.Records {
		if failure := rlh.processMessage(ctx, msg); failure != nil {
			failures = append(failures, *failure)
		}
	}

	return failures, nil
}

// processMessage handles individual SQS message processing.
func (rlh *RateLimitHandler) processMessage(ctx context.Context, msg events.SQSMessage) *events.SQSBatchItemFailure {
	if rlh.isFlushCommand(msg.Body) {
		rlh.handleFlushCommand(ctx, msg.MessageId)
		return nil
	}
	return rlh.handleCloudTrailEvent(ctx, msg)
}

// isFlushCommand checks if message is a flush command.
func (rlh *RateLimitHandler) isFlushCommand(body string) bool {
	var cmd FlushCommand
	return json.Unmarshal([]byte(body), &cmd) == nil && cmd.Flush
}

// handleFlushCommand processes flush commands.
func (rlh *RateLimitHandler) handleFlushCommand(ctx context.Context, messageId string) {
	rlh.Logger.Info("received FLUSH event from event bridge")
	if err := rlh.Batcher.FlushAll(ctx, time.Now()); err != nil {
		rlh.Logger.Error("flush error: %v", err)
		return
	}
	rlh.Logger.Info("flushed all CloudTrail EMF records due to flush message %s", messageId)
}

// handleCloudTrailEvent processes CloudTrail events.
func (rlh *RateLimitHandler) handleCloudTrailEvent(ctx context.Context, msg events.SQSMessage) *events.SQSBatchItemFailure {
	var ctEvent types.CloudTrailEvent
	if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
		// amazonq-ignore-next-line
		rlh.Logger.Error("failed to unmarshal SQS message %s: %v", msg.MessageId, err)
		return &events.SQSBatchItemFailure{ItemIdentifier: msg.MessageId}
	}
	// Logger automatically sanitizes all string arguments via SanitizingLogger wrapper
	rlh.Logger.Debug("processing CloudTrail event: eventName=%s region=%s requestId=%s", ctEvent.EventName, ctEvent.AWSRegion, ctEvent.RequestID)
	rlh.Batcher.Add(ctx, ctEvent.AWSRegion, ctEvent)
	return nil
}
