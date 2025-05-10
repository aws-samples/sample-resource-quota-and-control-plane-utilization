variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "regions" {
  description = "Comma‐separated list of regions for the lambdas"
  type        = string
  default     = "us-east-1"
}

variable "log_level" {
  description = "Lambda LOG_LEVEL env var"
  type        = string
  default     = "debug"
}

variable "cloudwatch_log_group" {
  description = "EMF CloudWatch Log Group"
  type        = string
  default     = "/lambda/ratelimit/emf"
}

variable "metric_namespace" {
  description = "CloudWatch Metric Namespace"
  type        = string
  default     = "Rate Limit"
}

variable "flush_interval" {
  description = "EMF flush interval (seconds)"
  type        = string
  default     = "45"
}

variable "assume_role_handler" {
  description = "Lambda handler name for AssumeRole"
  type        = string
  default     = "bootstrap"
}

variable "assume_web_identity_handler" {
  description = "Lambda handler name for AssumeRoleWithWebIdentity"
  type        = string
  default     = "bootstrap"
}

variable "layer_s3_bucket" {
  description = "S3 bucket for the CloudTrail extension layer"
  type        = string
  default     = "custom-monitoring-poc"
}

variable "layer_s3_key" {
  description = "S3 key for the CloudTrail extension layer"
  type        = string
  default     = "layers/emf-extension.zip"
}
