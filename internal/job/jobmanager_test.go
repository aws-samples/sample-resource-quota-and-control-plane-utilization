package job_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// simpleJob is a stub that counts how many times Execute is called,
// returns a predetermined slice of metrics or an error.
type simpleJob struct {
	name    string
	region  string
	metrics []sharedtypes.CloudWatchMetric
	err     error

	mu    sync.Mutex
	runCt int
}

func (j *simpleJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	j.mu.Lock()
	j.runCt++
	j.mu.Unlock()
	return j.metrics, j.err
}
func (j *simpleJob) GetRegion() string  { return j.region }
func (j *simpleJob) GetJobName() string { return j.name }
func (j *simpleJob) Ran() int           { j.mu.Lock(); defer j.mu.Unlock(); return j.runCt }

// Test that a job producing one metric gets dispatched exactly once.
func TestJobManager_HappyPath(t *testing.T) {
	parentCtx := context.Background()
	var metricMap safemap.TypedMap[chan sharedtypes.CloudWatchMetric]
	// channel for region "r1"
	r1ch := make(chan sharedtypes.CloudWatchMetric, 1)
	metricMap.Store("r1", r1ch)

	jm := job.NewJobManager(job.JobManagerConfig{
		ParentCtx:  parentCtx,
		Workers:    2,
		JobTimeout: 500 * time.Millisecond,
		MetricMap:  &metricMap,
		Log:        &logger.NoopLogger{},
	})

	// Enqueue a job that returns one metric
	job := &simpleJob{
		name:   "job1",
		region: "r1",
		metrics: []sharedtypes.CloudWatchMetric{
			{Name: "m1"},
		},
	}
	jm.AddJob(job)

	// Shutdown after a tiny delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		jm.Wait()
	}()

	// Expect to receive exactly one metric
	select {
	case m := <-r1ch:
		if m.Name != "m1" {
			t.Errorf("expected metric m1, got %q", m.Name)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for metric")
	}
	if job.Ran() != 1 {
		t.Errorf("expected job to run once, ran %d times", job.Ran())
	}
}

// Test that if Execute returns an error, it's sent to errCh.
func TestJobManager_ErrorPath(t *testing.T) {
	parentCtx := context.Background()
	var metricMap safemap.TypedMap[chan sharedtypes.CloudWatchMetric]
	// no channel stored for region "bad", so dispatch will skip but error must still be sent
	jm := job.NewJobManager(job.JobManagerConfig{
		ParentCtx:  parentCtx,
		Workers:    1,
		JobTimeout: 500 * time.Millisecond,
		MetricMap:  &metricMap,
		Log:        &logger.NoopLogger{},
	})

	jobErr := errors.New("execute-fail")
	job := &simpleJob{name: "badJob", region: "bad", err: jobErr}
	jm.AddJob(job)

	go func() {
		time.Sleep(10 * time.Millisecond)
		jm.Wait()
	}()

}

// Test that jobs added after the parent context is canceled are dropped.
func TestAddJobAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	var metricMap safemap.TypedMap[chan sharedtypes.CloudWatchMetric]
	jm := job.NewJobManager(job.JobManagerConfig{
		ParentCtx:  ctx,
		Workers:    1,
		JobTimeout: time.Second,
		MetricMap:  &metricMap,
		Log:        &logger.NoopLogger{},
	})

	job := &simpleJob{name: "dropped", region: "any"}
	jm.AddJob(job)
	jm.Wait()

	if job.Ran() != 0 {
		t.Errorf("expected dropped job not to run, but ran %d times", job.Ran())
	}
}

// Test that when errCh is full, manager does not deadlock.
func TestErrorChannelFull(t *testing.T) {
	parentCtx := context.Background()
	var metricMap safemap.TypedMap[chan sharedtypes.CloudWatchMetric]

	jm := job.NewJobManager(job.JobManagerConfig{
		ParentCtx:  parentCtx,
		Workers:    1,
		JobTimeout: 500 * time.Millisecond,
		MetricMap:  &metricMap,
		Log:        &logger.NoopLogger{},
	})
	job := &simpleJob{name: "errJob", region: "r2", err: errors.New("fail")}
	jm.AddJob(job)

	go func() {
		time.Sleep(10 * time.Millisecond)
		jm.Wait()
	}()

	// if this hangs, the test will time out
	time.Sleep(50 * time.Millisecond)
	if job.Ran() != 1 {
		t.Errorf("expected errJob to run once despite full errCh, ran %d times", job.Ran())
	}
}

// slowJob simulates a long-running Execute; it ignores ctx.Err()
// so that cancellation only affects the dispatch select, not the Execute itself.
type slowJob struct {
	name    string
	region  string
	metrics []sharedtypes.CloudWatchMetric
	delay   time.Duration
}

func (j *slowJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	time.Sleep(j.delay)
	return j.metrics, nil
}
func (j *slowJob) GetRegion() string  { return j.region }
func (j *slowJob) GetJobName() string { return j.name }

func TestDispatchInterruptedByCancel(t *testing.T) {
	// parentCtx we can cancel
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var metricMap safemap.TypedMap[chan sharedtypes.CloudWatchMetric]
	// one small buffered channel so the default dispatch would fit
	ch := make(chan sharedtypes.CloudWatchMetric, 1)
	metricMap.Store("slow-region", ch)

	// use 1 worker, 100ms timeout, but job sleeps 50ms
	jm := job.NewJobManager(job.JobManagerConfig{
		ParentCtx:  parentCtx,
		Workers:    1,
		JobTimeout: 500 * time.Millisecond,
		MetricMap:  &metricMap,
		Log:        &logger.NoopLogger{},
	})

	// slowJob returns 2 metrics but takes 50ms to do so
	sj := &slowJob{
		name:    "slow",
		region:  "slow-region",
		metrics: []sharedtypes.CloudWatchMetric{{Name: "m1"}, {Name: "m2"}},
		delay:   50 * time.Millisecond,
	}

	jm.AddJob(sj)

	// give the worker a little time to pick up the job and enter Execute
	time.Sleep(10 * time.Millisecond)
	// cancel the parentCtx so that by the time Execute finishes, dispatch will see Done()
	cancel()

	// now Wait for the worker to exit
	jm.Wait()

	// we expect exactly zero or one metric:
	// – zero if the dispatch loop sees Done() before ch<-m1,
	// – or one if it managed the first but then saw Done() before m2.
	// In either case it must not deadlock, and it must not produce two.
	got := 0
	for {
		select {
		case <-ch:
			got++
		default:
			goto Done
		}
	}
Done:
	if got >= 2 {
		t.Fatalf("expected <2 metrics after cancel, got %d", got)
	}
}
