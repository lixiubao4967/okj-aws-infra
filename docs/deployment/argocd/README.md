# okj-argo-manifests 项目学习指南

> 源码路径：`/Users/xiubao.li/Documents/Gitlab/okj-argo-manifests`
> 技术栈：Kustomize + ArgoCD ApplicationSet + Python 脚本
> 分析日期：2026-04-11

## 这个项目是什么

`okj-argo-manifests` 是 **GitOps 仓库**，是整个部署链的最后一段：

```
okj-cdk-exchange      → 创建 EKS 集群（AWS 资源）
okj-cdk8s-exchange    → 生成 K8s 资源 YAML（代码生成）
okj-argo-manifests    → 存储 YAML + 驱动 ArgoCD 同步到集群
```

它管理**所有 OKJ 服务在 EKS 上的部署状态**，100+ 个服务的 YAML 都在这里，ArgoCD 持续监听这个仓库并保持集群状态与 Git 一致。

## 文档索引

| 文档 | 内容 |
|------|------|
| [项目架构详解](./project-architecture.md) | base/overlay 结构、ApplicationSet 自动发现、完整部署流 |
| [操作手册](./operations.md) | 添加新服务、更新镜像、常用命令 |

## 30 秒快速理解

```
base/<服务名>/     ← cdk8s CI 自动生成，不要手动改
overlays/test/<服务名>/  ← 测试环境差异（SG、Ingress、镜像 tag）
overlays/prod/<服务名>/  ← 生产环境差异
       ↓
ArgoCD ApplicationSet 自动发现 overlays/* 下的目录
       ↓
每个目录 = 一个 ArgoCD Application
       ↓
ArgoCD 执行 kustomize build → apply 到集群
```

## 两个集群

| 集群 | 环境 | 域名 | 镜像仓库账号 |
|------|------|------|------------|
| okj-exchange-test | 测试 | *.okqa.work | 097102939699 |
| okj-exchange-prod | 生产 | *.okcoin.tokyo | 738595724739 |

## 常用命令

```bash
make check           # fmt-check + lint + validate（提交前必跑）
make fmt             # 自动格式化 YAML/Shell/Python
make validate        # kustomize build 所有 overlay（验证无语法错误）
make build OVERLAY=overlays/test/okj-uno-api   # 渲染单个 overlay 预览
make create-overlay APP=okj-new-service         # 创建新服务的 overlay
make delete-app APP=okj-old-service             # 删除服务
make gen-aws-overlays                           # 生成 AWS SG/ServiceAccount patch
```
