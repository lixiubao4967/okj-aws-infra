# OKJ AWS 基础设施文档

本仓库记录 AWS 各服务的操作文档与常用脚本。

## 目录结构

```
docs/
├── compute/        # 计算资源（EC2、Lambda）
├── networking/     # 网络（VPC、Route53、CloudFront）
├── storage/        # 存储（S3、EFS）
├── database/       # 数据库（RDS、DynamoDB）
├── security/       # 安全（IAM、KMS）
├── monitoring/     # 监控告警（CloudWatch、CloudTrail）
└── deployment/     # 部署（ECS、CodePipeline、CDK）

scripts/
└── aws-cli/        # 常用 AWS CLI 脚本
```

## 快速导航

- [EKS 新服务上线全流程](docs/deployment/new-service-onboarding.md)（四仓库协作原理 + Runbook，跨仓库端到端）
- [EC2 操作文档](docs/compute/ec2/)
- [VPC 配置](docs/networking/vpc/)
- [IAM 权限管理](docs/security/iam/)
- [CDK 基础设施即代码](docs/deployment/cdk/)（okj-cdk-exchange 项目学习指南）
- [CDK8s Kubernetes 资源管理](docs/deployment/cdk8s/)（okj-cdk8s-exchange 项目学习指南）
- [ArgoCD GitOps 部署](docs/deployment/argocd/)（okj-argo-manifests 项目学习指南）
- [运维工具箱](docs/deployment/ops-kit/)（okj-ops-kit：Aeron 探针、JVM 诊断、CI 镜像）
