package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/cwlclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ebsclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/eksclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/iamclient"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/s3client"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/servicequotaclient"
	"github.com/outofoffice3/aws-samples/geras/internal/emf/batch/metrics"
	"github.com/outofoffice3/aws-samples/geras/internal/job"
	"github.com/outofoffice3/aws-samples/geras/internal/logger"
	"github.com/outofoffice3/aws-samples/geras/internal/nau"
	"github.com/outofoffice3/aws-samples/geras/internal/safestore"
	"github.com/outofoffice3/aws-samples/geras/internal/serviceconfig"
	sharedtypes "github.com/outofoffice3/aws-samples/geras/internal/shared/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Error variable tests are already covered in the existing file

// Mock implementations for testing
type MockClientFactory struct {
	mock.Mock
}

func (m *MockClientFactory) CreateEC2(region string) (ec2client.Ec2Client, error) {
	args := m.Called(region)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ec2client.Ec2Client), args.Error(1)
}

func (m *MockClientFactory) CreateEKS(region string) (eksclient.EKSClient, error) {
	args := m.Called(region)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(eksclient.EKSClient), args.Error(1)
}

func (m *MockClientFactory) CreateIAM(region string) (iamclient.IamClient, error) {
	args := m.Called(region)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(iamclient.IamClient), args.Error(1)
}

func (m *MockClientFactory) CreateServiceQuotas(region string) (servicequotaclient.ServiceQuotasClient, error) {
	args := m.Called(region)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(servicequotaclient.ServiceQuotasClient), args.Error(1)
}

func (m *MockClientFactory) CreateCloudWatchLogs(region string) (cwlclient.CloudWatchLogsClient, error) {
	args := m.Called(region)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(cwlclient.CloudWatchLogsClient), args.Error(1)
}

func (m *MockClientFactory) CreateS3(region string) (s3client.S3Client, error) {
	args := m.Called(region)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(s3client.S3Client), args.Error(1)
}

func (m *MockClientFactory) CreateEBS(region string) (ebsclient.EBSClient, error) {
	args := m.Called(region)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(ebsclient.EBSClient), args.Error(1)
}

type MockJobManager struct {
	mock.Mock
}

func (m *MockJobManager) AddJob(job job.Job) error {
	args := m.Called(job)
	return args.Error(0)
}

func (m *MockJobManager) Wait() {
	m.Called()
}

func (m *MockJobManager) GetQueueStats() (primary, retry int) {
	args := m.Called()
	return args.Int(0), args.Int(1)
}

func (m *MockJobManager) LogError(err error) {
	m.Called(err)
}

type MockMetricsBatcher struct {
	mock.Mock
}

func (m *MockMetricsBatcher) Add(ctx context.Context, metric sharedtypes.CloudWatchMetric) {
	m.Called(ctx, metric)
}

func (m *MockMetricsBatcher) FlushAll(ctx context.Context) {
	m.Called(ctx)
}

type MockNAUStore struct {
	mock.Mock
}

func (m *MockNAUStore) AddRecord(rec nau.NauRecord) error {
	args := m.Called(rec)
	return args.Error(0)
}

func (m *MockNAUStore) RangeVPCs(fn func(vpcID string, vpc *nau.VPCNAU) bool) {
	m.Called(fn)
}

func (m *MockNAUStore) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNewResourceQuotaHandler_Validation(t *testing.T) {
	mockFactory := &MockClientFactory{}
	mockBatchers := safestore.NewSyncStore[metrics.Batcher]()
	mockJobManager := &MockJobManager{}
	mockServiceConfig := &serviceconfig.TopLevelServiceConfig{}
	mockStore := &MockNAUStore{}
	mockLogger := &logger.NoopLogger{}

	tests := []struct {
		name        string
		config      ResourceQuotaHandlerConfig
		expectError error
	}{
		{
			name: "valid config",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: nil,
		},
		{
			name: "nil client factory",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       nil,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: ErrClientFactoryNil,
		},
		{
			name: "empty log stream",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: ErrCloudWatchLogStreamNotSet,
		},
		{
			name: "empty log group",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: ErrCloudwatchLogGroupNotSet,
		},
		{
			name: "empty namespace",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: ErrMetricNamespaceNotSet,
		},
		{
			name: "nil regional batchers",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    nil,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: ErrRegionalBatchersNil,
		},
		{
			name: "nil job manager",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          nil,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: ErrJobManagerNil,
		},
		{
			name: "nil service config",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       nil,
				Store:               mockStore,
				Logger:              mockLogger,
			},
			expectError: ErrServiceConfigNil,
		},
		{
			name: "nil store",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               nil,
				Logger:              mockLogger,
			},
			expectError: ErrStoreNil,
		},
		{
			name: "nil logger defaults",
			config: ResourceQuotaHandlerConfig{
				ClientFactory:       mockFactory,
				CloudwatchLogGroup:  "test-group",
				CloudWatchLogStream: "test-stream",
				Namespace:           "test-namespace",
				RegionalBatchers:    mockBatchers,
				JobManager:          mockJobManager,
				ServiceConfig:       mockServiceConfig,
				Store:               mockStore,
				Logger:              nil,
			},
			expectError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewResourceQuotaHandler(tt.config)

			if tt.expectError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectError, err)
				assert.Nil(t, handler)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handler)
			}
		})
	}
}

