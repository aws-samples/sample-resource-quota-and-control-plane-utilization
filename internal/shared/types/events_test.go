package types

import (
	"encoding/json"
	"testing"
)

func TestScheduledEvent_JSONSerialization(t *testing.T) {
	event := ScheduledEvent{
		Version:    "0",
		ID:         "cdc73f9d-aea9-11e3-9d5a-835b769c0d9c",
		DetailType: "Scheduled Event",
		Source:     "aws.events",
		Account:    "123456789012",
		Time:       "2023-01-01T12:00:00Z",
		Region:     "us-east-1",
		Resources:  []string{"arn:aws:events:us-east-1:123456789012:rule/my-rule"},
		Detail:     json.RawMessage(`{"key": "value"}`),
	}
	
	// Marshal
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Unmarshal
	var unmarshaled ScheduledEvent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Verify all fields
	if unmarshaled.Version != event.Version {
		t.Errorf("Version = %q, want %q", unmarshaled.Version, event.Version)
	}
	if unmarshaled.ID != event.ID {
		t.Errorf("ID = %q, want %q", unmarshaled.ID, event.ID)
	}
	if unmarshaled.DetailType != event.DetailType {
		t.Errorf("DetailType = %q, want %q", unmarshaled.DetailType, event.DetailType)
	}
	if unmarshaled.Source != event.Source {
		t.Errorf("Source = %q, want %q", unmarshaled.Source, event.Source)
	}
	if unmarshaled.Account != event.Account {
		t.Errorf("Account = %q, want %q", unmarshaled.Account, event.Account)
	}
	if unmarshaled.Time != event.Time {
		t.Errorf("Time = %q, want %q", unmarshaled.Time, event.Time)
	}
	if unmarshaled.Region != event.Region {
		t.Errorf("Region = %q, want %q", unmarshaled.Region, event.Region)
	}
	if len(unmarshaled.Resources) != len(event.Resources) {
		t.Errorf("Resources length = %d, want %d", len(unmarshaled.Resources), len(event.Resources))
	} else if len(unmarshaled.Resources) > 0 && unmarshaled.Resources[0] != event.Resources[0] {
		t.Errorf("Resources[0] = %q, want %q", unmarshaled.Resources[0], event.Resources[0])
	}
	// Compare Detail as JSON content
	var expectedDetail, actualDetail interface{}
	if err := json.Unmarshal(event.Detail, &expectedDetail); err == nil {
		if err := json.Unmarshal(unmarshaled.Detail, &actualDetail); err == nil {
			expectedBytes, _ := json.Marshal(expectedDetail)
			actualBytes, _ := json.Marshal(actualDetail)
			if string(expectedBytes) != string(actualBytes) {
				t.Errorf("Detail JSON differs: got %s, want %s", actualBytes, expectedBytes)
			}
		} else {
			t.Errorf("Failed to unmarshal actual Detail: %v", err)
		}
	} else {
		// Fallback to string comparison for non-JSON content
		if string(unmarshaled.Detail) != string(event.Detail) {
			t.Errorf("Detail = %q, want %q", string(unmarshaled.Detail), string(event.Detail))
		}
	}
}

