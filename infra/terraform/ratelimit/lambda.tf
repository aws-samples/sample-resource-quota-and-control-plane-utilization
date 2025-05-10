resource "aws_lambda_function" "assume_role" {
  function_name = "AssumeRoleProcessor"
  handler       = var.assume_role_handler
  runtime       = "provided.al2"
  architectures = ["arm64"]
  role          = aws_iam_role.lambda_exec.arn
  filename      = "${path.module}/../../cmd/ratelimit/assume_role.zip"
  source_code_hash = filebase64sha256("${path.module}/../../cmd/ratelimit/assume_role.zip")

  environment {
    variables = {
      REGIONS               = var.regions
      LOG_LEVEL             = var.log_level
      CLOUDWATCH_LOG_GROUP  = var.cloudwatch_log_group
      METRIC_NAMESPACE      = var.metric_namespace
      FLUSH_INTERVAL        = var.flush_interval
    }
  }

  layers = [aws_lambda_layer_version.cloudtrail_extension_layer.arn]
}

resource "aws_lambda_function" "assume_role_web_identity" {
  function_name = "AssumeRoleWebIdentityProcessor"
  handler       = var.assume_web_identity_handler
  runtime       = "provided.al2"
  architectures = ["arm64"]
  role          = aws_iam_role.lambda_exec.arn
  filename      = "${path.module}/../../cmd/ratelimit/assume_web_identity.zip"
  source_code_hash = filebase64sha256("${path.module}/../../cmd/ratelimit/assume_web_identity.zip")

  environment {
    variables = {
      REGIONS               = var.regions
      LOG_LEVEL             = var.log_level
      CLOUDWATCH_LOG_GROUP  = var.cloudwatch_log_group
      METRIC_NAMESPACE      = var.metric_namespace
      FLUSH_INTERVAL        = var.flush_interval
    }
  }

  layers = [aws_lambda_layer_version.cloudtrail_extension_layer.arn]
}

resource "aws_lambda_layer_version" "cloudtrail_extension_layer" {
  filename          = "${path.module}/../../layers/emf-extension.zip"
  layer_name        = "CloudTrailExtensionLayer"
  compatible_runtimes = ["provided.al2"]
}

