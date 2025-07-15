package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCloudTrailEvent_MarshalJSON(t *testing.T) {
	eventTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	event := CloudTrailEvent{
		EventVersion: "1.05",
		EventTime:    eventTime,
		EventSource:  "ec2.amazonaws.com",
		EventName:    "DescribeInstances",
		AWSRegion:    "us-east-1",
		SourceIP:     "192.168.1.1",
		UserAgent:    "aws-cli/2.0.0",
		RequestID:    "12345678-1234-1234-1234-123456789012",
		EventID:      "87654321-4321-4321-4321-210987654321",
		UserIdentity: UserIdentity{
			Type:        "IAMUser",
			PrincipalId: "AIDACKCEVSQ6C2EXAMPLE",
			ARN:         "arn:aws:iam::123456789012:user/testuser",
			AccountId:   "123456789012",
		},
	}
	
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Verify key fields are present
	jsonStr := string(data)
	expectedFields := []string{
		"eventVersion", "eventTime", "eventSource", "eventName",
		"awsRegion", "sourceIPAddress", "userAgent", "requestID",
		"eventID", "userIdentity",
	}
	
	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("JSON should contain field %q", field)
		}
	}
}

func TestCloudTrailEvent_UnmarshalJSON(t *testing.T) {
	jsonData := `{
		"eventVersion": "1.05",
		"eventTime": "2023-01-01T12:00:00Z",
		"eventSource": "ec2.amazonaws.com",
		"eventName": "DescribeInstances",
		"awsRegion": "us-east-1",
		"sourceIPAddress": "192.168.1.1",
		"userAgent": "aws-cli/2.0.0",
		"requestID": "12345678-1234-1234-1234-123456789012",
		"eventID": "87654321-4321-4321-4321-210987654321",
		"userIdentity": {
			"type": "IAMUser",
			"principalId": "AIDACKCEVSQ6C2EXAMPLE",
			"arn": "arn:aws:iam::123456789012:user/testuser",
			"accountId": "123456789012"
		}
	}`
	
	var event CloudTrailEvent
	if err := json.Unmarshal([]byte(jsonData), &event); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Verify required fields
	if event.EventVersion != "1.05" {
		t.Errorf("EventVersion = %q, want \"1.05\"", event.EventVersion)
	}
	if event.EventName != "DescribeInstances" {
		t.Errorf("EventName = %q, want \"DescribeInstances\"", event.EventName)
	}
	if event.AWSRegion != "us-east-1" {
		t.Errorf("AWSRegion = %q, want \"us-east-1\"", event.AWSRegion)
	}
	if event.UserIdentity.Type != "IAMUser" {
		t.Errorf("UserIdentity.Type = %q, want \"IAMUser\"", event.UserIdentity.Type)
	}
}

func TestUserIdentity_JSONRoundTrip(t *testing.T) {
	creationTime := time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)
	identity := UserIdentity{
		Type:        "AssumedRole",
		PrincipalId: "AROACKCEVSQ6C2EXAMPLE:session-name",
		ARN:         "arn:aws:sts::123456789012:assumed-role/MyRole/session-name",
		AccountId:   "123456789012",
		InvokedBy:   "lambda.amazonaws.com",
		SessionContext: &SessionContextDetails{
			Attributes: struct {
				CreationDate     time.Time `json:"creationDate"`
				MfaAuthenticated string    `json:"mfaAuthenticated"`
			}{
				CreationDate:     creationTime,
				MfaAuthenticated: "false",
			},
			SessionIssuer: struct {
				Type        string `json:"type"`
				PrincipalId string `json:"principalId"`
				ARN         string `json:"arn"`
				UserName    string `json:"userName,omitempty"`
			}{
				Type:        "Role",
				PrincipalId: "AROACKCEVSQ6C2EXAMPLE",
				ARN:         "arn:aws:iam::123456789012:role/MyRole",
				UserName:    "MyRole",
			},
		},
	}
	
	// Marshal
	data, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Unmarshal
	var unmarshaled UserIdentity
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Verify
	if unmarshaled.Type != identity.Type {
		t.Errorf("Type = %q, want %q", unmarshaled.Type, identity.Type)
	}
	if unmarshaled.SessionContext == nil {
		t.Fatal("SessionContext should not be nil")
	}
	if !unmarshaled.SessionContext.Attributes.CreationDate.Equal(creationTime) {
		t.Errorf("CreationDate = %v, want %v", unmarshaled.SessionContext.Attributes.CreationDate, creationTime)
	}
}

