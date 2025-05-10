# Lambda Layer for your configuration file
resource "aws_lambda_layer_version" "config" {
  layer_name          = "resource-quota-config"
  description         = "Configuration for resource quota utilization solution"
  s3_bucket           = var.layer_s3_bucket
  s3_key              = var.layer_s3_key
  compatible_runtimes = ["provided.al2023"]
  compatible_architectures = ["arm64"]
}

