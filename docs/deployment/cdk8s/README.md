# okj-cdk8s-exchange 项目学习指南

> 源码路径：`/Users/xiubao.li/Documents/Gitlab/okj-cdk8s-exchange`
> 技术栈：CDK8s + Go 1.25
> 分析日期：2026-04-11

## 这个项目是什么

`okj-cdk8s-exchange` 用 **Go 语言 + CDK8s 框架**，以代码方式生成 Kubernetes 资源的 YAML manifest。
它是整个部署链的"第二段"——AWS CDK 建好集群，这个项目决定集群里跑什么。

```
okj-cdk-exchange（建 EKS 集群）
       ↓
okj-cdk8s-exchange（生成 K8s 资源 YAML）
       ↓
okj-argo-manifests（Kustomize overlay + ArgoCD 部署）
```

## 文档索引

| 文档 | 内容 |
|------|------|
| [项目架构详解](./project-architecture.md) | 整体结构、Construct 层、Service 层、ArgoCD 集成 |
| [如何添加新服务](./add-new-service.md) | 完整操作步骤 + 三类服务示例 |

## 30 秒快速理解

```
服务定义（ServiceChart 接口）
       ↓
BuildFunc 调用 Construct
       ↓ cdk8s.App.Synth()
okj-charts/<服务名>/<chart>.yaml
       ↓ make publish-manifests
okj-argo-manifests（ArgoCD 读这里）
```

每个服务 = 一个 Go 文件 + registry.go 注册 + `make synth` 生成 YAML

## 常用命令

```bash
make setup    # 安装工具 + 下载 Go modules
make synth    # 生成 okj-charts/ 下的所有 YAML
make check    # fmt + lint + test（提交前必跑）
make dev      # 同 check，开发时用
make publish-manifests  # synth + 推送到 okj-argo-manifests
```

## 服务规模（截至 2026-04-11）

| 分类 | 数量 | 说明 |
|------|------|------|
| exchange/ | 42 个 | 交易所业务服务 |
| aircraft/ | 27 个 | 撮合引擎相关服务 |
| uno/ | 15 个 | CI/CD 执行平台 |
| middleware/ | ~15 个 | 日志、监控、中间件 |
| infra/ | ~8 个 | 集群基础设施（命名空间、CRD） |
| helm/ | 3 个 | Helm operator（ALB、ESO、ExternalDNS） |