func TestSessionContextDetails_OptionalFields(t *testing.T) {
	// Test with minimal required fields
	jsonData := `{
		"type": "Root",
		"principalId": "123456789012",
		"arn": "arn:aws:iam::123456789012:root",
		"accountId": "123456789012"
	}`
	
	var identity UserIdentity
	if err := json.Unmarshal([]byte(jsonData), &identity); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Optional fields should be empty/nil
	if identity.InvokedBy != "" {
		t.Errorf("InvokedBy should be empty, got %q", identity.InvokedBy)
	}
	if identity.SessionContext != nil {
		t.Error("SessionContext should be nil for Root user")
	}
}

func TestCloudTrailEvent_RequiredFields(t *testing.T) {
	event := CloudTrailEvent{
		EventVersion: "1.05",
		EventTime:    time.Now(),
		EventSource:  "s3.amazonaws.com",
		EventName:    "GetObject",
		AWSRegion:    "us-west-2",
		SourceIP:     "10.0.0.1",
		UserAgent:    "aws-sdk-go/1.0.0",
		RequestID:    "req-123",
		EventID:      "evt-456",
		UserIdentity: UserIdentity{
			Type:      "IAMUser",
			AccountId: "123456789012",
		},
	}
	
	// Should marshal without error
	_, err := json.Marshal(event)
	if err != nil {
		t.Errorf("Marshal with required fields should not error, got %v", err)
	}
}

func TestCloudTrailEvent_OptionalFields(t *testing.T) {
	event := CloudTrailEvent{
		EventVersion:       "1.05",
		EventTime:          time.Now(),
		EventSource:        "s3.amazonaws.com",
		EventName:          "GetObject",
		AWSRegion:          "us-west-2",
		SourceIP:           "10.0.0.1",
		UserAgent:          "aws-sdk-go/1.0.0",
		RequestID:          "req-123",
		EventID:            "evt-456",
		UserIdentity:       UserIdentity{Type: "IAMUser"},
		RecipientAccountId: "987654321098",
		ErrorCode:          "AccessDenied",
		ErrorMessage:       "Access denied",
	}
	
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	jsonStr := string(data)
	
	// Optional fields should be present when set
	if !contains(jsonStr, "recipientAccountId") {
		t.Error("JSON should contain recipientAccountId when set")
	}
	if !contains(jsonStr, "errorCode") {
		t.Error("JSON should contain errorCode when set")
	}
	if !contains(jsonStr, "errorMessage") {
		t.Error("JSON should contain errorMessage when set")
	}
}

func TestUserIdentity_Types(t *testing.T) {
	identityTypes := []string{
		"Root", "IAMUser", "AssumedRole", "Role", "FederatedUser",
		"Directory", "Unknown", "AWSService", "AWSAccount", "IdentityCenterUser",
	}
	
	for _, idType := range identityTypes {
		identity := UserIdentity{
			Type:      idType,
			AccountId: "123456789012",
		}
		
		data, err := json.Marshal(identity)
		if err != nil {
			t.Errorf("Marshal identity type %q error = %v", idType, err)
			continue
		}
		
		var unmarshaled UserIdentity
		if err := json.Unmarshal(data, &unmarshaled); err != nil {
			t.Errorf("Unmarshal identity type %q error = %v", idType, err)
			continue
		}
		
		if unmarshaled.Type != idType {
			t.Errorf("Identity type %q != %q after round trip", unmarshaled.Type, idType)
		}
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || 
		s[len(s)-len(substr):] == substr || 
		containsAt(s, substr, 1))))
}

func containsAt(s, substr string, start int) bool {
	if start >= len(s) {
		return false
	}
	if start+len(substr) <= len(s) && s[start:start+len(substr)] == substr {
		return true
	}
	return containsAt(s, substr, start+1)
}