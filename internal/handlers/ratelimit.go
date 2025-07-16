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

	// Aggregate counters by region
	regionCounters := make(map[string]map[string]int)
	var flushCommands []string
	var failures []events.SQSBatchItemFailure
	
	// First pass: process all messages and aggregate counters
	for _, msg := range event.Records {
		if rlh.isFlushCommand(msg.Body) {
			// Track flush commands for later processing
			flushCommands = append(flushCommands, msg.MessageId)
			continue
		}
		
		// Process CloudTrail event
		if failure := rlh.aggregateCloudTrailEvent(msg, regionCounters); failure != nil {
			failures = append(failures, *failure)
		}
	}
	
	// Second pass: submit aggregated counters if any
	if len(regionCounters) > 0 {
		if err := rlh.Batcher.AddCounters(ctx, regionCounters); err != nil {
			rlh.Logger.Error("failed to add batch counters: %v", err)
			// If batch fails, mark all CloudTrail messages as failed
			for _, msg := range event.Records {
				if !rlh.isFlushCommand(msg.Body) {
					failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: msg.MessageId})
				}
			}
		}
	}
	
	// Third pass: process any flush commands
	for _, messageId := range flushCommands {
		rlh.handleFlushCommand(ctx, messageId)
	}

	return failures, nil
}

// aggregateCloudTrailEvent processes a CloudTrail event and adds its counters to the aggregation map.
func (rlh *RateLimitHandler) aggregateCloudTrailEvent(msg events.SQSMessage, regionCounters map[string]map[string]int) *events.SQSBatchItemFailure {
	var ctEvent types.CloudTrailEvent
	if err := json.Unmarshal([]byte(msg.Body), &ctEvent); err != nil {
		rlh.Logger.Error("failed to unmarshal SQS message %s: %v", msg.MessageId, err)
		return &events.SQSBatchItemFailure{ItemIdentifier: msg.MessageId}
	}
	
	// Validate region
	region := ctEvent.AWSRegion
	if region == "" {
		rlh.Logger.Error("missing region in CloudTrail event: %s", msg.MessageId)
		return &events.SQSBatchItemFailure{ItemIdentifier: msg.MessageId}
	}
	
	// Logger automatically sanitizes all string arguments via SanitizingLogger wrapper
	rlh.Logger.Debug("processing CloudTrail event: eventName=%s region=%s requestId=%s", 
		ctEvent.EventName, region, ctEvent.RequestID)
	
	// Generate counter keys
	propagateInvoker := false
	if batcher, ok := rlh.Batcher.(interface{ PropagateInvoker() bool }); ok {
		propagateInvoker = batcher.PropagateInvoker()
	}
	keys := cloudtrail.GenerateCounterKeys(ctEvent, propagateInvoker)
	
	// Ensure region map exists
	if regionCounters[region] == nil {
		regionCounters[region] = make(map[string]int)
	}
	
	// Update counters for each key
	for _, key := range keys {
		regionCounters[region][key]++
	}
	
	return nil
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


