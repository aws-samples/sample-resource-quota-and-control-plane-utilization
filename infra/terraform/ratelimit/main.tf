# Rate Limit Monitor - Terraform configuration for event-driven CloudTrail API monitoring
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
  required_version = ">= 1.0.0"
}

provider "aws" {}

###############################
# 1) FIFO SQS Queues          #
# Separate queues for each API #
###############################
resource "aws_sqs_queue" "assume_role_queue" {
  name                        = "assume-role-queue.fifo"
  fifo_queue                  = true
  content_based_deduplication = true
  visibility_timeout_seconds  = 30
}

resource "aws_sqs_queue" "assume_role_webidentity_queue" {
  name                        = "assume-role-web-identity-queue.fifo"
  fifo_queue                  = true
  content_based_deduplication = true
  visibility_timeout_seconds  = 30
}

###################################
# 2) IAM Roles & Attachments      #
# Lambda execution and EventBridge#
###################################
data "aws_iam_policy" "lambda_basic" {
  arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

data "aws_iam_policy" "lambda_sqs" {
  arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaSQSQueueExecutionRole"
}

resource "aws_iam_role" "lambda_exec" {
  name = "ratelimit_lambda_execution"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "lambda_basic" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = data.aws_iam_policy.lambda_basic.arn
}

resource "aws_iam_role_policy_attachment" "lambda_sqs" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = data.aws_iam_policy.lambda_sqs.arn
}

resource "aws_iam_role_policy" "describe_logs" {
  name = "DescribeLogResources"
  role = aws_iam_role.lambda_exec.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["logs:DescribeLogGroups","logs:DescribeLogStreams"]
      Resource = ["*"]
    }]
  })
}

# EventBridge → SQS delivery role
resource "aws_iam_role" "eb_to_sqs" {
  name = "ratelimit_eventbridge_delivery"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "eb_sqs_send" {
  name = "AllowEventBridgeSQSSend"
  role = aws_iam_role.eb_to_sqs.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["sqs:SendMessage","sqs:SendMessageBatch"]
      Resource = [
        aws_sqs_queue.assume_role_queue.arn,
        aws_sqs_queue.assume_role_webidentity_queue.arn
      ]
    }]
  })
}

##############################################
# 3) EventBridge Rules (scheduled & CT)     #
# Flush pings and CloudTrail event capture  #
##############################################
# Scheduled flush ping → AssumeRole
resource "aws_cloudwatch_event_rule" "keepalive_assume_role" {
  name                = "KeepAliveToAssumeRoleRule"
  description         = "Ping every minute to flush AssumeRole buffer"
  schedule_expression = "rate(1 minute)"
  state               = "ENABLED"
}

# Scheduled flush ping → WebIdentity
resource "aws_cloudwatch_event_rule" "keepalive_webidentity" {
  name                = "KeepAliveToWebIdentityRule"
  description         = "Ping every minute to flush WebIdentity buffer"
  schedule_expression = "rate(1 minute)"
  state               = "ENABLED"
}

# CloudTrail → AssumeRole
resource "aws_cloudwatch_event_rule" "assume_role" {
  name        = "AssumeRoleRule"
  description = "Watch for AssumeRole API calls"
  event_pattern = jsonencode({
    "detail-type": ["AWS API Call via CloudTrail"],
    "detail": {
      "eventName": ["AssumeRole"],
      "awsRegion": var.regions
    }
  })
}

# CloudTrail → AssumeRoleWithWebIdentity
resource "aws_cloudwatch_event_rule" "assume_role_webid" {
  name        = "AssumeRoleWithWebIdentityRule"
  description = "Watch for AssumeRoleWithWebIdentity API calls"
  event_pattern = jsonencode({
    "detail-type": ["AWS API Call via CloudTrail"],
    "detail": {
      "eventName": ["AssumeRoleWithWebIdentity"],
      "awsRegion": var.regions
    }
  })
}

###########################################
# 4) EventBridge → SQS Targets            #
# Route events to appropriate queues       #
###########################################
# keepalive → AssumeRoleQueue
resource "aws_cloudwatch_event_target" "keepalive_assume_role" {
  rule      = aws_cloudwatch_event_rule.keepalive_assume_role.name
  arn       = aws_sqs_queue.assume_role_queue.arn
  role_arn  = aws_iam_role.eb_to_sqs.arn

  input_transformer {
    input_paths = {
      scheduledTime = "$.time"
    }
    input_template = <<-EOF
{"flush": true, "scheduledTime": "<scheduledTime>"}
EOF
  }

  sqs_target {
    message_group_id = "flush-group"
  }
}

# keepalive → WebIdentityQueue
resource "aws_cloudwatch_event_target" "keepalive_webidentity" {
  rule      = aws_cloudwatch_event_rule.keepalive_webidentity.name
  arn       = aws_sqs_queue.assume_role_webidentity_queue.arn
  role_arn  = aws_iam_role.eb_to_sqs.arn

  input_transformer {
    input_paths = {
      scheduledTime = "$.time"
    }
    input_template = <<-EOF
{"flush": true, "scheduledTime": "<scheduledTime>"}
EOF
  }

  sqs_target {
    message_group_id = "flush-group"
  }
}

# CloudTrail → AssumeRoleQueue
resource "aws_cloudwatch_event_target" "assume_role" {
  rule      = aws_cloudwatch_event_rule.assume_role.name
  arn       = aws_sqs_queue.assume_role_queue.arn
  role_arn  = aws_iam_role.eb_to_sqs.arn

  input_path = "$.detail"

  sqs_target {
    message_group_id = "assume-role-group"
  }
}

