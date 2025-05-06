package job

import (
	"context"
	"fmt"
	"os"
	"time"

	"sync"

	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

const (
	// how many jobs we can buffer before AddJob blocks
	DefaultBufferSize = 100
)

// Job is the interface for work items.
type Job interface {
	// Execute should return zero or more metrics, or an error.
	// It receives a context (which may carry a per-job timeout) and the AWS config.
	Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error)
	GetRegion() string
	GetJobName() string
}

// JobManager runs jobs with a fixed-size worker pool, applies per-job timeouts,
// dispatches metrics into region-specific channels, reports errors, and respects
// an external parent context for graceful shutdown.
type JobManager struct {
	parentCtx  context.Context
	jobTimeout time.Duration
	jobCh      chan Job
	metricMap  *safemap.TypedMap[chan sharedtypes.CloudWatchMetric]
	workers    int
	log        logger.Logger
	shutdownWg sync.WaitGroup
}

type JobManagerConfig struct {
	ParentCtx  context.Context
	Workers    int
	JobTimeout time.Duration
	MetricMap  *safemap.TypedMap[chan sharedtypes.CloudWatchMetric]
	Log        logger.Logger
}

// NewJobManager returns a *JobManager that:
//   - watches parentCtx (cancelling parentCtx stops all workers),
//   - spins up exactly `workers` goroutines,
//   - buffers up to DefaultBufferSize pending jobs,
//   - bounds each Execute call by jobTimeout,
//   - emits Info+Debug logs,
//   - uses errCh & metricMap to report errors & metrics.
func NewJobManager(config JobManagerConfig) *JobManager {
	if config.Log == nil {
		logger.Init(logger.INFO, os.Stdout)
		config.Log = logger.Get()
	}
	jm := &JobManager{
		parentCtx:  config.ParentCtx,
		jobTimeout: config.JobTimeout,
		jobCh:      make(chan Job, DefaultBufferSize),
		metricMap:  config.MetricMap,
		workers:    config.Workers,
		log:        config.Log,
	}

	jm.log.Info("jobmanager: starting %d workers", config.Workers)
	jm.shutdownWg.Add(config.Workers)
	for i := range config.Workers {
		go jm.worker(i)
	}
	return jm
}

// AddJob enqueues a Job for processing, blocking if the buffer is full.
// If parentCtx is already cancelled, AddJob drops the job instead of blocking.
func (jm *JobManager) AddJob(job Job) {
	// check context cancellatoin first
	if jm.parentCtx.Err() != nil {
		jm.log.Debug("jobmanager: parent context cancelled—dropping job %s (region=%s)", job.GetJobName(), job.GetRegion())
		return
	}
	jm.jobCh <- job
	jm.log.Debug("jobmanager: enqueued job %s (region=%s)", job.GetJobName(), job.GetRegion())
}

// Wait signals no more jobs, then blocks until all workers have exited.
func (jm *JobManager) Wait() {
	close(jm.jobCh)
	jm.log.Info("jobmanager: waiting for workers to finish")
	jm.shutdownWg.Wait()
	jm.log.Info("jobmanager: all workers exited")
}

func (jm *JobManager) worker(id int) {
	defer jm.shutdownWg.Done()
	jm.log.Info("jobmanager: worker-%d started", id)

	// explicitly check if context is cancelled
	if jm.parentCtx.Err() != nil {
		jm.log.Info("jobmanager: worker-%d shutting down (parent context done)", id)
		return
	}

	for {
		select {
		case <-jm.parentCtx.Done():
			jm.log.Info("jobmanager: worker-%d shutting down (parent context done)", id)
			return

		case job, ok := <-jm.jobCh:
			if !ok {
				jm.log.Info("jobmanager: worker-%d shutting down (job channel closed)", id)
				return
			}

			jm.log.Debug("jobmanager: worker-%d executing job %s", id, job.GetJobName())

			// derive per-job context with timeout
			ctx, cancel := context.WithTimeout(jm.parentCtx, jm.jobTimeout)
			metrics, err := job.Execute(ctx)
			cancel() // always release the timer

			if err != nil {
				// log error
				jm.LogError(fmt.Errorf("worker-%d job %s returned error: %v", id, job.GetJobName(), err))
				continue
			}

			jm.log.Debug("jobmanager: worker-%d job %s returned %d metrics", id, job.GetJobName(), len(metrics))

			// dispatch metrics (blocking until there's room or parentCtx cancels)
			for _, m := range metrics {
				select {
				case <-jm.parentCtx.Done():
					jm.log.Info("jobmanager: worker-%d interrupted before dispatching all metrics", id)
					return
				default:
					ch, ok := jm.metricMap.Load(job.GetRegion())
					if !ok {
						jm.log.Error("jobmanager: no metric channel for region %s", job.GetRegion())
						continue
					}
					select {
					case ch <- m:
						jm.log.Debug("jobmanager: worker-%d dispatched metric %s for region %s", id, m.Name, job.GetRegion())
					case <-jm.parentCtx.Done():
						jm.log.Info("jobmanager: worker-%d interrupted while sending metric", id)
						return
					}
				}
			}
		}
	}
}

// LogError
func (jm *JobManager) LogError(err error) {
	jm.log.Error("jobmanager: error: %v", err)
}
