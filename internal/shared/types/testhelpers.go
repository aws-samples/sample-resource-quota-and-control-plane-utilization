package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// LoadTestData loads JSON test data from the testdata directory
func LoadTestData(t *testing.T, filename string) []byte {
	t.Helper()

	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		// amazonq-ignore-next-line
		t.Fatalf("Failed to read test data file %s: %v", filename, err)
	}
	return data
}

// MustParseTime parses a time string or fails the test
func MustParseTime(t *testing.T, timeStr string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		t.Fatalf("Failed to parse time %s: %v", timeStr, err)
	}
	return parsed
}

// AssertJSONEqual compares two JSON byte slices for equality
func AssertJSONEqual(t *testing.T, expected, actual []byte) {
	t.Helper()

	var expectedObj, actualObj interface{}

	if err := json.Unmarshal(expected, &expectedObj); err != nil {
		// amazonq-ignore-next-line
		t.Fatalf("Failed to unmarshal expected JSON: %v", err)
	}

	if err := json.Unmarshal(actual, &actualObj); err != nil {
		// amazonq-ignore-next-line
		t.Fatalf("Failed to unmarshal actual JSON: %v", err)
	}

	expectedJSON, _ := json.Marshal(expectedObj)
	actualJSON, _ := json.Marshal(actualObj)

	if string(expectedJSON) != string(actualJSON) {
		t.Errorf("JSON not equal:\nExpected: %s\nActual: %s", expectedJSON, actualJSON)
	}
}

// CreateTestCloudTrailEvent creates a basic CloudTrail event for testing
func CreateTestCloudTrailEvent() CloudTrailEvent {
	return CloudTrailEvent{
		EventVersion: "1.05",
		EventTime:    time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		EventSource:  "ec2.amazonaws.com",
		EventName:    "DescribeInstances",
		AWSRegion:    "us-east-1",
		SourceIP:     "192.168.1.1",
		UserAgent:    "aws-cli/2.0.0",
		RequestID:    "req-123",
		EventID:      "evt-456",
		UserIdentity: UserIdentity{
			Type:      "IAMUser",
			AccountId: "123456789012",
		},
	}
}

// CreateTestScheduledEvent creates a basic scheduled event for testing
func CreateTestScheduledEvent() ScheduledEvent {
	return ScheduledEvent{
		Version:    "0",
		ID:         "test-id",
		DetailType: "Scheduled Event",
		Source:     "aws.events",
		Account:    "123456789012",
		Time:       "2023-01-01T12:00:00Z",
		Region:     "us-east-1",
		Resources:  []string{"arn:aws:events:us-east-1:123456789012:rule/test-rule"},
		Detail:     json.RawMessage(`{"test": true}`),
	}
}
