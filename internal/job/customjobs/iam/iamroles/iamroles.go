package iamroles

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/iamclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// Error constants for IAM roles job
var (
	ErrListRoles      = errors.New("error listing IAM roles")
	ErrGetRolesQuota  = errors.New("error getting IAM roles quota")
)

// IamRoleJob will implement the Job interface
// It will calculate the total number of IAM roles and utilization percentage
type IamRoleJob struct {
	iamClient           iamclient.IamClient
	serviceQuotasClient servicequotaclient.ServiceQuotasClient
	jobName             string
	region              string
	Logger              logger.Logger
}

type IamRoleJobConfig struct {
	IamClient           iamclient.IamClient
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient
	Logger              logger.Logger
}

const (
	serviceQuotaCode     = "L-FE177D64" // IAM roles per account quota code
	serviceCode          = "iam"
)

// NewIamRoleJob will return a new IamRoleJob
func NewIamRoleJob(config IamRoleJobConfig) (job.Job, error) {
	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}
	job := &IamRoleJob{
		iamClient:           config.IamClient,
		serviceQuotasClient: config.ServiceQuotasClient,
		jobName:             string(sharedtypes.JobIAMRoleUtilization) + "-" + config.IamClient.GetRegion(),
		region:              config.IamClient.GetRegion(),
		Logger:              config.Logger,
	}

	return job, nil
}

// Execute will calculate the total number of IAM roles and utilization percentage
func (j *IamRoleJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	// Count all IAM roles
	var totalCount int64 = 0
	
	// Use paginator to retrieve all IAM roles
	paginator := iam.NewListRolesPaginator(j.iamClient, &iam.ListRolesInput{})
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			j.Logger.Error("%s failed to list IAM roles: %v", j.GetJobName(), err)
			return nil, fmt.Errorf("%w: %v", ErrListRoles, err)
		}
		totalCount += int64(len(output.Roles))
	}

	// Get the quota for IAM roles
	getServiceQuotaInput := &servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(serviceQuotaCode),
		ServiceCode: aws.String(serviceCode),
	}

	getServiceQuotaOutput, err := j.serviceQuotasClient.GetServiceQuota(ctx, getServiceQuotaInput)
	if err != nil {
		j.Logger.Error("%s failed to get IAM roles quota: %v", j.GetJobName(), err)
		return nil, fmt.Errorf("%w: %v", ErrGetRolesQuota, err)
	}
	
	quotaValue := aws.ToFloat64(getServiceQuotaOutput.Quota.Value)
	utilization := (float64(totalCount) / quotaValue) * float64(100)
	percent := strconv.FormatFloat(utilization, 'f', -1, 64)
	j.Logger.Info("%s total=%d, quota=%.2f, utilization=%q%%", j.GetJobName(), totalCount, quotaValue, percent)

	metric := sharedtypes.CloudWatchMetric{
		Name:      sharedtypes.JobIAMRoleUtilization,
		Value:     utilization,
		Unit:      sharedtypes.UnitPercent,
		Metadata:  nil,
		Timestamp: time.Now(),
	}

	return []sharedtypes.CloudWatchMetric{metric}, nil
}

// GetJobName will return the name of the job
func (j *IamRoleJob) GetJobName() string {
	return j.jobName
}

// GetRegion will return the region
func (j *IamRoleJob) GetRegion() string {
	return j.region
}