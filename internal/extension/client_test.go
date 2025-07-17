package extension

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockHTTPClient is a mock HTTP client for testing
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

// Helper function to create a test server and client
func setupTestServerAndClient(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	server := httptest.NewServer(handler)
	t.Cleanup(func() { server.Close() })
	
	// Extract the host:port part from the server URL
	hostPort := strings.TrimPrefix(server.URL, "http://")
	
	client := NewClient(hostPort)
	return server, client
}

func TestNewClient(t *testing.T) {
	client := NewClient("localhost:1234")
	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:1234/2020-01-01/extension", client.baseURL)
}

func TestRegister_Success(t *testing.T) {
	expectedResponse := RegisterResponse{
		FunctionName:    "test-function",
		FunctionVersion: "1",
		Handler:         "index.handler",
	}
	
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/2020-01-01/extension/register", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "test-extension", r.Header.Get(extensionNameHeader))
		
		// Read request body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		
		// Verify request body contains expected events
		var reqBody map[string]interface{}
		err = json.Unmarshal(body, &reqBody)
		require.NoError(t, err)
		
		events, ok := reqBody["events"].([]interface{})
		require.True(t, ok)
		assert.Contains(t, events, "INVOKE")
		assert.Contains(t, events, "SHUTDOWN")
		
		// Set response headers
		w.Header().Set(extensionIdentiferHeader, "test-extension-id")
		
		// Write response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedResponse)
	})
	
	// Call Register
	resp, err := client.Register(context.Background(), "test-extension")
	
	// Verify response
	require.NoError(t, err)
	assert.Equal(t, expectedResponse.FunctionName, resp.FunctionName)
	assert.Equal(t, expectedResponse.FunctionVersion, resp.FunctionVersion)
	assert.Equal(t, expectedResponse.Handler, resp.Handler)
	assert.Equal(t, "test-extension-id", client.extensionID)
}

func TestRegister_Error(t *testing.T) {
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	})
	
	// Call Register
	resp, err := client.Register(context.Background(), "test-extension")
	
	// Verify error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrRequestFailed)
}

func TestNextEvent_Success(t *testing.T) {
	expectedResponse := NextEventResponse{
		EventType:          Invoke,
		DeadlineMs:         1234567890,
		RequestID:          "test-request-id",
		InvokedFunctionArn: "arn:aws:lambda:us-east-1:123456789012:function:test-function",
		Tracing: Tracing{
			Type:  "X-Amzn-Trace-Id",
			Value: "test-trace-value",
		},
	}
	
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/2020-01-01/extension/event/next", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "test-extension-id", r.Header.Get(extensionIdentiferHeader))
		
		// Write response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedResponse)
	})
	
	// Set extension ID
	client.extensionID = "test-extension-id"
	
	// Call NextEvent
	resp, err := client.NextEvent(context.Background())
	
	// Verify response
	require.NoError(t, err)
	assert.Equal(t, expectedResponse.EventType, resp.EventType)
	assert.Equal(t, expectedResponse.DeadlineMs, resp.DeadlineMs)
	assert.Equal(t, expectedResponse.RequestID, resp.RequestID)
	assert.Equal(t, expectedResponse.InvokedFunctionArn, resp.InvokedFunctionArn)
	assert.Equal(t, expectedResponse.Tracing.Type, resp.Tracing.Type)
	assert.Equal(t, expectedResponse.Tracing.Value, resp.Tracing.Value)
}

func TestNextEvent_Error(t *testing.T) {
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	})
	
	// Set extension ID
	client.extensionID = "test-extension-id"
	
	// Call NextEvent
	resp, err := client.NextEvent(context.Background())
	
	// Verify error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrRequestFailed)
}

func TestInitError_Success(t *testing.T) {
	expectedResponse := StatusResponse{
		Status: "accepted",
	}
	
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/2020-01-01/extension/init/error", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "test-extension-id", r.Header.Get(extensionIdentiferHeader))
		assert.Equal(t, "test-error-type", r.Header.Get(extensionErrorType))
		
		// Write response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedResponse)
	})
	
	// Set extension ID
	client.extensionID = "test-extension-id"
	
	// Call InitError
	resp, err := client.InitError(context.Background(), "test-error-type")
	
	// Verify response
	require.NoError(t, err)
	assert.Equal(t, expectedResponse.Status, resp.Status)
}

func TestInitError_Error(t *testing.T) {
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	})
	
	// Set extension ID
	client.extensionID = "test-extension-id"
	
	// Call InitError
	resp, err := client.InitError(context.Background(), "test-error-type")
	
	// Verify error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrRequestFailed)
}

func TestExitError_Success(t *testing.T) {
	expectedResponse := StatusResponse{
		Status: "accepted",
	}
	
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/2020-01-01/extension/exit/error", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "test-extension-id", r.Header.Get(extensionIdentiferHeader))
		assert.Equal(t, "test-error-type", r.Header.Get(extensionErrorType))
		
		// Write response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(expectedResponse)
	})
	
	// Set extension ID
	client.extensionID = "test-extension-id"
	
	// Call ExitError
	resp, err := client.ExitError(context.Background(), "test-error-type")
	
	// Verify response
	require.NoError(t, err)
	assert.Equal(t, expectedResponse.Status, resp.Status)
}

func TestExitError_Error(t *testing.T) {
	_, client := setupTestServerAndClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	})
	
	// Set extension ID
	client.extensionID = "test-extension-id"
	
	// Call ExitError
	resp, err := client.ExitError(context.Background(), "test-error-type")
	
	// Verify error
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, ErrRequestFailed)
}

func TestEventType_Constants(t *testing.T) {
	// Verify EventType constants
	assert.Equal(t, EventType("INVOKE"), Invoke)
	assert.Equal(t, EventType("SHUTDOWN"), Shutdown)
}