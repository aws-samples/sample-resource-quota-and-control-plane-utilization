package oidcproviders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	sqTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

////////////////////////////////////////////////////////////////////////////////
// fakes

// fakeIAMClient implements iamclient.IamClient
type fakeIAMClient struct {
	Region    string
	Providers []iamTypes.OpenIDConnectProviderListEntry
	Err       error
}

func (f *fakeIAMClient) ListOpenIDConnectProviders(
	ctx context.Context,
	in *iam.ListOpenIDConnectProvidersInput,
	opts ...func(*iam.Options),
) (*iam.ListOpenIDConnectProvidersOutput, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return &iam.ListOpenIDConnectProvidersOutput{
		OpenIDConnectProviderList: f.Providers,
	}, nil
}

func (f *fakeIAMClient) ListRoles(
	ctx context.Context,
	in *iam.ListRolesInput,
	opts ...func(*iam.Options),
) (*iam.ListRolesOutput,
	error) {
	return &iam.ListRolesOutput{}, nil
}

func (f *fakeIAMClient) GetRegion() string { return f.Region }

// fakeQuotaClient implements servicequotaclient.ServiceQuotasClient
type fakeQuotaClient struct {
	Region string
	Value  float64
	Err    error
	Called bool
}

func (f *fakeQuotaClient) GetServiceQuota(
	ctx context.Context,
	in *servicequotas.GetServiceQuotaInput,
	opts ...func(*servicequotas.Options),
) (*servicequotas.GetServiceQuotaOutput, error) {
	f.Called = true
	if f.Err != nil {
		return nil, f.Err
	}
	return &servicequotas.GetServiceQuotaOutput{
		Quota: &sqTypes.ServiceQuota{Value: aws.Float64(f.Value)},
	}, nil
}

func (f *fakeQuotaClient) GetRegion() string { return f.Region }

////////////////////////////////////////////////////////////////////////////////
// tests

func TestOIDCProviderJob_Execute(t *testing.T) {
	tests := []struct {
		name         string
		iamProviders []iamTypes.OpenIDConnectProviderListEntry
		iamErr       error
		quotaValue   float64
		quotaErr     error
		expectError  bool
		expectPct    float64
		useNilLogger bool
	}{
		{
			name:         "success with two providers",
			iamProviders: []iamTypes.OpenIDConnectProviderListEntry{{}, {}},
			quotaValue:   10,
			expectError:  false,
			expectPct:    (2.0 / 10) * 100,
		},
		{
			name:        "IAM list error",
			iamErr:      errors.New("iam failure"),
			quotaValue:  100,
			expectError: true,
		},
		{
			name:         "quota error",
			iamProviders: []iamTypes.OpenIDConnectProviderListEntry{{}},
			quotaErr:     errors.New("quota fail"),
			expectError:  true,
		},
		{
			name:         "default logger on nil",
			iamProviders: []iamTypes.OpenIDConnectProviderListEntry{{}},
			quotaValue:   5,
			expectError:  false,
			expectPct:    (1.0 / 5) * 100,
			useNilLogger: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iamFake := &fakeIAMClient{
				Region:    "r-1",
				Providers: tc.iamProviders,
				Err:       tc.iamErr,
			}
			quotaFake := &fakeQuotaClient{
				Region: "r-1",
				Value:  tc.quotaValue,
				Err:    tc.quotaErr,
			}

			cfg := OIDCProviderJobConfig{
				IamClient:          iamFake,
				ServiceQuotasCliet: servicequotaclient.ServiceQuotasClient(quotaFake),
			}
			if !tc.useNilLogger {
				cfg.Logger = &logger.NoopLogger{}
			}
			job, err := NewOIDCProviderJob(cfg)
			if err != nil {
				t.Fatalf("NewOIDCProviderJob failed: %v", err)
			}

			metrics, err := job.Execute(context.Background())
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// one metric returned
			if len(metrics) != 1 {
				t.Fatalf("got %d metrics, want 1", len(metrics))
			}
			m := metrics[0]

			if m.Name != cloudwatchMetricName {
				t.Errorf("Name = %q, want %q", m.Name, cloudwatchMetricName)
			}
			// allow small time difference
			if time.Since(m.Timestamp) > time.Second {
				t.Errorf("Timestamp too old: %v", m.Timestamp)
			}
			if m.Unit != cwTypes.StandardUnitPercent {
				t.Errorf("Unit = %v, want %v", m.Unit, cwTypes.StandardUnitPercent)
			}
			if pct := m.Value; pct != tc.expectPct {
				t.Errorf("Value = %.2f, want %.2f", pct, tc.expectPct)
			}
			if !quotaFake.Called {
				t.Error("expected quota client to be called")
			}

			// getters
			if job.GetRegion() != "r-1" {
				t.Errorf("GetRegion = %q, want r-1", job.GetRegion())
			}
			if got := job.GetJobName(); got[:len(oidcProvidersJobPrefix)] != oidcProvidersJobPrefix {
				t.Errorf("GetJobName = %q, want prefix %q", got, oidcProvidersJobPrefix)
			}
		})
	}
}
