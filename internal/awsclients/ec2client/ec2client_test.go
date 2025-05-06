package ec2client_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/outofoffice3/aws-samples/geras/internal/awsclients/ec2client"
	"github.com/stretchr/testify/assert"
)

// Test NewEc2Client
func TestNewEc2Client_Success(t *testing.T) {
	// Create a new Ec2Client successfully
	ec2Client, err := ec2client.NewEc2Client(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "should not return error creating ec2 client")
	assert.NotNil(t, ec2Client, "ec2 client should not be nil")
	assert.IsType(t, &ec2client.Ec2ClientImpl{}, ec2Client, "ec2 client should be of type Ec2Client")
}

// Test NewEc2Client with invalid region
func TestNewEc2Client_InvalidRegion(t *testing.T) {
	// Create a new Ec2Client with an invalid region
	ec2Client, err := ec2client.NewEc2Client(aws.Config{}, "invalid-region")
	assert.Error(t, err, "should return error creating ec2 client")
	assert.Nil(t, ec2Client, "ec2 client should be nil")
}

// TestDescribeNetworkInterfaces_Success
func TestDescribeNetworkInterfaces_Success(t *testing.T) {
	// Create a new Ec2Client
	ec2Client, err := ec2client.NewEc2Client(aws.Config{}, "us-east-1")
	assert.NoError(t, err, "should not return error creating ec2 client")

	// Describe network interfaces successfully
	interfaces, err := ec2Client.DescribeNetworkInterfaces(context.Background(), &ec2.DescribeNetworkInterfacesInput{})
	assert.Error(t, err, "should return error describing network interfaces")
	assert.IsType(t, &ec2.DescribeNetworkInterfacesOutput{}, interfaces, "interfaces should be of type DescribeNetworkInterfacesOutput")
}
