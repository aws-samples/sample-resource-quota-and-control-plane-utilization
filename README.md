# Rate Limit & Resource Quota Monitoring Solution


## Solution Overview


This repository contains two complementary, serverless Go projects following AWS best practices for EMF-based metrics:


1. **Rate Limit Monitor** 
  An **event-driven** pipeline that captures control-plane API call rates and publishes CallCount metric to Cloudwatch in real-time.
  
    Pattern: EventBridge → SQS FIFO (single queue, per-rule message groups) → Lambda (batch of 10) → EMF in CloudWatch Logs (in batches).


2. **Resource Quota Utilization** 
  A scheduled lambda function that computes resource utilization across your account by making various describe calls, retrieving the current quota from Service Quotas and publishing a utilization metric (%) in Cloudwatch. 

    This project is aimed to capture utilization metrics for resources that do not have coverage natively. 

    As of now we have support for the following metrics, with plans to continuously add more based on customer feedback: 
  
    - total networker interface per region
    - VPC Nau (network address units)
    - total g3Storage 
    - total oidc providers
    - total EKS Clusters
    - total iam roles

---
## Repo Folder Structure

```bash
cmd / # entry point location for each project
    emf-extension/      # lambda extension   
            main.go 
    ratelimit/          # rate limit solution
            main.go 
    resourcequota/      # resource quota solution
            main.go 
infra / # folder for cloudformation templates 
      ratelimit/        # rate limit solution
              template.yaml 
      resourcequota     # resource quota solution
              template.yaml 
internal/   # folder for internal libraries useds
lambda-layer/ # directory for lambda layer
```

## Architecture Diagrams


### Rate Limit Monitor 
![Rate Limit Architecture](media/rate-limit-solution.png)

--------

### Resource Quota Utilization 
![Resource Quota Architecture](media/resource-quota-solution.png)


---


## Subproject READMEs
Please navigate to each projects README file for more details.

- [Rate Limit Solution → `cmd/ratelimit/README.md`](cmd/ratelimit/README.md) 
  
- [Resource Quota Utilization → `cmd/resourcequota/README.md`](cmd/resourcequota/README.md)
