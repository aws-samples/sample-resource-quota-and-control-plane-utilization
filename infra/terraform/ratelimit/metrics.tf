variable "metric_namespace" {
  type    = string
  default = "Rate Limit"
}

# 1) Raw CallCount + RPS for AssumeRole
resource "aws_cloudwatch_metric_alarm" "assume_role_rps" {
  alarm_name          = "AssumeRole-rps-alarm"
  alarm_description   = "AssumeRole API requests per second"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  threshold           = 1
  treat_missing_data  = "breaching"

  metric_query {
    id           = "assumeRoleCount"
    label        = "Raw CallCount"
    return_data  = false

    metric {
      namespace   = var.metric_namespace
      metric_name = "CallCount"
      dimensions = {
        eventName = "AssumeRole"
      }
      stat   = "Sum"
      period = 60
    }

    
  }

  metric_query {
    id          = "assumeRoleRps"
    expression  = "assumeRoleCount / 60"
    label       = "AssumeRole RPS"
    return_data = true
  }
}

# 2) Raw CallCount + RPS for AssumeRoleWithWebIdentity
resource "aws_cloudwatch_metric_alarm" "assume_web_identity_rps" {
  alarm_name          = "AssumeRoleWithWebIdentity-rps-alarm"
  alarm_description   = "AssumeRoleWithWebIdentity API requests per second"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  threshold           = 1
  treat_missing_data  = "breaching"

  metric_query {
    id           = "assumeWebCount"
    label        = "Raw CallCount"
    return_data  = false

    metric {
      namespace   = var.metric_namespace
      metric_name = "CallCount"
      dimensions = {
        eventName = "AssumeRoleWithWebIdentity"
      }
      stat   = "Sum"
      period = 60
    }

   
  }

  metric_query {
    id          = "assumeWebRps"
    expression  = "assumeWebCount / 60"
    label       = "AssumeRoleWithWebIdentity RPS"
    return_data = true
  }
}
