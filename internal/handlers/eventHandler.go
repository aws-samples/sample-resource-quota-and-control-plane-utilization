// Package handlers provides event handling interfaces and implementations
// for processing AWS Lambda events, scheduled events, and SQS messages.
package handlers

import (
	"context"
	"encoding/json"
)

// EventHandler defines a generic interface for processing events.
// Implementations handle specific event types like CloudWatch events or SQS messages.
type EventHandler interface {
	// HandleEvent processes an event with the provided context.
	// The event is passed as raw JSON to allow flexible event type handling.
	HandleEvent(ctx context.Context, event json.RawMessage) error
}
