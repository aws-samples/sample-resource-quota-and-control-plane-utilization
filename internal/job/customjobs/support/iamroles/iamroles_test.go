// iamroles_test.go
package iamroles

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/support"
	supportTypes "github.com/aws/aws-sdk-go-v2/service/support/types"
	supportclient "github.com/outofoffice3/aws-samples/geras/internal/awsclients/supportclient"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// --- fake -------------------------------------------------------------------

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
	optFns ...func(*support.Options),
) (*support.RefreshTrustedAdvisorCheckOutput, error) {
	f.ReceivedCheckID = aws.ToString(in.CheckId)
	return f.RefreshOut, f.RefreshErr
}

func (f *fakeSupportClient) GetRegion() string { return f.Region }

// ---------------------------------------------------------------------------

func TestIamRoleJob(t *testing.T) {
	cases := []struct {
		name          string
		out           *support.RefreshTrustedAdvisorCheckOutput
		err           error
		expectErr     bool
		expectCheckID string
		useNilLogger  bool
	}{
		{
			name: "success",
			out: &support.RefreshTrustedAdvisorCheckOutput{
				Status: &supportTypes.TrustedAdvisorCheckRefreshStatus{
					Status: aws.String("ok"),
				},
			},
			err:           nil,
			expectErr:     false,
			expectCheckID: IamRoleCheckId,
		},
		{
			name:      "api error",
			out:       nil,
			err:       errors.New("fail"),
			expectErr: true,
		},
		{
			name: "nil logger",
			out: &support.RefreshTrustedAdvisorCheckOutput{
				Status: &supportTypes.TrustedAdvisorCheckRefreshStatus{
					Status: aws.String("ok"),
				},
			},
			err:           nil,
			expectErr:     false,
			expectCheckID: IamRoleCheckId,
			useNilLogger:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSupportClient{
				Region:     "eu-central-1",
				RefreshOut: tc.out,
				RefreshErr: tc.err,
			}

			cfg := IamRoleJobConfig{SupportClient: supportclient.SupportClient(fake)}
			if !tc.useNilLogger {
				cfg.Logger = &logger.NoopLogger{}
			}

			job, err := NewIamRoleJob(cfg)
			if err != nil {
				t.Fatalf("NewIamRoleJob returned error: %v", err)
			}

			metrics, err := job.Execute(context.Background())
			if tc.expectErr {
				if err == nil {
					t.Fatal("expected an error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// on success, Execute always returns nil metrics
			if metrics != nil {
				t.Errorf("expected nil metrics slice, got %#v", metrics)
			}
			// correct CheckId forwarded?
			if fake.ReceivedCheckID != tc.expectCheckID {
				t.Errorf("RefreshTrustedAdvisorCheck called with %q; want %q",
					fake.ReceivedCheckID, tc.expectCheckID)
			}
			// GetRegion/GetJobName
			if job.GetRegion() != "eu-central-1" {
				t.Errorf("GetRegion = %q; want eu-central-1", job.GetRegion())
			}
			wantName := IamRoleJobPrefix + "-eu-central-1"
			if job.GetJobName() != wantName {
				t.Errorf("GetJobName = %q; want %q", job.GetJobName(), wantName)
			}
		})
	}
}
