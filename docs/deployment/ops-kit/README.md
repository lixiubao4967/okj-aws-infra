# okj-ops-kit 项目学习指南

> 源码路径：`/Users/xiubao.li/Documents/Gitlab/okj-ops-kit`
> 技术栈：Rust 1.93.1 + Python 3.14 + Docker（Debian 12）
> 分析日期：2026-04-11

## 这个项目是什么

`okj-ops-kit` 是整个 OKJ 平台的**运维工具箱**，提供三样东西：

| 产出物 | 用途 | 消费者 |
|-------|------|-------|
| `aeron-toolkit` 二进制 | Aeron 集群健康探针（sidecar） | Jove 系列撮合引擎 Pod |
| `jvm-toolkit` 二进制 | JVM 诊断工具（内存/连接/GC） | 所有 Java 服务 Pod |
| `okj-k8s-ci` Docker 镜像 | 统一 CI 基础镜像 | okj-cdk-exchange / okj-cdk8s-exchange / okj-argo-manifests |

二进制产物发布到 S3，容器构建时从 S3 拉取注入；CI 镜像推送到 ECR，所有仓库的 CI Job 都用它。

## 文档索引

| 文档 | 内容 |
|------|------|
| [工具详解](./tools.md) | aeron-toolkit 和 jvm-toolkit 的设计原理与使用方法 |
| [CI 镜像管理](./ci-image.md) | okj-k8s-ci 镜像构成、维护、版本升级流程 |

## 30 秒快速理解

```
开发者写 Rust 代码
       ↓ make check（fmt + lint + test）
GitLab CI 编译 musl 静态二进制（amd64/arm64）
       ↓ 烟雾测试验证退出码
上传到 S3（版本变更时）
       ↓
应用容器 Dockerfile 从 S3 拉取二进制
       ↓ 注入到 Pod
Kubernetes 健康探针 / 运维工程师直接使用
```

## 常用命令

```bash
make check         # fmt + lint + test（提交前必跑）
make fmt           # 自动格式化（Rust/Python/Shell/Markdown/YAML）
make test          # 运行所有 Rust 单元测试
make build-release # 本地构建 release 二进制
make ci-build      # 本地构建 okj-k8s-ci 镜像
make ci-push       # 多架构构建并推送镜像到 registry
```

## 与其他项目的关系

```
okj-ops-kit
  ├── aeron-toolkit → 注入到 okj-jove-* Pod（sidecar 模式）
  ├── jvm-toolkit   → 注入到所有 Java 服务 Pod（CLI 诊断）
  └── okj-k8s-ci    → 被以下仓库 CI 使用：
       ├── okj-cdk-exchange（Go + aws-cdk）
       ├── okj-cdk8s-exchange（Go + cdk8s）
       └── okj-argo-manifests（kustomize + yamllint）
```
