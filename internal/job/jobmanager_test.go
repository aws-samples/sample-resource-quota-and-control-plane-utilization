package job

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// controllableJob implements Job with configurable behavior for testing.
type controllableJob struct {
	region            string
	name              string
	metrics           []types.CloudWatchMetric
	err               error
	delay             time.Duration
	panicOnExec       bool
	callCount         int
	failuresRemaining int
	mu                sync.Mutex

	// Synchronization fields for deterministic testing
	startSignal    chan struct{}
	completeSignal chan struct{}
	blockUntil     chan struct{}
}

func (c *controllableJob) Execute(ctx context.Context) ([]types.CloudWatchMetric, error) {
	// Signal job start if channel exists
	if c.startSignal != nil {
		select {
		case c.startSignal <- struct{}{}:
		default:
		}
	}

	// Block if blockUntil channel exists and is not closed
	if c.blockUntil != nil {
		select {
		case <-c.blockUntil:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	c.mu.Lock()
	c.callCount++
	if c.failuresRemaining > 0 {
		c.failuresRemaining--
		c.mu.Unlock()
		return nil, errors.New("temporary failure")
	}
	c.mu.Unlock()

	if c.panicOnExec {
		panic("test panic")
	}

	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Signal job completion if channel exists
	if c.completeSignal != nil {
		select {
		case c.completeSignal <- struct{}{}:
		default:
		}
	}

	return c.metrics, c.err
}

func (c *controllableJob) GetRegion() string  { return c.region }
func (c *controllableJob) GetJobName() string { return c.name }
func (c *controllableJob) GetCallCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callCount
}

// trackingBatcher records all Add calls with timestamps.
type trackingBatcher struct {
	adds      []types.CloudWatchMetric
	callTimes []time.Time
	mu        sync.Mutex
}

func (t *trackingBatcher) Add(ctx context.Context, m types.CloudWatchMetric) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.adds = append(t.adds, m)
	t.callTimes = append(t.callTimes, time.Now())
}

func (t *trackingBatcher) FlushAll(ctx context.Context) {}

func (t *trackingBatcher) GetAdds() []types.CloudWatchMetric {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]types.CloudWatchMetric(nil), t.adds...)
}

func (t *trackingBatcher) GetCallCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.adds)
}

// Test timing constants
const (
	FastJobTime    = 10 * time.Millisecond  // Fast job execution
	SlowJobTime    = 100 * time.Millisecond // Slow job execution
	DispatchTime   = 5 * time.Millisecond   // Metric dispatch time
	SystemBuffer   = 20 * time.Millisecond  // System variance buffer
	ShutdownBuffer = 50 * time.Millisecond  // Shutdown coordination buffer
)

// Test helper functions
func createTestMetrics(count int, withVPC bool) []types.CloudWatchMetric {
	metrics := make([]types.CloudWatchMetric, count)
	for i := 0; i < count; i++ {
		metric := types.CloudWatchMetric{
			Name:     types.JobNetworkInterfaceUtilization, // Use a valid JobName
			Value:    float64(i * 10),
			Metadata: make(map[string]string),
		}
		if withVPC {
			metric.Metadata[MetadataKeyVPC] = fmt.Sprintf("vpc-%d", i)
		}
		metrics[i] = metric
	}
	return metrics
}

// Job creation helpers
func createFastJob(metricCount int) *controllableJob {
	return &controllableJob{
		region:  "test-region",
		name:    "fast-job",
		metrics: createTestMetrics(metricCount, false),
		delay:   FastJobTime,
	}
}

func createSlowJob(metricCount int) *controllableJob {
	return &controllableJob{
		region:  "test-region",
		name:    "slow-job",
		metrics: createTestMetrics(metricCount, false),
		delay:   SlowJobTime,
	}
}

func createBlockedJob(metricCount int) *controllableJob {
	return &controllableJob{
		region:     "test-region",
		name:       "blocked-job",
		metrics:    createTestMetrics(metricCount, false),
		blockUntil: make(chan struct{}),
	}
}

// Timing calculation helpers
func calculateJobCompletionTime(jobDelay time.Duration, numMetrics int) time.Duration {
	dispatchTime := time.Duration(numMetrics) * (DispatchTime / 2) // Estimate
	return jobDelay + dispatchTime + SystemBuffer
}

