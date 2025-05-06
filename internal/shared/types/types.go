package sharedtypes

import (
	"encoding/json"

	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"time"
)

// CloudTrailEvent represents a single cloudtrail event
type CloudTrailEvent struct {
	EventVersion string             `json:"eventVersion"`
	UserIdentity UserIdentityDetail `json:"userIdentity"`
	EventTime    time.Time          `json:"eventTime"`
	EventSource  string             `json:"eventSource"`
	EventName    string             `json:"eventName"`
	AWSRegion    string             `json:"awsRegion"`
	SourceIP     string             `json:"sourceIPAddress"`
	UserAgent    string             `json:"userAgent"`
	RequestID    string             `json:"requestID"`
	EventID      string             `json:"eventID"`
}

// Metric encapsulates the metric details, including metadataß
type CloudWatchMetric struct {
	Name      string
	Value     float64
	Unit      cwTypes.StandardUnit
	Timestamp time.Time
	Metadata  map[string]string
}

// UserIdentityDetail holds the nested userIdentity fields
type UserIdentityDetail struct {
	Type        string `json:"type"`
	PrincipalId string `json:"principalId"`
	ARN         string `json:"arn"`
}

// ScheduledEvent represents the structure of a scheduled event from CloudWatch or EventBridge
type ScheduledEvent struct {
	Version    string          `json:"version"`
	ID         string          `json:"id"`
	DetailType string          `json:"detail-type"`
	Source     string          `json:"source"`
	Account    string          `json:"account"`
	Time       string          `json:"time"`
	Region     string          `json:"region"`
	Resources  []string        `json:"resources"`
	Detail     json.RawMessage `json:"detail"`
}

// ErrorRecord represents one error event.
type ErrorRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Err       error     `json:"error"`
}

// Error Method to satisfy error interface.
func (e ErrorRecord) Error() string {
	return e.Err.Error()
}

// EMFRecord holds the EMF payload and timestamp.
type EMFRecord struct {
	Payload   []byte
	Timestamp int64
}
