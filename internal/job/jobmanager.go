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

// JobManager orchestrates concurrent job execution with worker pools and timeout management.
// It dispatches metrics to region-specific channels and handles graceful shutdown.
type JobManager struct {
	parentCtx  context.Context                    // Parent context for cancellation
	jobTimeout time.Duration                      // Timeout for individual job execution
	jobCh      chan Job                           // Buffered channel for job queue
	batcherMap *safemap.TypedMap[metrics.Batcher] // Region-specific metric channels
	workers    int                                // Number of worker goroutines
	log        logger.Logger                      // Logger instance
	shutdownWg sync.WaitGroup                     // Synchronization for worker shutdown
}

// JobManagerConfig contains configuration parameters for creating a JobManager.
type JobManagerConfig struct {
	ParentCtx  context.Context                    // Context for cancellation and shutdown
	Workers    int                                // Number of worker goroutines to spawn
	JobTimeout time.Duration                      // Maximum execution time per job
	BatcherMap *safemap.TypedMap[metrics.Batcher] // Region-to-channel mapping for metrics
	Log        logger.Logger                      // Logger for job execution events
}

// NewJobManager creates and starts a new JobManager with the specified configuration.
// It immediately spawns worker goroutines and begins processing jobs.
func NewJobManager(config JobManagerConfig) *JobManager {
	if config.Log == nil {
		logger.Init(logger.INFO, os.Stdout)
		config.Log = logger.Get()
	}
	jm := &JobManager{
		parentCtx:  config.ParentCtx,
		jobTimeout: config.JobTimeout,
		jobCh:      make(chan Job, DefaultBufferSize),
		batcherMap: config.BatcherMap,
		workers:    config.Workers,
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
// Blocks if the job buffer is full, unless the parent context is cancelled.
func (jm *JobManager) AddJob(job Job) {
	// check context cancellatoin first
	if jm.parentCtx.Err() != nil {
		jm.log.Debug("parent context cancelled—dropping job %s (region=%s)", job.GetJobName(), job.GetRegion())
		return
	}
	jm.jobCh <- job
	jm.log.Debug("enqueued job %s (region=%s)", job.GetJobName(), job.GetRegion())
}

// Wait closes the job channel and blocks until all workers complete.
// This should be called after all jobs have been submitted.
func (jm *JobManager) Wait() {
	close(jm.jobCh)
	jm.log.Info("waiting for workers to finish")
	jm.shutdownWg.Wait()
	jm.log.Info("all workers exited")
}

// worker runs in a goroutine and processes jobs from the job channel.
// Each worker applies timeouts and dispatches metrics to appropriate channels.
func (jm *JobManager) worker(id int) {
	defer jm.shutdownWg.Done()
	jm.log.Info("worker-%d started", id)

	// explicitly check if context is cancelled
	if jm.parentCtx.Err() != nil {
		jm.log.Info("worker-%d shutting down (parent context done)", id)
		return
	}

	for {
		select {
		case <-jm.parentCtx.Done():
			jm.log.Info("worker-%d shutting down (parent context done)", id)
			return

		case job, ok := <-jm.jobCh:
			if !ok {
				jm.log.Info("worker-%d shutting down (job channel closed)", id)
				return
			}

			jm.log.Info("worker-%d executing job %s", id, job.GetJobName())

			// derive per-job context with timeout
			ctx, cancel := context.WithTimeout(jm.parentCtx, jm.jobTimeout)
			metrics, err := job.Execute(ctx)
			cancel() // always release the timer

			if err != nil {
				// log error
				jm.LogError(fmt.Errorf("worker-%d job %s returned error: %v", id, job.GetJobName(), err))
				continue
			}

			jm.log.Info("worker-%d job %s returned %d metrics", id, job.GetJobName(), len(metrics))

			// dispatch metrics to the per-region batcher
			for _, m := range metrics {
				// stop if parent context was cancelled
				if jm.parentCtx.Err() != nil {
					jm.log.Info("worker-%d interrupted before dispatching all metrics", id)
					return
				}

				// look up the right batcher for this region
				batcher, ok := jm.batcherMap.Load(job.GetRegion())
				if !ok {
					jm.log.Error("no metric batcher for region %s", job.GetRegion())
					continue
				}

				// enqueue into the batcher
				batcher.Add(jm.parentCtx, m)

				// replicate your previous debug logging
				vpcID := m.Metadata["vpc"]
				util := strconv.FormatFloat(m.Value, 'f', -1, 64)
				if vpcID != "" {
					jm.log.Debug("worker-%d dispatched metric=%s value=%s%% for %s", id, m.Name, util, vpcID)
				} else {
					jm.log.Debug("worker-%d dispatched metric=%s value=%s%%", id, m.Name, util)
				}
			}
		}
	}
}

// LogError logs job execution errors with consistent formatting.
func (jm *JobManager) LogError(err error) {
	jm.log.Error("jobmanager: error: %v", err)
}