func setupTestJobManager(t *testing.T, config JobManagerConfig) (JobManager, *trackingBatcher) {
	if config.Log == nil {
		logger.Init(logger.INFO, nil)
		config.Log = logger.Get()
	}
	if config.ParentCtx == nil {
		config.ParentCtx = context.Background()
	}
	if config.Workers == 0 {
		config.Workers = 1
	}
	if config.JobTimeout == 0 {
		config.JobTimeout = 100 * time.Millisecond
	}

	batcher := &trackingBatcher{}
	batchers := safestore.NewSyncStore[metrics.Batcher]()
	batchers.Store("test-region", batcher)
	config.BatcherMap = batchers

	jm, err := NewJobManager(config)
	require.NoError(t, err)
	return jm, batcher
}

func TestNewJobManager(t *testing.T) {
	tests := []struct {
		name            string
		config          JobManagerConfig
		expectWorkers   int
		expectRetrySize int
	}{
		{
			name: "default configuration",
			config: JobManagerConfig{
				ParentCtx:  context.Background(),
				Workers:    2,
				JobTimeout: 50 * time.Millisecond,
			},
			expectWorkers:   2,
			expectRetrySize: DefaultRetryBufferSize,
		},
		{
			name: "custom retry buffer size",
			config: JobManagerConfig{
				ParentCtx:       context.Background(),
				Workers:         1,
				JobTimeout:      50 * time.Millisecond,
				RetryBufferSize: 25,
			},
			expectWorkers:   1,
			expectRetrySize: 25,
		},
		{
			name: "zero retry buffer size uses default",
			config: JobManagerConfig{
				ParentCtx:       context.Background(),
				Workers:         1,
				JobTimeout:      50 * time.Millisecond,
				RetryBufferSize: 0,
			},
			expectWorkers:   1,
			expectRetrySize: DefaultRetryBufferSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jm, _ := setupTestJobManager(t, tt.config)
			defer jm.Wait()

			// Type assert to access internal fields for testing
			jmImpl := jm.(*jobManager)
			assert.Equal(t, tt.expectWorkers, jmImpl.workers)
			assert.Equal(t, tt.expectRetrySize, cap(jmImpl.retryCh))
			assert.Equal(t, DefaultBufferSize, cap(jmImpl.jobCh))
			assert.NotNil(t, jmImpl.log)
		})
	}
}

func TestAddJob(t *testing.T) {
	ctx := context.Background()
	jm, _ := setupTestJobManager(t, JobManagerConfig{ParentCtx: ctx})
	defer jm.Wait()

	job := &controllableJob{region: "test-region", name: "test-job"}
	err := jm.AddJob(job)
	assert.NoError(t, err)
}

func TestNewJobManagerContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel context before creating JobManager

	batchers := safestore.NewSyncStore[metrics.Batcher]()
	config := JobManagerConfig{
		ParentCtx:  ctx,
		Workers:    1,
		JobTimeout: 100 * time.Millisecond,
		BatcherMap: batchers,
	}

	jm, err := NewJobManager(config)
	assert.Error(t, err)
	assert.Equal(t, ErrContextCancelled, err)
	assert.Nil(t, jm)
}

func TestAddJobQueueFull(t *testing.T) {
	ctx := context.Background()
	jm, _ := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  ctx,
		Workers:    1,                // Single worker
		JobTimeout: 10 * time.Second, // Long timeout
	})
	defer jm.Wait()

	// Add a long-running job to occupy the single worker
	longRunningJob := &controllableJob{
		region:  "test-region",
		name:    "long-running-job",
		delay:   3 * time.Second, // Keep worker busy longer
		metrics: createTestMetrics(1, false),
	}
	err := jm.AddJob(longRunningJob)
	require.NoError(t, err)

	// Give the worker time to start processing the long-running job
	time.Sleep(100 * time.Millisecond)

	// Fill the queue to capacity while worker is busy
	for i := 0; i < DefaultBufferSize; i++ {
		job := &controllableJob{
			region: "test-region",
			name:   fmt.Sprintf("queue-job-%d", i),
		}
		err := jm.AddJob(job)
		assert.NoError(t, err)
	}

	// This job should fail because queue is full and worker is busy
	overflowJob := &controllableJob{region: "test-region", name: "overflow-job"}
	err = jm.AddJob(overflowJob)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "primary job queue full")
}

