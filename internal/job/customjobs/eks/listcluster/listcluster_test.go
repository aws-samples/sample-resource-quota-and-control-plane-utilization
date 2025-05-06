// listcluster_test.go
package listcluster

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	sqTypes "github.com/aws/aws-sdk-go-v2/service/servicequotas/types"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/eksclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
)

// fakeQuotaClient implements servicequotaclient.ServiceQuotasClient.
type fakeQuotaClient struct {
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

func (f *fakeQuotaClient) GetRegion() string {
	return ""
}

// helper to build a FakeEKSClient pages
func makePages(counts ...int) []*eks.ListClustersOutput {
	var out []*eks.ListClustersOutput
	for _, n := range counts {
		cls := make([]string, n)
		for i := range cls {
			cls[i] = strconv.Itoa(i)
		}
		out = append(out, &eks.ListClustersOutput{Clusters: cls})
	}
	return out
}

func TestListClusterJob_WithYourFakeClient(t *testing.T) {
	cases := []struct {
		name        string
		pageOutputs []*eks.ListClustersOutput
		errOnPage   int
		quotaValue  float64
		quotaErr    error
		wantErr     bool
		wantPct     float64
	}{
		{
			name:        "2-page success",
			pageOutputs: makePages(2, 3),
			quotaValue:  10,
			wantErr:     false,
			wantPct:     float64(5) / 10 * 100, // (2+3)/10*100=50
			errOnPage:   -1,
		},
		{
			name:        "paginator error on second page",
			pageOutputs: makePages(1, 1),
			errOnPage:   1,
			quotaValue:  100,
			wantErr:     true,
		},
		{
			name:        "quota error",
			pageOutputs: makePages(1),
			quotaErr:    errors.New("quota boom"),
			wantErr:     true,
			errOnPage:   -1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// build FakeEKSClient
			eksFake := &eksclient.FakeEKSClient{
				Region:                  "r1",
				ListClustersPageOutputs: tc.pageOutputs,
				ErrOnListClustersCall:   tc.errOnPage,
			}
			// build fake quota client
			quotaFake := &fakeQuotaClient{Value: tc.quotaValue, Err: tc.quotaErr}

			job, err := NewListClusterJob(ListClusterJobConfig{
				EksClient:           eksFake,
				ServiceQuotasClient: servicequotaclient.ServiceQuotasClient(quotaFake),
				Logger:              nil,
			})
			if err != nil {
				t.Fatalf("NewListClusterJob failed: %v", err)
			}

			// execute
			met, err := job.Execute(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// expected one metric
			if len(met) != 1 {
				t.Fatalf("got %d metrics, want 1", len(met))
			}
			got := met[0]
			if got.Name != cloudwatchMetricName {
				t.Errorf("Name = %q, want %q", got.Name, cloudwatchMetricName)
			}
			if got.Value != tc.wantPct {
				t.Errorf("Value = %.2f, want %.2f", got.Value, tc.wantPct)
			}
			if !quotaFake.Called {
				t.Error("quota client was not called")
			}

			// also exercise getters
			if job.GetRegion() != "r1" {
				t.Errorf("GetRegion = %q", job.GetRegion())
			}
			if p := job.GetJobName(); len(p) == 0 || p[:len(listClusterJobPrefix)] != listClusterJobPrefix {
				t.Errorf("GetJobName = %q", p)
			}
		})
	}
}

func TestPaginationHelper(t *testing.T) {
	fake := &eksclient.FakeEKSClient{
		Region:                  "r2",
		ListClustersPageOutputs: makePages(1, 0),
		ErrOnListClustersCall:   -1,
	}
	p := eks.NewListClustersPaginator(fake, &eks.ListClustersInput{})
	var all []string
	for p.HasMorePages() {
		out, err := p.NextPage(context.Background())
		if err != nil {
			t.Fatalf("NextPage error: %v", err)
		}
		all = append(all, out.Clusters...)
	}
	if len(all) != 1 || all[0] != "0" {
		t.Errorf("pagination collected %v", all)
	}
}
