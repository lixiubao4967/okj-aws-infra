# 如何添加新资源

本文通过两个完整示例演示如何向 okj-cdk-exchange 项目添加新的 AWS 资源：

- **示例 A**：添加中间件类资源（以 SNS Topic 为例）
- **示例 B**：添加 EKS 应用服务（以新 API 服务为例）

---

## 前置知识

添加任何资源，最少需要修改/创建 3 个文件：

```
internal/constructs/xxx_construct.go   ← 新建
internal/stacks/[domain]/xxx_stack.go  ← 新建
cmd/main.go                            ← 修改（接入）
```

如果资源需要加入"总线"供其他 Stack 使用，还需要：
```
internal/props/props.go                ← 修改（加字段）
```

---

## 示例 A：添加中间件资源（SNS Topic）

### Step 1：创建 Construct

**文件：** `internal/constructs/sns_topic_construct.go`

Construct 是对单个 AWS 资源的封装，职责是：校验参数、创建资源、提供 getter。

```go
package constructs

import (
    "errors"
    "fmt"

    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/aws-cdk-go/awscdk/v2/awssns"
    awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
)

// Props：所有配置项，指针类型 = 可选
type SNSTopicConstructProps struct {
    TopicName   string
    DisplayName *string            // 可选：人类可读名称
    Tags        map[string]string
}

// Construct 结构体：嵌入 CDK 基类 + 暴露资源
type SNSTopicConstruct struct {
    awsconstructsv10.Construct
    topic awssns.Topic
}

// 构造函数：固定签名 (scope, id, props) -> (*T, error)
func NewSNSTopicConstruct(
    scope awsconstructsv10.Construct,
    id string,
    props *SNSTopicConstructProps,
) (*SNSTopicConstruct, error) {
    if err := validateSNSTopicProps(props); err != nil {
        return nil, err
    }

    var construct SNSTopicConstruct
    construct.Construct = awsconstructsv10.NewConstruct(scope, &id)

    construct.topic = awssns.NewTopic(construct.Construct, jsii.String(id), &awssns.TopicProps{
        TopicName:   jsii.String(props.TopicName),
        DisplayName: props.DisplayName,
    })

    // 打标签
    for k, v := range props.Tags {
        awscdk.Tags_Of(construct.topic).Add(jsii.String(k), jsii.String(v), nil)
    }

    return &construct, nil
}

// Getter 方法：让外部 Stack 能访问资源属性
func (s *SNSTopicConstruct) GetTopic() awssns.Topic {
    return s.topic
}

func (s *SNSTopicConstruct) GetTopicArn() *string {
    return s.topic.TopicArn()
}

func validateSNSTopicProps(props *SNSTopicConstructProps) error {
    if props == nil {
        return errors.New("SNSTopicConstructProps cannot be nil")
    }
    if props.TopicName == "" {
        return errors.New("TopicName is required")
    }
    return nil
}
```

---

### Step 2：创建 Stack

**文件：** `internal/stacks/middleware/sns_stack.go`

Stack 职责：把 Construct 打包成部署单元，管理前置依赖，提供配置工厂函数。

