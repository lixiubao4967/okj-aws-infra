---
name: add-cdk-resource
description: 向 cdk-mini 或 okj-cdk-exchange 添加新的 AWS 资源时使用。涵盖从 Construct 到 Stack 到 main.go 的完整流程。
---

# 添加新 CDK 资源 SOP

## 前置检查

先读 memory-bank，不要重新分析源码：
```
memory-bank/cdk-architecture.md    ← 4 层架构和规范
memory-bank/cdk-pitfalls.md        ← 常见错误
```

---

## 步骤一：创建 Construct

文件：`internal/constructs/<service>_construct.go`

```go
type XxxConstructProps struct {
    Name        string
    OptionalVal *string   // 指针 = 可选
}

type XxxConstruct struct {
    awsconstructsv10.Construct
    resource awsxxx.Xxx
}

func NewXxxConstruct(scope awsconstructsv10.Construct, id string, props *XxxConstructProps) (*XxxConstruct, error) {
    if err := validateXxxProps(props); err != nil {
        return nil, err
    }
    var c XxxConstruct
    c.Construct = awsconstructsv10.NewConstruct(scope, &id)
    // 创建资源...
    return &c, nil
}

func (c *XxxConstruct) GetResource() awsxxx.Xxx { return c.resource }
```

---

## 步骤二：创建 Stack

文件：`internal/stacks/<domain>/<service>_stack.go`

```go
type XxxConfig struct { /* Stack 级别配置 */ }

type XxxStackProps struct {
    awscdk.StackProps
    Config *XxxConfig   // 指针 = 可选
}

type XxxStack struct {
    awscdk.Stack
    Resource *customConstructs.XxxConstruct   // 暴露供外部引用
}

func NewXxxStack(scope, id, props) *XxxStack {
    var stack XxxStack
    stack.Stack = awscdk.NewStack(scope, &id, &props.StackProps)
    if props.Config != nil {
        resource, err := customConstructs.NewXxxConstruct(stack.Stack, id, &...)
        if err != nil {
            panic(fmt.Sprintf("failed to create xxx: %v", err))
        }
        stack.Resource = resource
    }
    return &stack
}

// 配置工厂函数：固化项目约定
func ConstructXxxConfig(env string, ...) *XxxConfig { return &XxxConfig{...} }
```

---

## 步骤三（可选）：加入 Props 总线

文件：`internal/props/props.go`

```go
type MiniStackProps struct {
    // ... 现有字段
    XxxStack *xxx.XxxStack  // 新增
}
```

---

## 步骤四：接入 main.go

```go
// 按依赖顺序，在合适位置添加
xxxStack := setupXxxStack(app, env, awsEnv, envConfig)

// 如果加入总线
okjProps := &props.MiniStackProps{
    // ... 现有字段
    XxxStack: xxxStack,
}

// 文件底部添加工厂函数
func setupXxxStack(app, env, awsEnv, envConfig) *xxx.XxxStack {
    return xxx.NewXxxStack(app, "cdk-mini-xxx-"+env, &xxx.XxxStackProps{
        StackProps: awscdk.StackProps{Env: awsEnv, Description: jsii.String("...")},
        Config: xxx.ConstructXxxConfig(envConfig.Xxx.Name),
    })
}
```

---

## 步骤五：验证

```bash
go build ./...       # 无编译错误
cdk synth            # 无 synth 错误，输出 CloudFormation 模板
cdk diff             # 预览与现有部署的差异
```

---

## 踩过的坑

| 问题 | 原因 | 解决 |
|------|------|------|
| `nil pointer dereference` | jsii 函数在 Construct 层调用 | 移到 Stack 层 |
| `unreachable code` | 函数内有两个 `return` | 删掉多余的 |
| 缩进混乱导致 `gofmt` 失败 | 混用 tab/space | 用 `gofmt -w .` 修复 |
| S3 桶删不掉 | AutoCleanup: false | 手动 `aws s3 rm s3://bucket --recursive` |
| DynamoDB 表残留 | AutoCleanup: false（预期行为） | 手动 `aws dynamodb delete-table` |
