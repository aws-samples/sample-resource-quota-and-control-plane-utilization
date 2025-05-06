// gp3storage_test.go
package gp3storage

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/support"
	supportTypes "github.com/aws/aws-sdk-go-v2/service/support/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/supportclient"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// --- fakes ------------------------------------------------------------------

// fakeSupportClient implements supportclient.SupportClient
type fakeSupportClient struct {
	Region          string
	RefreshOut      *support.RefreshTrustedAdvisorCheckOutput
	RefreshErr      error
	ReceivedCheckID string
}

func (f *fakeSupportClient) RefreshTrustedAdvisorCheck(
	ctx context.Context,
	in *support.RefreshTrustedAdvisorCheckInput,
	opts ...func(*support.Options),
) (*support.RefreshTrustedAdvisorCheckOutput, error) {
	f.ReceivedCheckID = aws.ToString(in.CheckId)
	return f.RefreshOut, f.RefreshErr
}

func (f *fakeSupportClient) GetRegion() string { return f.Region }

// ---------------------------------------------------------------------------

func TestGp3StorageJob_Execute(t *testing.T) {
	tests := []struct {
		name          string
		fakeOut       *support.RefreshTrustedAdvisorCheckOutput
		fakeErr       error
		expectError   bool
		expectCheckID string
		useNilLogger  bool
	}{
		{
			name: "success path",
			fakeOut: &support.RefreshTrustedAdvisorCheckOutput{
				Status: &supportTypes.TrustedAdvisorCheckRefreshStatus{
					Status: aws.String("success"),
				},
			},
			fakeErr:       nil,
			expectError:   false,
			expectCheckID: gp3StorageCheckId,
		},
		{
			name:        "error path",
			fakeOut:     nil,
			fakeErr:     errors.New("refresh fail"),
			expectError: true,
		},
		{
			name: "nil logger defaults",
			fakeOut: &support.RefreshTrustedAdvisorCheckOutput{
				Status: &supportTypes.TrustedAdvisorCheckRefreshStatus{
					Status: aws.String("ok"),
				},
			},
			fakeErr:       nil,
			expectError:   false,
			expectCheckID: gp3StorageCheckId,
			useNilLogger:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSupportClient{
				Region:     "us-west-2",
				RefreshOut: tc.fakeOut,
				RefreshErr: tc.fakeErr,
			}

			cfg := Gp3StorageJobConfig{SupportClient: supportclient.SupportClient(fake)}
			if !tc.useNilLogger {
				cfg.Logger = &logger.NoopLogger{}
			}

			j, err := NewGp3StorageJob(cfg)
			if err != nil {
				t.Fatalf("NewGp3StorageJob error: %v", err)
			}

			metrics, err := j.Execute(context.Background())
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Execute returns nil metrics on success
			if metrics != nil {
				t.Errorf("expected nil metrics slice, got %v", metrics)
			}
			// ensure we passed correct CheckId
			if fake.ReceivedCheckID != tc.expectCheckID {
				t.Errorf("RefreshTrustedAdvisorCheck called with %q, want %q",
					fake.ReceivedCheckID, tc.expectCheckID)
			}
			// verify region and job name
			if j.GetRegion() != "us-west-2" {
				t.Errorf("GetRegion = %q, want us-west-2", j.GetRegion())
			}
			wantPrefix := gp3JobPrefix + "-us-west-2"
			if j.GetJobName() != wantPrefix {
				t.Errorf("GetJobName = %q, want %q", j.GetJobName(), wantPrefix)
			}
		})
	}
}
