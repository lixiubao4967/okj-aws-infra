# CDK Go 常见陷阱与约定

从 okj-cdk-exchange 源码分析和实践中总结。

---

## 编码规范

### 错误：在 Construct 内调用 jsii AWS 函数

```go
// 错误：运行时 nil pointer panic，因为 Construct 的 scope 还未完全初始化
func NewMyConstruct(scope ...) {
    policy := awsiam.NewPolicyStatement(...)  // ← 不能在 Construct 里
}

// 正确：IAM 等 jsii 调用只能在 Stack 层
func (s *MyStack) createPolicy() {
    policy := awsiam.NewPolicyStatement(...)  // ← OK
}
```

### 错误：忽略 Construct 返回的 error

```go
// 错误：silent failure，问题延迟到 deploy 才暴露
construct, _ := NewRedisClusterConstruct(...)

// 正确：用 panic 让问题在 cdk synth 阶段暴露
construct, err := NewRedisClusterConstruct(...)
if err != nil {
    log.Panicf("failed to create redis construct: %v", err)
}
```

### 错误：Stack 依赖顺序写错

```go
// 错误：SNS Stack 还没创建就放进 props
okjProps := &props.OkjStackProps{SNSStack: nil}
snsStack := SetupSNSStack(...)  // 太晚了

// 正确：先创建，再组装进 props
snsStack := SetupSNSStack(...)
okjProps := &props.OkjStackProps{SNSStack: snsStack}
```

---

## CDK Go 特有问题

### jsii.String / jsii.Bool 包装

CDK Go 所有字符串参数都需要 `jsii.String()`，布尔值用 `jsii.Bool()`：

```go
// 错误：直接传字面量
BucketName: "my-bucket"

// 正确
BucketName: jsii.String("my-bucket")
```

### RemovalPolicy 使用

```go
// 正确写法
bucketProps.RemovalPolicy = awscdk.RemovalPolicy_DESTROY
```

### AutoDeleteObjects（S3 特有）

设置 `AutoCleanup: true` 时，CDK 会自动创建一个 Lambda 函数来清空桶，需要配合 `RemovalPolicy_DESTROY` 使用：

```go
bucketProps.AutoDeleteObjects = jsii.Bool(true)
bucketProps.RemovalPolicy = awscdk.RemovalPolicy_DESTROY
```

---

## 设计决策记录

### 为什么 DynamoDB 默认 AutoCleanup: false？

DynamoDB 数据有业务价值，`cdk destroy` 不应隐式删除数据。即使在练习/测试环境，数据意外丢失很难被发现。对比 S3：S3 的 AutoCleanup 是**机制**问题（必须先清空才能删桶），而 DynamoDB 删表是**数据**问题。

### 为什么 Stack 要拆分而不合并？

不同资源的**更新频率**不同：
- S3 桶名一旦确定极少变动
- DynamoDB 索引和容量模式可能随业务调整
- EKS 版本升级和 Aurora 版本升级是独立操作

独立 Stack 让你可以只 deploy 变更的部分，减少回滚范围。

### 为什么用配置工厂函数？

工厂函数（如 `ConstructTasksTableConfig`）的价值：
- 调用方只传**变化的参数**（tableName, env），稳定的约定（AutoCleanup 策略、计费模式）集中管理
- 同一套 Stack 代码可被多个环境复用，环境差异隔离在工厂函数里

---

## 常用命令

```bash
# 进入练习项目
cd practice/cdk-mini

# 编译检查
go build ./...

# 预览 CloudFormation 模板（不部署）
cdk synth

# 对比与当前状态的差异
cdk diff

# 部署所有 Stack
cdk deploy --all

# 销毁所有 Stack（注意：DynamoDB 表不会自动删除）
cdk destroy --all

# 手动删除 DynamoDB 表
aws dynamodb delete-table --table-name cdk-mini-tasks
```
