# Rate Limit & Resource Quota Monitoring Solution

## Solution(s) Overview

This repository contains two complementary, serverless Go projects following AWS best practices for EMF-based metrics:

---
#### ⚠️ Disclaimer ⚠️
This repository is provided as a functional example. It is not intended to represent a production-ready "drop-in" solution. Before using in any live environment, you should:

- Review and adjust IAM permissions to follow the principle of least privilege
- Review encryption at rest and in transit for all resources (SQS, Lambda, logs, etc.)
- Configure VPC, subnet, and security group settings according to your network requirements
- Implement proper monitoring, alerting, and log retention lifecycles
- Be aware of any costs associated with deploying in your account(s)

Use this sample as a starting point, not a drop-in solution. Customize this solution based on your organization’s security, reliability, and operational requirements.

---

### Rate Limit Monitor
An **event-driven** pipeline that captures control-plane API call rates and publishes RequestPerSecond metrics to CloudWatch in ~60-second intervals.

**Use Case:** Helps customers prevent throttling which can reduce production customer-facing impacting events. Invoker-level metrics allow customers to identify the source of consumption, enabling them to better redistribute resources and maintain room for growth from an RPS perspective.

Below is an example of an API-level RequestPerSecond metric. This metric is valuable to alarm on since it represents the total usage of an API.

![api level rps metric](/media/apimetric.png)

Below is an example of invoker-level RequestPerSecond metric. Each invoker will have their own dedicated metric for a given API.

This metric is useful for deeper analysis into where your consumption is coming from. 

![invoker level rps metric](/media/invokermetric.png)
  

### Resource Quota Utilization
A scheduled Lambda function that computes resource utilization across your account by making various describe calls, retrieving the current quota from Service Quotas and publishing utilization metrics (%) to CloudWatch.

**Use Case:** Valuable for dynamically generating utilization metrics which help prevent resource exhaustion. This is mainly a key concern for larger customers or ISVs who need proactive monitoring to avoid hitting service limits.

This project captures utilization metrics for resources that do not have native CloudWatch coverage.

Currently supported metrics (with plans to continuously add more based on customer feedback):
- Total network interfaces per region
- VPC NAU (Network Address Units)
- Total GP3 storage
- Total OIDC providers
- Total EKS clusters
- Total IAM roles

---
## Repo Folder Structure

```bash
cmd/ # entry point location for each project
    emf-extension/      # Lambda extension   
            main.go 
    ratelimit/          # rate limit solution
            main.go 
    resourcequota/      # resource quota solution
            main.go 
infra/                  # folder for deploying via CloudFormation and Terraform 
        /cloudformation 
                /ratelimit
                        template.yaml
                /resourcequota
                        template.yaml
        /terraform 
                /ratelimit
                        main.tf
                        variables.tf
                /resourcequota
                        main.tf
                        variables.tf
internal/   # folder for internal libraries used
lambda-layer/ # directory for Lambda layer
```

## Architecture Diagrams


### Rate Limit Monitor 
![Rate Limit Architecture](media/monitoring-solution-Page-8.drawio.png)

--------

### Resource Quota Utilization 
![Resource Quota Architecture](media/monitoring-solution-Page-6.drawio%20(1).png)


---


## Subproject READMEs
Please navigate to each project's README file for more details.

- [Rate Limit Solution → `cmd/ratelimit/README.md`](cmd/ratelimit/README.md) 
  
- [Resource Quota Utilization → `cmd/resourcequota/README.md`](cmd/resourcequota/README.md)
