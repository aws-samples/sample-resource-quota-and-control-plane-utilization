package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestJobName_String(t *testing.T) {
	tests := []struct {
		name     JobName
		expected string
	}{
		{JobNetworkInterfaceUtilization, "NetworkInterfaceUtilization"},
		{JobGP3StorageUtilization, "GP3StorageUtilization"},
		{JobIAMRoleUtilization, "IAMRoleUtilization"},
		{JobOIDCProviderUtilization, "OIDCProviderUtilization"},
		{JobEKSClusterUtilization, "EKSClusterUtilization"},
		{JobNetworkAddressUnitsUtilization, "NetworkAddressUnitsUtilization"},
	}

	for _, tt := range tests {
		if got := tt.name.String(); got != tt.expected {
			t.Errorf("JobName(%q).String() = %q, want %q", tt.name, got, tt.expected)
		}
	}
}

func TestJobName_Validate(t *testing.T) {
	tests := []struct {
		name    JobName
		wantErr bool
	}{
		{JobNetworkInterfaceUtilization, false},
		{JobGP3StorageUtilization, false},
		{JobIAMRoleUtilization, false},
		{JobOIDCProviderUtilization, false},
		{JobEKSClusterUtilization, false},
		{JobNetworkAddressUnitsUtilization, false},
		{"InvalidJobName", true},
		{"", true},
	}

	for _, tt := range tests {
		err := tt.name.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("JobName(%q).Validate() error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
		if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidJobName) {
			t.Errorf("JobName(%q).Validate() error = %v, want wrapped ErrInvalidJobName", tt.name, err)
		}
	}
}

func TestJobName_JSONSerialization(t *testing.T) {
	// Test marshaling and unmarshaling JobName
	original := JobNetworkInterfaceUtilization
	
	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	
	// Verify it's marshaled as a string
	if string(data) != `"NetworkInterfaceUtilization"` {
		t.Errorf("Marshaled data = %s, want \"NetworkInterfaceUtilization\"", data)
	}
	
	// Unmarshal
	var unmarshaled JobName
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	
	// Verify
	if unmarshaled != original {
		t.Errorf("Unmarshaled = %q, want %q", unmarshaled, original)
	}
}

func TestJobName_Constants(t *testing.T) {
	// Verify all constants are defined correctly
	if JobNetworkInterfaceUtilization != "NetworkInterfaceUtilization" {
		t.Errorf("JobNetworkInterfaceUtilization = %q, want \"NetworkInterfaceUtilization\"", JobNetworkInterfaceUtilization)
	}
	if JobGP3StorageUtilization != "GP3StorageUtilization" {
		t.Errorf("JobGP3StorageUtilization = %q, want \"GP3StorageUtilization\"", JobGP3StorageUtilization)
	}
	if JobIAMRoleUtilization != "IAMRoleUtilization" {
		t.Errorf("JobIAMRoleUtilization = %q, want \"IAMRoleUtilization\"", JobIAMRoleUtilization)
	}
	if JobOIDCProviderUtilization != "OIDCProviderUtilization" {
		t.Errorf("JobOIDCProviderUtilization = %q, want \"OIDCProviderUtilization\"", JobOIDCProviderUtilization)
	}
	if JobEKSClusterUtilization != "EKSClusterUtilization" {
		t.Errorf("JobEKSClusterUtilization = %q, want \"EKSClusterUtilization\"", JobEKSClusterUtilization)
	}
	if JobNetworkAddressUnitsUtilization != "NetworkAddressUnitsUtilization" {
		t.Errorf("JobNetworkAddressUnitsUtilization = %q, want \"NetworkAddressUnitsUtilization\"", JobNetworkAddressUnitsUtilization)
	}
}