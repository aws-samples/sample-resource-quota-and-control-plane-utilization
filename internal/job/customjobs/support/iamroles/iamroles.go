package iamroles

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/support"
	supportclient "github.com/outofoffice3/aws-samples/geras/internal/awsclients/supportclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// IamRolesJob will implement the Job interface
// It will refresh the trusted advisor check for gp3 storage

type IamRoleJob struct {
	supportClient supportclient.SupportClient
	jobName       string
	region        string
	Logger        logger.Logger
}

type IamRoleJobConfig struct {
	SupportClient supportclient.SupportClient
	Logger        logger.Logger
}

const (
	IamRoleCheckId   string = "oQ7TT0l7J9"
	IamRoleJobPrefix string = "iamRoles"
)

// NewIamRoleCollector will return a new IamRoleCollector
func NewIamRoleJob(config IamRoleJobConfig) (job.Job, error) {
	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}
	job := &IamRoleJob{
		supportClient: config.SupportClient,
		jobName:       IamRoleJobPrefix + "-" + config.SupportClient.GetRegion(),
		region:        config.SupportClient.GetRegion(),
		Logger:        config.Logger,
	}

	return job, nil
}

// Collect will return the total number of IAM roles in a given region
func (irc *IamRoleJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	output, err := irc.supportClient.RefreshTrustedAdvisorCheck(ctx, &support.RefreshTrustedAdvisorCheckInput{
		CheckId: aws.String(IamRoleCheckId),
	})
	if err != nil {
		return nil, err
	}
	irc.Logger.Debug("%s refresh trusted advisor check staus : %v", irc.GetJobName(), output.Status)
	return nil, nil
}

// GetJobName will return the name of the job
func (irc *IamRoleJob) GetJobName() string {
	return irc.jobName
}

// get region
func (irc *IamRoleJob) GetRegion() string {
	return irc.region
}
