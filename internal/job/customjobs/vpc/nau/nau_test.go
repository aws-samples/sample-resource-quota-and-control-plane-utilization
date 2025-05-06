// internal/vpcnau/job_test.go
package vpcnau

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	sqTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	"github.com/stretchr/testify/assert"

	"github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// fakeCalc is a stub implementation of nau.NAUCalculator
type fakeCalc struct {
	out    map[string]int64
	err    error
	region string
}

func (f *fakeCalc) CalculateVPCNAU(ctx context.Context) (map[string]int64, error) {
	return f.out, f.err
}

func (f *fakeCalc) GetRegion() string {
	return f.region
}

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

func TestNewVPCNAUJob_Getters(t *testing.T) {
	c := &fakeCalc{out: nil, region: "us-test-1"}
	j, err := NewVPCNAUJob(VPCNAUConfig{
		NauCalculator:       c,
		ServiceQuotasClient: &fakeQuotaClient{},
		Logger:              nil,
	})
	assert.NoError(t, err, "should construct without error")
	assert.Equal(t, "vpcNAU-us-test-1", j.GetJobName(), "job name must include prefix and region")
	assert.Equal(t, "us-test-1", j.GetRegion(), "region getter should match calculator")
}

func TestExecute_Success(t *testing.T) {
	nowBefore := time.Now()
	// inject two VPCs with different unit counts
	outMap := map[string]int64{
		"vpc-A": 5,
		"vpc-B": 0,
	}
	calc := &fakeCalc{out: outMap, region: "eu-west-1"}
	quotaValue := 100
	j, _ := NewVPCNAUJob(VPCNAUConfig{
		NauCalculator: calc,
		ServiceQuotasClient: &fakeQuotaClient{
			Value: float64(quotaValue),
		},
		Logger: &logger.NoopLogger{},
	})

	mets, err := j.Execute(context.Background())
	assert.NoError(t, err, "no error when calculator succeeds")
	// should produce exactly len(outMap) metrics
	assert.Len(t, mets, len(outMap), "should emit one metric per VPC")

	// Track which vpcs we've seen
	seen := map[string]bool{"vpc-A": false, "vpc-B": false}
	for _, m := range mets {
		// name, unit, metadata, timestamp
		assert.Equal(t, cloudwatchMetricName, m.Name, "metric name should be constant")
		assert.Equal(t, types.StandardUnitPercent, m.Unit, "unit should be percent")
		v := m.Metadata["vpc"]
		// valid vpc id
		_, ok := outMap[v]
		assert.True(t, ok, "metadata[vpc] must be one of the input keys")
		assert.Equal(t, float64(outMap[v])/float64(quotaValue), m.Value, "Value must match injected NAU")
		// timestamp between nowBefore and nowAfter
		assert.True(t, m.Timestamp.After(nowBefore), "timestamp should be set to now")
		seen[v] = true
	}
	for id, was := range seen {
		assert.Truef(t, was, "must have emitted metric for %s", id)
	}
}

func TestExecute_CalcError(t *testing.T) {
	want := errors.New("calculator failure")
	calc := &fakeCalc{err: want, region: "ap-south-1"}
	j, _ := NewVPCNAUJob(VPCNAUConfig{
		NauCalculator: calc,
		ServiceQuotasClient: &fakeQuotaClient{
			Value: 100,
		},
		Logger: &logger.NoopLogger{},
	})

	mets, err := j.Execute(context.Background())
	assert.Nil(t, mets, "metrics should be nil on error")
	assert.Equal(t, want, err, "error must propagate from calculator")
}
