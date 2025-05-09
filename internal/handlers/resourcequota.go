package handlers

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	metricemfbatcher "github.com/outofoffice3/aws-samples/geras/internal/emfbatcher/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/generics/safemap"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/serviceconfig"
)

const (
	// error messages
	ClientFactoryNilErrMsg                    = "client factory is nil"
	CloudwatchLogGroupNotSetErrMsg            = "cloudwatch log group is not set"
	CloudWatchLogStreamNotSetErrMsg           = "cloudwatch log stream is not set"
	MetricNamespaceNotSetErrMsg               = "metric namespace is not set"
	ErrEMFBatcherNilErrMsg                    = "error emf batcher is nil"
	RegionalCloudwatchMetricBatchersNilErrMsg = "regional cloudwatch metric batchers is nil"
	JobManagerNilErrMsg                       = "job manager is nil"
	ServiceConfigNilErrMsg                    = "service config is nil"
)

// ResourceQuotaHandler handles scheduled events
type ResourceQuotaHandler struct {
	ClientFactory                    cwlclient.ClientFactory
	CloudwatchLogGroup               string
	CloudWatchLogGroupStream         string
	Namespace                        string
	RegionalCloudwatchMetricBatchers *safemap.TypedMap[metricemfbatcher.CloudWatchMetricBatcher]
	JobManager                       *job.JobManager
	ServiceConfig                    *serviceconfig.TopLevelServiceConfig
	Logger                           logger.Logger
}

type ResourceQuotaHandlerConfig struct {
	ClientFactory                    cwlclient.ClientFactory
	CloudwatchLogGroup               string
	CloudWatchLogGroupStream         string
	Namespace                        string
	RegionalCloudwatchMetricBatchers *safemap.TypedMap[metricemfbatcher.CloudWatchMetricBatcher]
	JobManager                       *job.JobManager
	ServiceConfig                    *serviceconfig.TopLevelServiceConfig
	Logger                           logger.Logger
}

// New ResourceQuotaHandler creates new handler
func NewResourceQuotaHandler(config ResourceQuotaHandlerConfig) (*ResourceQuotaHandler, error) {
	// perform nil checks
	// if the logger is not set, use the default logger
	if config.Logger == nil {
		config.Logger = logger.Get()
	}
	if config.ClientFactory == nil {
		return nil, errors.New(ClientFactoryNilErrMsg)
	}
	if config.CloudWatchLogGroupStream == "" {
		return nil, errors.New(CloudWatchLogStreamNotSetErrMsg)
	}
	if config.CloudwatchLogGroup == "" {
		return nil, errors.New(CloudwatchLogGroupNotSetErrMsg)
	}
	if config.Namespace == "" {
		return nil, errors.New(MetricNamespaceNotSetErrMsg)
	}

	if config.RegionalCloudwatchMetricBatchers == nil {
		return nil, errors.New(RegionalCloudwatchMetricBatchersNilErrMsg)
	}
	if config.JobManager == nil {
		return nil, errors.New(JobManagerNilErrMsg)
	}
	if config.ServiceConfig == nil {
		return nil, errors.New(ServiceConfigNilErrMsg)
	}

	rqh := &ResourceQuotaHandler{
		ClientFactory:                    config.ClientFactory,
		CloudwatchLogGroup:               config.CloudwatchLogGroup,
		CloudWatchLogGroupStream:         config.CloudWatchLogGroupStream,
		Namespace:                        config.Namespace,
		RegionalCloudwatchMetricBatchers: config.RegionalCloudwatchMetricBatchers,
		JobManager:                       config.JobManager,
		ServiceConfig:                    config.ServiceConfig,
		Logger:                           config.Logger,
	}
	return rqh, nil
}

// HandleEvent process the scheduled event
func (h *ResourceQuotaHandler) HandleEvent(ctx context.Context, event events.CloudWatchEvent) error {

	h.Logger.Info("resource handler handling event %+v", event)

	h.JobManager.Wait() // wait for all jobs to complete
	h.Logger.Info("all jobs completed")
	h.Logger.Info("waiting for cloudwatch metric batchers to complete")
	// wait for each cloudwatch metric batcher to complete
	// use the range function for safemap to wait for each batcher to commplete
	h.RegionalCloudwatchMetricBatchers.Range(func(key string, value metricemfbatcher.CloudWatchMetricBatcher) bool {
		value.Batcher.Wait()
		return true
	})
	h.Logger.Info("cloudwatch metric emf batchers completed in all regions")
	h.Logger.Info("resource handler completed")
	return nil
}

// Handle Init Error
func HandleInitError(logger logger.Logger, err error) {
	logger.Error("error initializing service: %v", err)
	os.Exit(1)
}