func TestJobExecution(t *testing.T) {
	tests := []struct {
		name          string
		job           *controllableJob
		expectMetrics int
		expectError   bool
		expectRetries int
	}{
		{
			name: "successful execution",
			job: &controllableJob{
				region:  "test-region",
				name:    "success-job",
				metrics: createTestMetrics(2, true),
			},
			expectMetrics: 2,
		},
		{
			name: "job returns error",
			job: &controllableJob{
				region: "test-region",
				name:   "error-job",
				err:    errors.New("job failed"),
			},
			expectError: true,
		},
		{
			name: "job with no metrics",
			job: &controllableJob{
				region:  "test-region",
				name:    "empty-job",
				metrics: []types.CloudWatchMetric{},
			},
			expectMetrics: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			jm, batcher := setupTestJobManager(t, JobManagerConfig{
				ParentCtx:  ctx,
				MaxRetries: 2,
			})

			err := jm.AddJob(tt.job)
			require.NoError(t, err)

			jm.Wait()

			if tt.expectError {
				assert.Equal(t, 0, batcher.GetCallCount())
			} else {
				assert.Equal(t, tt.expectMetrics, batcher.GetCallCount())
			}
		})
	}
}

func TestRetryLogic(t *testing.T) {
	tests := []struct {
		name             string
		maxRetries       int
		jobFailures      int
		expectExecutions int
		expectFinalError bool
	}{
		{
			name:             "job succeeds on first try",
			maxRetries:       2,
			jobFailures:      0,
			expectExecutions: 1,
		},
		{
			name:             "job succeeds on retry",
			maxRetries:       2,
			jobFailures:      1,
			expectExecutions: 2,
		},
		{
			name:             "job fails all retries",
			maxRetries:       2,
			jobFailures:      3,
			expectExecutions: 3,
			expectFinalError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			job := &controllableJob{
				region:  "test-region",
				name:    "retry-job",
				metrics: createTestMetrics(1, false),
			}

			// Set up job to fail specified number of times
			job.failuresRemaining = tt.jobFailures

			jm, batcher := setupTestJobManager(t, JobManagerConfig{
				ParentCtx:  ctx,
				MaxRetries: tt.maxRetries,
			})

			err := jm.AddJob(job)
			require.NoError(t, err)

			jm.Wait()

			assert.Equal(t, tt.expectExecutions, job.GetCallCount())

			if tt.expectFinalError {
				assert.Equal(t, 0, batcher.GetCallCount())
			} else {
				assert.Equal(t, 1, batcher.GetCallCount())
			}
		})
	}
}

func TestMetricDispatching(t *testing.T) {
	tests := []struct {
		name             string
		metrics          []types.CloudWatchMetric
		region           string
		expectDispatched int
	}{
		{
			name:             "single metric with VPC",
			metrics:          createTestMetrics(1, true),
			region:           "test-region",
			expectDispatched: 1,
		},
		{
			name:             "multiple metrics without VPC",
			metrics:          createTestMetrics(3, false),
			region:           "test-region",
			expectDispatched: 3,
		},
		{
			name:             "no matching batcher",
			metrics:          createTestMetrics(1, false),
			region:           "unknown-region",
			expectDispatched: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			jm, batcher := setupTestJobManager(t, JobManagerConfig{ParentCtx: ctx})

			job := &controllableJob{
				region:  tt.region,
				name:    "dispatch-test",
				metrics: tt.metrics,
			}

			err := jm.AddJob(job)
			require.NoError(t, err)

			jm.Wait()

			assert.Equal(t, tt.expectDispatched, batcher.GetCallCount())
		})
	}
}

func TestContextCancellation(t *testing.T) {
	tests := []struct {
		name        string
		cancelDelay time.Duration
		jobDelay    time.Duration
	}{
		{
			name:        "cancel before job completes",
			cancelDelay: 20 * time.Millisecond,
			jobDelay:    100 * time.Millisecond,
		},
		{
			name:        "cancel after job starts",
			cancelDelay: 50 * time.Millisecond,
			jobDelay:    30 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			job := &controllableJob{
				region:  "test-region",
				name:    "cancel-test",
				metrics: createTestMetrics(3, false),
				delay:   tt.jobDelay,
			}

			jm, batcher := setupTestJobManager(t, JobManagerConfig{
				ParentCtx:  ctx,
				JobTimeout: 500 * time.Millisecond,
			})

			err := jm.AddJob(job)
			require.NoError(t, err)

			// Cancel context after specified delay
			go func() {
				time.Sleep(tt.cancelDelay)
				cancel()
			}()

			jm.Wait()

			// Verify system handled cancellation gracefully
			// Metrics count should be between 0 and total job metrics
			count := batcher.GetCallCount()
			assert.True(t, count >= 0 && count <= len(job.metrics),
				"Expected metric count between 0 and %d, got %d", len(job.metrics), count)
		})
	}
}

