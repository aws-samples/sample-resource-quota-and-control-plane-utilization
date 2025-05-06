package handlers

import (
	"context"
	"encoding/json"
)

// EventHandler defines a generic interface for handling events
type EventHandler interface {
	HandleEvent(ctx context.Context, event json.RawMessage) error
}
