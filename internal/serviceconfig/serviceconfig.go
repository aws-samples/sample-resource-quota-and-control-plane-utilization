// Package serviceconfig provides configuration management for AWS service
// quota metrics and rate limit APIs, including validation and loading from files.
package serviceconfig

import (
	"encoding/json"
	"fmt"
	"os"

	applogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// QuotaMetric represents an individual quota metric to be monitored
// for AWS service limits and usage tracking.
type QuotaMetric struct {
	Name string `json:"name"` // Name of the quota metric (e.g., "networkInterfaces")
}

// RateLimitAPIs represents an API endpoint to monitor for rate limiting.
// Used to track API call frequency and throttling behavior.
type RateLimitAPIs struct {
	Name string `json:"name"` // Name of the API to track (e.g., "assumeRole")
}

// ServiceConfig represents configuration for a specific AWS service.
// Services may have quota metrics, rate limit APIs, or both depending on monitoring needs.
type ServiceConfig struct {
	QuotaMetrics  []QuotaMetric   `json:"quotaMetrics,omitempty"`  // Quota metrics to monitor
	RateLimitAPIs []RateLimitAPIs `json:"rateLimitAPIs,omitempty"` // API endpoints to track for rate limiting
}

// TopLevelServiceConfig represents the complete configuration structure
// containing all services and regions to monitor.
type TopLevelServiceConfig struct {
	Services map[string]ServiceConfig `json:"services"` // Map of service name to its configuration
	Regions  []string                 `json:"regions"`  // List of AWS regions to monitor
}

// LoadConfigFromFile reads and parses a JSON configuration file.
// Returns the parsed configuration or an error if reading or parsing fails.
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

// Validation error variables for different AWS services.
var (
	// ErrInvalidEC2Metric indicates an unsupported EC2 quota metric.
	ErrInvalidEC2Metric = fmt.Errorf("invalid EC2 quota metric")
	// ErrInvalidEKSMetric indicates an unsupported EKS quota metric.
	ErrInvalidEKSMetric = fmt.Errorf("invalid EKS quota metric")
	// ErrInvalidIAMMetric indicates an unsupported IAM quota metric.
	ErrInvalidIAMMetric = fmt.Errorf("invalid IAM quota metric")
	// ErrInvalidEBSMetric indicates an unsupported EBS quota metric.
	ErrInvalidEBSMetric = fmt.Errorf("invalid EBS quota metric")
	// ErrInvalidVPCMetric indicates an unsupported VPC quota metric.
	ErrInvalidVPCMetric = fmt.Errorf("invalid VPC quota metric")
	// ErrInvalidSTSApi indicates an unsupported STS API for rate limiting.
	ErrInvalidSTSApi = fmt.Errorf("invalid STS api")
)

// ValidateEC2QuotaMetrics validates that all EC2 quota metrics are supported.
// Currently supports: networkInterfaces.
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

// ValidateEKSQuotaMetrics validates that all EKS quota metrics are supported.
// Currently supports: listClusters.
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

// ValidateIAMQuotaMetrics validates that all IAM quota metrics are supported.
// Currently supports: iamRoles, oidcProviders.
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

// ValidateEBSQuotaMetrics validates that all EBS quota metrics are supported.
// Currently supports: gp3storage.
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

// ValidateVPCQuotaMetrics validates that all VPC quota metrics are supported.
// Currently supports: nau (Network Address Usage).
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

// ValidateSTSRateLimitApis validates that all STS rate limit APIs are supported.
// Currently supports: assumeRole, assumeRoleWithWebIdentity.
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

// ValidateRateLimitConfig validates the rate limit configuration for all services.
// Returns an error if any service has invalid rate limit API configurations.
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

// ValidateQuotaMetricConfig validates the quota metric configuration for all services.
// Returns an error if any service has invalid quota metric configurations.
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
