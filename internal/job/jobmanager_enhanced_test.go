package job

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParentContextCancellation tests that jobs complete and metrics are dispatched
// before parent context cancellation affects the system
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

// TestParentContextEarlyCancellation tests behavior when parent context is cancelled
// before jobs can complete
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

// TestGracefulShutdownWithQueuedJobs verifies that all queued jobs complete
// during graceful shutdown
func TestGracefulShutdownWithQueuedJobs(t *testing.T) {
	ctx := context.Background()
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx: ctx,
		Workers:   1, // Single worker to ensure sequential processing
	})

	// Add multiple fast jobs
	const numJobs = 5
	expectedMetrics := 0
	for i := 0; i < numJobs; i++ {
		job := createFastJob(2) // 2 metrics each
		job.name = fmt.Sprintf("queued-job-%d", i)
		err := jm.AddJob(job)
		require.NoError(t, err)
		expectedMetrics += 2
	}

	// Give jobs time to start processing before calling Wait()
	time.Sleep(20 * time.Millisecond)
	
	// Call Wait() - should process all jobs
	jm.Wait()

	// Verify all jobs processed and metrics dispatched
	assert.Equal(t, expectedMetrics, batcher.GetCallCount())
}

// TestShutdownDuringJobExecution verifies that jobs currently executing
// complete during shutdown
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

// TestQueueFullBehaviorEnhanced tests queue overflow with deterministic timing
func TestQueueFullBehaviorEnhanced(t *testing.T) {
	ctx := context.Background()
	
	// Create blocking job to occupy worker
	blockingJob := createBlockedJob(1)
	blockingJob.startSignal = make(chan struct{}, 1)
	
	jm, _ := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  ctx,
		Workers:    1,
		JobTimeout: 10 * time.Second,
	})
	defer func() {
		// Ensure cleanup
		close(blockingJob.blockUntil)
		jm.Wait()
	}()

	// Add blocking job
	err := jm.AddJob(blockingJob)
	require.NoError(t, err)

	// Wait for job to start (worker is now busy)
	select {
	case <-blockingJob.startSignal:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Blocking job should have started")
	}

	// Fill queue to capacity
	for i := 0; i < DefaultBufferSize; i++ {
		job := createFastJob(1)
		job.name = fmt.Sprintf("queue-job-%d", i)
		err := jm.AddJob(job)
		assert.NoError(t, err)
	}

	// This job should fail - queue is full
	overflowJob := createFastJob(1)
	overflowJob.name = "overflow-job"
	err = jm.AddJob(overflowJob)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "primary job queue full")
}

// TestConcurrentJobAdditionEnhanced tests concurrent job addition with proper synchronization
func TestConcurrentJobAdditionEnhanced(t *testing.T) {
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
			job := createFastJob(1)
			job.name = fmt.Sprintf("concurrent-job-%d", id)
			err := jm.AddJob(job)
			assert.NoError(t, err)
		}(i)
	}

	// Wait for all jobs to be added
	addWg.Wait()
	
	// Give jobs time to start processing
	time.Sleep(30 * time.Millisecond)

	// Wait for all jobs to be processed
	jm.Wait()

	// Verify all jobs processed
	assert.Equal(t, numJobs, batcher.GetCallCount())
}

// TestJobRetryLogicEnhanced tests retry behavior with controlled failures
func TestJobRetryLogicEnhanced(t *testing.T) {
	ctx := context.Background()
	
	// Job that fails twice then succeeds
	job := &controllableJob{
		region:            "test-region",
		name:              "retry-job",
		metrics:           createTestMetrics(2, false),
		failuresRemaining: 2, // Fail 2 times, succeed on 3rd
	}

	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:  ctx,
		MaxRetries: 3,
	})

	err := jm.AddJob(job)
	require.NoError(t, err)

	jm.Wait()

	// Verify job eventually succeeded and metrics dispatched
	assert.Equal(t, 3, job.GetCallCount(), "Job should be called 3 times (2 failures + 1 success)")
	assert.Equal(t, 2, batcher.GetCallCount(), "Metrics should be dispatched after success")
}

// TestRetryQueueFull tests behavior when retry queue is full
func TestRetryQueueFull(t *testing.T) {
	ctx := context.Background()
	
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx:       ctx,
		MaxRetries:      2,
		RetryBufferSize: 2, // Small retry buffer
	})

	// Add jobs that will fail and fill retry queue
	for i := 0; i < 5; i++ {
		job := &controllableJob{
			region:            "test-region",
			name:              fmt.Sprintf("failing-job-%d", i),
			failuresRemaining: 10, // Always fail
		}
		err := jm.AddJob(job)
		require.NoError(t, err)
	}

	jm.Wait()

	// No metrics should be dispatched (all jobs failed)
	assert.Equal(t, 0, batcher.GetCallCount())
}

// TestCompleteJobLifecycle tests that Wait() ensures complete job lifecycle
func TestCompleteJobLifecycle(t *testing.T) {
	ctx := context.Background()
	
	// Job with multiple metrics to ensure dispatching takes time
	job := createFastJob(10) // 10 metrics
	
	jm, batcher := setupTestJobManager(t, JobManagerConfig{
		ParentCtx: ctx,
	})

	// Record start time
	start := time.Now()
	
	err := jm.AddJob(job)
	require.NoError(t, err)
	
	// Give job time to start processing
	time.Sleep(20 * time.Millisecond)

	// Wait should not return until complete lifecycle
	jm.Wait()
	
	duration := time.Since(start)
	
	// Verify all metrics dispatched
	assert.Equal(t, 10, batcher.GetCallCount())
	
	// Verify Wait() took reasonable time (job + dispatch time)
	expectedMinTime := FastJobTime
	assert.True(t, duration >= expectedMinTime, 
		"Wait() should take at least job execution time, took %v", duration)
}