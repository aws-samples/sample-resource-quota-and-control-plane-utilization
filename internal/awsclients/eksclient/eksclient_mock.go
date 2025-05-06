package eksclient

import (
	"context"
	"errors"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
)

// FakeEKSClient implements the necessary EKS API for testing, with AWS-style pagination.
type FakeEKSClient struct {
	Region                  string
	ListClustersPageOutputs []*eks.ListClustersOutput
	ErrOnListClustersCall   int
	callCount               int
}

// ListClusters simulates paginated ListClusters calls using NextToken.
func (f *FakeEKSClient) ListClusters(
	ctx context.Context,
	input *eks.ListClustersInput,
	optFns ...func(*eks.Options),
) (*eks.ListClustersOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	// Inject error on specified call index.
	if f.callCount == f.ErrOnListClustersCall {
		return nil, errors.New("error listing eks clusters")
	}

	// Determine page index from NextToken.
	idx := 0
	if input.NextToken != nil {
		i, err := strconv.Atoi(*input.NextToken)
		if err != nil {
			return nil, err
		}
		idx = i
	}

	// Select the appropriate page or return empty.
	var out *eks.ListClustersOutput
	if idx < len(f.ListClustersPageOutputs) {
		page := f.ListClustersPageOutputs[idx]
		out = &eks.ListClustersOutput{Clusters: page.Clusters}
	} else {
		out = &eks.ListClustersOutput{}
	}

	// Set NextToken if more pages remain.
	if idx+1 < len(f.ListClustersPageOutputs) {
		next := strconv.Itoa(idx + 1)
		out.NextToken = aws.String(next)
	}

	f.callCount++
	return out, nil
}

// Reset clears the internal call counter.
func (f *FakeEKSClient) Reset() {
	f.callCount = 0
}

// GetRegion returns the client's configured region.
func (f *FakeEKSClient) GetRegion() string {
	return f.Region
}
