package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
)

// fakeJob implements Job for testing.
type fakeJob struct {
	region  string
	name    string
	metrics []sharedtypes.CloudWatchMetric
	err     error
}

func (f *fakeJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	return f.metrics, f.err
}
func (f *fakeJob) GetRegion() string  { return f.region }
func (f *fakeJob) GetJobName() string { return f.name }

// fakeBatcher records Add calls.
type fakeBatcher struct {
	adds []sharedtypes.CloudWatchMetric
}

func (fb *fakeBatcher) Add(ctx context.Context, m sharedtypes.CloudWatchMetric) {
	fb.adds = append(fb.adds, m)
}
func (fb *fakeBatcher) FlushAll(ctx context.Context) {
	// no-op
}

func TestJobManagerScenarios(t *testing.T) {
	logger.Init(logger.INFO, nil)
	log := logger.Get()

	tests := []struct {
		name            string
		cancelBeforeAdd bool
		jobs            []*fakeJob
		expectAdds      int
	}{
		{
			name:       "normal case",
			jobs:       []*fakeJob{{region: "r1", name: "job1", metrics: []sharedtypes.CloudWatchMetric{{Name: "m1", Value: 1.23}}}},
			expectAdds: 1,
		},
		{
			name:       "job returns error",
			jobs:       []*fakeJob{{region: "r1", name: "jobErr", metrics: nil, err: errors.New("fail")}},
			expectAdds: 0,
		},
		{
			name:       "no matching batcher",
			jobs:       []*fakeJob{{region: "rX", name: "jobX", metrics: []sharedtypes.CloudWatchMetric{{Name: "mX", Value: 0}}}},
			expectAdds: 0,
		},
		{
			name:            "context cancelled",
			cancelBeforeAdd: true,
			jobs:            []*fakeJob{{region: "r1", name: "job1", metrics: []sharedtypes.CloudWatchMetric{{Name: "m1", Value: 1}}}},
			expectAdds:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setup context
			ctx, cancel := context.WithCancel(context.Background())
			if tt.cancelBeforeAdd {
				cancel()
			}

			// setup batcher map
			batchers := safemap.TypedMap[metrics.Batcher]{}
			fb := &fakeBatcher{}
			batchers.Store("r1", fb)

			// create manager
			jm := NewJobManager(JobManagerConfig{
				ParentCtx:  ctx,
				Workers:    1,
				JobTimeout: 50 * time.Millisecond,
				BatcherMap: &batchers,
				Log:        log,
			})

			// enqueue jobs
			for _, job := range tt.jobs {
				jm.AddJob(job)
			}
			// invoking LogError for coverage
			jm.LogError(errors.New("test error"))

			// wait for all
			jm.Wait()

			// assert adds
			assert.Equal(t, tt.expectAdds, len(fb.adds))

			cancel()
		})
	}
}
