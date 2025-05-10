package gp3storage

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/support"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/supportclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// gp3StorageJob will implement the Job interface
// It will refresh the trusted advisor check to get
// utilization of gp3 storage against the service quota

type Gp3StorageJob struct {
	supportClient supportclient.SupportClient
	jobName       string
	region        string
	Logger        logger.Logger
}

type Gp3StorageJobConfig struct {
	SupportClient supportclient.SupportClient
	Logger        logger.Logger
}

const (
	gp3StorageCheckId string = "dH7RR0l6J3"
	gp3JobPrefix      string = "gp3Storage"
)

// NewGp3StorageJob will return a new instance of gp3Storage Job
func NewGp3StorageJob(config Gp3StorageJobConfig) (job.Job, error) {
	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}

	job := &Gp3StorageJob{
		supportClient: config.SupportClient,
		jobName:       gp3JobPrefix + "-" + config.SupportClient.GetRegion(),
		region:        config.SupportClient.GetRegion(),
		Logger:        config.Logger,
	}

	return job, nil
}

// Execute will refresh the trusted advisor for gp3 storage
func (j *Gp3StorageJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	// make call to trusted advisor via support API
	output, err := j.supportClient.RefreshTrustedAdvisorCheck(ctx, &support.RefreshTrustedAdvisorCheckInput{
		CheckId: aws.String(gp3StorageCheckId),
	})
	if err != nil {
		return nil, err
	}
	j.Logger.Debug("%s refresh trusted advisor check status: %v", j.GetJobName(), output.Status)
	return nil, nil
}

// GetJobName will return the name of the job
func (j *Gp3StorageJob) GetJobName() string {
	return j.jobName
}

// get region
func (j *Gp3StorageJob) GetRegion() string {
	return j.region
}
