// Package job provides a concurrent job execution framework with worker pools,
// timeout management, and region-specific metric dispatching for AWS resource monitoring.
package job

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"sync"

	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
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

// Job represents a unit of work that can be executed by the job manager.
// Jobs typically perform AWS resource monitoring and return CloudWatch metrics.
type Job interface {
	// Execute performs the job's work and returns metrics or an error.
	// The context may include timeouts and cancellation signals.
	Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error)
	// GetRegion returns the AWS region this job operates in.
	GetRegion() string
	// GetJobName returns a unique identifier for this job.
	GetJobName() string
}

// jobWithRetry wraps a Job with retry tracking information.
type jobWithRetry struct {
	job     Job
	attempt int
}

// JobManager orchestrates concurrent job execution with worker pools and timeout management.
// It dispatches metrics to region-specific channels and handles graceful shutdown.
type JobManager struct {
	parentCtx  context.Context                    // Parent context for cancellation
	jobTimeout time.Duration                      // Timeout for individual job execution
	jobCh      chan *jobWithRetry                 // Buffered channel for primary job queue
	retryCh    chan *jobWithRetry                 // Buffered channel for retry job queue
	batcherMap *safemap.TypedMap[metrics.Batcher] // Region-specific metric channels
	workers    int                                // Number of worker goroutines
	maxRetries int                                // Maximum retry attempts per job
	log        logger.Logger                      // Logger instance
	shutdownWg sync.WaitGroup                     // Synchronization for worker shutdown
}

// JobManagerConfig contains configuration parameters for creating a JobManager.
type JobManagerConfig struct {
	ParentCtx       context.Context                    // Context for cancellation and shutdown
	Workers         int                                // Number of worker goroutines to spawn
	JobTimeout      time.Duration                      // Maximum execution time per job
	BatcherMap      *safemap.TypedMap[metrics.Batcher] // Region-to-channel mapping for metrics
	MaxRetries      int                                // Maximum retry attempts per job
	RetryBufferSize int                                // Buffer size for retry queue (optional)
	Log             logger.Logger                      // Logger for job execution events
}

// NewJobManager creates and starts a new JobManager with the specified configuration.
// It immediately spawns worker goroutines and begins processing jobs.
func NewJobManager(config JobManagerConfig) *JobManager {
	if config.Log == nil {
		logger.Init(logger.INFO, os.Stdout)
		config.Log = logger.Get()
	}
	
	retryBufferSize := config.RetryBufferSize
	if retryBufferSize <= 0 {
		retryBufferSize = DefaultRetryBufferSize
	}
	
	jm := &JobManager{
		parentCtx:  config.ParentCtx,
		jobTimeout: config.JobTimeout,
		jobCh:      make(chan *jobWithRetry, DefaultBufferSize),
		retryCh:    make(chan *jobWithRetry, retryBufferSize),
		batcherMap: config.BatcherMap,
		workers:    config.Workers,
		maxRetries: config.MaxRetries,
		log:        config.Log,
	}

	jm.log.Info("starting %d workers", config.Workers)
	jm.shutdownWg.Add(config.Workers)
	for i := range config.Workers {
		go jm.worker(i)
	}
	return jm
}

// AddJob enqueues a job for execution by the worker pool.
// Returns an error if the job cannot be enqueued (queue full or context cancelled).
func (jm *JobManager) AddJob(job Job) error {
	jobWrapper := &jobWithRetry{job: job, attempt: 0}
	
	select {
	case jm.jobCh <- jobWrapper:
		jm.log.Debug("enqueued job %s (region=%s)", job.GetJobName(), job.GetRegion())
		return nil
	case <-jm.parentCtx.Done():
		return fmt.Errorf("context cancelled")
	default:
		return fmt.Errorf("primary job queue full")
	}
}

// Wait closes the job channels and blocks until all workers complete.
// This should be called after all jobs have been submitted.
func (jm *JobManager) Wait() {
	close(jm.jobCh)
	close(jm.retryCh)
	jm.log.Info("waiting for workers to finish processing all jobs")
	jm.shutdownWg.Wait()
	jm.log.Info("all workers exited")
}

