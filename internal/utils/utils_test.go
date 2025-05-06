package utils_test

import (
	"testing"

	"github.com/outofoffice3/aws-samples/geras/internal/utils"
	"github.com/stretchr/testify/assert"
)

func TestIsValidRegion_ValidRegions(t *testing.T) {
	// List all the regions we defined as valid.
	valid := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2", "af-south-1", "ap-east-1",
		"ap-south-1", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-southeast-1",
		"ap-southeast-2", "ca-central-1", "eu-central-1", "eu-west-1", "eu-west-2",
		"eu-west-3", "eu-north-1", "me-south-1", "sa-east-1",
	}
	for _, region := range valid {
		assert.True(t, utils.IsValidRegion(region), "expected %s to be valid", region)
	}
}

func TestIsValidRegion_InvalidRegions(t *testing.T) {
	// Define some values that are not valid regions.
	invalid := []string{
		"us-east", "us-west", "eu-central", "invalid-region", "", "US-EAST-1", "ap-northeast", "ca-east-1",
	}
	for _, region := range invalid {
		assert.False(t, utils.IsValidRegion(region), "expected %s to be invalid", region)
	}
}
