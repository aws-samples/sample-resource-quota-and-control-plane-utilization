package types

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestErrorRecord_Error(t *testing.T) {
	timestamp := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	record := ErrorRecord{
		Timestamp: timestamp,
		Message:   "test error message",
		Code:      "ERR001",
	}
	
	expected := "[2023-01-01T12:00:00Z] test error message"
	if got := record.Error(); got != expected {
		t.Errorf("ErrorRecord.Error() = %q, want %q", got, expected)
	}
}

func TestNewErrorRecord(t *testing.T) {
	testErr := errors.New("test error")
	before := time.Now()
	record := NewErrorRecord(testErr)
	after := time.Now()
	
	if record.Message != "test error" {
		t.Errorf("NewErrorRecord().Message = %q, want \"test error\"", record.Message)
	}
	if record.Code != "" {
		t.Errorf("NewErrorRecord().Code = %q, want \"\"", record.Code)
	}
	if record.Timestamp.Before(before) || record.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", record.Timestamp, before, after)
	}
}

func TestNewErrorRecordWithCode(t *testing.T) {
	testErr := errors.New("test error")
	code := "ERR001"
	before := time.Now()
	record := NewErrorRecordWithCode(testErr, code)
	after := time.Now()
	
	if record.Message != "test error" {
		t.Errorf("NewErrorRecordWithCode().Message = %q, want \"test error\"", record.Message)
	}
	if record.Code != code {
		t.Errorf("NewErrorRecordWithCode().Code = %q, want %q", record.Code, code)
	}
	if record.Timestamp.Before(before) || record.Timestamp.After(after) {
		t.Errorf("Timestamp %v not between %v and %v", record.Timestamp, before, after)
	}
}

func TestErrorRecord_JSONSerialization(t *testing.T) {
	timestamp := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	record := ErrorRecord{
		Timestamp: timestamp,
		Message:   "test error",
		Code:      "ERR001",
	}
	
	// Marshal
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Unmarshal
	var unmarshaled ErrorRecord
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Verify
	if !unmarshaled.Timestamp.Equal(record.Timestamp) {
		t.Errorf("Unmarshaled Timestamp = %v, want %v", unmarshaled.Timestamp, record.Timestamp)
	}
	if unmarshaled.Message != record.Message {
		t.Errorf("Unmarshaled Message = %q, want %q", unmarshaled.Message, record.Message)
	}
	if unmarshaled.Code != record.Code {
		t.Errorf("Unmarshaled Code = %q, want %q", unmarshaled.Code, record.Code)
	}
}

func TestErrorRecord_TimestampFormat(t *testing.T) {
	timestamp := time.Date(2023, 12, 25, 15, 30, 45, 123456789, time.UTC)
	record := ErrorRecord{
		Timestamp: timestamp,
		Message:   "test",
	}
	
	errorStr := record.Error()
	if !strings.Contains(errorStr, "2023-12-25T15:30:45Z") {
		t.Errorf("Error string %q should contain RFC3339 formatted timestamp", errorStr)
	}
}

func TestErrorRecord_EmptyCode(t *testing.T) {
	record := ErrorRecord{
		Timestamp: time.Now(),
		Message:   "test error",
		Code:      "",
	}
	
	// Marshal to JSON
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Verify empty code is omitted (omitempty behavior)
	jsonStr := string(data)
	if strings.Contains(jsonStr, "code") {
		t.Errorf("JSON %q should not contain 'code' field when empty", jsonStr)
	}
}

func TestErrorRecord_WithCode(t *testing.T) {
	record := ErrorRecord{
		Timestamp: time.Now(),
		Message:   "test error",
		Code:      "ERR001",
	}
	
	// Marshal to JSON
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Verify code is included when present
	jsonStr := string(data)
	if !strings.Contains(jsonStr, "ERR001") {
		t.Errorf("JSON %q should contain code 'ERR001'", jsonStr)
	}
}