variable "regions" {
  description = "List of AWS regions to deploy against (e.g. [\"us-east-1\",\"us-west-2\"])."
  type        = list(string)
  default = [ "us-east-1" ]
}

variable "log_level" {
  description = "Log level for the Go lambdas (debug, info, warn, error)."
  type        = string
  default     = "debug"
}

variable "cloudwatch_log_group" {
  description = "CloudWatch log group for EMF output."
  type        = string
  default     = "/lambda/ratelimit/emf"
}

variable "metric_namespace" {
  description = "CloudWatch namespace for your Rate Limit metrics."
  type        = string
  default     = "Rate Limit"
}
variable "propagate_iam_principal" {
  description = "Whether to emit per‐principal metrics (true/false)."
  type        = bool
  default     = false
}

variable "extension_bucket" {
  description = "S3 bucket where emf-extension.zip is published"
  type        = string
  default     = "custom-monitoring-poc"
}

variable "extension_s3_key" {
  description = "S3 key (path) to the EMF extension zip"
  type        = string
  default     = "emf/emf-extension.zip"
}

