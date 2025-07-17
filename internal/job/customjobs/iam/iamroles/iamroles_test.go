package iamroles

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	sqTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
)

// fakeIAMClient implements iamclient.IamClient
type fakeIAMClient struct {
	Region    string
	Roles     []iamTypes.Role
	NextMarker string
	IsTruncated bool
	ListRolesErr error
	ListRolesCalls int
}

func (f *fakeIAMClient) ListRoles(
	ctx context.Context,
	in *iam.ListRolesInput,
	opts ...func(*iam.Options),
) (*iam.ListRolesOutput, error) {
	f.ListRolesCalls++
	
	if f.ListRolesErr != nil {
		return nil, f.ListRolesErr
	}
	
	// For pagination testing
	if f.ListRolesCalls == 1 && f.IsTruncated {
		return &iam.ListRolesOutput{
			Roles: f.Roles[:len(f.Roles)/2],
			IsTruncated: true,
			Marker: aws.String(f.NextMarker),
		}, nil
	} else if f.ListRolesCalls == 2 && f.IsTruncated && in.Marker != nil && *in.Marker == f.NextMarker {
		// Second page of results
		return &iam.ListRolesOutput{
			Roles: f.Roles[len(f.Roles)/2:],
			IsTruncated: false,
		}, nil
	}
	
	return &iam.ListRolesOutput{
		Roles: f.Roles,
		IsTruncated: false,
	}, nil
}

func (f *fakeIAMClient) ListOpenIDConnectProviders(
	ctx context.Context,
	in *iam.ListOpenIDConnectProvidersInput,
	opts ...func(*iam.Options),
) (*iam.ListOpenIDConnectProvidersOutput, error) {
	return &iam.ListOpenIDConnectProvidersOutput{}, nil
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

// Helper to create test roles
func createTestRoles(count int) []iamTypes.Role {
	roles := make([]iamTypes.Role, count)
	for i := range roles {
		roles[i] = iamTypes.Role{
			RoleName: aws.String("role-" + string(rune('A'+i))),
		}
	}
	return roles
}

func TestIamRoleJob_Execute(t *testing.T) {
	tests := []struct {
		name         string
		roles        []iamTypes.Role
		isTruncated  bool
		nextMarker   string
		listRolesErr error
		quotaValue   float64
		quotaErr     error
		expectError  bool
		expectPct    float64
		useNilLogger bool
	}{
		{
			name:        "success with roles",
			roles:       createTestRoles(5),
			quotaValue:  100,
			expectError: false,
			expectPct:   5.0,
		},
		{
			name:         "pagination test",
			roles:        createTestRoles(10),
			isTruncated:  true,
			nextMarker:   "nextpage",
			quotaValue:   100,
			expectError:  false,
			expectPct:    10.0,
		},
		{
			name:         "list roles error",
			listRolesErr: errors.New("list roles failed"),
			quotaValue:   100,
			expectError:  true,
		},
		{
			name:        "quota error",
			roles:       createTestRoles(3),
			quotaErr:    errors.New("quota fail"),
			expectError: true,
		},
		{
			name:         "default logger on nil",
			roles:        createTestRoles(2),
			quotaValue:   100,
			expectError:  false,
			expectPct:    2.0,
			useNilLogger: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iamFake := &fakeIAMClient{
				Region:      "us-east-1",
				Roles:       tc.roles,
				IsTruncated: tc.isTruncated,
				NextMarker:  tc.nextMarker,
				ListRolesErr: tc.listRolesErr,
			}
			
			quotaFake := &fakeQuotaClient{
				Region: "us-east-1",
				Value:  tc.quotaValue,
				Err:    tc.quotaErr,
			}

			cfg := IamRoleJobConfig{
				IamClient:           iamFake,
				ServiceQuotasClient: quotaFake,
			}
			if !tc.useNilLogger {
				cfg.Logger = &logger.NoopLogger{}
			}
			
			job, err := NewIamRoleJob(cfg)
			assert.NoError(t, err, "NewIamRoleJob should not fail")

			metrics, err := job.Execute(context.Background())
			if tc.expectError {
				assert.Error(t, err, "expected error but got none")
				return
			}
			assert.NoError(t, err, "unexpected error")

			// one metric returned
			assert.Len(t, metrics, 1, "should return exactly one metric")
			m := metrics[0]

			assert.Equal(t, sharedtypes.JobIAMRoleUtilization, m.Name, "metric name mismatch")
			assert.Equal(t, sharedtypes.UnitPercent, m.Unit, "unit should be percent")
			assert.Equal(t, tc.expectPct, m.Value, "utilization percentage mismatch")
			assert.True(t, quotaFake.Called, "quota client should be called")
			assert.True(t, time.Since(m.Timestamp) < time.Second, "timestamp should be recent")

			// Check getters
			assert.Equal(t, "us-east-1", job.GetRegion(), "region mismatch")
			assert.Contains(t, job.GetJobName(), string(sharedtypes.JobIAMRoleUtilization), "job name should contain metric name")
		})
	}
}