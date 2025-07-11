# Resource Quota Monitor - Terraform variables

variable "cloudwatch_log_group" {
  description = "CloudWatch Log Group for EMF"
  type        = string
  default     = "/lambda/resource-quota/emf"
}

variable "metric_namespace" {
  description = "Metric Namespace for resource quota utilization metrics"
  type        = string
  default     = "Resource Quota Utilization"
}

variable "log_level" {
  description = "Log level for Lambda function logging"
  type        = string
  default     = "debug"
}

variable "lambda_layer_path" {
  description = "Path inside Lambda Layer where config lives"
  type        = string
  default     = "/opt/config/config.json"
}

variable "s3_bucket_name" {
  description = "S3 bucket to hold the resource-quota manifest"
  type        = string
  default     = "resource-quota-utilization-2025"
}

variable "home_region" {
  description = "Home region for the manifest file"
  type        = string
  default     = "us-east-1"
}

variable "regions" { 
  description = "list of regions you want to generate metrics for"
  type        = list(string)
  default     = ["us-east-1"]
}