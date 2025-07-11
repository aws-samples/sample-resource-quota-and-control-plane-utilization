# Resource Quota Monitoring Solution

1. [Overview](#overview)
2. [Deployment Guide](#deployment-guide)
    - [Environment Variables](#environment-variables)
    - [Deploying w/ AWS SAM / Cloudformation](#deploying-with-aws-sam--cloudformation)
    - [Deploying w/ Terraform](#deploying-with-terraform)
3. [Tips: Automating Deployment](#tip-automating-builds--deployment)


## Overview 

The Resource Quota Solution does the following : 
- captures total counts of various resources specific via your config file
- gets the total allocation from Service Quotas api 
- produces utilization % and sends metric to cloudwatch logs via EMF (Embedded Metric Format)

![Architecture Diagram](../../media/monitoring-solution-Page-6.drawio%20(1).png)

## Valid Metrics per service 

We will add mmore metrics based on customer feedback but below is what we have converage for today. 

``` bash 
- ec2 
  - networkInterfaces
- eks 
  - listClusters
- vpc 
  - nau
- iam 
  - iamRoles
  - oidcProviders
- ebs
  - gp3Storage
```
#### ⚠️ Attention⚠️
For the `iamRoles` and `gp3Storage` metric, we use the Support API to perform `RefreshTrustedAdvisorCheck` against the Trusted Advisor service.  You need at least business support for this metric to work, if not, the solution will throw a 404 exception but it will continue to calculate other metrics.

## Deployment Guide

---
#### ⚠️ Disclaimer ⚠️
This repository is provided as a functional example to demonstrate how you might capture control-plane events, buffer them through SQS, and emit EMF metrics via Lambda. It is not intended to represent a production-ready "drop in" solution. Before using in any live environment, you should:

- Review and adjust IAM permissions to follow the principle of least privilege
- Review encryption at rest and in transit for all resources (SQS, Lambda, logs, etc.)
- Configure VPC, subnet, and security group settings according to your network requirements
- Implement proper monitoring, alerting, and log retention lifecycles
- Be aware of any costs associated with deploying in your account(s)

Use this sample as a starting point, not a drop-in solution. Customize this solution based on your organization’s security, reliability, and operational requirements.

---

This project can be deployed via [CloudFormation / AWS SAM](#deploying-with-aws-sam--cloudformation) or [Terraform](#deploying-with-terraform).  

When deploying with AWS SAM:
- deploy sam template

When deploying with Terraform:
- build / package lambda function 
- create / apply terraform plan

### Prerequisites

- [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)  
- [AWS SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html) (latest)  
- [Go v1.22.1](https://go.dev/doc/install) or higher (for local development)

**Tools**

| Tool         | Version      | Install                                                                                          |
|--------------|--------------|--------------------------------------------------------------------------------------------------|
| Go           | ≥1.23.0      | https://golang.org/dl/                                                                           |
| Terraform    | ≥1.0.0       | https://learn.hashicorp.com/tutorials/terraform/install-cli                                      |
| AWS SAM CLI  | ≥1.142.1     | https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html |
| AWS CLI      | Latest       | https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html                    |

### Environment Variables

| Name             | Description                                                                   | Default |
|------------------|-------------------------------------------------------------------------------|---------|
| LOG_LEVEL        | Log verbosity (DEBUG, INFO, WARN, ERROR)                                      | INFO |
| LOG_GROUP_NAME   | CloudWatch Logs group name for EMF output                                     |  /lambda/resourcequota/emf  |
| METRIC_NAMESPACE | CloudWatch Metric Namespace                                                   |  Resource Quota Utilization |
| LAMBDA_LAYER_PATH | Path to the location of the config.json file in the Lambda layer. If you made any changes to the Lambda layer, you need to update this variable accordingly. | /opt/config/config.json |


### Deploying with AWS SAM / Cloudformation

Prior to deploying, please review the resource below to see what AWS SAM / Cloudformation will deploy. 

#### ⚠️ Warning ⚠️
Please make changes to the template based on your specific environment / security requirements.  This is a functional sample but is not intended to be production ready by default

#### Resources

| Logical ID                  | Type                            | Description                                                    | Key Properties                                                                                                                                                       |
|-----------------------------|---------------------------------|----------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **LambdaExecutionRole**     | `AWS::IAM::Role`                | IAM role assumed by the ResourceQuota Lambda                   | • Trust: `lambda.amazonaws.com`  <br>• ManagedPolicyArns: `AWSLambdaBasicExecutionRole`  <br>• Inline policy covering EC2, EKS, IAM, Support, EFS, CloudWatchLogs, S3 |
| **ConfigFileLambdaLayer**   | `AWS::Serverless::LayerVersion` | Layer packaging your `config/` directory                       | • ContentUri: `../../../lambda-layer/`  <br>• CompatibleRuntimes: `provided.al2023`  <br>• Architectures: `arm64`                                                     |
| **ResourceQuotaFunction**   | `AWS::Serverless::Function`     | Main Go Lambda that computes & publishes quota EMF metrics     | • CodeUri: `../../../cmd/resourcequota`  <br>• Handler: `bootstrap`  <br>• Role: `LambdaExecutionRole` ARN  <br>• Layers: `[ConfigFileLambdaLayer]`  <br>• Schedule: rate(5 min) |
| **ResourceQuotaErrorFilter**| `AWS::Logs::MetricFilter`       | Scans the Lambda’s application-log group for `"ERROR"` events  | • FilterPattern: `"ERROR"`  <br>• LogGroupName: `/aws/lambda/${ResourceQuotaFunction}`  <br>• MetricName: `Error Count`  <br>• MetricNamespace: `${MetricNamespace}`     |
| **ResourceQuotaErrorAlarm** | `AWS::CloudWatch::Alarm`        | Fires when any ERROR-log appears in your Lambda’s logs         | • Namespace: `${MetricNamespace}`  <br>• MetricName: `Error Count`  <br>• Dimension: LogGroupName=`/aws/lambda/${ResourceQuotaFunction}`  <br>• Threshold: `1`          |

---

#### Parameters

| Parameter Name         | Type   | Description                                                             | Default                           | Valid Choices                   |
|--------------------|--------|-------------------------------------------------------------------------|-----------------------------------|---------------------------------|
| CloudWatchLogGroup | String | CloudWatch Log Group where EMF metrics are published                    | `/lambda/resource-quota/emf`      | —                               |
| MetricNamespace    | String | CloudWatch Metric Namespace for quota-utilization metrics               | `Resource Quota Utilization`      | —                               |
| LogLevel           | String | Log verbosity for the Lambda function                                   | `info`                            | `debug`, `info`, `warn`, `error`|
| LambdaLayerPath    | String | File path inside the layer where your `config.json` lives                | `/opt/config/config.json`         | —                               |
| S3BucketName       | String | S3 bucket for storing the resource-quota manifest file                  | `resource-quota-utilization-2025` | —                               |
| HomeRegion         | String | “Home” AWS region used by the ResourceQuota job (e.g. for cross-region) | `us-east-1`                       | —                               |
| Regions            | String | Comma-separated list of AWS regions to generate metrics for             | `us-east-1`                       | —                               |

---

#### Steps 

1. **Clone the repository**
```bash
git clone https://github.com/aws-samples/sample-resource-quota-and-control-plane-utilization
```
2. **Build & deploy with AWS SAM / Cloudformation**

Navigate to the `infra/cloudformation/resourcequota` folder. Ensure there is a template.yaml file located in that directory. 
```bash 
root-dir/
        infra/
            cloudformation/
                    resourcequota/
                          template.yaml
```

From the `infra/cloudformation/resourcequota` directory, run the commands below to build and deploy the application. 

```bash
sam build
sam deploy --guided
```

> **Tip:** Use `sam deploy --guided` on your first deployment 

---

### Deploying With Terraform

Prior to deploying, please review the resource below to see what Terraform will deploy.  

#### ⚠️ Warning ⚠️
Please make changes to the template based on your specific environment and security requirements. This is a functional sample but is not verified to be production-ready by default.

#### Resources 
| Resource                                           | Type                              | Description                                                              | Key Properties                                                                                                                              |
|----------------------------------------------------|-----------------------------------|--------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| `aws_iam_role.lambda_exec`                         | `aws_iam_role`                    | IAM role assumed by the ResourceQuota Lambda                             | `assume_role_policy` allowing `lambda.amazonaws.com`                                                                                        |
| `aws_iam_role_policy_attachment.attach_basic`      | `aws_iam_role_policy_attachment`  | Attach AWSLambdaBasicExecutionRole                                       | `role = aws_iam_role.lambda_exec`, `policy_arn = AWSLambdaBasicExecutionRole`                                                               |
| `aws_iam_role_policy.resource_quota`               | `aws_iam_role_policy`             | Inline policy granting permissions for EC2, EKS, IAM, Support, etc.      | JSON policy document with actions for `ec2:Describe*`, `eks:ListClusters`, `servicequotas:GetServiceQuota`, `s3:PutObject`, `s3:ListBucket` |
| `data.archive_file.lambda_config_layer`            | `archive_file`                    | Zips up the local `lambda-layer` directory into a `.zip`                 | `source_dir = ../../../lambda-layer`, `output_path = lambda-layer.zip`                                                                       |
| `aws_lambda_layer_version.config`                  | `aws_lambda_layer_version`        | Publishes the zipped config folder as a Lambda Layer                     | `filename = data.archive_file.lambda_config_layer.output_path`, `compatible_runtimes = ["provided.al2023"]`                                  |
| `aws_lambda_function.resource_quota`               | `aws_lambda_function`             | The ResourceQuota Go Lambda                                              | `filename = dist/resourcequota/resourcequota.zip`, environment variables (including `REGIONS`), runtime, handler, layers                     |
| `aws_cloudwatch_event_rule.every_5m`               | `aws_cloudwatch_event_rule`       | Schedules the Lambda to run every 5 minutes                              | `schedule_expression = "rate(5 minutes)"`, `name = "ResourceQuotaEveryFiveMinutes"`                                                         |
| `aws_cloudwatch_event_target.every_5m_target`      | `aws_cloudwatch_event_target`     | Binds the 5-min rule to the Lambda function                              | `rule = aws_cloudwatch_event_rule.every_5m.name`, `arn = aws_lambda_function.resource_quota.arn`                                             |
| `aws_lambda_permission.allow_event`                | `aws_lambda_permission`           | Grants EventBridge permission to invoke the Lambda                       | `function_name = aws_lambda_function.resource_quota.function_name`, `principal = "events.amazonaws.com"`                                     |
| `aws_cloudwatch_log_metric_filter.resource_quota_error` | `aws_cloudwatch_log_metric_filter` | Metric filter for any `"ERROR"` logs in the Lambda’s native log group     | `log_group_name = "/aws/lambda/${aws_lambda_function.resource_quota.function_name}"`, `pattern = "\"ERROR\""`, transforms → ErrorCount       |
| `aws_cloudwatch_metric_alarm.resource_quota_error_alarm` | `aws_cloudwatch_metric_alarm`     | Alarm when the ResourceQuota Lambda emits any ERROR logs                  | `namespace = var.metric_namespace`, `metric_name = "ErrorCount"`, `threshold = 1`, `statistic = "Sum"`, `dimensions = { LogGroupName = ... }` |
 ---

#### Variables 

| Name                  | Type           | Description                                            | Default                            |
|-----------------------|----------------|--------------------------------------------------------|------------------------------------|
| `cloudwatch_log_group`| string         | CloudWatch Log Group for EMF output                    | `/lambda/resource-quota/emf`       |
| `metric_namespace`    | string         | Metric Namespace for resource quota utilization        | `Resource Quota Utilization`       |
| `log_level`           | string         | Log level for the Lambda function                      | `debug`                            |
| `lambda_layer_path`   | string         | Path inside Lambda Layer where config lives            | `/opt/config/config.json`          |
| `s3_bucket_name`      | string         | S3 bucket to hold the resource-quota manifest          | `resource-quota-utilization-2025`  |
| `home_region`         | string         | Home AWS region for the manifest file                  | `us-east-1`                        |
| `regions`             | list(string)   | List of regions you want to generate metrics for       | `["us-east-1"]`                    |

#### Steps

1. **Clone the repo**  
   ```bash
   git clone https://github.com/aws-samples/sample-resource-quota-and-control-plane-utilization
   cd sample-resource-quota-and-control-plane-utilization
   ```

2. **Build the Resource Quota Lambda Function**

We provide a dedicated Makefile (`Makefile.resourcequota`) to compile & package the ResourceQuota function.  Terraform is configured use this directory to pull the artifact and deploy to AWS:

> NOTE: If you wish to push the artifact to S3 or another location instead, please ensure you edit the infra/terraform/main.tf file accordinly to reflect these changes


```bash 
# Compile and package the ResourceQuota lambda binary into dist/resourcequota/resourcequota.zip
make -f Makefile.resourcequota all

# Remove build artifacts for ResourceQuota only
make -f Makefile.resourcequota clean


dist/
└── resourcequota/
    ├── bootstrap             ← ARM64 Linux binary
    └── resourcequota.zip     ← zip containing only the executable
```

3. **Initialize and apply Terraform plan**
```bash
# Initialize Terraform
terraform init

# Create an execution plan and save it to tfplan
terraform plan -out=tfplan \
  -var="cloudwatch_log_group=/lambda/resource-quota/emf" \
  -var="metric_namespace=Resource Quota Utilization" \
  -var="log_level=info" \
  -var="lambda_layer_path=/opt/config/config.json" \
  -var="s3_bucket_name=my-quota-manifest-bucket" \
  -var="home_region=us-east-1" \
  -var='regions=["us-east-1","us-west-2"]'

# Review the plan:
terraform show tfplan

# Apply the saved plan
terraform apply -auto-approve tfplan
```

---

## Tip: Automating Builds & Deployment

You can integrate these `make` and deployment steps into a CI/CD pipeline (GitHub Actions, GitLab CI, Jenkins, etc.) or run them inside a dedicated build container as a **pre-deployment hook**. This ensures:

- **Reproducible artifacts** in a clean environment  
- **Early failure detection** in pull requests  
- **Consistent deployments** across all agents  
- **Decoupling** developers from local workstation dependencies  

Simply invoke your `make` targets and SAM/Terraform commands in your pipeline’s build stage prior to the deploy stage.