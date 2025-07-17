package listcluster

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/servicequotas"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/eksclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
)

// Error constants for EKS list clusters job
var (
	ErrListClusters     = errors.New("error listing EKS clusters")
	ErrGetClustersQuota = errors.New("error getting EKS clusters quota")
)

// ListClusterJob will implment the Job interface
// It will calculate the total number of clusters in a given region
type ListClusterJob struct {
	EksClient           eksclient.EKSClient
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient
	jobName             string
	region              string
	Logger              logger.Logger
}

type ListClusterJobConfig struct {
	EksClient           eksclient.EKSClient
	ServiceQuotasClient servicequotaclient.ServiceQuotasClient
	Logger              logger.Logger
}

const (
	quotaCode            = "L-1194D53C"
	ServiceCode          = "eks"
)

func NewListClusterJob(config ListClusterJobConfig) (job.Job, error) {
	if config.Logger == nil {
		config.Logger = &logger.NoopLogger{}
	}
	job := &ListClusterJob{
		EksClient:           config.EksClient,
		ServiceQuotasClient: config.ServiceQuotasClient,
		jobName:             string(sharedtypes.JobEKSClusterUtilization) + "-" + config.EksClient.GetRegion(),
		region:              config.EksClient.GetRegion(),
		Logger:              config.Logger,
	}

	return job, nil
}

func (lj *ListClusterJob) Execute(ctx context.Context) ([]sharedtypes.CloudWatchMetric, error) {
	input := &eks.ListClustersInput{}
	var totalCount int64 = 0

	// use aws sdk paginator to retrieve all eks clusters
	paginator := eks.NewListClustersPaginator(lj.EksClient, input)
	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			lj.Logger.Error("%s failed to list EKS clusters: %v", lj.GetJobName(), err)
			return nil, fmt.Errorf("%w: %v", ErrListClusters, err)
		}
		totalCount += int64(len(output.Clusters))
	}

	// call service quota api to get quota limit for eks clusters
	getServiceQuotaInput := &servicequotas.GetServiceQuotaInput{
		QuotaCode:   aws.String(quotaCode),
		ServiceCode: aws.String(ServiceCode),
	}

	getServiceQuotaOutput, err := lj.ServiceQuotasClient.GetServiceQuota(ctx, getServiceQuotaInput)
	if err != nil {
		lj.Logger.Error("%s failed to get EKS clusters quota: %v", lj.GetJobName(), err)
		return nil, fmt.Errorf("%w: %v", ErrGetClustersQuota, err)
	}
	quotaValue := aws.ToFloat64(getServiceQuotaOutput.Quota.Value)
	utilization := (float64(totalCount) / quotaValue) * 100
	percent := strconv.FormatFloat(utilization, 'f', -1, 64)
	lj.Logger.Debug("%s total=%d , quota=%.2f, utilization=%q%%", lj.GetJobName(), totalCount, quotaValue, percent)
	
	metric := sharedtypes.CloudWatchMetric{
		Name:      sharedtypes.JobEKSClusterUtilization,
		Value:     utilization,
		Unit:      sharedtypes.UnitPercent,
		Metadata:  nil,
		Timestamp: time.Now(),
	}
	return []sharedtypes.CloudWatchMetric{metric}, nil
}

// GetJobName return the name of the job
func (lj *ListClusterJob) GetJobName() string {
	return lj.jobName
}

// GetRegion return the region of the job
func (lj *ListClusterJob) GetRegion() string {
	return lj.region
}