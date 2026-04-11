# cdk-mini 部署指南

个人 AWS 账号的 CDK 实践项目，复刻 okj-cdk-exchange 的 4 层架构。
部署完成后会创建 3 个低成本资源：S3 桶、SNS Topic、DynamoDB 表。

---

## 前置条件

| 工具 | 检查命令 | 备注 |
|------|---------|------|
| Go 1.21+ | `go version` | 已安装 1.26 |
| Node.js 18+ | `node --version` | CDK CLI 依赖 Node，已安装 25 |
| AWS CLI v2 | `aws --version` | 已安装 2.34 |
| CDK CLI | `cdk --version` | **尚未安装**，见 Step 1 |
| AWS 凭证 | `aws sts get-caller-identity` | **尚未配置**，见 Step 2 |

---

## Step 1：安装 CDK CLI

```bash
npm install -g aws-cdk
cdk --version   # 预期输出：2.x.x (build ...)
```

---

## Step 2：配置 AWS 凭证

### 方式 A：长期访问密钥（IAM User）

```bash
aws configure
# 依次填入：
#   AWS Access Key ID:     <你的 Access Key>
#   AWS Secret Access Key: <你的 Secret Key>
#   Default region name:   ap-northeast-1
#   Default output format: json
```

### 方式 B：SSO 登录（推荐，如已配置 SSO Profile）

```bash
aws sso login --profile <profile-name>
export AWS_PROFILE=<profile-name>
```

**验证凭证有效：**

```bash
aws sts get-caller-identity
# 预期输出：包含 Account、UserId、Arn 的 JSON
```

---

## Step 3：填写账号配置

编辑 `config/personal.yaml`，替换占位值：

```bash
# 查询你的 AWS 账号 ID
aws sts get-caller-identity --query Account --output text
```

```yaml
# config/personal.yaml
account: "123456789012"          # ← 替换为你的 12 位账号 ID
region: "ap-northeast-1"         # ← 可改为其他区域

s3:
  bucketName: "cdk-mini-practice-123456789012"   # ← 末尾加账号 ID 确保全球唯一

sns:
  topicName: "cdk-mini-notifications"
  displayName: "CDK Mini Practice Notifications"

dynamodb:
  tableName: "cdk-mini-tasks"
  partitionKey: "taskId"
```

> **注意**：S3 桶名全球唯一，推荐格式 `cdk-mini-practice-{账号ID}`，避免和他人冲突导致部署失败。

---

## Step 4：Bootstrap（首次部署必须执行一次）

```bash
cd practice/cdk-mini

cdk bootstrap aws://<账号ID>/<区域>
# 示例：cdk bootstrap aws://123456789012/ap-northeast-1
```

Bootstrap 会在你的账号里创建一个名为 `CDKToolkit` 的 CloudFormation Stack，包含：
- S3 桶：存放 CDK 打包的资源文件
- IAM Role：CDK 部署时使用的执行角色

> **只需执行一次**，同一账号+区域组合共用。

---

## Step 5：预览（不实际部署）

```bash
cdk synth
```

预期输出：打印 3 个 Stack 的 CloudFormation 模板（JSON 格式），无错误信息。

常见错误排查见文末[注意事项](#注意事项)。

---

## Step 6：部署

```bash
# 部署所有 Stack（推荐，一次完成）
cdk deploy --all

# 或逐个部署，方便观察每步结果
cdk deploy cdk-mini-s3-personal
cdk deploy cdk-mini-dynamo-personal
cdk deploy cdk-mini-sns-personal
```

部署过程中会提示确认 IAM 变更，输入 `y` 继续。

预期耗时：约 2-3 分钟（S3 和 SNS 很快，DynamoDB 稍慢）。

---

## Step 7：验证资源

```bash
# 验证 S3 桶
aws s3 ls | grep cdk-mini

# 验证 DynamoDB 表
aws dynamodb list-tables --query "TableNames[?contains(@, 'cdk-mini')]"

# 验证 SNS Topic
aws sns list-topics --query "Topics[*].TopicArn" | grep cdk-mini
```

---

## Step 8：清理（避免产生费用）

```bash
cdk destroy --all
```

| 资源 | 销毁行为 | 原因 |
|------|---------|------|
| S3 桶 | 自动清空并删除 | `AutoCleanup: true`，CDK 创建 Lambda 帮你清空 |
| DynamoDB 表 | **保留，不删除** | `AutoCleanup: false`，需手动删除 |
| SNS Topic | 自动删除 | 无数据，直接删除 |

手动删除 DynamoDB 表：

```bash
aws dynamodb delete-table --table-name cdk-mini-tasks
```

---

## 注意事项

| 问题 | 原因 | 解决方法 |
|------|------|---------|
| `account is required` | `personal.yaml` 中 `account` 未替换 | 填入真实 12 位账号 ID |
| `BucketAlreadyExists` | S3 桶名被他人占用 | 在桶名末尾加账号 ID 或随机后缀 |
| `ExpiredToken` | AWS 临时凭证过期 | 重新执行 `aws sso login` 或 `aws configure` |
| `CDKToolkit not found` | 未执行 bootstrap | 执行 Step 4 |
| `cdk synth` 输出为空 | Go 编译失败 | 先在项目根目录执行 `go build ./...` 排查 |
| DynamoDB 表删不掉 | `AutoCleanup: false` 是预期行为 | 手动执行 `aws dynamodb delete-table` |

---

## 费用说明

| 资源 | 月费估算 | 免费额度 |
|------|---------|---------|
| S3 桶（空桶） | $0 | 5GB 存储免费 |
| SNS Topic（不发消息） | $0 | 100 万次发布免费 |
| DynamoDB（按需计费） | $0 | 25GB 存储 + 2.5M 读写免费 |
| CDKToolkit S3 桶 | ~$0.01/月 | Bootstrap 产生，长期存在 |

> 实践完成后执行 `cdk destroy --all` + 手动删 DynamoDB 表，费用归零。

---

## 项目结构回顾

```
cdk-mini/
├── cmd/main.go                          # 入口：按依赖顺序创建所有 Stack
├── config/personal.yaml                 # 你的 AWS 账号配置（需手动填写）
├── internal/
│   ├── config/config.go                 # YAML → EnvConfig 结构体（含校验）
│   ├── constructs/                      # Layer 1：单个 AWS 资源封装
│   │   ├── s3_bucket_construct.go
│   │   ├── sns_topic_construct.go
│   │   └── dynamodb_construct.go
│   ├── stacks/                          # Layer 2：Stack 打包 + 配置工厂函数
│   │   ├── storage/s3_stack.go
│   │   ├── storage/dynamodb_stack.go    # ← 你实现了 ConstructTasksTableConfig
│   │   └── messaging/sns_stack.go
│   └── props/props.go                   # Layer 3：跨 Stack 引用总线
└── cdk.json                             # CDK 入口配置
```

对应 okj-cdk-exchange 的 4 层架构：**Construct → Stack → Props → main.go**。
