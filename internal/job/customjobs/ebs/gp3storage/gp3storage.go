package gp3storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// Error constants for GP3 storage job
var (
	ErrDescribeVolumes = errors.New("error describing GP3 volumes")
	ErrGetGP3Quota     = errors.New("error getting GP3 storage quota")
)

// Gp3StorageJob will implement the Job interface
// It will calculate the total GP3 storage used and utilization percentage
type Gp3StorageJob struct {
	ec2Client           ec2client.Ec2Client
	serviceQuotasClient servicequotaclient.ServiceQuotasClient
	jobName             string
	region              string
	Logger              logger.Logger
}

type Gp3StorageJobConfig struct {
	Ec2Client           ec2client.Ec2Client
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient
	Logger              logger.Logger
}

const (
	serviceQuotaCode    = "L-7A658B76" // GP3 volume storage quota code
	serviceCode         = "ebs"
	bytesPerTiB         = 1099511627776 // 1 TiB in bytes
)

// NewGp3StorageJob will return a new instance of gp3Storage Job
func NewGp3StorageJob(config Gp3StorageJobConfig) (job.Job, error) {
	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}

	job := &Gp3StorageJob{
		ec2Client:           config.Ec2Client,
		serviceQuotasClient: config.ServiceQuotasClient,
		jobName:             string(sharedtypes.JobGP3StorageUtilization) + "-" + config.Ec2Client.GetRegion(),
		region:              config.Ec2Client.GetRegion(),
		Logger:              config.Logger,
	}

	return job, nil
}

// Execute will calculate the total GP3 storage used and utilization percentage
func (j *Gp3StorageJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	// Filter for GP3 volumes
	filter := types.Filter{
		Name:   aws.String("volume-type"),
		Values: []string{"gp3"},
	}

	var totalSizeBytes int64 = 0

	// Use paginator to retrieve all GP3 volumes
	paginator := ec2.NewDescribeVolumesPaginator(j.ec2Client, &ec2.DescribeVolumesInput{
		Filters: []types.Filter{filter},
	})
	
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			j.Logger.Error("%s failed to describe volumes: %v", j.GetJobName(), err)
			return nil, fmt.Errorf("%w: %v", ErrDescribeVolumes, err)
		}

		// Sum up the size of all GP3 volumes (size is in GiB, convert to bytes)
		for _, volume := range output.Volumes {
			totalSizeBytes += int64(*volume.Size) * 1073741824 // GiB to bytes
		}
	}

	// Convert total size to TiB for quota comparison
	totalSizeTiB := float64(totalSizeBytes) / float64(bytesPerTiB)

	// Get the quota for GP3 storage (in TiB)
	getServiceQuotaInput := &servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(serviceQuotaCode),
		ServiceCode: aws.String(serviceCode),
	}

	getServiceQuotaOutput, err := j.serviceQuotasClient.GetServiceQuota(ctx, getServiceQuotaInput)
	if err != nil {
		j.Logger.Error("%s failed to get GP3 storage quota: %v", j.GetJobName(), err)
		return nil, fmt.Errorf("%w: %v", ErrGetGP3Quota, err)
	}

	quotaValue := aws.ToFloat64(getServiceQuotaOutput.Quota.Value)
	utilization := (totalSizeTiB / quotaValue) * 100
	percent := strconv.FormatFloat(utilization, 'f', -1, 64)
	j.Logger.Info("%s total=%.2f TiB, quota=%.2f TiB, utilization=%q%%", j.GetJobName(), totalSizeTiB, quotaValue, percent)

	metric := sharedtypes.CloudWatchMetric{
		Name:      sharedtypes.JobGP3StorageUtilization,
		Value:     utilization,
		Unit:      sharedtypes.UnitPercent,
		Metadata:  nil,
		Timestamp: time.Now(),
	}

	return []sharedtypes.CloudWatchMetric{metric}, nil
}

// GetJobName will return the name of the job
func (j *Gp3StorageJob) GetJobName() string {
	return j.jobName
}

// GetRegion will return the region
func (j *Gp3StorageJob) GetRegion() string {
	return j.region
}