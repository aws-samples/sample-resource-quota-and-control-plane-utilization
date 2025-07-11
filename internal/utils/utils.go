// Package utils provides utility functions for AWS region validation,
// log stream name generation, and ARN parsing operations.
package utils

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// AwsRegion is a string alias for AWS region codes that provides
// type safety for region validation and operations.
type AwsRegion string

// String returns the string representation of the AWS region.
func (r AwsRegion) String() string { return string(r) }

// Supported AWS regions as typed constants.
// These represent the complete set of AWS regions supported by this application.
const (
	AwsRegionUSEast1      AwsRegion = "us-east-1"
	AwsRegionUSEast2      AwsRegion = "us-east-2"
	AwsRegionUSWest1      AwsRegion = "us-west-1"
	AwsRegionUSWest2      AwsRegion = "us-west-2"
	AwsRegionAFSouth1     AwsRegion = "af-south-1"
	AwsRegionAPEast1      AwsRegion = "ap-east-1"
	AwsRegionAPSouth1     AwsRegion = "ap-south-1"
	AwsRegionAPNortheast1 AwsRegion = "ap-northeast-1"
	AwsRegionAPNortheast2 AwsRegion = "ap-northeast-2"
	AwsRegionAPNortheast3 AwsRegion = "ap-northeast-3"
	AwsRegionAPSoutheast1 AwsRegion = "ap-southeast-1"
	AwsRegionAPSoutheast2 AwsRegion = "ap-southeast-2"
	AwsRegionCACentral1   AwsRegion = "ca-central-1"
	AwsRegionEUCentral1   AwsRegion = "eu-central-1"
	AwsRegionEUWest1      AwsRegion = "eu-west-1"
	AwsRegionEUWest2      AwsRegion = "eu-west-2"
	AwsRegionEUWest3      AwsRegion = "eu-west-3"
	AwsRegionEUNorth1     AwsRegion = "eu-north-1"
	AwsRegionMESouth1     AwsRegion = "me-south-1"
	AwsRegionSAEast1      AwsRegion = "sa-east-1"
)

// validRegions is a lookup map containing all supported AWS regions
// for efficient validation operations.
var validRegions = map[AwsRegion]struct{}{
	AwsRegionUSEast1:      {},
	AwsRegionUSEast2:      {},
	AwsRegionUSWest1:      {},
	AwsRegionUSWest2:      {},
	AwsRegionAFSouth1:     {},
	AwsRegionAPEast1:      {},
	AwsRegionAPSouth1:     {},
	AwsRegionAPNortheast1: {},
	AwsRegionAPNortheast2: {},
	AwsRegionAPNortheast3: {},
	AwsRegionAPSoutheast1: {},
	AwsRegionAPSoutheast2: {},
	AwsRegionCACentral1:   {},
	AwsRegionEUCentral1:   {},
	AwsRegionEUWest1:      {},
	AwsRegionEUWest2:      {},
	AwsRegionEUWest3:      {},
	AwsRegionEUNorth1:     {},
	AwsRegionMESouth1:     {},
	AwsRegionSAEast1:      {},
}

// IsValidRegion validates whether the provided region string
// matches one of the supported AWS regions.
// Returns true if the region is supported, false otherwise.
func IsValidRegion(region string) bool {
	_, ok := validRegions[AwsRegion(region)]
	return ok
}

// ParseAwsRegion attempts to cast & validate a string into AwsRegion.
// Returns an error if the string isn’t in your known list.
func ParseAwsRegion(region string) (AwsRegion, error) {
	r := AwsRegion(region)
	if _, ok := validRegions[r]; !ok {
		return "", fmt.Errorf("invalid AWS region: %q", region)
	}
	return r, nil
}

// Time layout constants for log stream naming.
const (
	// LogStreamTimeLayout defines the timestamp format used in log stream names.
	LogStreamTimeLayout = "2006/01/02/15/04/05.000"
)

// MakeStreamName generates a log stream name for CloudWatch logs.
// If running in AWS Lambda, it uses the Lambda log stream name from environment.
// Otherwise, it creates a name using current timestamp and hostname.
func MakeStreamName() string {
	if name := os.Getenv("AWS_LAMBDA_LOG_STREAM_NAME"); name != "" {
		return name
	}
	host, _ := os.Hostname()
	ts := time.Now().UTC().Format(LogStreamTimeLayout)
	return fmt.Sprintf("%s-%s", ts, host)
}

var (
	// logInjectionPattern matches characters that could be used for log injection
	logInjectionPattern = regexp.MustCompile(`[\r\n\t\x00-\x1f\x7f-\x9f]`)
)

// SanitizeLogString sanitizes input strings to prevent log injection attacks.
// It removes or replaces control characters, newlines, and other potentially
// dangerous characters that could be used to manipulate log output.
func SanitizeLogString(input string) string {
	return logInjectionPattern.ReplaceAllString(input, "_")
}

// ExtractRoleNameFromArn parses an AWS ARN and extracts the role name.
// Supports both "role" and "assumed-role" resource types.
// Returns the role name if successful, or an error if the ARN is invalid
// or doesn't represent a role resource.
func ExtractRoleNameFromArn(arn string) (string, error) {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) != 6 {
		return "", fmt.Errorf("%w, %s", ErrInvalidArn, arn)
	}
	resource := parts[5]
	fields := strings.Split(resource, "/")
	if len(fields) < 2 {
		return "", fmt.Errorf("%w, %s", ErrUnexpectedResourceFormat, arn)
	}

	switch fields[0] {
	case "role", "assumed-role":
		return fields[1], nil
	default:
		return "", fmt.Errorf("%w, %s", ErrArnResourceIsNotARole, arn)
	}
}
