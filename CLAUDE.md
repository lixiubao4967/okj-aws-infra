# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 仓库用途

本仓库是 OKJ 团队的 AWS 基础设施操作文档库，记录各 AWS 服务的操作手册与常用 CLI 脚本。文档以中文撰写。

## 文档结构

- `docs/<服务分类>/<服务名>/` — 操作文档，按 AWS 服务类型组织
  - `docs/compute/ec2/` — EC2 操作（克隆实例等）
  - `docs/deployment/` — 部署相关（ArgoCD / CDK / CDK8s / ops-kit）
  - `docs/storage/fsx/` — FSx 文件服务器迁移方案（Samba+LDAP → FSx File Gateway）
- `scripts/aws-cli/` — 可直接执行的 AWS CLI Shell 脚本

## 文档规范

新增操作文档时遵循以下约定：
- 文件命名使用小写连字符，如 `clone-instance.md`
- 若操作有控制台和 CLI 两种方式，优先写控制台方式（标注「推荐」），再写 CLI 方式
- 注意事项统一用表格呈现
- 若文档对应有脚本，在文档末尾用相对路径链接到 `scripts/aws-cli/`

## 脚本规范

`scripts/aws-cli/` 下的脚本：
- 使用 `set -euo pipefail`
- 必须参数通过位置参数传入，缺失时给出明确提示
- 脚本顶部注释说明用法和示例

## 内部系统地址

文档中涉及以下内部系统，引用时使用固定链接：

| 系统 | 地址 | 用途 |
|------|------|------|
| ClickHouse Log View | https://log-view.okcoin.tokyo/query | 日志监控 |
| Nacos | https://nacos-admin.okcoin.tokyo/ | 服务注册与配置中心 |
| Grafana | https://grafana.okcoin.tokyo/ | 监控指标看板 |

## 实例命名规范

EC2 实例名按可用区区分，格式为 `<服务名>-<az><序号>`，如：
- `app-c01` — c 区第 1 台
- `app-a01` — a 区第 1 台（从 c 区克隆时需相应修改名称）

---

## CDK 知识库（memory-bank）

本仓库包含对 `okj-cdk-exchange` 的深度分析结果，**遇到 CDK 相关问题先读 memory-bank，不要重新分析源码**：

| 文件 | 内容 |
|------|------|
| `memory-bank/cdk-architecture.md` | 4 层架构（Construct/Stack/Props/main.go）、规范、创建顺序 |
| `memory-bank/cdk-services-map.md` | 117 个服务目录、AWS 资源 → Construct 对照表 |
| `memory-bank/cdk-pitfalls.md` | 已知陷阱、设计决策、常用命令 |
| `memory-bank/progress.md` | 当前进度和待完成事项 |

**okj-cdk-exchange 源码位置**：`/Users/xiubao.li/Documents/Gitlab/okj-cdk-exchange`（只在 memory-bank 信息不足时才去读源码）

## CDK 练习项目

`practice/cdk-mini/` — 复刻 okj-cdk-exchange 4 层架构的个人 AWS 部署练习，部署资源：S3 + SNS + DynamoDB。

部署指南：`practice/cdk-mini/docs/deploy.md`

## CDK8s 知识库（memory-bank）

本仓库包含对 `okj-cdk8s-exchange` 的深度分析结果，**遇到 CDK8s 相关问题先读 memory-bank**：

| 文件 | 内容 |
|------|------|
| `memory-bank/cdk8s-architecture.md` | 4 层架构（Spec/Builder/Construct/App）、5 个服务分层、编码约定 |

**okj-cdk8s-exchange 源码位置**：`/Users/xiubao.li/Documents/Gitlab/okj-cdk8s-exchange`

## CDK8s 练习项目

`practice/cdk8s-mini/` — 复刻 okj-cdk8s-exchange 4 层架构的个人 K8s manifests 练习，生成资源：
- **StatelessApp**：Nginx Deployment + ClusterIP Service（services/web/）
- **StatelessApp**：Redis Deployment + ClusterIP Service（services/cache/）
- **StatefulApp**：Redis StatefulSet + Headless Service + volumeClaimTemplates（services/cache/）

架构层次：`internal/spec/` → `services/` → `internal/constructs/` → `cmd/main.go`

已知坑：cdk8s Go JSII binding v2.70 的 K8s Quantity 序列化 bug 导致 `storage: null`，部署前需手动改为 `storage: 1Gi`。
