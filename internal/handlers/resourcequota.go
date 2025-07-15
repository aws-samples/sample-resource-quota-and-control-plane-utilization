// Package handlers provides AWS Lambda handlers for resource quota monitoring.
package handlers

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	"github.com/outofoffice3/aws-samples/geras/internal/serviceconfig"
)

var (
	// Error variables for resource quota handler validation.
	ErrClientFactoryNil          = errors.New("client factory is nil")
	ErrCloudwatchLogGroupNotSet  = errors.New("cloudwatch log group is not set")
	ErrCloudWatchLogStreamNotSet = errors.New("cloudwatch log stream is not set")
	ErrMetricNamespaceNotSet     = errors.New("metric namespace is not set")
	ErrRegionalBatchersNil       = errors.New("regional metric batchers is nil")
	ErrJobManagerNil             = errors.New("job manager is nil")
	ErrServiceConfigNil          = errors.New("service config is nil")
	ErrStoreNil                  = errors.New("nau store is nil")
	ErrMetricFlushFailed         = errors.New("failed to flush metrics")
	ErrStoreCloseFailed          = errors.New("failed to close store")
)

// ResourceQuotaEventHandler defines the interface for handling CloudWatch events.
type ResourceQuotaEventHandler interface {
	HandleEvent(ctx context.Context, event events.CloudWatchEvent) error
}

// ResourceQuotaHandler processes scheduled CloudWatch events to trigger
// resource quota monitoring jobs, coordinate metric collection, and close the NAU store.
type ResourceQuotaHandler struct {
	ClientFactory       factory.ClientFactory
	CloudwatchLogGroup  string
	CloudWatchLogStream string
	Namespace           string
	RegionalBatchers    safestore.Store[metrics.Batcher]
	JobManager          job.JobManager
	ServiceConfig       *serviceconfig.TopLevelServiceConfig
	Store               nau.AccountNauStore
	Logger              logger.Logger
}

// ResourceQuotaHandlerConfig contains configuration parameters for ResourceQuotaHandler.
type ResourceQuotaHandlerConfig struct {
	ClientFactory       factory.ClientFactory
	CloudwatchLogGroup  string
	CloudWatchLogStream string
	Namespace           string
	RegionalBatchers    safestore.Store[metrics.Batcher]
	JobManager          job.JobManager
	ServiceConfig       *serviceconfig.TopLevelServiceConfig
	Store               nau.AccountNauStore
	Logger              logger.Logger
}

// NewResourceQuotaHandler creates a new resource quota handler with validation.
func NewResourceQuotaHandler(config ResourceQuotaHandlerConfig) (*ResourceQuotaHandler, error) {
	if config.Logger == nil {
		config.Logger = logger.Get()
	}
	if config.ClientFactory == nil {
		return nil, ErrClientFactoryNil
	}
	if config.CloudWatchLogStream == "" {
		return nil, ErrCloudWatchLogStreamNotSet
	}
	if config.CloudwatchLogGroup == "" {
		return nil, ErrCloudwatchLogGroupNotSet
	}
	if config.Namespace == "" {
		return nil, ErrMetricNamespaceNotSet
	}
	if config.RegionalBatchers == nil {
		return nil, ErrRegionalBatchersNil
	}
	if config.JobManager == nil {
		return nil, ErrJobManagerNil
	}
	if config.ServiceConfig == nil {
		return nil, ErrServiceConfigNil
	}
	if config.Store == nil {
		return nil, ErrStoreNil
	}

	return &ResourceQuotaHandler{
		ClientFactory:       config.ClientFactory,
		CloudwatchLogGroup:  config.CloudwatchLogGroup,
		CloudWatchLogStream: config.CloudWatchLogStream,
		Namespace:           config.Namespace,
		RegionalBatchers:    config.RegionalBatchers,
		JobManager:          config.JobManager,
		ServiceConfig:       config.ServiceConfig,
		Store:               config.Store,
		Logger:              config.Logger,
	}, nil
}

// HandleEvent processes scheduled CloudWatch events by coordinating job execution,
// flushing all metrics, closing the NAU store, and returning any error.
func (h *ResourceQuotaHandler) HandleEvent(ctx context.Context, event events.CloudWatchEvent) error {
	h.Logger.Info("resource handler handling event %+v", event)

	// 1) Wait for all jobs to finish
	h.JobManager.Wait()
	h.Logger.Info("all jobs completed")

	// 2) Flush metrics for each region
	if err := h.flushAllMetrics(ctx); err != nil {
		return err
	}

	// 3) Close the NAU store
	if err := h.closeStore(); err != nil {
		return err
	}

	h.Logger.Info("resource handler completed")
	return nil
}

// flushAllMetrics flushes metrics for all regions.
func (h *ResourceQuotaHandler) flushAllMetrics(ctx context.Context) error {
	h.Logger.Info("flushing metrics to CloudWatch Logs for all regions")
	h.RegionalBatchers.Range(func(region string, batcher metrics.Batcher) bool {
		batcher.FlushAll(ctx)
		return true
	})
	h.Logger.Info("cloudwatch metric batchers completed in all regions")
	return nil
}

// closeStore closes the NAU store and handles errors.
func (h *ResourceQuotaHandler) closeStore() error {
	h.Logger.Info("closing NAU store")
	if err := h.Store.Close(); err != nil {
		h.Logger.Error("error closing NAU store: %v", err)
		return ErrStoreCloseFailed
	}
	h.Logger.Info("NAU store closed")
	return nil
}

// HandleInitError logs initialization errors and terminates the application.
func HandleInitError(logger logger.Logger, err error) {
	logger.Error("error initializing service: %v", err)
	os.Exit(1)
}
