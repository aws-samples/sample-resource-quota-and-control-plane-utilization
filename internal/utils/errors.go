// Package utils provides error definitions for utility functions.
package utils

import "errors"

// Error variables for ARN parsing operations.
var (
	// ErrInvalidArn indicates that the provided ARN string is malformed.
	ErrInvalidArn error = errors.New("invalid arn")
	// ErrUnexpectedResourceFormat indicates that the ARN resource section has an unexpected format.
	ErrUnexpectedResourceFormat error = errors.New("unexpected resource format")
	// ErrArnResourceIsNotARole indicates that the ARN does not represent a role resource.
	ErrArnResourceIsNotARole error = errors.New("arn resource is not a role")
)
