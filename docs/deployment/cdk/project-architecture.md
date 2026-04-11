# okj-cdk-exchange 架构详解

## 目录结构一览

```
okj-cdk-exchange/
├── cmd/main.go                          # 入口：按顺序创建所有 Stack
├── config/
│   ├── test.yaml                        # 测试环境配置（账号、VPC ID、AMI 等）
│   └── prod.yaml                        # 生产环境配置
├── internal/
│   ├── config/config.go                 # YAML → EnvConfig 结构体
│   ├── props/props.go                   # OkjStackProps：所有 Stack 的"总线"
│   ├── constructs/                      # Layer 1：单个 AWS 资源封装
│   │   ├── redis_cluster_construct.go
│   │   ├── msk_construct.go
│   │   ├── s3_construct.go
│   │   ├── aurora_construct.go
│   │   └── ...（约 30 个 construct）
│   └── stacks/
│       ├── network/                     # Layer 2：网络 Stack
│       │   ├── subnet_stack.go
│       │   └── security_group_stack.go
│       ├── infra/                       # Layer 2：核心基础设施 Stack
│       │   ├── s3_stack.go
│       │   ├── aurora_stack.go
│       │   ├── eks_stack.go
│       │   └── ...
│       ├── middleware/                  # Layer 2：中间件 Stack
│       │   ├── redis_cluster_stack.go
│       │   └── msk_stack.go
│       ├── application/                 # Layer 2：应用 Stack 模板
│       │   └── okj_generic_app_stack.go
│       └── image/                      # AMI 镜像构建 Stack
├── services/
│   ├── structs.go                       # EKSService 接口定义
│   ├── builder.go                       # 接口 → Stack 的组装器
│   ├── helper/                          # 各类 Stack 的配置工厂函数
│   └── uno/                             # UNO 服务具体实现
└── go.mod
```

---

## 4 层架构详解

### Layer 1：Construct（积木）

**位置：** `internal/constructs/`
**职责：** 封装单个 AWS 资源，做参数校验，提供 getter

```go
// 典型结构：以 Redis 为例
type RedisClusterConstructProps struct {
    ClusterName          string
    Engine               *string        // "redis" 或 "valkey"
    EngineVersion        *string        // 如 "7.0"
    NodeType             string         // 如 "cache.t4g.micro"
    NumNodeGroups        int
    SecurityGroup        *SecurityGroupConstruct
    SubnetGroup          awselasticache.CfnSubnetGroup
    Route53HostedZone    awsroute53.IHostedZone
    // ...
}

type RedisClusterConstruct struct {
    awsconstructsv10.Construct           // 嵌入 CDK Construct 基类
    cluster   awselasticache.CfnReplicationGroup
    dnsRecord *Route53CnameRecordConstruct
}

func NewRedisClusterConstruct(scope, id string, props *...) (*RedisClusterConstruct, error) {
    // 1. 校验 props
    // 2. 创建 CfnReplicationGroup（底层 CloudFormation 资源）
    // 3. 可选：创建 Route53 CNAME 记录
    return &construct, nil
}

// getter 方法
func (r *RedisClusterConstruct) GetCluster() awselasticache.CfnReplicationGroup
func (r *RedisClusterConstruct) GetConfigurationEndpoint() *string
```

**关键规范：**
- 构造函数返回 `(*T, error)`，调用方用 `log.Panicf` 处理错误
- 指针字段表示"可选"，nil 时跳过
- CDK jsii 调用（如 `awsiam.NewPolicyStatement`）只能在 Stack 层，不能在 Construct 内

---

### Layer 2：Stack（装箱单元）

**位置：** `internal/stacks/`
**职责：** 把相关 Construct 打包成一个 CloudFormation Stack，管理前置依赖（如子网组、安全组引用）

```go
// 典型结构：以 Redis Stack 为例
type RedisClusterConfig struct {
    ClusterName   string
    NodeType      string
    Subnets       []awsec2.ISubnet         // 来自 SubnetStack
    SecurityGroup *SecurityGroupConstruct   // 来自 SecurityGroupStack
    // ...
}

type RedisClusterStack struct {
    awscdk.Stack                           // 嵌入 CDK Stack 基类
    SubnetGroup  awselasticache.CfnSubnetGroup
    RedisCluster *RedisClusterConstruct    // 暴露给外部使用
}

func NewRedisClusterStack(scope, id string, props *RedisClusterStackProps) *RedisClusterStack {
    stack.Stack = awscdk.NewStack(scope, &id, &props.StackProps)
    stack.createSubnetGroup(props.Config.Subnets)  // 创建前置依赖
    stack.createRedisCluster(props.Config)
    return &stack
}

// 配置工厂函数（固化常用参数）
func ConstructAircraftValkeyConfig(env string, subnets []awsec2.ISubnet,
    sg *SecurityGroupConstruct, hostedZone awsroute53.IHostedZone) *RedisClusterConfig {
    return &RedisClusterConfig{
        ClusterName:  fmt.Sprintf("okj-aircraft-valkey-%s", env),
        Engine:       jsii.String("valkey"),
        EngineVersion: jsii.String("8.2"),
        NodeType:     "cache.t4g.micro",
        NumNodeGroups: 3,
        // ...
    }
}
```

---

### Layer 3：OkjStackProps（总线）

**位置：** `internal/props/props.go`
**职责：** 把所有已创建的 Stack 汇聚成一个结构体，方便跨 Stack 引用

