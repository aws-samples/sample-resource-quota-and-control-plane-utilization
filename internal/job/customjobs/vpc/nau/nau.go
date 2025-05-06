package vpcnau

import (
	"context"
	"sort"
	"time"

	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// VPCNAUJob computes network attachment units (NAU) per VPC
// and emits one metric per VPC.
type VPCNAUJob struct {
	nauCalculator nau.NAUCalculator
	jobName       string
	region        string
	Logger        logger.Logger
}

type VPCNAUConfig struct {
	NauCalculator nau.NAUCalculator
	Logger        logger.Logger
}

const (
	// Prefix for the job name
	VPCNAUJobPrefix      = "vpcNAU"
	cloudwatchMetricName = "vpcNAU"
)

// NewVPCNAUJob constructs a new VPCNAUJob
func NewVPCNAUJob(
	config VPCNAUConfig,
) (job.Job, error) {

	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}

	job := &VPCNAUJob{
		nauCalculator: config.NauCalculator,
		jobName:       VPCNAUJobPrefix + "-" + config.NauCalculator.GetRegion(),
		region:        config.NauCalculator.GetRegion(),
		Logger:        config.Logger,
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

	// Convert to CloudWatch metrics
	out := make([]sharedtypes.CloudWatchMetric, 0, len(keys))
	for _, vpcId := range keys {
		vpcNAU := output[vpcId]
		metric := sharedtypes.CloudWatchMetric{
			Name:      cloudwatchMetricName,
			Value:     float64(vpcNAU),
			Unit:      cwTypes.StandardUnitCount,
			Metadata:  map[string]string{"vpc": vpcId},
			Timestamp: now,
		}
		out = append(out, metric)
		j.Logger.Info("vpcNAU: added metric for VPC=%s → units=%d", vpcId, vpcNAU)
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
