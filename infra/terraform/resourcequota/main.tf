# Resource Quota Monitor - Terraform configuration for scheduled resource utilization monitoring
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }

    archive = { 
      source = "hashicorp/archive"
      version = "~> 2.0"
    }
  }
  required_version = ">= 1.0.0"
}

provider "aws" {}


###############################
# 1) IAM Role & Policy        #
# Lambda execution permissions#
###############################
data "aws_iam_policy" "basic_exec" {
  arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role" "lambda_exec" {
  name = "resource_quota_lambda_exec"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "attach_basic" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = data.aws_iam_policy.basic_exec.arn
}

resource "aws_iam_role_policy" "resource_quota" {
  name = "ResourceQuotaPolicy"
  role = aws_iam_role.lambda_exec.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = [
          # EC2
          "ec2:DescribeNetworkInterfaces","ec2:DescribeNatGateways",
          "ec2:DescribeVpcEndpoints","ec2:DescribeSubnets",
          "ec2:DescribeTransitGatewayVpcAttachments","ec2:DescribeVpcs",
          # EKS
          "eks:ListClusters",
          # IAM
          "iam:ListOpenIDConnectProviders","iam:ListRoles",
          # Support
          "support:RefreshTrustedAdvisorCheck",
          # ELBv2
          "elasticloadbalancing:DescribeLoadBalancers",
          # EFS
          "elasticfilesystem:DescribeFileSystems",
          "elasticfilesystem:DescribeMountTargets",
          # CloudWatchLogs
          "logs:DescribeLogGroups","logs:CreateLogGroup",
          "logs:DescribeLogStreams","logs:CreateLogStream",
          "logs:PutLogEvents",
          # Service Quotas
          "servicequotas:GetServiceQuota"
        ]
        Resource = ["*"]
      },
      {
        Effect   = "Allow"
        Action   = ["s3:PutObject","s3:PutObjectAcl"]
        Resource = ["arn:aws:s3:::${var.s3_bucket_name}/*"]
      },
      {
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = ["arn:aws:s3:::${var.s3_bucket_name}/*"]
      }
    ]
  })
}

################################################
# 2) Lambda Layer (config file)                #
# Service configuration for quota monitoring    #
################################################

data "archive_file" "lambda_config_layer" {
  type        = "zip"
  source_dir  = "${path.module}/../../../lambda-layer"
  output_path = "${path.module}/lambda-layer.zip"
}
resource "aws_lambda_layer_version" "config" {
  layer_name          = "resource-quota-config"
  description         = "Configuration for resource quota utilization"
  compatible_runtimes = ["provided.al2023"]
  compatible_architectures = ["arm64"]
  filename = data.archive_file.lambda_config_layer.output_path
}

################################################
# 3) Go Lambda + Schedule                     #
# Scheduled resource quota monitoring function #
################################################
locals {
  # path.module == root/infra/terraform/ratelimit
  repo_root        = abspath("${path.module}/../../..")      # back up from ratelimit → terraform → infra → root
  resourcequota_zip = "${local.repo_root}/dist/resourcequota/resourcequota.zip"
}

# compile & zip from build steps into `build/bootstrap.zip`
resource "aws_lambda_function" "resource_quota" {
  function_name = "geras-resource-quota"
  filename      = local.resourcequota_zip
  role          = aws_iam_role.lambda_exec.arn
  handler       = "bootstrap"
  runtime       = "provided.al2"
  architectures = ["arm64"]
  timeout       = 900

  layers = [aws_lambda_layer_version.config.arn]

  environment {
    variables = {
      LAMBDA_LAYER_PATH     = var.lambda_layer_path
      CLOUDWATCH_LOG_GROUP  = var.cloudwatch_log_group
      METRIC_NAMESPACE      = var.metric_namespace
      LOG_LEVEL             = var.log_level
      S3_BUCKET             = var.s3_bucket_name
      HOME_REGION           = var.home_region
      REGIONS               = join(",", var.regions)
    }
  }
}

# scheduled rule every 5 minutes
resource "aws_cloudwatch_event_rule" "every_5m" {
  name                = "ResourceQuotaEveryFiveMinutes"
  schedule_expression = "rate(5 minutes)"
}

resource "aws_cloudwatch_event_target" "every_5m_target" {
  rule      = aws_cloudwatch_event_rule.every_5m.name
  arn       = aws_lambda_function.resource_quota.arn
}

# grant invoke permission to EventBridge
resource "aws_lambda_permission" "allow_event" {
  statement_id  = "AllowExecutionFromEvents"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.resource_quota.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.every_5m.arn
}

#############################
# 4) Error Metric & Alarm   #
# Monitor Lambda errors     #
#############################

# 1) Metric filter on the Lambda’s native log group 
resource "aws_cloudwatch_log_metric_filter" "resource_quota_error" {
  name           = "ResourceQuota-ErrorFilter"
  log_group_name = "/aws/lambda/${aws_lambda_function.resource_quota.function_name}"
  pattern        = "\"ERROR\""

  metric_transformation {
    name      = "ErrorCount"
    namespace = var.metric_namespace
    value     = "1"
  }
}

# 2) Alarm on the ErrorCount metric
resource "aws_cloudwatch_metric_alarm" "resource_quota_error_alarm" {
  alarm_name          = "ResourceQuota-Error-Alarm"
  alarm_description   = "Fires if the ResourceQuota Lambda emits any ERROR logs"
  namespace           = var.metric_namespace
  metric_name         = aws_cloudwatch_log_metric_filter.resource_quota_error.metric_transformation[0].name
  statistic           = "Sum"
  period              = 60
  evaluation_periods  = 1
  threshold           = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  treat_missing_data  = "notBreaching"

  dimensions = {
    LogGroupName = "/aws/lambda/${aws_lambda_function.resource_quota.function_name}"
  }
}


################################
# 5) Output the function ARN  #
################################
output "resource_quota_function_arn" {
  description = "ARN of the ResourceQuota Lambda"
  value       = aws_lambda_function.resource_quota.arn
}
