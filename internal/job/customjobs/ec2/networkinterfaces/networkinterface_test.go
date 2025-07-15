// networkinterfaces_test.go
package networkinterfaces

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	sqTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/stretchr/testify/assert"
)

// stubQuotaClient reuses your servicequotaclient interface
type stubQuotaClient struct {
	value  float64
	err    error
	called bool
}

func (s *stubQuotaClient) GetServiceQuota(ctx context.Context, in *servicequotas.GetServiceQuotaInput, _ ...func(*servicequotas.Options)) (*servicequotas.GetServiceQuotaOutput, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	return &servicequotas.GetServiceQuotaOutput{
		Quota: &sqTypes.ServiceQuota{Value: aws.Float64(s.value)},
	}, nil
}

func (s *stubQuotaClient) GetRegion() string {
	return ""
}

func TestExecute_SuccessUsingFakeEC2(t *testing.T) {
	// prepare your FakeEC2Client
	fake := &ec2client.FakeEC2Client{
		Region: "r1",
		// two pages: 3 ENIs, then 2 ENIs
		DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{
			{NetworkInterfaces: make([]ec2Types.NetworkInterface, 3)},
			{NetworkInterfaces: make([]ec2Types.NetworkInterface, 2)},
		},
		ErrOnDescribeENICall: -1,
	}
	// stub quota = 10
	q := &stubQuotaClient{value: 10.0}
	job, err := NewNetworkInterfaceJob(NetworkInterfaceJobConfig{
		Ec2Client:           fake,
		ServiceQuotasClient: q,
		Logger:              nil,
	})
	assert.NoError(t, err, "NewNetworkInterfaceJob should not fail")

	met, err := job.Execute(context.Background())
	assert.NoError(t, err, "Execute should not return error")

	// should get one metric: (3+2)/10*100 = 50
	assert.Len(t, met, 1, "should return exactly one metric")
	m := met[0]
	assert.Equal(t, cloudwatchMetricName, m.Name, "metric name mismatch")
	assert.Equal(t, 50.0, m.Value, "expected 50%% utilization")
	assert.True(t, q.called, "quota client should be called")
	assert.Equal(t, "r1", job.GetRegion(), "region mismatch")
	assert.True(t, startsWith(job.GetJobName(), networkInterfaceJobPrefix+"-r1"), "job name should contain prefix and region")
}

func TestExecute_QuotaError(t *testing.T) {
	fake := &ec2client.FakeEC2Client{
		Region:                         "r2",
		DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{{}},
		ErrOnDescribeENICall:           -1,
	}
	q := &stubQuotaClient{err: errors.New("quota boom")}
	job, _ := NewNetworkInterfaceJob(NetworkInterfaceJobConfig{
		Ec2Client:           fake,
		ServiceQuotasClient: q,
		Logger:              &logger.NoopLogger{},
	})

	_, err := job.Execute(context.Background())
	assert.Error(t, err, "should return error")
	assert.Contains(t, err.Error(), "quota boom", "should contain expected error message")
}

func TestExecute_PaginatorError(t *testing.T) {
	fake := &ec2client.FakeEC2Client{
		Region:                         "r3",
		DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{{}, {}},
		ErrOnDescribeENICall:           1, // error on first page
	}
	q := &stubQuotaClient{value: 1}
	job, _ := NewNetworkInterfaceJob(NetworkInterfaceJobConfig{
		Ec2Client:           fake,
		ServiceQuotasClient: q,
		Logger:              &logger.NoopLogger{},
	})

	_, err := job.Execute(context.Background())
	assert.Error(t, err, "should return pagination error")
	assert.Contains(t, err.Error(), "error", "should contain error message")
}

// Test that DescribeNetworkInterfaces pagination respects NextToken
func TestFakeEC2Pagination(t *testing.T) {
	fake := &ec2client.FakeEC2Client{
		Region: "r4",
		DescribeNetworkInterfacesPages: []*ec2.DescribeNetworkInterfacesOutput{
			{NetworkInterfaces: make([]ec2Types.NetworkInterface, 1)},
			{NetworkInterfaces: make([]ec2Types.NetworkInterface, 0)},
		},
		ErrOnDescribeENICall: -1,
	}
	// manually page through using the AWS paginator to confirm the fake
	p := ec2.NewDescribeNetworkInterfacesPaginator(fake, &ec2.DescribeNetworkInterfacesInput{})
	count := 0
	for p.HasMorePages() {
		out, err := p.NextPage(context.Background())
		assert.NoError(t, err, "pagination should not error")
		count += len(out.NetworkInterfaces)
	}
	assert.Equal(t, 1, count, "should get exactly 1 interface total")
}

// helper
func startsWith(s, pref string) bool {
	return len(s) >= len(pref) && s[:len(pref)] == pref
}
