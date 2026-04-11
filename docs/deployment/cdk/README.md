# okj-cdk-exchange 项目学习指南

> 源码路径：`/Users/xiubao.li/Documents/Gitlab/okj-cdk-exchange`
> 技术栈：AWS CDK v2.236.0 + Go 1.26.0
> 分析日期：2026-04-11

## 这个项目是什么

`okj-cdk-exchange` 是用 **AWS CDK（Go 语言版）** 编写的基础设施即代码（IaC）项目。
它管理整个 OKJ Exchange 平台在 AWS 上的网络、存储、中间件、应用服务等所有云资源。

部署时只需运行：
```bash
cdk synth --context env=prod   # 生成 CloudFormation 模板（只预览，不部署）
cdk deploy --context env=prod  # 真正部署到 AWS
cdk diff --context env=prod    # 对比当前部署和代码的差异
```

## 文档索引

| 文档 | 内容 |
|------|------|
| [项目架构详解](./project-architecture.md) | 4 层结构、文件组织、数据流 |
| [如何添加新资源](./add-new-resource.md) | 完整操作手册 + 实例演示 |

## 30 秒快速理解

```
Construct（积木）→ Stack（装箱）→ main.go（运货）
```

- **Construct**：一个 AWS 资源的封装（如 Redis、S3、RDS），带参数校验和 getter
- **Stack**：把一组相关 Construct 打包成一个 CloudFormation Stack
- **Service**（可选）：声明一个 EKS 应用需要什么权限/身份
- **main.go**：按依赖顺序创建所有 Stack，组装成完整基础设施

## 常用开发命令

```bash
make check    # 格式化 + vet + lint + 测试（每次提交前必跑）
make validate # 提 PR 前运行
go test ./... # 单独运行测试
```

## 环境说明

项目支持两个环境，通过 `--context env=xxx` 切换：

| 环境 | 配置文件 |
|------|---------|
| `test` | `config/test.yaml` |
| `prod` | `config/prod.yaml` |
