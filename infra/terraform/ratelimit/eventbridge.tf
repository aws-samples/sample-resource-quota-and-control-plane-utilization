resource "aws_iam_role" "eventbridge_delivery" {
  name               = "eventbridge-delivery-role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy" "eventbridge_sqs_send" {
  role   = aws_iam_role.eventbridge_delivery.id
  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["sqs:SendMessage", "sqs:SendMessageBatch"]
      Resource = aws_sqs_queue.events.arn
    }]
  })
}

resource "aws_cloudwatch_event_rule" "assume_role" {
  name        = "AssumeRoleRule"
  event_bus_name = "default"
  event_pattern = jsonencode({
    "detail-type": ["AWS API Call via CloudTrail"]
    detail: {
      eventName: ["AssumeRole"]
    }
  })
}

resource "aws_cloudwatch_event_target" "to_sqs_assume_role" {
  rule      = aws_cloudwatch_event_rule.assume_role.name
  target_id = "ToSQS"
  arn       = aws_sqs_queue.events.arn
  role_arn  = aws_iam_role.eventbridge_delivery.arn
  input_path = "$.detail"
  sqs_target {
    message_group_id = "assume-role-group"
  }
}

resource "aws_cloudwatch_event_rule" "assume_role_web_identity" {
  name        = "AssumeRoleWithWebIdentityRule"
  event_bus_name = "default"
  event_pattern = jsonencode({
    "detail-type": ["AWS API Call via CloudTrail"]
    detail: {
      eventName: ["AssumeRoleWithWebIdentity"]
    }
  })
}

resource "aws_cloudwatch_event_target" "to_sqs_assume_role_web_identity" {
  rule      = aws_cloudwatch_event_rule.assume_role_web_identity.name
  target_id = "ToSQS"
  arn       = aws_sqs_queue.events.arn
  role_arn  = aws_iam_role.eventbridge_delivery.arn
  input_path = "$.detail"
  sqs_target {
    message_group_id = "assume-role-web-identity-group"
  }
}
