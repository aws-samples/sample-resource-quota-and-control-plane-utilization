package utils

import (
	"fmt"
	"os"
	"time"
)

// validRegions is a set of valid AWS regions.
var validRegions = map[string]struct{}{
	"us-east-1":      {},
	"us-east-2":      {},
	"us-west-1":      {},
	"us-west-2":      {},
	"af-south-1":     {},
	"ap-east-1":      {},
	"ap-south-1":     {},
	"ap-northeast-1": {},
	"ap-northeast-2": {},
	"ap-northeast-3": {},
	"ap-southeast-1": {},
	"ap-southeast-2": {},
	"ca-central-1":   {},
	"eu-central-1":   {},
	"eu-west-1":      {},
	"eu-west-2":      {},
	"eu-west-3":      {},
	"eu-north-1":     {},
	"me-south-1":     {},
	"sa-east-1":      {},
}

// IsValidRegion returns true if the provided region string is a valid AWS region.
func IsValidRegion(region string) bool {
	_, ok := validRegions[region]
	return ok
}

const (
	LogStreamTimeLayout = "20060102-150405" // YYYYMMDD-HHMMSS
)

func MakeStreamName() string {
	host, _ := os.Hostname()                           // get hostname
	ts := time.Now().UTC().Format(LogStreamTimeLayout) // format current time
	return fmt.Sprintf("%s-%s", ts, host)
}
