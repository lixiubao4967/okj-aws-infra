# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 仓库用途

本仓库是 OKJ 团队的 AWS 基础设施操作文档库，记录各 AWS 服务的操作手册与常用 CLI 脚本。文档以中文撰写。

## 文档结构

- `docs/<服务分类>/<服务名>/` — 操作文档，按 AWS 服务类型组织
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
