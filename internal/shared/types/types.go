// Package sharedtypes provides common data structures used across
// the application for CloudTrail events, metrics, and error handling.
package sharedtypes

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CloudTrailEvent represents a parsed CloudTrail log event.
type CloudTrailEvent struct {
	EventVersion       string       `json:"eventVersion"`
	RecipientAccountId string       `json:"recipientAccountId,omitempty"`
	UserIdentity       UserIdentity `json:"userIdentity"`
	EventTime          time.Time    `json:"eventTime"`
	EventSource        string       `json:"eventSource"`
	EventName          string       `json:"eventName"`
	AWSRegion          string       `json:"awsRegion"`
	SourceIP           string       `json:"sourceIPAddress"`
	UserAgent          string       `json:"userAgent"`
	RequestID          string       `json:"requestID"`
	EventID            string       `json:"eventID"`

	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`

	OnBehalfOf *struct {
		UserId           string `json:"userId"`
		IdentityStoreArn string `json:"identityStoreArn"`
	} `json:"onBehalfOf,omitempty"`

	ResponseElements struct {
		AssumedRoleUser struct {
			ARN string `json:"arn"`
		} `json:"assumedRoleUser"`
	} `json:"responseElements,omitempty"`
}

type UserIdentity struct {
	Type           string                 `json:"type"`
	PrincipalId    string                 `json:"principalId"`
	ARN            string                 `json:"arn"`
	AccountId      string                 `json:"accountId"`
	InvokedBy      string                 `json:"invokedBy,omitempty"`
	SessionContext *SessionContextDetails `json:"sessionContext,omitempty"`
}

type SessionContextDetails struct {
	Attributes struct {
		CreationDate     time.Time `json:"creationDate"`
		MfaAuthenticated string    `json:"mfaAuthenticated"`
	} `json:"attributes"`
	SessionIssuer struct {
		Type        string `json:"type"`
		PrincipalId string `json:"principalId"`
		ARN         string `json:"arn"`
		UserName    string `json:"userName,omitempty"`
	} `json:"sessionIssuer"`
}

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

// ErrorRecord represents a timestamped error event for tracking
// and logging purposes throughout the application.
type ErrorRecord struct {
	Timestamp time.Time `json:"timestamp"` // When the error occurred
	Err       error     `json:"error"`     // The actual error
}

// Error implements the error interface, returning the underlying error message.
func (e ErrorRecord) Error() string {
	return e.Err.Error()
}

type MetricUnit string

const (
	UnitCount   MetricUnit = "Count"
	UnitPercent MetricUnit = "Percent"
)

var (
	ErrInvalidMetricUnit = errors.New("invalid metric unit")
)

func (u MetricUnit) UnitToString() string {
	if u == "" {
		return string(UnitCount)
	}
	return string(u)
}

func (u MetricUnit) Validate() error {
	switch u {
	case UnitCount, UnitPercent:
		return nil
	}
	return fmt.Errorf("%w, %s", ErrInvalidMetricUnit, u)
}

// CloudWatchMetric represents a metric data point to be sent to CloudWatch,
// including value, unit, timestamp, and associated metadata.
type CloudWatchMetric struct {
	Name      string  // Metric name
	Value     float64 // Metric value
	Unit      MetricUnit
	Timestamp time.Time         // Metric timestamp
	Metadata  map[string]string // Additional metric dimensions and metadata
}
