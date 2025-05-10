resource "aws_sqs_queue" "events" {
  name                         = "events-queue.fifo"
  fifo_queue                   = true
  content_based_deduplication  = true
  visibility_timeout_seconds   = 30
}

resource "aws_sqs_queue_policy" "allow_eventbridge" {
  queue_url = aws_sqs_queue.events.id

  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [
      {
        Sid       = "AllowAssumeRole"
        Effect    = "Allow"
        Principal = { Service = "events.amazonaws.com" }
        Action    = ["sqs:SendMessage", "sqs:SendMessageBatch"]
        Resource  = aws_sqs_queue.events.arn
      },
      {
        Sid       = "AllowAssumeRoleWeb"
        Effect    = "Allow"
        Principal = { Service = "events.amazonaws.com" }
        Action    = ["sqs:SendMessage", "sqs:SendMessageBatch"]
        Resource  = aws_sqs_queue.events.arn
      }
    ]
  })
}
