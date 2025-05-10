package vpcnau

import (
	"context"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// VPCNAUJob computes network attachment units (NAU) per VPC
// and emits one metric per VPC.
type VPCNAUJob struct {
	nauCalculator       nau.NAUCalculator
	serviceQuotasClient servicequotaclient.ServiceQuotasClient
	jobName             string
	region              string
	Logger              logger.Logger
}

type VPCNAUConfig struct {
	NauCalculator       nau.NAUCalculator
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient
	Logger              logger.Logger
}

const (
	// Prefix for the job name
	VPCNAUJobPrefix      = "vpcNAU"
	cloudwatchMetricName = "vpcNAU"
	quotaCode            = "L-BB24F6E5"
	serviceCode          = "vpc"
)

// NewVPCNAUJob constructs a new VPCNAUJob
func NewVPCNAUJob(
	config VPCNAUConfig,
) (job.Job, error) {

	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}

	job := &VPCNAUJob{
		nauCalculator:       config.NauCalculator,
		serviceQuotasClient: config.ServiceQuotasClient,
		jobName:             VPCNAUJobPrefix + "-" + config.NauCalculator.GetRegion(),
		region:              config.NauCalculator.GetRegion(),
		Logger:              config.Logger,
	}

	return job, nil
}

func (j *VPCNAUJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	// Get the raw NAU totals per VPC
	output, err := j.nauCalculator.CalculateVPCNAU(ctx)
	if err != nil {
		return nil, err
	}

	// Capture timestamp once for all metrics
	now := time.Now()

	// If you want deterministic ordering, you can sort the VPC IDs:
	keys := make([]string, 0, len(output))
	for id := range output {
		keys = append(keys, id)
	}
	sort.Strings(keys)

	// get current vpc nau allocation from service quotas
	getServiceQuotaOutput, err := j.serviceQuotasClient.GetServiceQuota(ctx, &servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(quotaCode),
		ServiceCode: aws.String(serviceCode),
	})
	if err != nil {
		return nil, err
	}
	quotaValue := getServiceQuotaOutput.Quota.Value

	// Convert to CloudWatch metrics
	out := make([]sharedtypes.CloudWatchMetric, 0, len(keys))
	for _, vpcId := range keys {
		j.Logger.Debug("%s calculating nau utilization for VPC=%s", j.GetJobName(), vpcId)
		vpcNAU := output[vpcId]
		j.Logger.Debug("%s : units %d, quota value %v", j.GetJobName(), vpcNAU, *quotaValue)
		nauUtilization := float64(vpcNAU) / float64(*quotaValue)
		metric := sharedtypes.CloudWatchMetric{
			Name:      cloudwatchMetricName,
			Value:     nauUtilization,
			Unit:      cwTypes.StandardUnitPercent,
			Metadata:  map[string]string{"vpc": vpcId},
			Timestamp: now,
		}
		out = append(out, metric)
		j.Logger.Debug("%s : added metric for VPC=%s → nau utilization=%v", j.GetJobName(), vpcId, nauUtilization)
	}
	return out, nil
}

// GetJobName returns the job's name
func (j *VPCNAUJob) GetJobName() string {
	return j.jobName
}

// GetRegion returns the AWS region
func (j *VPCNAUJob) GetRegion() string {
	return j.region
}
