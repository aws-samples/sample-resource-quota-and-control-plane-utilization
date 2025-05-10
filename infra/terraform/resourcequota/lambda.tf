# The ResourceQuota Lambda function
resource "aws_lambda_function" "resource_quota" {
  function_name = "geras-resource-quota"
  filename      = var.lambda_code_path
  handler       = var.lambda_handler
  runtime       = "provided.al2023"
  architectures = ["arm64"]
  role          = aws_iam_role.lambda_exec.arn
  layers        = [ aws_lambda_layer_version.config.arn ]

  environment {
    variables = {
      LAMBDA_LAYER_PATH     = var.lambda_layer_path
      CLOUDWATCH_LOG_GROUP  = var.cloudwatch_log_group
      METRIC_NAMESPACE      = var.metric_namespace
      LOG_LEVEL             = var.log_level
    }
  }

  timeout = 900  # 15 minutes
}
