package networkinterfaces

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"

	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// NetworkInterfaceJob will implement the Collector interface
// Its will calculate the total amount of network interfaces in a given region

type NetworkInterfaceJob struct {
	ec2Client           ec2client.Ec2Client
	serviceQuotasClient servicequotaclient.ServiceQuotasClient
	Logger              logger.Logger
	jobName             string
	region              string
}

type NetworkInterfaceJobConfig struct {
	Ec2Client           ec2client.Ec2Client
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient
	Logger              logger.Logger
}

const (
	networkInterfaceJobPrefix = "networkInterfaces"
	cloudwatchMetricName      = "networkInterfaces"
	quotaCode                 = "L-DF5E4CA3"
	servicename               = "vpc"
)

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

func (nic *NetworkInterfaceJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
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

	nic.Logger.Debug("%s total count : %v", nic.jobName, totalCount)

	// call servicequota api to get current quota limit for network interfaces
	getServiceQuotaInput := &servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(quotaCode),
		ServiceCode: aws.String(servicename),
	}

	getServiceQuotaOutput, err := nic.serviceQuotasClient.GetServiceQuota(ctx, getServiceQuotaInput)
	if err != nil {
		return nil, err
	}
	quotaValue := getServiceQuotaOutput.Quota.Value
	nic.Logger.Debug("%s quota value : %v", nic.jobName, *quotaValue)
	utilization := (float64(totalCount) / *quotaValue) * float64(100)
	nic.Logger.Debug("%s utilization %.2f%%\n", nic.GetJobName(), utilization)

	metric := sharedtypes.CloudWatchMetric{
		Name:      cloudwatchMetricName,
		Value:     utilization,
		Unit:      cwTypes.StandardUnitPercent,
		Metadata:  nil,
		Timestamp: time.Now(),
	}
	return []sharedtypes.CloudWatchMetric{metric}, nil
}

// GetJobName return the name of the job
func (nic *NetworkInterfaceJob) GetJobName() string {
	return nic.jobName
}

// get region
func (nic *NetworkInterfaceJob) GetRegion() string {
	return nic.region
}
