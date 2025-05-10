output "queue_url" {
  description = "URL of the FIFO queue"
  value       = aws_sqs_queue.events.id
}

output "assume_role_function_arn" {
  description = "ARN of the AssumeRole Lambda"
  value       = aws_lambda_function.assume_role.arn
}

output "assume_role_web_identity_function_arn" {
  description = "ARN of the AssumeRoleWithWebIdentity Lambda"
  value       = aws_lambda_function.assume_role_web_identity.arn
}
