# okj-cdk-exchange 架构速查

源码位置：`/Users/xiubao.li/Documents/Gitlab/okj-cdk-exchange`

---

## 4 层结构

```
Layer 1  internal/constructs/      单个 AWS 资源封装（~33 个）
Layer 2  internal/stacks/          按域打包成 CloudFormation Stack
Layer 3  internal/props/props.go   OkjStackProps：跨 Stack 引用总线
Layer 4  cmd/main.go               按依赖顺序组装所有 Stack
```

### Layer 1：Construct 规范

```go
// 固定签名：(scope, id, props) → (*T, error)
func NewXxxConstruct(scope awsconstructsv10.Construct, id string, props *XxxProps) (*XxxConstruct, error)

// 结构体嵌入 CDK 基类
type XxxConstruct struct {
    awsconstructsv10.Construct
    resource awsxxx.Xxx   // 私有，通过 getter 暴露
}
```

**关键规范：**
- 指针字段 = 可选（nil 时跳过）
- 调用方用 `log.Panicf` 处理 error，不允许 `_, _ =` 忽略
- CDK jsii 调用（`awsiam.NewPolicyStatement` 等）**只能在 Stack 层**，不能在 Construct 内

### Layer 2：Stack 规范

```go
type XxxStack struct {
    awscdk.Stack
    // 暴露给外部引用的字段（如 SubnetGroup、ClusterEndpoint）
}

func NewXxxStack(scope, id, props) *XxxStack {
    stack.Stack = awscdk.NewStack(scope, &id, &props.StackProps)
    // ... 创建资源
}

// 配置工厂函数：固化项目约定，调用方只传动态参数
func ConstructXxxConfig(env string, ...) *XxxConfig { ... }
```

### Layer 3：OkjStackProps（总线）

所有 Stack 创建后放入 `OkjStackProps`，后续 Stack 通过它取跨域引用：
- `okjProps.SubnetStack.GetPrivateSubnets()`
- `okjProps.SecurityGroupStack.SdValkeyAircraft`
- `okjProps.IAMStack.GlobalSecretReadPolicy`

---

## main.go 创建顺序

```
1. network:   SubnetStack → SecurityGroupStack
2. infra:     ProvisionedStack → ParamStoreStack → S3Stack → SecretStack
              → DynamoDBStack → IAMStack → AuroraStack → ECRStack → EKSStack
3. image:     RecipeStack → *ImageStack（14 种 AMI）
4. middleware: MSKStack → CacheStack（Redis/Valkey）
5. app:       frontend.* → association.*（117 个 EKS 服务）
```

---

## EKS 服务模式（最常用）

实现 `EKSService` 接口 → `EKSServiceBuilder.Build()` 自动创建 IAM Role + Pod Identity + ECR Repo：

```go
type MyService struct{ okjProps *props.OkjStackProps }
func (s *MyService) Name() string      { return "okj-my-service" }
func (s *MyService) Namespace() string { return "okj-exchange" }
func (s *MyService) ManagedPolicies()  []awsiam.IManagedPolicy { ... }
func (s *MyService) PolicyStatements() []awsiam.PolicyStatement { return nil }
func (s *MyService) ContainerBuilder() *application.ContainerBuildConfig { return nil }
```

---

## 子网类型（SubnetStack 提供）

| 方法 | 用途 |
|------|------|
| `GetPrivateSubnets()` | EKS Pod、EC2 应用实例 |
| `GetStorageSubnets()` | RDS Aurora、ElastiCache、MSK |
| `GetEgressSubnets()` | AMI 构建（需 NAT 出口） |
| `GetS3Endpoint()` | VPC Endpoint 引用 |

---

## 安全组使用模式

统一由 `SecurityGroupStack` 预创建，其他 Stack 只引用：
- `SdValkeyAircraft` → Redis
- `SdAuroraUno` / `SdAuroraMiddleware` → Aurora
- `SdMskAircraft` → MSK Kafka

---

## 配置系统

- 入口：`cdk.json` → `"app": "go mod download && go run ./cmd/main.go"`
- 配置文件：`config/test.yaml`、`config/prod.yaml`
- 切换环境：`cdk deploy --context env=prod`
- 默认环境：`test`

---

## 技术栈版本

| 组件 | 版本 |
|------|------|
| Go | 1.26.0 |
| AWS CDK Go | v2.241.0 |
| constructs-go | v10.5.1 |
| jsii-runtime-go | v1.127.0 |