```go
package middleware

import (
    "fmt"

    customConstructs "okj-cdk-exchange/internal/constructs"

    "github.com/aws/aws-cdk-go/awscdk/v2"
    awsconstructsv10 "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
)

// Config：Stack 级别的配置（比 ConstructProps 更高层，可省略低层细节）
type SNSTopicConfig struct {
    TopicName   string
    DisplayName *string
    Tags        map[string]string
}

// StackProps：嵌入 awscdk.StackProps（含 Account/Region/Description）
type SNSStackProps struct {
    awscdk.StackProps
    NotificationTopic *SNSTopicConfig // 指针 = 可选
}

// Stack 结构体：嵌入 awscdk.Stack + 暴露 Construct 供外部访问
type SNSStack struct {
    awscdk.Stack
    NotificationTopic *customConstructs.SNSTopicConstruct
}

// NewSNSStack：Stack 构造函数
func NewSNSStack(
    scope awsconstructsv10.Construct,
    id string,
    props *SNSStackProps,
) *SNSStack {
    var stack SNSStack
    stack.Stack = awscdk.NewStack(scope, &id, &props.StackProps)

    if props.NotificationTopic != nil {
        stack.createNotificationTopic(props.NotificationTopic)
    }

    return &stack
}

func (s *SNSStack) createNotificationTopic(config *SNSTopicConfig) {
    topicProps := &customConstructs.SNSTopicConstructProps{
        TopicName:   config.TopicName,
        DisplayName: config.DisplayName,
        Tags:        config.Tags,
    }

    topic, err := customConstructs.NewSNSTopicConstruct(
        s.Stack, config.TopicName, topicProps)
    if err != nil {
        // 项目约定：构造失败直接 panic，在 cdk synth 阶段暴露问题
        panic(fmt.Sprintf("failed to create SNS topic %s: %v", config.TopicName, err))
    }

    s.NotificationTopic = topic
}

// 配置工厂函数：固化项目约定，调用方只需传 env + 动态参数
func ConstructNotificationTopicConfig(env string) *SNSTopicConfig {
    return &SNSTopicConfig{
        TopicName:   fmt.Sprintf("okj-notifications-%s", env),
        DisplayName: jsii.String("OKJ Exchange Notifications"),
        Tags: map[string]string{
            "Service":     "notifications",
            "Environment": env,
        },
    }
}
```

---

### Step 3（可选）：加入 OkjStackProps 总线

如果其他 Stack 需要引用这个 SNS Topic（比如 Lambda 订阅），需要把它加进总线。

**文件：** `internal/props/props.go`

```go
type OkjStackProps struct {
    // ... 现有字段 ...
    SNSStack *middleware.SNSStack   // 新增
}
```

---

### Step 4：在 main.go 中接入

**文件：** `cmd/main.go`

```go
func main() {
    // ... 现有代码 ...

    // 在 CacheStack 之后添加 SNS Stack（它不依赖缓存，但习惯上中间件一起创建）
    snsStack := SetupSNSStack(app, env, *envConfig)

    // 如果加入了总线，把它放进 okjProps
    okjProps := &props.OkjStackProps{
        // ... 现有字段 ...
        SNSStack: snsStack,
    }

    // ... 后续代码 ...
}

// 在文件底部添加工厂函数
func SetupSNSStack(
    app awscdk.App,
    env string,
    envConfig config.EnvConfig,
) *middleware.SNSStack {
    return middleware.NewSNSStack(app, "okj-exchange-sns-stack-"+env, &middleware.SNSStackProps{
        StackProps: awscdk.StackProps{
            Env: &awscdk.Environment{
                Account: jsii.String(envConfig.Account),
                Region:  jsii.String(envConfig.Region),
            },
            Description: jsii.String("SNS notification topics for OKJ Exchange"),
        },
        NotificationTopic: middleware.ConstructNotificationTopicConfig(env),
    })
}
```

---

## 示例 B：添加 EKS 应用服务

EKS 服务有专属模式：通过 `EKSService` 接口声明需求，由 `EKSServiceBuilder` 自动组装 Stack。
这种模式不需要手写 Stack，只需实现接口。

### Step 1：实现 EKSService 接口

**文件：** `services/uno/okj_my_api.go`（参考现有的 `okj_uno_api.go`）

```go
package uno

import (
    "okj-cdk-exchange/internal/props"
    "okj-cdk-exchange/internal/stacks/application"

    "github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
)

type OkjMyAPIService struct {
    okjProps *props.OkjStackProps
}

func NewOkjMyAPIService(okjProps *props.OkjStackProps) *OkjMyAPIService {
    return &OkjMyAPIService{okjProps: okjProps}
}

// Name：CDK Stack ID，也是 ECR 仓库名、IAM Role 名的前缀
func (s *OkjMyAPIService) Name() string { return "okj-my-api" }

// Namespace：Kubernetes namespace
func (s *OkjMyAPIService) Namespace() string { return "okj-exchange" }

// ManagedPolicies：需要挂载的 AWS 托管策略
func (s *OkjMyAPIService) ManagedPolicies() []awsiam.IManagedPolicy {
    return []awsiam.IManagedPolicy{
        s.okjProps.IAMStack.GlobalSecretReadPolicy,  // 读取 Secrets Manager
    }
}

// PolicyStatements：自定义内联策略（如需要访问特定 SNS）
func (s *OkjMyAPIService) PolicyStatements() []awsiam.PolicyStatement {
    return []awsiam.PolicyStatement{
        awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
            Actions:   jsii.Strings("sns:Publish"),
            Resources: jsii.Strings(*s.okjProps.SNSStack.NotificationTopic.GetTopicArn()),
        }),
    }
}

// ContainerBuilder：nil = 镜像由外部 CI 构建；非 nil = CDK 创建 CodeBuild 流水线
func (s *OkjMyAPIService) ContainerBuilder() *application.ContainerBuildConfig { return nil }
```

