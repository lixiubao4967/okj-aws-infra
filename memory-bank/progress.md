# 进度记录

## 已完成

- [x] 阅读 okj-cdk-exchange 源码，整理 4 层架构文档（2026-04-11）
  - 文档：`docs/deployment/cdk/project-architecture.md`
  - 文档：`docs/deployment/cdk/add-new-resource.md`
- [x] 搭建 `practice/cdk-mini` 练习项目（2026-04-11）
  - 复刻 okj-cdk-exchange 的 4 层架构
  - 部署资源：S3 + SNS Topic + DynamoDB（低成本，月费接近 $0）
  - 完成所有代码，编译通过
  - 文档：`practice/cdk-mini/docs/deploy.md`

## 待完成（明天继续）

- [ ] 安装 CDK CLI：`npm install -g aws-cdk`
- [ ] 配置个人 AWS 凭证：`aws configure`
- [ ] 填写 `practice/cdk-mini/config/personal.yaml`（替换 `YOUR_AWS_ACCOUNT_ID`）
- [ ] 执行 CDK Bootstrap：`cdk bootstrap aws://<账号>/<区域>`
- [ ] 首次部署：`cdk deploy --all`
- [ ] 验证 3 个资源创建成功
- [ ] 执行清理：`cdk destroy --all` + 手动删 DynamoDB 表

## 下一步可探索

- [ ] 在 cdk-mini 中添加第 4 个资源（如 SQS Queue），完整走一遍"添加新资源"流程
- [ ] 对照 `okj-cdk-exchange` 的 `services/uno/` 实现一个 EKS 服务接口（需要 EKS 环境）
- [ ] 研究 okj-cdk-exchange 的 `internal/stacks/infra/eks_stack.go`，理解 EKS 集群配置方式

## 参考文档

| 文档 | 路径 |
|------|------|
| CDK 架构详解 | `docs/deployment/cdk/project-architecture.md` |
| 添加新资源 SOP | `docs/deployment/cdk/add-new-resource.md` |
| cdk-mini 部署指南 | `practice/cdk-mini/docs/deploy.md` |
| CDK 架构速查 | `memory-bank/cdk-architecture.md` |
| 服务与资源速查 | `memory-bank/cdk-services-map.md` |
| 陷阱与约定 | `memory-bank/cdk-pitfalls.md` |
