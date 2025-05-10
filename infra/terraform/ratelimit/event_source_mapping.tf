resource "aws_lambda_event_source_mapping" "assume_role_mapping" {
  event_source_arn                   = aws_sqs_queue.events.arn
  function_name                      = aws_lambda_function.assume_role.arn
  batch_size                         = 10
  maximum_batching_window_in_seconds = 0

  # tell Lambda to return failed items for custom SQS retries
  function_response_types = ["ReportBatchItemFailures"]
}

resource "aws_lambda_event_source_mapping" "assume_role_web_identity_mapping" {
  event_source_arn                   = aws_sqs_queue.events.arn
  function_name                      = aws_lambda_function.assume_role_web_identity.arn
  batch_size                         = 10
  maximum_batching_window_in_seconds = 0

  function_response_types = ["ReportBatchItemFailures"]
}