### Step 2：在 association 或 service setup 中注册

**文件：** `services/uno/setup.go`（或现有的 `SetupEKSService` 调用处）

```go
func SetupUnoServices(okjProps *props.OkjStackProps, envConfig config.EnvConfig, env string) {
    builder := services.NewEKSServiceBuilder(okjProps, envConfig, env)

    // 现有服务
    builder.Build(uno.NewOkjUnoAPIService(okjProps))

    // 新增服务
    builder.Build(uno.NewOkjMyAPIService(okjProps))
}
```

Builder 会自动为你创建：
- IAM Role（信任 EKS Pod Identity）
- EKS Pod Identity Association
- ECR Repository（镜像仓库）
- S3 Bucket（如果实现了 `S3BucketProvider` 接口）

---

## 快速参考：不同资源类型对应的 Stack 目录

| 资源类型 | Stack 目录 | 示例 |
|---------|-----------|------|
| 网络（子网、SG） | `internal/stacks/network/` | `subnet_stack.go` |
| 存储（S3、DynamoDB） | `internal/stacks/infra/` | `s3_stack.go` |
| 数据库（Aurora、Redis） | `internal/stacks/middleware/` 或 `infra/` | `redis_cluster_stack.go` |
| 消息队列（MSK、SNS、SQS） | `internal/stacks/middleware/` | `msk_stack.go` |
| 应用服务（EKS Pod） | 实现 `EKSService` 接口 | `services/uno/` |
| 镜像构建（AMI、容器）| `internal/stacks/image/` | `recipe_stack.go` |

---

## 常见错误与规范

### 错误 1：在 Construct 内调用 jsii AWS 函数

```go
// 错误：会导致 nil pointer panic
func NewMyConstruct(...) {
    policy := awsiam.NewPolicyStatement(...)  // ← 不能在 Construct 里
}

// 正确：在 Stack 层创建
func (s *MyStack) createPolicy() {
    policy := awsiam.NewPolicyStatement(...)  // ← Stack 层可以
}
```

### 错误 2：构造失败不 panic

```go
// 错误：忽略了 error
construct, _ := NewRedisClusterConstruct(...)

// 正确：任何错误都在 cdk synth 时暴露
construct, err := NewRedisClusterConstruct(...)
if err != nil {
    log.Panicf("failed to create redis construct: %v", err)
}
```

### 错误 3：Stack 依赖顺序写错

如果 Stack A 依赖 Stack B 的输出，必须先创建 Stack B：

```go
// 错误：SNS 还没创建就把它传进 okjProps
okjProps := &props.OkjStackProps{SNSStack: nil}
snsStack := SetupSNSStack(...)  // ← 太晚了

// 正确：先创建，再组装
snsStack := SetupSNSStack(...)
okjProps := &props.OkjStackProps{SNSStack: snsStack}
```

### 错误 4：忽略 lint 规范

项目 lint 规则严格（`cyclop`、`wsl_v5`、`gofumpt`），提交前必须运行：

```bash
make check   # 自动格式化 + lint + 测试
```

---

## 验证新资源

```bash
# 1. 语法检查
go build ./...

# 2. lint + 测试
make check

# 3. 预览生成的 CloudFormation 模板（不部署）
cdk synth --context env=test

# 4. 对比与当前部署的差异
cdk diff --context env=test
```