```go
type OkjStackProps struct {
    App                 awsconstructsv10.Construct
    EnvConfig           *config.EnvConfig
    ProvisionedStack    *infra.ProvisionedStack    // VPC、HostedZone 引用
    ParameterStoreStack *infra.ParameterStoreStack
    S3Stack             *infra.S3Stack
    SecretStack         *infra.SecretStack
    DynamoDBStack       *infra.DynamoDBStack
    IAMStack            *infra.IAMStack            // 公共 IAM 策略
    AuroraStack         *infra.AuroraStack
    SubnetStack         *network.SubnetStack
    SecurityGroupStack  *network.SecurityGroupStack
    RecipeStack         *ami.RecipeStack
    ECRStack            *infra.ECRStack
}
```

这个结构体就像"零件仓库"——后续创建的 Stack/Service 只需传入 `okjProps`，就能拿到任何已创建的资源。

---

### Layer 4：main.go（组装线）

**位置：** `cmd/main.go`
**职责：** 按依赖顺序创建所有 Stack，最终调用 `app.Synth()` 生成 CloudFormation 模板

#### 创建顺序（有严格依赖关系）

```
1. 网络层（其他所有 Stack 都依赖）
   ├── SubnetStack          → 提供各类型子网
   └── SecurityGroupStack   → 提供各服务安全组

2. 核心基础设施
   ├── ProvisionedStack     → VPC + HostedZone 引用
   ├── ParameterStoreStack  → SSM 参数
   ├── S3Stack              → 多个 S3 桶
   ├── SecretStack          → Secrets Manager
   ├── DynamoDBStack        → DynamoDB 表
   ├── IAMStack             → 公共 IAM 策略（依赖 S3/Secret/Dynamo）
   ├── AuroraStack          → Aurora 数据库
   ├── ECRStack             → 容器镜像仓库
   └── EKSStack             → EKS 集群

3. 镜像构建
   ├── RecipeStack          → AMI 配方
   └── *ImageStack          → 各类基础镜像

4. 中间件（依赖网络层）
   ├── MskStack             → Kafka
   └── CacheStack           → Redis/Valkey

5. 应用服务（依赖所有上层）
   ├── frontend.*           → 前端部署
   └── association.*        → EKS 服务绑定
```

---

## 配置系统

### YAML 结构（config/test.yaml 节选）

```yaml
account: "097102939699"
region: "ap-northeast-1"
network:
  exchangeVpc:
    vpcId: "vpc-xxxxxxxx"
    cidrPrefix: "10.130."
  privateHostZoneId: "Z..."
  privateHostName: "internal.okcoin.japan"
  albCertificateArn: "arn:aws:acm:..."
ec2:
  imageV1: "ami-..."        # 基础 AMI ID（环境不同值不同）
s3BucketConfig:
  redshiftData: "arn:aws:s3:::okj-redshift-data-rehe"
  # ... 其他已有桶的 ARN
```

### 配置加载流程

```go
// main.go
env := getEnvironment(app)           // 从 CDK context 读 "test" 或 "prod"
envConfig, err := config.LoadConfig(env)  // 读 config/{env}.yaml

// 之后所有 Stack 都从 envConfig 取值：
envConfig.Account     // AWS 账号 ID
envConfig.Region      // 区域
envConfig.Network.ExchangeVpc.VpcId  // VPC ID
```

---

## EKS Service 模式（应用层）

这是最常用的模式，用于把一个 Go/Java 服务接入 EKS。

### 1. 定义服务（声明需要什么）

```go
type MyService struct {
    okjProps *props.OkjStackProps
}

func (s *MyService) Name() string      { return "okj-my-service" }
func (s *MyService) Namespace() string { return "okj-exchange" }

func (s *MyService) ManagedPolicies() []awsiam.IManagedPolicy {
    return []awsiam.IManagedPolicy{
        s.okjProps.IAMStack.GlobalSecretReadPolicy,  // 读取 Secrets
    }
}

func (s *MyService) PolicyStatements() []awsiam.PolicyStatement { return nil }
func (s *MyService) ContainerBuilder() *application.ContainerBuildConfig { return nil }
```

### 2. Builder 自动组装 Stack

```go
builder := services.NewEKSServiceBuilder(okjProps, envConfig, env)
stack := builder.Build(new(MyService))
// 自动创建：IAM Role、Pod Identity Association、ECR Repository
```

---

## 安全组使用模式

安全组由 `SecurityGroupStack` **集中预创建**，其他 Stack **只引用**，不自行创建：

```go
// SecurityGroupStack 创建
SdValkeyAircraft *SecurityGroupConstruct  // Redis SG
SdAuroraUno      *SecurityGroupConstruct  // Aurora SG
SdMskAircraft    *SecurityGroupConstruct  // MSK SG

// 其他 Stack 引用（传入即可，不再创建）
ConstructAircraftValkeyConfig(
    env,
    subnetStack.GetStorageSubnets(),
    securityGroupStack.SdValkeyAircraft,  // 直接引用
    provisionStack.GetPrivateHostedZone(),
)
```

---

## 子网类型

`SubnetStack` 提供不同用途的子网：

| 方法 | 用途 |
|------|------|
| `GetPrivateSubnets()` | 应用工作负载（EKS Pod、EC2 实例） |
| `GetStorageSubnets()` | 存储服务（RDS、ElastiCache、MSK） |
| `GetEgressSubnets()` | 镜像构建（需要 NAT 出口） |
| `GetS3Endpoint()` | VPC Endpoint 引用（S3 访问） |
