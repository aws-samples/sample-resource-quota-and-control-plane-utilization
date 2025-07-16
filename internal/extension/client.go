// Package extension provides a client for the AWS Lambda Extensions API,
// enabling extensions to register, receive events, and report errors.
package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

// Error variables for better error handling and testing
var (
	ErrRequestFailed = errors.New("request failed")
)

// RegisterResponse contains the Lambda function details returned after extension registration.
type RegisterResponse struct {
	FunctionName    string `json:"functionName"`    // Name of the Lambda function
	FunctionVersion string `json:"functionVersion"` // Version of the Lambda function
	Handler         string `json:"handler"`         // Function handler name
}

// NextEventResponse contains event details from the Lambda Extensions API.
type NextEventResponse struct {
	EventType          EventType `json:"eventType"`          // Type of event (INVOKE or SHUTDOWN)
	DeadlineMs         int64     `json:"deadlineMs"`         // Event deadline in milliseconds
	RequestID          string    `json:"requestId"`          // Unique request identifier
	InvokedFunctionArn string    `json:"invokedFunctionArn"` // ARN of the invoked function
	Tracing            Tracing   `json:"tracing"`            // Tracing information
}

// Tracing contains AWS X-Ray tracing information for the Lambda invocation.
type Tracing struct {
	Type  string `json:"type"`  // Tracing type (e.g., "X-Amzn-Trace-Id")
	Value string `json:"value"` // Tracing header value
}

// StatusResponse contains the status returned from error reporting endpoints.
type StatusResponse struct {
	Status string `json:"status"` // Status of the error report
}

// EventType represents the type of events received from the Lambda Extensions API.
type EventType string

const (
	// Invoke represents a Lambda function invocation event.
	Invoke EventType = "INVOKE"
	// Shutdown represents a Lambda runtime shutdown event.
	Shutdown EventType = "SHUTDOWN"

	// HTTP header constants for Lambda Extensions API.
	extensionNameHeader      = "Lambda-Extension-Name"
	extensionIdentiferHeader = "Lambda-Extension-Identifier"
	extensionErrorType       = "Lambda-Extension-Function-Error-Type"
)

// Client provides methods to interact with the AWS Lambda Extensions API.
type Client struct {
	baseURL     string       // Base URL for the Extensions API
	httpClient  *http.Client // HTTP client for API requests
	extensionID string       // Extension identifier received during registration
}

// NewClient creates a new Lambda Extensions API client.
func NewClient(awsLambdaRuntimeAPI string) *Client {
	baseURL := fmt.Sprintf("http://%s/2020-01-01/extension", awsLambdaRuntimeAPI)
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// Register registers the extension with the Lambda Extensions API.
// Returns function details and stores the extension ID for subsequent API calls.
func (e *Client) Register(ctx context.Context, filename string) (*RegisterResponse, error) {
	const action = "/register"
	url := e.baseURL + action

	reqBody, err := json.Marshal(map[string]interface{}{
		"events": []EventType{Invoke, Shutdown},
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(extensionNameHeader, filename)
	httpRes, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, fmt.Errorf("%w with status %s", ErrRequestFailed, httpRes.Status)
	}
	defer httpRes.Body.Close()
	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}
	res := RegisterResponse{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}
	e.extensionID = httpRes.Header.Get(extensionIdentiferHeader)
	print(e.extensionID)
	return &res, nil
}

// NextEvent blocks while polling for the next Lambda event (invoke or shutdown).
// This is a long-polling operation that waits for events from the Lambda runtime.
func (e *Client) NextEvent(ctx context.Context) (*NextEventResponse, error) {
	const action = "/event/next"
	url := e.baseURL + action

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(extensionIdentiferHeader, e.extensionID)
	httpRes, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, fmt.Errorf("%w with status %s", ErrRequestFailed, httpRes.Status)
	}
	defer httpRes.Body.Close()
	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}
	res := NextEventResponse{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// InitError reports an initialization error to the Lambda platform.
// Use this when the extension registered successfully but failed during initialization.
func (e *Client) InitError(ctx context.Context, errorType string) (*StatusResponse, error) {
	const action = "/init/error"
	url := e.baseURL + action

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(extensionIdentiferHeader, e.extensionID)
	httpReq.Header.Set(extensionErrorType, errorType)
	httpRes, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, fmt.Errorf("%w with status %s", ErrRequestFailed, httpRes.Status)
	}
	defer httpRes.Body.Close()
	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}
	res := StatusResponse{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// ExitError reports an unexpected error to the Lambda platform before exiting.
// Use this when the extension encounters a fatal error during operation.
func (e *Client) ExitError(ctx context.Context, errorType string) (*StatusResponse, error) {
	const action = "/exit/error"
	url := e.baseURL + action

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(extensionIdentiferHeader, e.extensionID)
	httpReq.Header.Set(extensionErrorType, errorType)
	httpRes, err := e.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, fmt.Errorf("%w with status %s", ErrRequestFailed, httpRes.Status)
	}
	defer httpRes.Body.Close()
	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}
	res := StatusResponse{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
