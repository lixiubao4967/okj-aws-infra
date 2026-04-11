---
description: CDK Go 项目操作规则，适用于 practice/cdk-mini 练习项目和 okj-cdk-exchange 源码分析
globs: ["practice/**/*.go", "practice/**/cdk.json", "practice/**/go.mod"]
---

# CDK Go 操作规则

## 读源码前先查 memory-bank

修改或分析 CDK 代码前，**先读以下文件**，避免重复分析 okj-cdk-exchange 源码：

- `memory-bank/cdk-architecture.md` — 4 层架构、Construct/Stack 规范、main.go 创建顺序
- `memory-bank/cdk-services-map.md` — 117 个服务的目录映射、AWS 资源与 Construct 对照表
- `memory-bank/cdk-pitfalls.md` — 已知陷阱、设计决策、常用命令

## 编码约定

- Construct 构造函数固定签名：`(scope, id, props) → (*T, error)`
- 调用方用 `log.Panicf` 处理 error，**不允许** `_, _ =` 忽略
- `awsiam.NewPolicyStatement` 等 jsii 函数**只能在 Stack 层**，不能在 Construct 内
- 所有字符串参数用 `jsii.String()`，布尔值用 `jsii.Bool()`
- `RemovalPolicy_DESTROY` 配合 `AutoDeleteObjects` 使用（S3）

## 添加新 AWS 资源的步骤

1. `internal/constructs/xxx_construct.go` — 封装单个资源
2. `internal/stacks/<domain>/xxx_stack.go` — 打包成 Stack + 配置工厂函数
3. `internal/props/props.go` — 如需跨 Stack 引用，加入总线
4. `cmd/main.go` — 按依赖顺序接入

## AWS 凭证安全

- 凭证通过 `aws configure` 或环境变量设置，**不写入任何项目文件**
- `config/personal.yaml` 含账号 ID，不提交到公共仓库

## 验证命令

```bash
cd practice/cdk-mini
go build ./...      # 编译检查
cdk synth           # 生成 CloudFormation 模板（不部署）
cdk diff            # 对比差异
cdk deploy --all    # 部署
```
