package types

import "encoding/json"

// ScheduledEvent represents an EventBridge or CloudWatch scheduled event
// used to trigger periodic operations like quota monitoring.
type ScheduledEvent struct {
	Version    string          `json:"version"`     // Event format version
	ID         string          `json:"id"`          // Unique event identifier
	DetailType string          `json:"detail-type"` // Type of event detail
	Source     string          `json:"source"`      // Event source identifier
	Account    string          `json:"account"`     // AWS account ID
	Time       string          `json:"time"`        // Event timestamp
	Region     string          `json:"region"`      // AWS region
	Resources  []string        `json:"resources"`   // Associated AWS resources
	Detail     json.RawMessage `json:"detail"`      // Event-specific details as raw JSON
}