func TestScheduledEvent_RawDetail(t *testing.T) {
	tests := []struct {
		name       string
		detailJSON string
		wantValid  bool
	}{
		{
			name:       "valid JSON object",
			detailJSON: `{"scheduled": true, "rule": "my-rule"}`,
			wantValid:  true,
		},
		{
			name:       "valid JSON array",
			detailJSON: `["item1", "item2"]`,
			wantValid:  true,
		},
		{
			name:       "valid JSON string",
			detailJSON: `"simple string"`,
			wantValid:  true,
		},
		{
			name:       "empty JSON object",
			detailJSON: `{}`,
			wantValid:  true,
		},
		{
			name:       "null JSON",
			detailJSON: `null`,
			wantValid:  true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := ScheduledEvent{
				Version:    "0",
				ID:         "test-id",
				DetailType: "Test Event",
				Source:     "test.source",
				Account:    "123456789012",
				Time:       "2023-01-01T12:00:00Z",
				Region:     "us-east-1",
				Resources:  []string{},
				Detail:     json.RawMessage(tt.detailJSON),
			}
			
			// Marshal
			data, err := json.Marshal(event)
			if err != nil {
				if tt.wantValid {
					t.Errorf("json.Marshal() error = %v, want nil", err)
				}
				return
			}
			
			// Unmarshal
			var unmarshaled ScheduledEvent
			if err := json.Unmarshal(data, &unmarshaled); err != nil {
				if tt.wantValid {
					t.Errorf("json.Unmarshal() error = %v, want nil", err)
				}
				return
			}
			
			// Verify Detail field by comparing JSON content
			var expectedDetail, actualDetail interface{}
			if err := json.Unmarshal([]byte(tt.detailJSON), &expectedDetail); err != nil {
				t.Fatalf("Failed to unmarshal expected detail JSON: %v", err)
			}
			if err := json.Unmarshal(unmarshaled.Detail, &actualDetail); err != nil {
				t.Fatalf("Failed to unmarshal actual detail JSON: %v", err)
			}
			
			expectedBytes, _ := json.Marshal(expectedDetail)
			actualBytes, _ := json.Marshal(actualDetail)
			if string(expectedBytes) != string(actualBytes) {
				t.Errorf("Detail JSON content differs: got %s, want %s", actualBytes, expectedBytes)
			}
		})
	}
}

func TestScheduledEvent_RequiredFields(t *testing.T) {
	// Test with minimal required fields for EventBridge
	event := ScheduledEvent{
		Version:    "0",
		ID:         "test-id",
		DetailType: "Test Event",
		Source:     "test.source",
		Account:    "123456789012",
		Time:       "2023-01-01T12:00:00Z",
		Region:     "us-east-1",
		Resources:  []string{},
		Detail:     json.RawMessage(`{}`),
	}
	
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	var unmarshaled ScheduledEvent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Verify all required fields are preserved
	if unmarshaled.Version != "0" {
		t.Errorf("Version = %q, want \"0\"", unmarshaled.Version)
	}
	if unmarshaled.Source != "test.source" {
		t.Errorf("Source = %q, want \"test.source\"", unmarshaled.Source)
	}
	if unmarshaled.DetailType != "Test Event" {
		t.Errorf("DetailType = %q, want \"Test Event\"", unmarshaled.DetailType)
	}
}

func TestScheduledEvent_EmptyResources(t *testing.T) {
	event := ScheduledEvent{
		Version:    "0",
		ID:         "test-id",
		DetailType: "Test Event",
		Source:     "test.source",
		Account:    "123456789012",
		Time:       "2023-01-01T12:00:00Z",
		Region:     "us-east-1",
		Resources:  []string{}, // Empty resources array
		Detail:     json.RawMessage(`{}`),
	}
	
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	var unmarshaled ScheduledEvent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	if unmarshaled.Resources == nil {
		t.Error("Resources should be empty array, not nil")
	}
	if len(unmarshaled.Resources) != 0 {
		t.Errorf("Resources length = %d, want 0", len(unmarshaled.Resources))
	}
}

func TestScheduledEvent_MultipleResources(t *testing.T) {
	resources := []string{
		"arn:aws:events:us-east-1:123456789012:rule/rule1",
		"arn:aws:events:us-east-1:123456789012:rule/rule2",
		"arn:aws:lambda:us-east-1:123456789012:function:my-function",
	}
	
	event := ScheduledEvent{
		Version:    "0",
		ID:         "test-id",
		DetailType: "Test Event",
		Source:     "test.source",
		Account:    "123456789012",
		Time:       "2023-01-01T12:00:00Z",
		Region:     "us-east-1",
		Resources:  resources,
		Detail:     json.RawMessage(`{}`),
	}
	
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	var unmarshaled ScheduledEvent
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	if len(unmarshaled.Resources) != len(resources) {
		t.Errorf("Resources length = %d, want %d", len(unmarshaled.Resources), len(resources))
	}
	
	for i, resource := range resources {
		if i < len(unmarshaled.Resources) && unmarshaled.Resources[i] != resource {
			t.Errorf("Resources[%d] = %q, want %q", i, unmarshaled.Resources[i], resource)
		}
	}
}