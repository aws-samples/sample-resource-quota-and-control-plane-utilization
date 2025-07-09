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
# 2) IAM Role & Policy        #
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
# 3) Lambda Layer (config file)                #
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
# 4) Go Lambda + Schedule                     #
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

################################
# 5) Output the function ARN  #
################################
output "resource_quota_function_arn" {
  description = "ARN of the ResourceQuota Lambda"
  value       = aws_lambda_function.resource_quota.arn
}
