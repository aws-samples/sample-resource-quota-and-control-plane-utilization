package types

import (
	"fmt"
	"time"
)

// ErrorRecord represents a timestamped error event for tracking
// and logging purposes throughout the application.
type ErrorRecord struct {
	Timestamp time.Time `json:"timestamp"` // When the error occurred
	Message   string    `json:"message"`   // Error message
	Code      string    `json:"code,omitempty"` // Optional error code
}

// Error implements the error interface.
func (e ErrorRecord) Error() string {
	return fmt.Sprintf("[%s] %s", e.Timestamp.Format(time.RFC3339), e.Message)
}

// NewErrorRecord creates a new error record with current timestamp.
func NewErrorRecord(err error) ErrorRecord {
	return ErrorRecord{
		Timestamp: time.Now(),
		Message:   err.Error(),
	}
}

// NewErrorRecordWithCode creates a new error record with error code.
func NewErrorRecordWithCode(err error, code string) ErrorRecord {
	return ErrorRecord{
		Timestamp: time.Now(),
		Message:   err.Error(),
		Code:      code,
	}
}