# CloudTrail → WebIdentityQueue
resource "aws_cloudwatch_event_target" "assume_role_webid" {
  rule      = aws_cloudwatch_event_rule.assume_role_webid.name
  arn       = aws_sqs_queue.assume_role_webidentity_queue.arn
  role_arn  = aws_iam_role.eb_to_sqs.arn

  input_path = "$.detail"

  sqs_target {
    message_group_id = "assume-role-web-identity-group"
  }
}

##################################
# 5) SQS Queue Policies          #
##################################
resource "aws_sqs_queue_policy" "assume_role_queue" {
  queue_url = aws_sqs_queue.assume_role_queue.id
  policy    = aws_iam_role_policy.eb_sqs_send.policy
}

resource "aws_sqs_queue_policy" "assume_role_webid_queue" {
  queue_url = aws_sqs_queue.assume_role_webidentity_queue.id
  policy    = aws_iam_role_policy.eb_sqs_send.policy
}

#################################################
# 6) Go Lambdas + Event-Source Mappings         #
# Process SQS messages and publish EMF metrics  #
#################################################
locals {
  # path.module == root/infra/terraform/ratelimit
  repo_root        = abspath("${path.module}/../../..")      # back up from ratelimit → terraform → infra → root
  ratelimit_zip    = "${local.repo_root}/dist/ratelimit/ratelimit.zip"
}


# AssumeRole Lambda
resource "aws_lambda_function" "assume_role" {
  function_name = "AssumeRoleProcessor"
  filename      = local.ratelimit_zip
  role          = aws_iam_role.lambda_exec.arn
  handler       = "bootstrap"
  runtime       = "provided.al2"
  architectures = ["arm64"]

  layers = [
    aws_lambda_layer_version.rate_limit_extension.arn,
  ]

  environment {
    variables = {
      REGIONS                 = join(",", var.regions)
      LOG_LEVEL               = var.log_level
      CLOUDWATCH_LOG_GROUP    = var.cloudwatch_log_group
      METRIC_NAMESPACE        = var.metric_namespace
      PROPAGATE_INVOKER = var.propgate_invoker
    }
  }
}

# SQS → Lambda for AssumeRole
resource "aws_lambda_event_source_mapping" "assume_role_sqs" {
  event_source_arn        = aws_sqs_queue.assume_role_queue.arn
  function_name           = aws_lambda_function.assume_role.arn
  batch_size              = 10
  function_response_types = ["ReportBatchItemFailures"]
}

# WebIdentity Lambda
resource "aws_lambda_function" "assume_role_webidentity" {
  function_name = "AssumeRoleWithWebIdentityProcessor"
  filename      = local.ratelimit_zip
  role          = aws_iam_role.lambda_exec.arn
  handler       = "bootstrap"
  runtime       = "provided.al2"
  architectures = ["arm64"]

  layers = [ 
    aws_lambda_layer_version.rate_limit_extension.arn,
   ]

  environment {
    variables = {
      REGIONS                 = join(",", var.regions)
      LOG_LEVEL               = var.log_level
      CLOUDWATCH_LOG_GROUP    = var.cloudwatch_log_group
      METRIC_NAMESPACE        = var.metric_namespace
      PROPAGATE_INVOKER = var.propgate_invoker
    }
  }
}

# SQS → Lambda for WebIdentity
resource "aws_lambda_event_source_mapping" "assume_role_webidentity_sqs" {
  event_source_arn        = aws_sqs_queue.assume_role_webidentity_queue.arn
  function_name           = aws_lambda_function.assume_role_webidentity.arn
  batch_size              = 10
  function_response_types = ["ReportBatchItemFailures"]
}

####################################
# 7) CloudWatch RPS Alarms         #
# Monitor API requests per second  #
####################################
resource "aws_cloudwatch_metric_alarm" "assume_role_rps" {
  alarm_name          = "AssumeRole-rps-alarm"
  alarm_description   = "AssumeRole API requests per second"
  namespace = var.metric_namespace
  metric_name = "RequestsPerSecond"
  dimensions = {
    eventName = "AssumeRole"
  }
  statistic = "Maximum"
  period = 60
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  threshold           = 50
  treat_missing_data  = "missing"
}

resource "aws_cloudwatch_metric_alarm" "assume_role_webidentity_rps" {
  alarm_name          = "AssumeRoleWithWebIdentity-rps-alarm"
  alarm_description   = "AssumeRoleWithWebIdentity API requests per second"
  namespace = var.metric_namespace
  metric_name = "RequestsPerSecond"
  dimensions = {
    eventName = "AssumeRoleWithWebIdentity"
  }
  statistic = "Maximum"
  period = 60
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  threshold           = 50
  treat_missing_data  = "missing"
}

# ──────────────────────────────────────────────────────────
# 8) EMF Extension Layer
# ──────────────────────────────────────────────────────────
resource "aws_lambda_layer_version" "rate_limit_extension" {
  layer_name               = "RateLimitExtensionLayer"
  description              = "EMF Lambda Extension for flushing /tmp EMFs before shutdown"
  compatible_runtimes      = ["provided.al2"]
  compatible_architectures = ["arm64"]

  s3_bucket = var.extension_bucket
  s3_key    = var.extension_s3_key
}

