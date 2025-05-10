variable "aws_region" {
  description = "AWS region to deploy into"
  type        = string
  default     = "us-east-1"
}

variable "cloudwatch_log_group" {
  description = "CloudWatch Log Group for EMF"
  type        = string
  default     = "/lambda/resource-quota/emf"
}

variable "metric_namespace" {
  description = "Metric Namespace for resource quota utilization"
  type        = string
  default     = "Resource Quota Utilization"
}

variable "log_level" {
  description = "Log level for Lambda function"
  type        = string
  default     = "debug"
}

variable "lambda_layer_path" {
  description = "Path within the layer zip where your config.json lives"
  type        = string
  default     = "/opt/config/config.json"
}

variable "layer_s3_bucket" {
  description = "S3 bucket containing the config layer zip"
  type        = string
  default     = "custom-monitoring-poc"
}

variable "layer_s3_key" {
  description = "S3 key for the config layer zip"
  type        = string
  default     = "layers/layer.zip"
}

variable "lambda_code_path" {
  description = "Path to your compiled Go binary (zip) for the ResourceQuota function"
  type        = string
  default     = "../../../cmd/resourcequota/function.zip"
}

variable "lambda_handler" {
  description = "Handler name inside your Go binary (usually 'bootstrap')"
  type        = string
  default     = "bootstrap"
}

