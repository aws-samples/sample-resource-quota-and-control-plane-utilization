// Package serviceconfig provides configuration management for AWS service
// quota metrics and rate limit APIs, including validation and loading from files.
package serviceconfig

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/outofoffice3/aws-samples/geras/internal/constants"
	applogger "github.com/outofoffice3/aws-samples/geras/internal/logger"
)

// QuotaMetric represents an individual quota metric to be monitored
// for AWS service limits and usage tracking.
type QuotaMetric struct {
	Name string `json:"name"` // Name of the quota metric (e.g., "networkInterfaces")
}

// ServiceConfig represents configuration for a specific AWS service.
// Services may have quota metrics, rate limit APIs, or both depending on monitoring needs.
type ServiceConfig struct {
	QuotaMetrics []QuotaMetric `json:"quotaMetrics,omitempty"` // Quota metrics to monitor
}

// TopLevelServiceConfig represents the complete configuration structure
// containing all services and regions to monitor.
type TopLevelServiceConfig struct {
	Services map[string]ServiceConfig `json:"services"` // Map of service name to its configuration
}

// LoadConfigFromFile reads and parses a JSON configuration file.
// Returns the parsed configuration or an error if reading or parsing fails.
func LoadConfigFromFile(filePath string, logger applogger.Logger) (*TopLevelServiceConfig, error) {
	if logger == nil {
		logger = applogger.Get()
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

// Error variables for validation
var (
	// ErrInvalidQuotaMetric indicates an unsupported quota metric for a service
	ErrInvalidQuotaMetric = fmt.Errorf("invalid quota metric")
	
	// ErrUnsupportedService indicates an unknown or unsupported service
	ErrUnsupportedService = fmt.Errorf("unsupported service")
)

// ValidateQuotaMetricConfig validates the quota metric configuration for all services.
// Returns an error if any service has invalid quota metric configurations or if an unknown service is configured.
func ValidateQuotaMetricConfig(cfg TopLevelServiceConfig, logger applogger.Logger) error {
	if logger == nil {
		logger = applogger.Get()
	}

	for serviceName, serviceCfg := range cfg.Services {
		// Check if service is supported
		if _, serviceExists := constants.ServiceJobMap[serviceName]; !serviceExists {
			logger.Error("unsupported service configured: %s", serviceName)
			return fmt.Errorf("%w: %s", ErrUnsupportedService, serviceName)
		}

		// Check if jobs are supported for this service
		for _, qm := range serviceCfg.QuotaMetrics {
			if !constants.IsValidServiceJob(serviceName, qm.Name) {
				logger.Error("invalid quota metric %q for service %q", qm.Name, serviceName)
				return fmt.Errorf("%w: %s for service %s", ErrInvalidQuotaMetric, qm.Name, serviceName)
			}
		}
	}

	logger.Debug("quota metric config validated")
	return nil
}
