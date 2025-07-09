package oidcproviders

import (
	"context"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"

	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/iamclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedTypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// OIDCProvider will implement the Job interface
// It will calculate the total number of OIDC providers in a given region

type OIDCProviderJob struct {
	IamClient           iamclient.IamClient
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient
	jobName             string
	region              string
	Logger              logger.Logger
}

type OIDCProviderJobConfig struct {
	IamClient          iamclient.IamClient
	ServiceQuotasCliet servicequotaclient.ServiceQuotasClient
	Logger             logger.Logger
}

const (
	oidcProvidersJobPrefix = "oidcProviders"
	cloudwatchMetricName   = "oidcProviders"
	serviceQuotaCode       = "L-858F3967"
	serviceCode            = "iam"
)

// NewOIDCProviderJob will create a new OIDCProviderJob
func NewOIDCProviderJob(config OIDCProviderJobConfig) (job.Job, error) {
	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}
	job := &OIDCProviderJob{
		IamClient:           config.IamClient,
		ServiceQuotasClient: config.ServiceQuotasCliet,
		jobName:             oidcProvidersJobPrefix + "-" + config.IamClient.GetRegion(),
		region:              config.ServiceQuotasCliet.GetRegion(),
		Logger:              config.Logger,
	}

	return job, nil
}

// Execute will return the total number of OIDC providers in a given region
func (j *OIDCProviderJob) Execute(ctx context.Context) ([]sharedTypes.CloudWatchMetric, error) {

	input := &iam.ListOpenIDConnectProvidersInput{}
	var totalCount int64 = 0
	// make call to list oidc providers and calculate total amount
	oidcProvidersOutput, err := j.IamClient.ListOpenIDConnectProviders(ctx, input)
	if err != nil {
		return nil, err
	}

	totalCount = int64(len(oidcProvidersOutput.OpenIDConnectProviderList))

	// get the quota for oidc providers
	getServiceQuotaInput := &servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(serviceQuotaCode),
		ServiceCode: aws.String(serviceCode),
	}

	getServiceQuotaOutput, err := j.ServiceQuotasClient.GetServiceQuota(ctx, getServiceQuotaInput)
	if err != nil {
		return nil, err
	}
	quotaValue := aws.ToFloat64(getServiceQuotaOutput.Quota.Value)
	utilization := (float64(totalCount) / quotaValue) * float64(100)
	percent := strconv.FormatFloat(utilization, 'f', -1, 64)
	j.Logger.Info("%s total=%d, quota=%.2f, utilization=%q%%", j.GetJobName(), totalCount, quotaValue, percent)

	meric := sharedTypes.CloudWatchMetric{
		Name:      cloudwatchMetricName,
		Value:     utilization,
		Unit:      sharedTypes.UnitPercent,
		Metadata:  nil,
		Timestamp: time.Now(),
	}

	return []sharedTypes.CloudWatchMetric{meric}, nil
}

// GetJobName will return the job name
func (j *OIDCProviderJob) GetJobName() string {
	return j.jobName
}

// GetRegion will return the region
func (j *OIDCProviderJob) GetRegion() string {
	return j.region
}
