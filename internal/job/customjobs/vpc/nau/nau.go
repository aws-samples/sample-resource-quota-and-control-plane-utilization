// Package vpcnau provides a job implementation for monitoring
// VPC Network Address Usage (NAU) against service quotas.
package vpcnau

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// Error constants for VPC NAU job
var (
	ErrCalculateNAU     = errors.New("error calculating NAU")
	ErrGetNAUQuota      = errors.New("error getting NAU quota")
)

// VPCNAUJob calculates Network Address Usage (NAU) for each VPC in a region
// and generates utilization metrics against service quotas.
type VPCNAUJob struct {
	nauCalculator       nau.NauCalculatorV2                    // NAU calculation engine
	serviceQuotasClient servicequotaclient.ServiceQuotasClient // Service Quotas client for limits
	jobName             string                                 // Unique job identifier
	region              string                                 // AWS region being monitored
	Logger              logger.Logger                          // Logger instance
}

// VPCNAUConfig contains configuration for creating a VPCNAUJob.
type VPCNAUConfig struct {
	NauCalculator       nau.NauCalculatorV2                    // NAU calculator instance
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient // Service Quotas client
	Logger              logger.Logger                          // Logger for job operations
}

const (
	// AWS Service Quotas identifiers for VPC NAU.
	quotaCode   = "L-BB24F6E5" // VPC network address usage quota code
	serviceCode = "vpc"        // VPC service name in Service Quotas
)

// NewVPCNAUJob creates a new VPC NAU monitoring job.
func NewVPCNAUJob(
	config VPCNAUConfig,
) (job.Job, error) {

	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}

	job := &VPCNAUJob{
		nauCalculator:       config.NauCalculator,
		serviceQuotasClient: config.ServiceQuotasClient,
		jobName:             string(sharedtypes.JobNetworkAddressUnitsUtilization) + "-" + config.NauCalculator.GetRegion(),
		region:              config.NauCalculator.GetRegion(),
		Logger:              config.Logger,
	}

	return job, nil
}

// Execute calculates NAU for all VPCs and generates utilization metrics.
// Returns one CloudWatch metric per VPC with utilization percentage.
func (j *VPCNAUJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	// Get the raw NAU totals per VPC
	output, err := j.nauCalculator.CalculateNau()
	if err != nil {
		j.Logger.Error("%s failed to calculate NAU: %v", j.GetJobName(), err)
		return nil, fmt.Errorf("%w: %v", ErrCalculateNAU, err)
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
		j.Logger.Error("%s failed to get NAU quota: %v", j.GetJobName(), err)
		return nil, fmt.Errorf("%w: %v", ErrGetNAUQuota, err)
	}
	quotaValue := aws.ToFloat64(getServiceQuotaOutput.Quota.Value)

	// Convert to CloudWatch metrics
	out := make([]sharedtypes.CloudWatchMetric, 0, len(keys))
	for _, vpcId := range keys {
		j.Logger.Debug("%s calculating nau utilization for %s", j.GetJobName(), vpcId)
		vpcNAU := output[vpcId]
		j.Logger.Debug("%s : units %d, quota value %.2f", j.GetJobName(), vpcNAU, quotaValue)
		nauUtilization := float64(vpcNAU) / float64(quotaValue)
		metric := sharedtypes.CloudWatchMetric{
			Name:      sharedtypes.JobNetworkAddressUnitsUtilization,
			Value:     nauUtilization,
			Unit:      sharedtypes.UnitPercent,
			Metadata:  map[string]string{"vpc": vpcId},
			Timestamp: now,
		}
		out = append(out, metric)
		percent := strconv.FormatFloat(nauUtilization, 'f', -1, 64)
		j.Logger.Debug("%s : added metric for %s → nau utilization=%q%%", j.GetJobName(), vpcId, percent)
	}
	return out, nil
}

// GetJobName returns the unique identifier for this job.
func (j *VPCNAUJob) GetJobName() string {
	return j.jobName
}

// GetRegion returns the AWS region this job monitors.
func (j *VPCNAUJob) GetRegion() string {
	return j.region
}