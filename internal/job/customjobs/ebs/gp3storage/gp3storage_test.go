package gp3storage

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	sqTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
)

// fakeEC2Client implements ec2client.Ec2Client for testing
type fakeEC2Client struct {
	Region               string
	DescribeVolumesPages []*ec2.DescribeVolumesOutput
	ErrOnDescribeVolumes int
	callCount            int
}

func (f *fakeEC2Client) DescribeVolumes(
	ctx context.Context,
	params *ec2.DescribeVolumesInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeVolumesOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	
	if f.callCount == f.ErrOnDescribeVolumes {
		return nil, errors.New("error describing volumes")
	}
	
	idx := 0
	if params.NextToken != nil {
		i, err := strconv.Atoi(*params.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}
	
	var out *ec2.DescribeVolumesOutput
	if idx < len(f.DescribeVolumesPages) {
		page := f.DescribeVolumesPages[idx]
		out = &ec2.DescribeVolumesOutput{
			Volumes: page.Volumes,
		}
	} else {
		out = &ec2.DescribeVolumesOutput{}
	}
	
	if idx+1 < len(f.DescribeVolumesPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}
	
	f.callCount++
	return out, nil
}

// Implement other required methods of the Ec2Client interface with empty implementations
func (f *fakeEC2Client) DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return nil, nil
}

func (f *fakeEC2Client) DescribeNetworkInterfaces(ctx context.Context, params *ec2.DescribeNetworkInterfacesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error) {
	return nil, nil
}

func (f *fakeEC2Client) DescribeNatGateways(ctx context.Context, params *ec2.DescribeNatGatewaysInput, optFns ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	return nil, nil
}

func (f *fakeEC2Client) DescribeVpcEndpoints(ctx context.Context, params *ec2.DescribeVpcEndpointsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error) {
	return nil, nil
}

func (f *fakeEC2Client) DescribeSubnets(ctx context.Context, params *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return nil, nil
}

func (f *fakeEC2Client) DescribeTransitGatewayVpcAttachments(ctx context.Context, params *ec2.DescribeTransitGatewayVpcAttachmentsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	return nil, nil
}

func (f *fakeEC2Client) DescribeAvailabilityZones(ctx context.Context, params *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return nil, nil
}

func (f *fakeEC2Client) GetRegion() string { return f.Region }

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

// Helper to create test volumes
func createTestVolumes(count int, sizeGiB int32) []ec2Types.Volume {
	volumes := make([]ec2Types.Volume, count)
	for i := range volumes {
		volumes[i] = ec2Types.Volume{
			Size:       aws.Int32(sizeGiB),
			VolumeType: ec2Types.VolumeTypeGp3,
		}
	}
	return volumes
}

// Helper to build DescribeVolumesOutput pages
func makeVolumesPages(sizes ...int32) []*ec2.DescribeVolumesOutput {
	var out []*ec2.DescribeVolumesOutput
	for _, size := range sizes {
		out = append(out, &ec2.DescribeVolumesOutput{
			Volumes: createTestVolumes(1, size),
		})
	}
	return out
}

func TestGp3StorageJob_Execute(t *testing.T) {
	tests := []struct {
		name               string
		volumePages        []*ec2.DescribeVolumesOutput
		errOnPage          int
		quotaValue         float64
		quotaErr           error
		expectError        bool
		expectPct          float64
		useNilLogger       bool
	}{
		{
			name:        "success with volumes",
			volumePages: makeVolumesPages(100, 200, 300), // 600 GiB total
			errOnPage:   -1,
			quotaValue:  50, // 50 TiB quota
			expectError: false,
			expectPct:   (600 * 1073741824 / float64(bytesPerTiB)) / 50 * 100, // ~1.17%
		},
		{
			name:        "pagination test",
			volumePages: makeVolumesPages(500, 500), // 1000 GiB total
			errOnPage:   -1,
			quotaValue:  50, // 50 TiB quota
			expectError: false,
			expectPct:   (1000 * 1073741824 / float64(bytesPerTiB)) / 50 * 100, // ~1.96%
		},
		{
			name:        "describe volumes error",
			volumePages: makeVolumesPages(100),
			errOnPage:   0,
			quotaValue:  50,
			expectError: true,
		},
		{
			name:        "quota error",
			volumePages: makeVolumesPages(100),
			errOnPage:   -1,
			quotaErr:    errors.New("quota fail"),
			expectError: true,
		},
		{
			name:         "default logger on nil",
			volumePages:  makeVolumesPages(100),
			errOnPage:    -1,
			quotaValue:   50,
			expectError:  false,
			expectPct:    (100 * 1073741824 / float64(bytesPerTiB)) / 50 * 100, // ~0.2%
			useNilLogger: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ec2Fake := &fakeEC2Client{
				Region:               "us-east-1",
				DescribeVolumesPages: tc.volumePages,
				ErrOnDescribeVolumes: tc.errOnPage,
			}
			
			quotaFake := &fakeQuotaClient{
				Region: "us-east-1",
				Value:  tc.quotaValue,
				Err:    tc.quotaErr,
			}

			cfg := Gp3StorageJobConfig{
				Ec2Client:           ec2Fake,
				ServiceQuotasClient: quotaFake,
			}
			if !tc.useNilLogger {
				cfg.Logger = &logger.NoopLogger{}
			}
			
			job, err := NewGp3StorageJob(cfg)
			assert.NoError(t, err, "NewGp3StorageJob should not fail")

			metrics, err := job.Execute(context.Background())
			if tc.expectError {
				assert.Error(t, err, "expected error but got none")
				return
			}
			assert.NoError(t, err, "unexpected error")

			// one metric returned
			assert.Len(t, metrics, 1, "should return exactly one metric")
			m := metrics[0]

			assert.Equal(t, sharedtypes.JobGP3StorageUtilization, m.Name, "metric name mismatch")
			assert.Equal(t, sharedtypes.UnitPercent, m.Unit, "unit should be percent")
			assert.InDelta(t, tc.expectPct, m.Value, 0.01, "utilization percentage mismatch")
			assert.True(t, quotaFake.Called, "quota client should be called")
			assert.True(t, time.Since(m.Timestamp) < time.Second, "timestamp should be recent")

			// Check getters
			assert.Equal(t, "us-east-1", job.GetRegion(), "region mismatch")
			assert.Contains(t, job.GetJobName(), string(sharedtypes.JobGP3StorageUtilization), "job name should contain metric name")
		})
	}
}