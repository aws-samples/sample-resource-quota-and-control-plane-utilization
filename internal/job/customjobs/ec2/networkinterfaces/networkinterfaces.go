// Package networkinterfaces provides a job implementation for monitoring
// EC2 network interface usage against service quotas.
package networkinterfaces

import (
	"context"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// NetworkInterfaceJob monitors EC2 network interface usage against service quotas.
// It counts all network interfaces in a region and calculates utilization percentage.
type NetworkInterfaceJob struct {
	ec2Client           ec2client.Ec2Client                    // EC2 client for listing network interfaces
	serviceQuotasClient servicequotaclient.ServiceQuotasClient // Service Quotas client for quota limits
	Logger              logger.Logger                          // Logger instance
	jobName             string                                 // Unique job identifier
	region              string                                 // AWS region being monitored
}

// NetworkInterfaceJobConfig contains configuration for creating a NetworkInterfaceJob.
type NetworkInterfaceJobConfig struct {
	Ec2Client           ec2client.Ec2Client                    // EC2 client for API calls
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient // Service Quotas client for limits
	Logger              logger.Logger                          // Logger for job operations
}

const (
	// Job and metric naming constants.
	networkInterfaceJobPrefix = "networkInterfaces"
	cloudwatchMetricName      = "networkInterfaces"
	// AWS Service Quotas identifiers for network interfaces.
	quotaCode   = "L-DF5E4CA3" // Network interfaces per region quota code
	servicename = "vpc"        // VPC service name in Service Quotas
)

// NewNetworkInterfaceJob creates a new network interface monitoring job.
func NewNetworkInterfaceJob(config NetworkInterfaceJobConfig) (job.Job, error) {
	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}

	nic := &NetworkInterfaceJob{
		ec2Client:           config.Ec2Client,
		serviceQuotasClient: config.ServiceQuotasClient,
		jobName:             networkInterfaceJobPrefix + "-" + config.Ec2Client.GetRegion(),
		region:              config.Ec2Client.GetRegion(),
		Logger:              config.Logger,
	}

	return nic, nil
}

// Execute counts network interfaces and calculates quota utilization.
// Returns a CloudWatch metric with the utilization percentage.
func (nic *NetworkInterfaceJob) Execute(ctx context.Context) ([]types.CloudWatchMetric, error) {
	input := &ec2.DescribeNetworkInterfacesInput{}
	var totalCount int64 = 0

	// use aws sdk paginator to retrieve all network interfaces
	paginator := ec2.NewDescribeNetworkInterfacesPaginator(nic.ec2Client, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		totalCount += int64(len(output.NetworkInterfaces))
	}

	// call servicequota api to get current quota limit for network interfaces
	getServiceQuotaInput := &servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(quotaCode),
		ServiceCode: aws.String(servicename),
	}

	getServiceQuotaOutput, err := nic.serviceQuotasClient.GetServiceQuota(ctx, getServiceQuotaInput)
	if err != nil {
		return nil, err
	}
	quotaValue := aws.ToFloat64(getServiceQuotaOutput.Quota.Value)
	utilization := (float64(totalCount) / quotaValue) * float64(100)
	percent := strconv.FormatFloat(utilization, 'f', -1, 64)
	nic.Logger.Debug("%s total=%d, quota=%.2f, utilization=%q%%", nic.GetJobName(), totalCount, quotaValue, percent)
	metric := types.CloudWatchMetric{
		Name:      cloudwatchMetricName,
		Value:     utilization,
		Unit:      types.UnitPercent,
		Metadata:  nil,
		Timestamp: time.Now(),
	}
	return []types.CloudWatchMetric{metric}, nil
}

// GetJobName returns the unique identifier for this job.
func (nic *NetworkInterfaceJob) GetJobName() string {
	return nic.jobName
}

// GetRegion returns the AWS region this job monitors.
func (nic *NetworkInterfaceJob) GetRegion() string {
	return nic.region
}
