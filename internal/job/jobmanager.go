// Package job provides a concurrent job execution framework with worker pools,
// timeout management, and region-specific metric dispatching for AWS resource monitoring.
package job

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (
	// DefaultBufferSize defines the maximum number of jobs that can be queued
	// before AddJob calls start blocking.
	DefaultBufferSize = 100
	// DefaultRetryBufferSize defines the maximum number of retry jobs that can be queued.
	DefaultRetryBufferSize = 50
	// MetadataKeyVPC is the key for VPC ID in metric metadata.
	MetadataKeyVPC = "vpc"
)

// Error variables
var (
	ErrContextCancelled   = errors.New("context is already cancelled")
	ErrMaxRetriesExceeded = errors.New("job exceeded maximum retry attempts")
	ErrJobManagerShutdown = errors.New("job manager is shutting down")
)

// Job represents a unit of work that can be executed by the job manager.
// Jobs typically perform AWS resource monitoring and return CloudWatch metrics.
type Job interface {
	// Execute performs the job's work and returns metrics or an error.
	// The context may include timeouts and cancellation signals.
	Execute(ctx context.Context) ([]types.CloudWatchMetric, error)
	// GetRegion returns the AWS region this job operates in.
	GetRegion() string
	// GetJobName returns a unique identifier for this job.
	GetJobName() string
}

// JobManager defines the interface for job coordination and execution.
type JobManager interface {
	// AddJob enqueues a job for execution by the worker pool.
	AddJob(job Job) error
	// Wait closes the job queue and waits for all jobs to complete.
	Wait()
	// GetQueueStats returns the current depth of primary and retry queues.
	GetQueueStats() (primary, retry int)
	// LogError logs job execution errors.
	LogError(err error)
}

// jobWithRetry wraps a Job with retry tracking information.
type jobWithRetry struct {
	job     Job
	attempt int
}

// jobManager implements JobManager interface with concurrent job execution, worker pools and timeout management.
// It dispatches metrics to region-specific channels and handles graceful shutdown.
type jobManager struct {
	parentCtx      context.Context                  // Parent context for cancellation
	shutdownCtx    context.Context                  // Internal shutdown context
	shutdownCancel context.CancelFunc               // Cancel function for shutdown
	jobTimeout     time.Duration                    // Timeout for individual job execution
	jobCh          chan *jobWithRetry               // Buffered channel for primary job queue
	retryCh        chan *jobWithRetry               // Buffered channel for retry job queue
	batcherMap     safestore.Store[metrics.Batcher] // Region-specific metric channels
	workers        int                              // Number of worker goroutines
	maxRetries     int                              // Maximum retry attempts per job
	log            logger.Logger                    // Logger instance
	shutdownWg     sync.WaitGroup                   // Synchronization for worker shutdown
	shutdownOnce   sync.Once                        // Ensure shutdown happens once
}

// JobManagerConfig contains configuration parameters for creating a JobManager.
type JobManagerConfig struct {
	ParentCtx       context.Context                  // Context for cancellation and shutdown
	Workers         int                              // Number of worker goroutines to spawn
	JobTimeout      time.Duration                    // Maximum execution time per job
	BatcherMap      safestore.Store[metrics.Batcher] // Region-to-channel mapping for metrics
	MaxRetries      int                              // Maximum retry attempts per job
	RetryBufferSize int                              // Buffer size for retry queue (optional)
	Log             logger.Logger                    // Logger for job execution events
}

// NewJobManager creates and starts a new JobManager with the specified configuration.
// Returns an error if the context is already cancelled.
func NewJobManager(config JobManagerConfig) (JobManager, error) {
	// Check if context is already cancelled
	if config.ParentCtx != nil && config.ParentCtx.Err() != nil {
		return nil, ErrContextCancelled
	}

	if config.Log == nil {
		logger.Init(logger.INFO, os.Stdout)
		config.Log = logger.Get()
	}

	retryBufferSize := config.RetryBufferSize
	if retryBufferSize <= 0 {
		retryBufferSize = DefaultRetryBufferSize
	}

	// Default to 1 worker if 0 or negative
	workers := config.Workers
	if workers <= 0 {
		workers = 1
	}

	// Create internal shutdown context
	shutdownCtx, shutdownCancel := context.WithCancel(config.ParentCtx)

	jm := &jobManager{
		parentCtx:      config.ParentCtx,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		jobTimeout:     config.JobTimeout,
		jobCh:          make(chan *jobWithRetry, DefaultBufferSize),
		retryCh:        make(chan *jobWithRetry, retryBufferSize),
		batcherMap:     config.BatcherMap,
		workers:        workers,
		maxRetries:     config.MaxRetries,
		log:            config.Log,
	}

	// Double-check context before starting workers
	if config.ParentCtx.Err() != nil {
		return nil, ErrContextCancelled
	}

	jm.log.Info("starting %d workers", workers)
	jm.shutdownWg.Add(workers)
	for i := range workers {
		go jm.worker(i)
	}
	return jm, nil
}

// AddJob enqueues a job for execution by the worker pool.
// Returns an error if the job cannot be enqueued (queue full or shutdown in progress).
func (jm *jobManager) AddJob(job Job) error {
	jobWrapper := &jobWithRetry{job: job, attempt: 0}

	select {
	case jm.jobCh <- jobWrapper:
		jm.log.Debug("enqueued job %s (region=%s)", job.GetJobName(), job.GetRegion())
		return nil
	case <-jm.shutdownCtx.Done():
		return ErrJobManagerShutdown
	default:
		return fmt.Errorf("primary job queue full")
	}
}

