package serviceconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/s3client"
	applogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// QuotaMetric reprsents an individual metric entity used for both quota and rate limits
type QuotaMetric struct {
	Name string `json:"name"`
}

// RateLimitAPIs represent the api name that you would like to track
type RateLimitAPIs struct {
	Name string `json:"name"`
}

// ServiceConfig represents the service configuration (iam, ec2, eks etc..)
// Some service might only have quota limits metrics or rate limit metrics
type ServiceConfig struct {
	QuotaMetrics  []QuotaMetric   `json:"quotaMetrics,omitempty"`
	RateLimitAPIs []RateLimitAPIs `json:"rateLimitAPIs,omitempty"`
}

// TopLevelServiceConfig represents the top level configuration structure
type TopLevelServiceConfig struct {
	Services map[string]ServiceConfig `json:"services"`
	Regions  []string                 `json:"regions"`
}

// LoadConfig reads the configuration file at the given file path and unmarshals
// it into a Config struct
func LoadConfigFromFile(filePath string, logger applogger.Logger) (*TopLevelServiceConfig, error) {
	if logger == nil {
		logger = &applogger.NoopLogger{}
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("failed to read config file %q: %w", filePath, err)
		return nil, fmt.Errorf("failed to read config file %q: %w", filePath, err)
	}

	var cfg TopLevelServiceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		logger.Error("failed to unmarshal config file %q: %w", filePath, err)
		return nil, fmt.Errorf("failed to unmarshal config file %q: %w", filePath, err)
	}
	return &cfg, nil
}

// LoadConfig reads the cofiguration file from S3 given the full key
// unmarshals into a Config struct
func LoadConfigFromS3(bucket, key string, client s3client.S3Client) (*TopLevelServiceConfig, error) {
	data, err := client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", key, err)
	}

	dataBytes, err := io.ReadAll(data.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", key, err)
	}

	var cfg TopLevelServiceConfig
	if err := json.Unmarshal(dataBytes, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file %q: %w", key, err)
	}
	return &cfg, nil
}

// Validation Errors
var (
	ErrInvalidEC2Metric = fmt.Errorf("invalid EC2 quota metric")
	ErrInvalidEKSMetric = fmt.Errorf("invalid EKS quota metric")
	ErrInvalidIAMMetric = fmt.Errorf("invalid IAM quota metric")
	ErrInvalidEBSMetric = fmt.Errorf("invalid EBS quota metric")
	ErrInvalidVPCMetric = fmt.Errorf("invalid VPC quota metric")
	ErrInvalidSTSApi    = fmt.Errorf("invalid STS api")
)

func ValidateEC2QuotaMetrics(service ServiceConfig) error {
	validMetrics := map[string]struct{}{
		"networkInterfaces": {},
	}
	for _, metric := range service.QuotaMetrics {
		if _, ok := validMetrics[metric.Name]; !ok {
			return fmt.Errorf("%w: %s", ErrInvalidEC2Metric, metric.Name)
		}
	}
	return nil
}

func ValidateEKSQuotaMetrics(service ServiceConfig) error {
	validAPIs := map[string]struct{}{
		"listClusters": {},
	}
	for _, api := range service.QuotaMetrics {
		if _, ok := validAPIs[api.Name]; !ok {
			return fmt.Errorf("%w: %s", ErrInvalidEKSMetric, api.Name)
		}
	}
	return nil
}

func ValidateIAMQuotaMetrics(service ServiceConfig) error {
	validMetrics := map[string]struct{}{
		"iamRoles":      {},
		"oidcProviders": {},
	}
	for _, metric := range service.QuotaMetrics {
		if _, ok := validMetrics[metric.Name]; !ok {
			return fmt.Errorf("%w: %s", ErrInvalidIAMMetric, metric.Name)
		}
	}
	return nil
}

func ValidateEBSQuotaMetrics(service ServiceConfig) error {
	validMetrics := map[string]struct{}{
		"gp3storage": {},
	}
	for _, metric := range service.QuotaMetrics {
		if _, ok := validMetrics[metric.Name]; !ok {
			return fmt.Errorf("%w: %s", ErrInvalidEBSMetric, metric.Name)
		}
	}
	return nil
}

func ValidateVPCQuotaMetrics(service ServiceConfig) error {
	validMetrics := map[string]struct{}{
		"nau": {},
	}
	for _, metric := range service.QuotaMetrics {
		if _, ok := validMetrics[metric.Name]; !ok {
			return fmt.Errorf("%w: %s", ErrInvalidVPCMetric, metric.Name)
		}
	}
	return nil
}

func ValidateSTSRateLimitApis(service ServiceConfig) error {
	validRateLimitApis := map[string]struct{}{
		"assumeRole":                {},
		"assumeRoleWithWebIdentity": {},
	}

	for _, metric := range service.RateLimitAPIs {
		if _, ok := validRateLimitApis[metric.Name]; !ok {
			return fmt.Errorf("%w: %s", ErrInvalidSTSApi, metric.Name)
		}
	}
	return nil
}

// Validates the Rate Limit Config
func ValidateRateLimitConfig(cfg TopLevelServiceConfig, logger applogger.Logger) error {
	if logger == nil {
		logger = &applogger.NoopLogger{}
	}
	for serviceName, serviceCfg := range cfg.Services {
		switch serviceName {
		case "sts":
			logger.Info("validating sts rate limit config")
			if err := ValidateSTSRateLimitApis(serviceCfg); err != nil {
				logger.Error("invalid sts rate limit config : %v", err)
				return err
			}
		default:
			logger.Warn("no rate limit config for service %s", serviceName)
		}
	}
	logger.Info("rate limit config validated")
	return nil
}

func ValidateQuotaMetricConfig(cfg TopLevelServiceConfig, logger applogger.Logger) error {
	if logger == nil {
		logger = &applogger.NoopLogger{}
	}
	for serviceName, serviceCfg := range cfg.Services {
		switch serviceName {
		case "ec2":
			if err := ValidateEC2QuotaMetrics(serviceCfg); err != nil {
				logger.Error("invalid ec2 quota config : %v", err)
				return err
			}
		case "eks":
			if err := ValidateEKSQuotaMetrics(serviceCfg); err != nil {
				logger.Error("invalid eks quota config : %v", err)
				return err
			}
		case "iam":
			if err := ValidateIAMQuotaMetrics(serviceCfg); err != nil {
				logger.Error("invalid iam quota config : %v", err)
				return err
			}
		case "ebs":
			if err := ValidateEBSQuotaMetrics(serviceCfg); err != nil {
				logger.Error("invalid ebs quota config : %v", err)
				return err
			}
		case "vpc":
			if err := ValidateVPCQuotaMetrics(serviceCfg); err != nil {
				logger.Error("invalid vpc quota config : %v", err)
				return err
			}
		default:
			logger.Warn("no quota config for service %s", serviceName)
		}
	}
	logger.Debug("quota metric config validated")
	return nil
}
