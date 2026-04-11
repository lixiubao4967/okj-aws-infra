# okj-cdk-exchange 服务与 AWS 资源速查

源码位置：`/Users/xiubao.li/Documents/Gitlab/okj-cdk-exchange`

---

## 服务目录（117 个应用）

| 目录 | 数量 | 说明 |
|------|------|------|
| `services/aircraft/` | 39 | 核心交易系统：Push、策略引擎、Canal 数据同步、etcd |
| `services/stateless/` | 43 | 业务 API、网关、处理器（水平扩展） |
| `services/uno/` | 17 | UNO 工作流平台：Builder 执行器、部署器、Flyway 迁移 |
| `services/middleware/` | 13 | 可观测性：VictoriaMetrics 套件、ClickHouse、Kafka、Fluent Bit |
| `services/stateful/` | 5 | 需要持久化的有状态应用 |
| `services/frontend/` | 1 | UNO 前端（React/Vue） |

---

## AWS 服务 → Construct 对照

| AWS 服务 | Construct 文件 | Stack 文件 |
|---------|---------------|-----------|
| ElastiCache Redis/Valkey | `redis_cluster_construct.go` | `middleware/redis_cluster_stack.go` |
| MSK Kafka | `msk_construct.go` | `middleware/msk_stack.go` |
| Aurora MySQL/PostgreSQL | `aurora_construct.go` | `infra/aurora_stack.go` |
| DynamoDB | `dynamodb_construct.go` | `infra/dynamodb_stack.go` |
| S3 | `s3_construct.go` | `infra/s3_stack.go` |
| ALB | `alb_construct_cfn.go` | — |
| NLB | `nlb_construct.go` | — |
| EKS | — | `infra/eks_stack.go` + `infra/k8s/` |
| EC2 Image Builder | `ami_pipeline_construct.go` | `image/` |
| ECR | — | `infra/ecr_stack.go` |
| CloudFront | `cloudfront_construct.go` | `application/application_cloudfront_stack.go` |
| Route53 | `route53_record_construct.go` | — |
| KMS | `kms_construct.go` | — |
| Secrets Manager | `secrets_manager_construct.go` | `infra/secret_stack.go` |
| SSM Parameter Store | `ssm_param_construct.go` | `infra/parameter_store_stack.go` |
| Security Group | `security_group_construct.go` | `network/security_group_stack.go` |
| Subnet / Route Table | `subnet_construct.go` | `network/subnet_stack.go` |
| VPC Endpoint | `vpc_endpoint_construct.go` | — |

---

## Aurora 集群配置

| 集群名 | 引擎 | 用途 |
|--------|------|------|
| OkjUno | PostgreSQL Aurora | UNO 工作流平台数据库 |
| OkjMiddleware | MySQL Aurora 3.08.2 | 中间件数据库 |
| OkjAircraft | Aurora MySQL | 交易核心系统数据库 |

---

## DynamoDB 表

| 表名 | 分区键 | 用途 |
|------|--------|------|
| UnoExecutorResultsTable | — | UNO 执行结果存储 |
| UnoExecutorLockTable | — | 分布式锁 |

---

## S3 桶（7 个）

| 变量名 | 用途 |
|--------|------|
| ApplicationBucket | 应用制品存储 |
| LogBackupBucket | 日志备份 |
| LogClickhouseStorageBucket | ClickHouse 冷数据 |
| YumRepoBucket | 内部 YUM 软件源 |
| I18nBucket | 国际化资源 |
| RuntimeBucket | 运行时配置 |
| RaftlogBucket | Raft 日志 |

---

## ECR 仓库（12 个）

Java8/Java17 基础镜像、UNO 各类执行器镜像、构建工具镜像

---

## AMI 镜像构建（14 种）

| 镜像 Stack | 内容 |
|-----------|------|
| BaseImageStack | 基础 OS（Amazon Linux 2023） |
| BaseContainerImageStack | Docker 容器基础镜像 |
| MonitorImageStack | Prometheus + Grafana |
| LogKafkaImageStack | Kafka 日志节点 |
| LogClickhouseImageStack | ClickHouse 分析数据库 |
| LogClickvisualImageStack | ClickVisual 日志 UI |
| GitLabRunnerImageStack | GitLab CI 自托管执行器 |
| RecipeStack | 镜像配方（被其他镜像 Stack 引用） |