// Wait initiates shutdown and waits for all workers to complete.
// After calling Wait, AddJob will return ErrJobManagerShutdown.
func (jm *jobManager) Wait() {
	jm.shutdownOnce.Do(func() {
		jm.log.Info("initiating shutdown")
		jm.shutdownCancel()
	})

	jm.log.Info("waiting for workers to finish")
	jm.shutdownWg.Wait()
	jm.log.Info("all workers exited")
}

// worker runs in a goroutine and processes jobs from both channels.
// Workers continue processing until shutdown is signaled AND both queues are empty.
func (jm *jobManager) worker(id int) {
	defer jm.shutdownWg.Done()
	jm.log.Info("worker-%d started", id)

	for {
		select {
		case jobWrapper, ok := <-jm.jobCh:
			if ok {
				jm.executeJob(jobWrapper, id)
				continue
			}
			// Primary channel closed, continue with retry queue only
		case retryJob, ok := <-jm.retryCh:
			if ok {
				jm.executeJob(retryJob, id)
				continue
			}
			// Retry channel closed, exit
			return
		case <-jm.shutdownCtx.Done():
			// Shutdown signaled, check if queues are empty
			if len(jm.jobCh) == 0 && len(jm.retryCh) == 0 {
				jm.log.Info("worker-%d shutting down (queues empty)", id)
				return
			}
			// Continue processing remaining jobs
		}
	}
}

// executeJob executes a single job with proper timeout and cleanup.
func (jm *jobManager) executeJob(jobWrapper *jobWithRetry, workerID int) {
	job := jobWrapper.job
	jm.log.Info("worker-%d executing job %s (attempt %d)", workerID, job.GetJobName(), jobWrapper.attempt+1)

	ctx, cancel := context.WithTimeout(jm.parentCtx, jm.jobTimeout)
	defer cancel()

	metrics, err := job.Execute(ctx)
	if err != nil {
		jm.handleJobError(jobWrapper, err, workerID)
		return
	}

	jm.log.Info("worker-%d job %s returned %d metrics", workerID, job.GetJobName(), len(metrics))
	jm.dispatchMetrics(metrics, job, workerID)
}

// dispatchMetrics handles dispatching all metrics from a job to their respective batchers.
func (jm *jobManager) dispatchMetrics(metrics []types.CloudWatchMetric, job Job, workerID int) {
	for _, m := range metrics {
		if jm.parentCtx.Err() != nil {
			jm.log.Info("worker-%d interrupted before dispatching all metrics", workerID)
			return
		}
		jm.dispatchSingleMetric(m, job.GetRegion(), workerID)
	}
}

// dispatchSingleMetric dispatches a single metric to the appropriate regional batcher.
func (jm *jobManager) dispatchSingleMetric(metric types.CloudWatchMetric, region string, workerID int) {
	batcher, ok := jm.batcherMap.Load(region)
	if !ok {
		jm.log.Error("no metric batcher for region %s", region)
		return
	}

	batcher.Add(jm.parentCtx, metric)
	jm.logMetricDispatch(metric, workerID)
}

// logMetricDispatch logs the dispatching of a metric with appropriate detail level.
func (jm *jobManager) logMetricDispatch(metric types.CloudWatchMetric, workerID int) {
	vpcID := metric.Metadata[MetadataKeyVPC]
	util := strconv.FormatFloat(metric.Value, 'f', -1, 64)

	if vpcID != "" {
		jm.log.Debug("worker-%d dispatched metric=%s value=%s%% for %s", workerID, metric.Name, util, vpcID)
	} else {
		jm.log.Debug("worker-%d dispatched metric=%s value=%s%%", workerID, metric.Name, util)
	}
}

// handleJobError handles job execution errors with retry logic.
// Failed jobs are queued in the retry channel to avoid blocking new jobs.
func (jm *jobManager) handleJobError(jobWrapper *jobWithRetry, err error, workerID int) {
	if jobWrapper.attempt < jm.maxRetries {
		jobWrapper.attempt++

		// Try to send to retry channel (non-blocking)
		select {
		case jm.retryCh <- jobWrapper:
			jm.log.Warn("worker-%d job %s queued for retry (attempt %d/%d): %v",
				workerID, jobWrapper.job.GetJobName(), jobWrapper.attempt, jm.maxRetries+1, err)
			return
		default:
			// Channel full - treat as final failure
			jm.log.Error("retry queue full, dropping job %s after %d attempts",
				jobWrapper.job.GetJobName(), jobWrapper.attempt)
		}
	}

	// Final failure - log with specific error
	finalErr := fmt.Errorf("%w: %s failed after %d attempts: %v",
		ErrMaxRetriesExceeded, jobWrapper.job.GetJobName(), jm.maxRetries+1, err)
	jm.LogError(finalErr)
}

// GetQueueStats returns the current depth of primary and retry queues for monitoring.
func (jm *jobManager) GetQueueStats() (primary, retry int) {
	return len(jm.jobCh), len(jm.retryCh)
}

// LogError logs job execution errors with consistent formatting.
func (jm *jobManager) LogError(err error) {
	jm.log.Error("jobmanager: error: %v", err)
}
