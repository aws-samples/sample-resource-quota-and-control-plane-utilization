package types

import (
	"errors"
	"fmt"
)

// Error constants for JobName validation
var (
	ErrInvalidJobName = errors.New("invalid job name")
)

// JobName represents a strongly-typed identifier for resource quota job metrics.
// This ensures consistent naming across the application and provides type safety.
type JobName string

// Job name constants for all resource quota jobs
const (
	// EC2 network interfaces utilization metric
	JobNetworkInterfaceUtilization JobName = "NetworkInterfaceUtilization"
	
	// EBS GP3 storage utilization metric
	JobGP3StorageUtilization JobName = "GP3StorageUtilization"
	
	// IAM roles utilization metric
	JobIAMRoleUtilization JobName = "IAMRoleUtilization"
	
	// IAM OIDC providers utilization metric
	JobOIDCProviderUtilization JobName = "OIDCProviderUtilization"
	
	// EKS clusters utilization metric
	JobEKSClusterUtilization JobName = "EKSClusterUtilization"
	
	// VPC Network Address Units utilization metric
	JobNetworkAddressUnitsUtilization JobName = "NetworkAddressUnitsUtilization"
)

// String implements the fmt.Stringer interface for JobName
func (j JobName) String() string {
	return string(j)
}

// Validate checks if the job name is valid
func (j JobName) Validate() error {
	switch j {
	case JobNetworkInterfaceUtilization,
		JobGP3StorageUtilization,
		JobIAMRoleUtilization,
		JobOIDCProviderUtilization,
		JobEKSClusterUtilization,
		JobNetworkAddressUnitsUtilization:
		return nil
	}
	return fmt.Errorf("%w: %s", ErrInvalidJobName, j)
}