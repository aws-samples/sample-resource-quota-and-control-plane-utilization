// internal/awsclients/efsclient/fake.go
package efsclient

import (
	"context"
	"errors"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
)

// FakeEFSClient implements EFSClient with AWS‐style pagination, context checks,
// and error injection exactly like the EC2 fake client.
type FakeEFSClient struct {
	Region string

	// pages for paginator calls:
	DescribeFileSystemsPages  []*efs.DescribeFileSystemsOutput
	DescribeMountTargetsPages []*efs.DescribeMountTargetsOutput

	// “throw on this call index” for each paginated method:
	ErrOnDescribeFileSystemsCall  int
	ErrOnDescribeMountTargetsCall int

	// internal counters:
	callFSCount int
	callMTCount int
}

// DescribeFileSystems paginates DescribeFileSystemsPages, honoring NextToken and errors.
func (f *FakeEFSClient) DescribeFileSystems(
	ctx context.Context,
	in *efs.DescribeFileSystemsInput,
	optFns ...func(*efs.Options),
) (*efs.DescribeFileSystemsOutput, error) {
	// respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	// inject user‐configured error
	if f.callFSCount == f.ErrOnDescribeFileSystemsCall {
		return nil, errors.New("efs describefilesystems error")
	}
	// determine page index from NextToken
	idx := 0
	if in.Marker != nil {
		i, err := strconv.Atoi(*in.Marker)
		if err != nil {
			return nil, err
		}
		idx = i
	}
	// pick the right page or an empty one
	var out *efs.DescribeFileSystemsOutput
	if idx < len(f.DescribeFileSystemsPages) {
		p := f.DescribeFileSystemsPages[idx]
		out = &efs.DescribeFileSystemsOutput{
			FileSystems: p.FileSystems,
		}
	} else {
		out = &efs.DescribeFileSystemsOutput{}
	}
	// set NextToken if more pages remain
	if idx+1 < len(f.DescribeFileSystemsPages) {
		out.Marker = aws.String(strconv.Itoa(idx + 1))
	}
	f.callFSCount++
	return out, nil
}

// DescribeMountTargets paginates DescribeMountTargetsPages, honoring NextToken and errors.
func (f *FakeEFSClient) DescribeMountTargets(
	ctx context.Context,
	in *efs.DescribeMountTargetsInput,
	optFns ...func(*efs.Options),
) (*efs.DescribeMountTargetsOutput, error) {
	// respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	// inject user‐configured error
	if f.callMTCount == f.ErrOnDescribeMountTargetsCall {
		return nil, errors.New("efs describemounttargets error")
	}
	// determine page index
	idx := 0
	if in.Marker != nil {
		i, err := strconv.Atoi(*in.Marker)
		if err != nil {
			return nil, err
		}
		idx = i
	}
	// pick the right page or an empty one
	var out *efs.DescribeMountTargetsOutput
	if idx < len(f.DescribeMountTargetsPages) {
		p := f.DescribeMountTargetsPages[idx]
		out = &efs.DescribeMountTargetsOutput{
			MountTargets: p.MountTargets,
		}
	} else {
		out = &efs.DescribeMountTargetsOutput{}
	}
	// set NextToken if more pages remain
	if idx+1 < len(f.DescribeMountTargetsPages) {
		out.NextMarker = aws.String(strconv.Itoa(idx + 1))
	}
	f.callMTCount++
	return out, nil
}

// Reset clears the internal call counters.
func (f *FakeEFSClient) Reset() {
	f.callFSCount = 0
	f.callMTCount = 0
}

// GetRegion returns the configured region.
func (f *FakeEFSClient) GetRegion() string {
	return f.Region
}
