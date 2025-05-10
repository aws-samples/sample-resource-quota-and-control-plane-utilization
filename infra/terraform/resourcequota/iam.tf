# Execution role for the ResourceQuota Lambda
resource "aws_iam_role" "lambda_exec" {
  name = "GerasResourceQuotaExecRole"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

# Attach AWSLambdaBasicExecutionRole for logs
resource "aws_iam_role_policy_attachment" "basic_exec" {
  role       = aws_iam_role.lambda_exec.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

# Inline policy for resource-quota permissions
resource "aws_iam_role_policy" "resource_quota_policy" {
  name = "ResourceQuotaPolicy"
  role = aws_iam_role.lambda_exec.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = [
          "ec2:DescribeNetworkInterfaces",
          "ec2:DescribeNatGateways",
          "ec2:DescribeVpcEndpoints",
          "ec2:DescribeSubnets",
          "ec2:DescribeTransitGatewayVpcAttachments",
          "ec2:DescribeVpcs",
          "eks:ListClusters",
          "iam:ListOpenIDConnectProviders",
          "iam:ListRoles",
          "support:RefreshTrustedAdvisorCheck",
          "elasticloadbalancing:DescribeLoadBalancers",
          "elasticfilesystem:DescribeFileSystems",
          "elasticfilesystem:DescribeMountTargets",
          "logs:DescribeLogGroups",
          "logs:CreateLogGroup",
          "logs:DescribeLogStreams",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
          "servicequotas:GetServiceQuota"
        ],
        Resource = ["*"]
      }
    ]
  })
}
