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
└── deployment/     # 部署（ECS、CodePipeline）

scripts/
└── aws-cli/        # 常用 AWS CLI 脚本
```

## 快速导航

- [EC2 操作文档](docs/compute/ec2/)
- [VPC 配置](docs/networking/vpc/)
- [IAM 权限管理](docs/security/iam/)
