// Package types provides common data structures used across the application.
package types

import "time"

// CloudTrailEvent represents a parsed CloudTrail log event.
type CloudTrailEvent struct {
	EventVersion       string       `json:"eventVersion"`
	EventTime          time.Time    `json:"eventTime"`
	EventSource        string       `json:"eventSource"`
	EventName          string       `json:"eventName"`
	AWSRegion          string       `json:"awsRegion"`
	SourceIP           string       `json:"sourceIPAddress"`
	UserAgent          string       `json:"userAgent"`
	RequestID          string       `json:"requestID"`
	EventID            string       `json:"eventID"`
	UserIdentity       UserIdentity `json:"userIdentity"`
	RecipientAccountId string       `json:"recipientAccountId,omitempty"`
	ErrorCode          string       `json:"errorCode,omitempty"`
	ErrorMessage       string       `json:"errorMessage,omitempty"`
	OnBehalfOf         *struct {
		UserId           string `json:"userId"`
		IdentityStoreArn string `json:"identityStoreArn"`
	} `json:"onBehalfOf,omitempty"`
	ResponseElements struct {
		AssumedRoleUser struct {
			ARN string `json:"arn"`
		} `json:"assumedRoleUser"`
	} `json:"responseElements,omitempty"`
}

// UserIdentity represents the identity information from CloudTrail events.
type UserIdentity struct {
	Type           string                 `json:"type"`
	PrincipalId    string                 `json:"principalId"`
	ARN            string                 `json:"arn"`
	AccountId      string                 `json:"accountId"`
	InvokedBy      string                 `json:"invokedBy,omitempty"`
	SessionContext *SessionContextDetails `json:"sessionContext,omitempty"`
}

// SessionContextDetails contains session-specific information.
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