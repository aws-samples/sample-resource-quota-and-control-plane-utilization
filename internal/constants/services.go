package constants

import "strings"

// Service names used in configuration and job creation
const (
	ServiceEC2 = "ec2"
	ServiceEBS = "ebs"
	ServiceIAM = "iam"
	ServiceVPC = "vpc"
	ServiceEKS = "eks"
)

// Job names used in configuration and job creation
const (
	JobNetworkInterfaces = "networkInterfaces"
	JobGP3Storage        = "gp3Storage"
	JobOIDCProviders     = "oidcProviders"
	JobIAMRoles          = "iamRoles"
	JobNAU               = "nau"
	JobListClusters      = "listClusters"
)

// ServiceJobMap maps services to their supported jobs
var ServiceJobMap = map[string][]string{
	ServiceEC2: {JobNetworkInterfaces},
	ServiceEBS: {JobGP3Storage},
	ServiceIAM: {JobOIDCProviders, JobIAMRoles},
	ServiceVPC: {JobNAU},
	ServiceEKS: {JobListClusters},
}

// IsValidServiceJob checks if a job is valid for a given service
func IsValidServiceJob(service, job string) bool {
	jobs, exists := ServiceJobMap[service]
	if !exists {
		return false
	}
	
	for _, validJob := range jobs {
		if strings.EqualFold(validJob, job) {
			return true
		}
	}
	return false
}