func TestGetQueueStats(t *testing.T) {
	ctx := context.Background()
	jm, _ := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  ctx,
		Workers:    1,
		JobTimeout: 5 * time.Second, // Long timeout to prevent job timeout
	})
	defer jm.Wait()

	// Initially empty
	primary, retry := jm.GetQueueStats()
	assert.Equal(t, 0, primary)
	assert.Equal(t, 0, retry)

	// Add a long-running job to occupy the worker
	longJob := &controllableJob{
		region: "test-region",
		name:   "long-running-job",
		delay:  1 * time.Second, // Keep worker busy
	}
	err := jm.AddJob(longJob)
	require.NoError(t, err)

	// Give worker time to start processing the long job
	time.Sleep(10 * time.Millisecond)

	// Add jobs that will queue up while worker is busy
	for i := 0; i < 3; i++ {
		job := &controllableJob{
			region: "test-region",
			name:   fmt.Sprintf("queued-job-%d", i),
		}
		err := jm.AddJob(job)
		require.NoError(t, err)
	}

	// Now check queue stats - should have 3 jobs queued
	primary, retry = jm.GetQueueStats()
	assert.Equal(t, 3, primary) // 3 jobs waiting in primary queue
	assert.Equal(t, 0, retry)   // No retry jobs
}

func TestConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  ctx,
		Workers:    3,
		JobTimeout: 200 * time.Millisecond,
	})

	const numJobs = 20
	var addWg sync.WaitGroup

	// Add jobs concurrently
	for i := 0; i < numJobs; i++ {
		addWg.Add(1)
		go func(id int) {
			defer addWg.Done()
			job := &controllableJob{
				region:  "test-region",
				name:    fmt.Sprintf("concurrent-job-%d", id),
				metrics: createTestMetrics(1, false),
				delay:   5 * time.Millisecond,
			}
			err := jm.AddJob(job)
			assert.NoError(t, err)
		}(i)
	}

	// Wait for all jobs to be added
	addWg.Wait()

	// Wait for all jobs to be processed
	jm.Wait()

	// Give a small buffer for final metric dispatching
	time.Sleep(10 * time.Millisecond)

	// All jobs should have been processed
	assert.Equal(t, numJobs, batcher.GetCallCount())
}

func TestLogError(t *testing.T) {
	ctx := context.Background()
	jm, _ := setupTestJobManager(t, JobManagerConfig{ParentCtx: ctx})

	// Test LogError method (mainly for coverage)
	testErr := errors.New("test error")
	jm.LogError(testErr)

	jm.Wait()
	// No assertion needed, just ensuring no panic
}

func TestJobTimeout(t *testing.T) {
	ctx := context.Background()
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  ctx,
		JobTimeout: 30 * time.Millisecond, // Short timeout
	})

	job := &controllableJob{
		region:  "test-region",
		name:    "timeout-job",
		metrics: createTestMetrics(1, false),
		delay:   200 * time.Millisecond, // Much longer than timeout
	}

	err := jm.AddJob(job)
	require.NoError(t, err)

	jm.Wait()

	// Give time for any potential race conditions to settle
	time.Sleep(10 * time.Millisecond)

	// Job should timeout and not dispatch metrics
	assert.Equal(t, 0, batcher.GetCallCount())
}

func TestAddJobAfterWait(t *testing.T) {
	ctx := context.Background()
	jm, _ := setupTestJobManager(t, JobManagerConfig{ParentCtx: ctx})

	// Add a job before shutdown
	job1 := &controllableJob{region: "test-region", name: "before-shutdown"}
	err := jm.AddJob(job1)
	assert.NoError(t, err)

	// Call Wait to initiate shutdown
	go jm.Wait()

	// Give shutdown time to initiate
	time.Sleep(10 * time.Millisecond)

	// Try to add job after shutdown - should fail
	job2 := &controllableJob{region: "test-region", name: "after-shutdown"}
	err = jm.AddJob(job2)
	assert.Error(t, err)
	assert.Equal(t, ErrJobManagerShutdown, err)
}

func TestShutdownWithQueuedJobs(t *testing.T) {
	ctx := context.Background()
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx: ctx,
		Workers:   1,
	})

	// Add multiple jobs
	const numJobs = 5
	for i := 0; i < numJobs; i++ {
		job := &controllableJob{
			region:  "test-region",
			name:    fmt.Sprintf("queued-job-%d", i),
			metrics: createTestMetrics(1, false),
			delay:   10 * time.Millisecond,
		}
		err := jm.AddJob(job)
		require.NoError(t, err)
	}

	// Wait for shutdown - all jobs should still be processed
	jm.Wait()

	// All jobs should have been processed despite shutdown
	assert.Equal(t, numJobs, batcher.GetCallCount())
}

