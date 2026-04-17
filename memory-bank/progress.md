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

## cdk-mini 进阶练习（按顺序）

> 已理解 4 层架构，以下任务逐步覆盖真实 okj-cdk-exchange 的核心模式

- [ ] **任务 3：多环境隔离**（可独立先做）
  - 新增 `config/dev.yaml`、`config/prod.yaml`
  - dev：AutoCleanup=true；prod：AutoCleanup=false + DeletionProtection
  - 验证：`cdk deploy --context env=dev` vs `env=prod`

- [ ] **任务 1：IAM 跨 Stack 引用**（其他任务的前置）
  - 新建 IAM Policy，用 `S3Stack.PracticeBucket.GetBucketArn()` 跨 Stack 取 ARN
  - 真正用起来 `internal/props/props.go` 的 MiniStackProps 总线

- [ ] **任务 4：CfnOutput 输出**（理解 Props 传值后再对比）
  - 在 Stack 中用 `CfnOutput` 导出 ARN，用 `Fn.importValue()` 在其他 Stack 引用
  - 对比与 Props 总线传值的区别：编译期引用 vs CloudFormation 运行期引用

- [ ] **任务 2：DynamoDB + Lambda 联动**（依赖任务 1 的 IAM 模式）
  - 新建 Lambda Construct + LambdaStack，读写 DynamoDB
  - 跨 Stack IAM 授权：`DynamoDBStack.TasksTable.GetTableArn()`

## CDK8s 练习（practice/cdk8s-mini）

- [x] 分析 okj-cdk8s-exchange 架构，整理文档（2026-04-16）
  - 文档：`memory-bank/cdk8s-architecture.md`
- [x] 搭建 `practice/cdk8s-mini` 骨架（2026-04-16）
  - 复刻 okj-cdk8s-exchange 的 4 层架构：Spec / Construct / Registry / main.go
  - 实现 StatelessAppConstruct（Deployment + Service + stable selector patch）
  - 完成 NginxChart 服务，`go run cmd/main.go` 生成 YAML 验证通过
  - 输出：`mini-charts/nginx/nginx.yaml`

- [x] **任务 2**：NginxChart 实现 `Resources()` 覆盖 cdk8s-plus 默认资源限制（2026-04-17）
- [x] **任务 3**：添加 `services/cache/redis.go`，完整走一遍"新增服务"流程（2026-04-17）
- [x] **任务 4**：实现 StatefulApp 模式（StatefulSet + Headless Service + volumeClaimTemplates）（2026-04-17）
- [x] **修复**：nginx/redis 的 `readOnlyRootFilesystem: false`（2026-04-17）

### cdk8s-mini 已知坑

- cdk8s Go JSII binding v2.70：K8s Quantity 类型在 JsonPatch 上下文中序列化为 null
  - 影响：StatefulSet volumeClaimTemplates 的 `storage` 字段为 null
  - 绕过：生成 YAML 后手动改 `storage: null` → `storage: 1Gi`
  - 根本解：改用 `k8s.NewKubeStatefulSet`（完整 typed API）

- [x] 分析 `okj-argo-manifests` 架构（2026-04-17）
  - 文档：`memory-bank/argo-architecture.md`
  - 模式：ApplicationSet + Git 目录生成器 + Kustomize base/overlays
  - cdk8s 生成 base/ → overlays/ 环境特化 → ArgoCD 自动同步到 EKS
- [x] 搭建 `practice/argo-mini` 练习项目（2026-04-17）
  - 复刻 okj-argo-manifests 三层 GitOps 结构：base / overlays / argocd
  - 服务：nginx + redis（复用 cdk8s-mini 生成的 YAML）
  - dev/prod 两套 overlay（副本数、资源限制差异化）
  - ApplicationSet + AppProject YAML，含 git-directory 生成器
  - 文档：`practice/argo-mini/README.md`（含 4 个本地练习任务）

## 本地 K8s 环境（k3s）

- [x] 在 Mac M2 上搭建本地 K8s 集群（2026-04-17）
  - 方案：Multipass（Ubuntu 22.04 VM）+ k3s
  - VM：`multipass launch -n k3s-server -c 2 -m 2G -d 10G 22.04`
  - k3s 安装：`curl -sfL https://get.k3s.io | sh -`
  - kubeconfig 手动合并到 `~/.kube/config`，context 名 `k3s`
  - 坑：cluster name 必须填写，空字符串会导致 `cannot locate cluster k3s`
  - 验证：`kubectl get nodes` → `k3s-server Ready`
- [ ] 将 cdk8s-mini 部署到 k3s

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