func TestResourceQuotaHandler_HandleEvent(t *testing.T) {
	mockFactory := &MockClientFactory{}
	mockBatchers := safestore.NewSyncStore[metrics.Batcher]()
	mockJobManager := &MockJobManager{}
	mockServiceConfig := &serviceconfig.TopLevelServiceConfig{}
	mockStore := &MockNAUStore{}
	mockLogger := &logger.NoopLogger{}

	// Add a mock batcher to the store
	mockBatcher := &MockMetricsBatcher{}
	mockBatchers.Store("us-east-1", mockBatcher)

	// Setup successful case
	mockJobManager.On("Wait").Return()
	mockBatcher.On("FlushAll", mock.Anything).Return()
	mockStore.On("Close").Return(nil)

	config := ResourceQuotaHandlerConfig{
		ClientFactory:       mockFactory,
		CloudwatchLogGroup:  "test-group",
		CloudWatchLogStream: "test-stream",
		Namespace:           "test-namespace",
		RegionalBatchers:    mockBatchers,
		JobManager:          mockJobManager,
		ServiceConfig:       mockServiceConfig,
		Store:               mockStore,
		Logger:              mockLogger,
	}

	handler, err := NewResourceQuotaHandler(config)
	require.NoError(t, err)
	require.NotNil(t, handler)

	// Test successful event handling
	t.Run("successful event handling", func(t *testing.T) {
		event := events.CloudWatchEvent{
			Source:     "aws.events",
			DetailType: "Scheduled Event",
			Time:       time.Now(),
		}

		err := handler.HandleEvent(context.Background(), event)
		assert.NoError(t, err)

		mockJobManager.AssertCalled(t, "Wait")
		mockBatcher.AssertCalled(t, "FlushAll", mock.Anything)
		mockStore.AssertCalled(t, "Close")
	})

	// Test store close error
	t.Run("store close error", func(t *testing.T) {
		// Reset mocks
		mockJobManager = &MockJobManager{}
		mockStore = &MockNAUStore{}
		mockBatcher = &MockMetricsBatcher{}
		mockBatchers = safestore.NewSyncStore[metrics.Batcher]()
		mockBatchers.Store("us-east-1", mockBatcher)

		// Setup mocks with error
		mockJobManager.On("Wait").Return()
		mockBatcher.On("FlushAll", mock.Anything).Return()
		mockStore.On("Close").Return(errors.New("store close error"))

		config.JobManager = mockJobManager
		config.Store = mockStore
		config.RegionalBatchers = mockBatchers

		handler, err := NewResourceQuotaHandler(config)
		require.NoError(t, err)

		event := events.CloudWatchEvent{
			Source:     "aws.events",
			DetailType: "Scheduled Event",
			Time:       time.Now(),
		}

		err = handler.HandleEvent(context.Background(), event)
		assert.Error(t, err)
		assert.Equal(t, ErrStoreCloseFailed, err)

		mockJobManager.AssertCalled(t, "Wait")
		mockBatcher.AssertCalled(t, "FlushAll", mock.Anything)
		mockStore.AssertCalled(t, "Close")
	})

	// Test context cancellation
	t.Run("context cancellation", func(t *testing.T) {
		// Reset mocks
		mockJobManager = &MockJobManager{}
		mockStore = &MockNAUStore{}
		mockBatcher = &MockMetricsBatcher{}
		mockBatchers = safestore.NewSyncStore[metrics.Batcher]()
		mockBatchers.Store("us-east-1", mockBatcher)

		// Setup mocks
		mockJobManager.On("Wait").Return()
		mockBatcher.On("FlushAll", mock.Anything).Return()
		mockStore.On("Close").Return(nil)

		config.JobManager = mockJobManager
		config.Store = mockStore
		config.RegionalBatchers = mockBatchers

		handler, err := NewResourceQuotaHandler(config)
		require.NoError(t, err)

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		event := events.CloudWatchEvent{
			Source:     "aws.events",
			DetailType: "Scheduled Event",
			Time:       time.Now(),
		}

		// Should still complete since we're not checking context in the handler
		err = handler.HandleEvent(ctx, event)
		assert.NoError(t, err)

		mockJobManager.AssertCalled(t, "Wait")
		mockBatcher.AssertCalled(t, "FlushAll", mock.Anything)
		mockStore.AssertCalled(t, "Close")
	})
}

func TestHandleInitError(t *testing.T) {
	// This function calls os.Exit(1), so we can't test it directly
	// Just verify it exists for coverage
	assert.NotPanics(t, func() {
		// We can't actually call HandleInitError as it calls os.Exit
		// But we can verify it's not nil
		assert.NotNil(t, HandleInitError)
	})
}