// worker runs in a goroutine and processes jobs from both channels with priority.
// Primary jobs are processed first, then retry jobs when primary queue is empty.
func (jm *JobManager) worker(id int) {
	defer jm.shutdownWg.Done()
	jm.log.Info("worker-%d started", id)

	for {
		// Priority 1: Try primary job queue first
		select {
		case jobWrapper, ok := <-jm.jobCh:
			if ok {
				jm.executeJob(jobWrapper, id)
				continue
			}
			// Primary queue closed, drain retry queue
			jm.drainRetryQueue(id)
			return
		case <-jm.parentCtx.Done():
			jm.log.Info("worker-%d shutting down (context cancelled)", id)
			return
		default:
			// Primary queue empty, check retry queue
		}

		// Priority 2: Check retry queue when primary is empty
		select {
		case retryJob, ok := <-jm.retryCh:
			if ok {
				jm.executeJob(retryJob, id)
				continue
			}
			// Both queues closed
			return
		case <-jm.parentCtx.Done():
			jm.log.Info("worker-%d shutting down (context cancelled)", id)
			return
		default:
			// Both queues empty, brief yield
			continue
		}
	}
}

// drainRetryQueue processes remaining retry jobs after primary queue is closed.
func (jm *JobManager) drainRetryQueue(workerID int) {
	jm.log.Info("worker-%d draining retry queue", workerID)
	for {
		select {
		case retryJob, ok := <-jm.retryCh:
			if !ok {
				return // Retry queue closed and empty
			}
			jm.executeJob(retryJob, workerID)
		case <-jm.parentCtx.Done():
			return // Context cancelled
		}
	}
}

// executeJob executes a single job with proper timeout and cleanup.
func (jm *JobManager) executeJob(jobWrapper *jobWithRetry, workerID int) {
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
func (jm *JobManager) dispatchMetrics(metrics []sharedtypes.CloudWatchMetric, job Job, workerID int) {
	for _, m := range metrics {
		if jm.parentCtx.Err() != nil {
			jm.log.Info("worker-%d interrupted before dispatching all metrics", workerID)
			return
		}
		jm.dispatchSingleMetric(m, job.GetRegion(), workerID)
	}
}

// dispatchSingleMetric dispatches a single metric to the appropriate regional batcher.
func (jm *JobManager) dispatchSingleMetric(metric sharedtypes.CloudWatchMetric, region string, workerID int) {
	batcher, ok := jm.batcherMap.Load(region)
	if !ok {
		jm.log.Error("no metric batcher for region %s", region)
		return
	}

	batcher.Add(jm.parentCtx, metric)
	jm.logMetricDispatch(metric, workerID)
}

// logMetricDispatch logs the dispatching of a metric with appropriate detail level.
func (jm *JobManager) logMetricDispatch(metric sharedtypes.CloudWatchMetric, workerID int) {
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
func (jm *JobManager) handleJobError(jobWrapper *jobWithRetry, err error, workerID int) {
	if jobWrapper.attempt < jm.maxRetries {
		jobWrapper.attempt++
		
		// Non-blocking retry enqueue
		select {
		case jm.retryCh <- jobWrapper:
			jm.log.Warn("worker-%d job %s queued for retry (attempt %d/%d): %v", 
				workerID, jobWrapper.job.GetJobName(), jobWrapper.attempt, jm.maxRetries+1, err)
		default:
			jm.log.Error("retry queue full, dropping job %s after %d attempts", 
				jobWrapper.job.GetJobName(), jobWrapper.attempt)
		}
		return
	}
	
	// Final failure
	jm.LogError(fmt.Errorf("job %s failed after %d attempts: %v", 
		jobWrapper.job.GetJobName(), jm.maxRetries+1, err))
}

// GetQueueStats returns the current depth of primary and retry queues for monitoring.
func (jm *JobManager) GetQueueStats() (primary, retry int) {
	return len(jm.jobCh), len(jm.retryCh)
}

// LogError logs job execution errors with consistent formatting.
func (jm *JobManager) LogError(err error) {
	jm.log.Error("jobmanager: error: %v", err)
}
