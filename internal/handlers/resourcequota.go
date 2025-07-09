// Package handlers provides AWS Lambda handlers for resource quota monitoring.
package handlers

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/factory"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	"github.com/outofoffice3/aws-samples/geras/internal/serviceconfig"
)

const (
	// Error message constants for resource quota handler validation.
	ClientFactoryNilErrMsg          = "client factory is nil"
	CloudwatchLogGroupNotSetErrMsg  = "cloudwatch log group is not set"
	CloudWatchLogStreamNotSetErrMsg = "cloudwatch log stream is not set"
	MetricNamespaceNotSetErrMsg     = "metric namespace is not set"
	RegionalBatchersNilErrMsg       = "regional metric batchers is nil"
	JobManagerNilErrMsg             = "job manager is nil"
	ServiceConfigNilErrMsg          = "service config is nil"
	StoreNilErrMsg                  = "nau store is nil"
)

// ResourceQuotaHandler processes scheduled CloudWatch events to trigger
// resource quota monitoring jobs, coordinate metric collection, and close the NAU store.
type ResourceQuotaHandler struct {
	ClientFactory       factory.ClientFactory
	CloudwatchLogGroup  string
	CloudWatchLogStream string
	Namespace           string
	RegionalBatchers    *safemap.TypedMap[metrics.Batcher]
	JobManager          *job.JobManager
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
	RegionalBatchers    *safemap.TypedMap[metrics.Batcher]
	JobManager          *job.JobManager
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
		return nil, errors.New(ClientFactoryNilErrMsg)
	}
	if config.CloudWatchLogStream == "" {
		return nil, errors.New(CloudWatchLogStreamNotSetErrMsg)
	}
	if config.CloudwatchLogGroup == "" {
		return nil, errors.New(CloudwatchLogGroupNotSetErrMsg)
	}
	if config.Namespace == "" {
		return nil, errors.New(MetricNamespaceNotSetErrMsg)
	}
	if config.RegionalBatchers == nil {
		return nil, errors.New(RegionalBatchersNilErrMsg)
	}
	if config.JobManager == nil {
		return nil, errors.New(JobManagerNilErrMsg)
	}
	if config.ServiceConfig == nil {
		return nil, errors.New(ServiceConfigNilErrMsg)
	}
	if config.Store == nil {
		return nil, errors.New(StoreNilErrMsg)
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
	h.Logger.Info("flushing metrics to CloudWatch Logs for all regions")
	h.RegionalBatchers.Range(func(region string, batcher metrics.Batcher) bool {
		batcher.FlushAll(ctx)
		return true
	})
	h.Logger.Info("cloudwatch metric batchers completed in all regions")

	// 3) Close the NAU store (was previously on JobManager.Wait path)
	h.Logger.Info("closing NAU store")
	if err := h.Store.Close(); err != nil {
		h.Logger.Error("error closing NAU store: %v", err)
		return err
	}
	h.Logger.Info("NAU store closed")

	h.Logger.Info("resource handler completed")
	return nil
}

// HandleInitError logs initialization errors and terminates the application.
func HandleInitError(logger logger.Logger, err error) {
	logger.Error("error initializing service: %v", err)
	os.Exit(1)
}
