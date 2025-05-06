// internal/vpcnau/job_test.go
package vpcnau

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
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

func TestNewVPCNAUJob_Getters(t *testing.T) {
	c := &fakeCalc{out: nil, region: "us-test-1"}
	j, err := NewVPCNAUJob(VPCNAUConfig{
		NauCalculator: c,
		Logger:        nil,
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
	j, _ := NewVPCNAUJob(VPCNAUConfig{
		NauCalculator: calc,
		Logger:        &logger.NoopLogger{},
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
		assert.Equal(t, types.StandardUnitCount, m.Unit, "unit should be Count")
		v := m.Metadata["vpc"]
		// valid vpc id
		_, ok := outMap[v]
		assert.True(t, ok, "metadata[vpc] must be one of the input keys")
		assert.Equal(t, float64(outMap[v]), m.Value, "Value must match injected NAU")
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
		Logger:        &logger.NoopLogger{},
	})

	mets, err := j.Execute(context.Background())
	assert.Nil(t, mets, "metrics should be nil on error")
	assert.Equal(t, want, err, "error must propagate from calculator")
}
