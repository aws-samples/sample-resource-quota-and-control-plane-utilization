package ebsclient

import (
	"context"
	"errors"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// FakeEBSClient implements EBSClient for testing with AWS-style pagination
type FakeEBSClient struct {
	Region               string
	DescribeVolumesPages []*ec2.DescribeVolumesOutput
	ErrOnDescribeVolumes int
	callCount            int
}

// DescribeVolumes simulates paginated DescribeVolumes calls using NextToken
func (f *FakeEBSClient) DescribeVolumes(
	ctx context.Context,
	in *ec2.DescribeVolumesInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeVolumesOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Inject error on specified call index
	if f.callCount == f.ErrOnDescribeVolumes {
		return nil, errors.New("error describing volumes")
	}

	// Determine page index from NextToken
	idx := 0
	if in.NextToken != nil {
		i, err := strconv.Atoi(*in.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	// Select the appropriate page or return empty
	var out *ec2.DescribeVolumesOutput
	if idx < len(f.DescribeVolumesPages) {
		page := f.DescribeVolumesPages[idx]
		out = &ec2.DescribeVolumesOutput{
			Volumes: page.Volumes,
		}
	} else {
		out = &ec2.DescribeVolumesOutput{}
	}

	// Set NextToken if more pages remain
	if idx+1 < len(f.DescribeVolumesPages) {
		out.NextToken = aws.String(strconv.Itoa(idx + 1))
	}

	f.callCount++
	return out, nil
}

// Reset clears the internal call counter
func (f *FakeEBSClient) Reset() {
	f.callCount = 0
}

// GetRegion returns the client's configured region
func (f *FakeEBSClient) GetRegion() string {
	return f.Region
}
