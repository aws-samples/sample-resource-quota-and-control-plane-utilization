# Resource Quota Monitoring Solution

1. [Overview](#overview)
2. [Configuration](#configuration)
3. [Deployment](#deployment)
4. [Created Resources](#created-resources)
5. [Viewinng Metrics](#viewing-metrics)
6. [Testing / Code Coverage](#testing--code-coverage)

![Architecture Diagram](../../media/resource-quota-solution.png)

## Overview 

The Resource Quota Solution does the following : 
- captures total counts of various resources specific via your config file
- gets the total allocation from Service Quotas api 
- produces utilization % and send metric to cloudwatch logs via EMF (Embedded Metric Format)

## Configuration 

This solution requires the following envionment variables

| Name             | Description                                                                   | Default |
|------------------|-------------------------------------------------------------------------------|---------|
| LOG_LEVEL        | Log verbosity (DEBUG, INFO, WARN, ERROR)                                      | INFO |
| **LOG_GROUP_NAME   | CloudWatch Logs group name for EMF output                                     |  /lambda/resourcequota/emf  |
| **METRIC_NAMESPACE | CloudWatch Metric Namespace                                                   |  Resource Quota Utilization |
| LAMBDA_LAYER_PATH | path to the location of the config.json file in the lambda layer.  If you made any changes to the lambda layer. You need to make sure to update this variable accordingly. | /opt/config/config.json |

## Deployment 

### Prerequisites

- AWS CLI v2  
- AWS SAM CLI (latest)  
- Go v1.22.1 or higher (for local development)

### Build & Deploy

1. Clone the repository 
```bash
git clone https://github.com/aws-samples/sample-resource-quota-and-control-plane-utilization
```

We need to make sure the lambda layer is uploaded to S3 prior to deployment, so that when we deploy the cloudformation template, it will pull the layer.zip from s3. 

If you plan to use the sample layer we have provided. Navigate to the directory
```bash 
cd lambda-layer
zip -r lambda-layer.zip . # This will produce a lambda-layer.zip file which contians the lambda layer 

# Our sample layer has the directory structure 
config/ 
    config.json # This is the file lambda will read to initialize 
```
Our sample config.json will look like what is shown below.  The structure of the config file a map of service names (ec2, ebs, iam etc).  Each map will contain an array of `quotaMetrics`.  This tells the solution will metrics it needs to capture.  These map to the metric we currently have coverage for.  As more metrics are added to the solution, we will update the config file accordingly. 

```json 
{
  "services": {
    "ec2": {
      "quotaMetrics": [
        {
          "name": "networkInterfaces"
        }
      ]
    },
    "ebs" : { 
      "quotaMetrics" : [
        {
          "name": "gp3storage"
        }
      ]
    },
    "iam": {
      "quotaMetrics": [
        {
          "name": "oidcProviders"          
        },
        {
          "name": "iamRoles"
        }
      ]
    },
    "vpc" :{ 
      "quotaMetrics" : [
        { 
          "name": "nau"
        }
      ]
    },
    "eks" : { 
      "quotaMetrics" : [
        {
          "name": "listClusters"
        }
      ]
    },
    "sts": {
      "rateLimitAPIs": [
        {
          "name": "assumeRole"
        },
        {
          "name": "assumeRoleWithWebIdentity"
        }
      ]
    }
  },
  "regions": [
    "us-east-1",
    "us-west-2"
  ]
}
```

Finall you need to uploaded the resulting lambda-layer.zip file to an s3 bucket and keep track of the full URI as we will need to add it to the cloudformation template. 

2. Navigate to the Rate Limit infrastructure folder.  Ensure there is a template.yaml file located in that directory. 
```bash 
root-dir/
        infra/
            ratelimit/
                    template.yaml
```

In the template.yaml you need to ensure you input your s3 bucket and key for the lambda layer : 

```yaml 
# Lambda Layer that stores the configuration for the solution
  ConfigFileLambdaLayer:
    Type: AWS::Lambda::LayerVersion
    Properties:
      LayerName: resource-quota-config
      Description: Configuration for resource quota utilization solution 
      Content:
        S3Bucket: custom-monitoring-poc ## Overwrite with your s3 bucket name 
        S3Key: layers/layer.zip # Overwrite with your object key
      CompatibleArchitectures:
        - arm64
      CompatibleRuntimes:
        - provided.al2023

```
Save your changes 

3. From that directory will run the commands below to build and deploy the application. 

```bash
sam build
sam deploy --guided
```

>Tip: Use sam deploy --guided on your first deployment to set a stack name and parameters.

#### What if my stack creation fails? 
If your stack creation fails, due to the nature of cloudformation, you will have to delete the stack before you can deploy it under the same name. 

```bash
# Deleting cloudformation stack 
aws cloudformation delete-stack --stack-name ### YOUR STACK NAME HERE

# Wait for cloudformation to finish delete (optional)
aws cloudformation wait stack-delete-complete --stack-name ### YOUR STACK NAME HERE
```
Once cloudformation has successfully deleted the stack, you may deploy your changes using the sam build and sam deploy referenced earlier.

## Created Resources

## Viewing Metrics

## Testing / Code Coverage 