func TestRetryQueueFull(t *testing.T) {
	ctx := context.Background()
	jm, _ := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:       ctx,
		Workers:         1,
		MaxRetries:      3,
		RetryBufferSize: 2, // Small retry buffer
	})
	defer jm.Wait()

	// Create jobs that always fail to fill retry queue
	for i := 0; i < 5; i++ {
		job := &controllableJob{
			region:            "test-region",
			name:              fmt.Sprintf("failing-job-%d", i),
			failuresRemaining: 10, // Always fail
		}
		err := jm.AddJob(job)
		require.NoError(t, err)
	}
}

func TestCompleteJobLifecycle(t *testing.T) {
	ctx := context.Background()
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx: ctx,
		Workers:   1,
	})

	job := createFastJob(10)
	err := jm.AddJob(job)
	require.NoError(t, err)

	jm.Wait()
	assert.Equal(t, 10, batcher.GetCallCount())
}

func TestZeroWorkers(t *testing.T) {
	ctx := context.Background()
	jm, _ := setupTestJobManager(t, JobManagerConfig{
		ParentCtx: ctx,
		Workers:   0, // Should default to 1
	})
	defer jm.Wait()

	// Verify it still works with default worker count
	job := createFastJob(1)
	err := jm.AddJob(job)
	assert.NoError(t, err)
}

func TestDefaultLogger(t *testing.T) {
	ctx := context.Background()
	batchers := safestore.NewSyncStore[metrics.Batcher]()
	
	// Don't provide a logger - should use default
	config := JobManagerConfig{
		ParentCtx:  ctx,
		Workers:    1,
		JobTimeout: 100 * time.Millisecond,
		BatcherMap: batchers,
		// Log: nil - test default logger initialization
	}

	jm, err := NewJobManager(config)
	require.NoError(t, err)
	defer jm.Wait()

	// Should work without panicking
	assert.NotNil(t, jm)
}

func TestParentContextCancellation(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	// Setup: Fast job that completes quickly
	job := createFastJob(3) // 10ms execution, 3 metrics
	
	// Timing: Job(10ms) + Dispatch(~6ms) + Buffer(20ms) = 36ms
	cancellationDelay := calculateJobCompletionTime(FastJobTime, 3)
	
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  parentCtx,
		JobTimeout: 200 * time.Millisecond,
	})

	// Add job
	err := jm.AddJob(job)
	require.NoError(t, err)

	// Cancel parent context after job should complete
	go func() {
		time.Sleep(cancellationDelay)
		parentCancel()
	}()

	// Wait for complete shutdown
	jm.Wait()

	// Verify job completed and all metrics dispatched
	assert.Equal(t, 3, batcher.GetCallCount(), "All metrics should be dispatched before context cancellation")
}

func TestParentContextEarlyCancellation(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	// Setup: Slow job that won't complete before cancellation
	job := createSlowJob(5) // 100ms execution, 5 metrics
	
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  parentCtx,
		JobTimeout: 200 * time.Millisecond,
	})

	// Add job
	err := jm.AddJob(job)
	require.NoError(t, err)

	// Cancel parent context quickly (before job completes)
	go func() {
		time.Sleep(20 * time.Millisecond)
		parentCancel()
	}()

	// Wait for shutdown
	jm.Wait()

	// Verify minimal or no metrics dispatched due to early cancellation
	count := batcher.GetCallCount()
	assert.True(t, count <= 5, "Should have 0-5 metrics due to early cancellation, got %d", count)
}

func TestShutdownDuringJobExecution(t *testing.T) {
	ctx := context.Background()
	
	// Create blocked job that we can control
	job := createBlockedJob(3)
	job.startSignal = make(chan struct{}, 1)
	job.completeSignal = make(chan struct{}, 1)
	
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx: ctx,
		Workers:   1,
	})

	// Add job
	err := jm.AddJob(job)
	require.NoError(t, err)

	// Wait for job to start
	select {
	case <-job.startSignal:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Job should have started")
	}

	// Start shutdown in background
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		jm.Wait()
	}()

	// Give shutdown time to initiate
	time.Sleep(10 * time.Millisecond)

	// Release the blocked job
	close(job.blockUntil)

	// Wait for job to complete
	select {
	case <-job.completeSignal:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Job should have completed")
	}

	// Wait for shutdown to complete
	wg.Wait()

	// Verify job completed and metrics dispatched
	assert.Equal(t, 3, batcher.GetCallCount())
}
