# okj-cdk-exchange 实战部署记录

> 记录向 stage 等新环境部署 okj-cdk-exchange 时遇到的问题与解决方法。

---

## Stage 环境部署概览

部署命令一览：

```bash
cdk synth --context env=stage
cdk diff  --context env=stage
cdk deploy <stack-name> --context env=stage --require-approval never
```

Stage 环境关键配置（`config/stage.yaml`）：

| 字段 | 值 | 备注 |
|------|-----|------|
| VPC | `vpc-0cb040cb4fe3fd7bb` | stage 独立 VPC，CIDR `10.110.0.0/16` |
| cidrPrefix | `10.110.` | 与 test（`10.131.`）不冲突 |
| VPC Peering | `pcx-07c274def35713c07` | 对端为 `stage_admin_vpc`（`10.111.0.0/16`）|
| privateHostZoneId | `Z0703662156ZXI8LR739G` | Stage 专属 Route53 私有 Zone |
| 证书 | `*.okqa.work` wildcard | Stage 与 test 共用，无需单独申请 |

---

## Stack 部署顺序（新环境）

新环境首次完整部署需按层次推进，后一层依赖前一层的 CloudFormation 输出（跨 Stack Export/Import）。

### 依赖关系图

```
Layer 1: subnet ──┬── security-groups
                  └── provision-resources
                            │
Layer 2:          ├── param-store  (无依赖，可先行)
                  ├── ecr-stack    (无依赖，可先行)
                  ├── dynamodb     (无依赖，可先行)
                  ├── image-recipe (无 env 后缀，全环境共用)
                  ├── s3-stack ←── subnet (S3 VPC Endpoint)
                  └── secret-stack (无依赖)
                            │
Layer 3:          └── iam-stack ←── s3 + secret + dynamodb
                            │
Layer 4:          ├── aurora-stack ←── provision + subnet + sg + secret
                  ├── msk-stack    ←── provision + subnet + sg
                  ├── cache-stack  ←── provision + subnet + sg
                  └── eks          ←── subnet + sg
                            │
Layer 5:          ├── base-image           ←── aurora(SSM endpoint) + param-store + s3 + iam + secret + recipe
                  └── base-container-image ←── ecr + subnet + sg + s3
                            │
Layer 6:          └── 所有业务服务 stacks  ←── eks + 以上所有
```

### Stage 部署进度表（2026-05-01）

| 层次 | Stack | 状态 | 部署命令 |
|------|-------|------|---------|
| **Layer 1** | `subnet-stage` | ✅ | — |
| | `security-groups-stage` | ✅ | — |
| | `provision-resources-stage` | ✅ | — |
| **Layer 2** | `param-store-stage` | ✅ | — |
| | `ecr-stack-stage` | ✅ | — |
| | `dynamodb-stack-stage` | ✅ | — |
| | `image-recipe-stack` | ✅ 共用 | — |
| | `s3-stack-stage` | ✅ 2026-05-01 | — |
| | `secret-stack-stage` | ✅ 2026-05-01（cdk import）| — |
| **Layer 3** | `iam-stack-stage` | ✅ 2026-05-01 | — |
| **Layer 4** | `aurora-stack-stage` | ✅ 2026-05-01 | — |
| | `msk-stack-stage` | ✅ 2026-05-01 | — |
| | `cache-stack-stage` | ✅ 2026-05-01 | — |
| | `eks-stage` | ✅ 2026-05-01（代码修复后重建）| — |
| **Layer 5** | `base-image-stage` | ✅ | — |
| | `base-container-image-stage` | ✅ | — |
| **Layer 6** | 所有 `okj-*-stage` 服务 | ✅ 2026-06-10 全量完成（128/128）| — |

> **进度更新（2026-06-10）**：`cdk deploy --all` 全量跑通，128 个 stack 全部 ✅。过程中踩了坑 4（S3/ECR 空孤儿资源）与坑 5（ECR 仓库双重定义），均已解决并记录在下方。
>
> **概念备忘**：`base-image` 造 AMI（EC2/executor 用），`base-container-image` 造容器基础镜像（推 ECR，EKS Pod `FROM`）。`cdk deploy` 只建 Image Builder 流水线，**不产镜像**；流水线还需触发执行才会烤出镜像并更新 param-store 的 `pipeline-ami-id`。
>
> **后续待办**：触发 Image Builder 流水线产出 AMI / 容器镜像；CI 首次构建各服务镜像推入对应 ECR 仓库（当前服务仓库均为空）。

### 分层部署命令（按顺序执行）

```bash
# 公共参数
CTX="--context env=stage --require-approval never"

# Layer 2（剩余，可并行）
cdk deploy okj-exchange-s3-stack-stage okj-exchange-secret-stack-stage $CTX

# Layer 3
cdk deploy okj-exchange-iam-stack-stage $CTX

# Layer 4（可并行）
cdk deploy okj-exchange-aurora-stack-stage okj-exchange-msk-stack-stage \
  okj-exchange-cache-stack-stage okj-exchange-eks-stage $CTX

# Layer 5（可并行）
cdk deploy okj-exchange-base-image-stage okj-exchange-base-container-image-stage $CTX

# Layer 6（全量，CDK 自动处理服务间顺序）
cdk deploy --all $CTX
```

> **注意**：同一条 `cdk deploy` 中列多个 stack，CDK 会并行部署它们，但不会检查 stack 之间的依赖顺序。必须确保上一层全部 `CREATE/UPDATE_COMPLETE` 再执行下一层命令。

---

## 踩坑记录

### 坑 1：SSM Parameter Store 大量并发创建导致部署失败

**Stack**：`okj-exchange-param-store-stage`

**现象**：`cdk deploy` 时多个 SSM 参数同时 `CREATE_FAILED`，错误信息：

```
Resource handler returned message: "Error occurred during operation 'PutParameter'."
HandlerErrorCode: GeneralServiceException
```

随后触发回滚，回滚又因同样原因失败，stack 卡在 `ROLLBACK_FAILED` 状态。

**根本原因**：

该 stack 一次性创建约 **170 个 SSM 参数**（51 个服务 × 3 个参数 + 17 个镜像参数），CloudFormation 并发写入。SSM Parameter Store 标准吞吐限制是 **40 TPS**，并发超限后返回 `GeneralServiceException`（AWS 把 throttling 错误也包装成这个通用错误码）。

> **识别技巧**：如果 artifact 参数（纯字符串，无 `DataType: aws:ec2:image`）也失败，说明不是 AMI 验证问题，而是限速。

**解决步骤**：

1. **预先开启 SSM 高吞吐**（在首次部署前执行，将限速从 40 TPS → 100 TPS）：

```bash
aws ssm update-service-setting \
    --setting-id arn:aws:ssm:ap-northeast-1:097102939699:servicesetting/ssm/parameter-store/high-throughput-enabled \
    --setting-value true
```

2. **清理卡住的 stack**（stack 已处于 ROLLBACK_FAILED，无法自动恢复）：

```bash
aws cloudformation delete-stack --stack-name okj-exchange-param-store-stage
# 等待删除完成（约 1-2 分钟）
aws cloudformation wait stack-delete-complete --stack-name okj-exchange-param-store-stage
```

3. **重新部署**：

```bash
cdk deploy okj-exchange-param-store-stage --context env=stage --require-approval never
```

**费用影响**：高吞吐模式下标准参数的 API 调用会按量计费（约 $0.05/10,000 次），CI/CD 低频场景可忽略不计。

**规律**：test/prod 环境未遇到此问题，因为它们的参数早已存在，后续部署是 update 操作（并发压力远小于首次全量 create）。**给任何新环境首次部署 param-store stack 前，都要先执行上述第 1 步。**

---

### 坑 2：Secrets Manager secret 已存在导致 secret-stack 部署失败

**Stack**：`okj-exchange-secret-stack-stage`

**现象**：`cdk deploy` 时所有 Secret 资源同时 `CREATE_FAILED`：

```
Resource handler returned message: "The operation failed because the secret
okj-exchange-stage-uno-global already exists."
HandlerErrorCode: AlreadyExists
```

stack 回滚后进入 `ROLLBACK_COMPLETE` 状态。

**根本原因**：

Secrets Manager 中已存在同名 secret（由之前手动创建或上一次部署残留），且内含真实密码数据，CloudFormation 无法覆盖创建。

> **注意**：先用 `describe-secret` 确认 `DeletedDate` 字段：
> - `DeletedDate` 有值 → secret 处于待删除保护期，强制删除后重建即可
> - `DeletedDate` 为 null → secret **活跃且可能有真实数据**，必须走 import 路径，不能删除

**解决步骤（secret 有真实数据的情况）**：

1. **查出所有冲突 secret 的完整 ARN**（import 必须用 ARN，不能用名称）：

```bash
for secret in \
  okj-exchange-stage-uno-global \
  okj-exchange-stage-wallet-vault-common \
  okj-exchange-stage-eks-cluster-secret \
  okj-exchange-stage-global-db-root-secret; do
  echo -n "$secret → "
  aws secretsmanager describe-secret --secret-id $secret --query 'ARN' --output text
done
```

2. **删除 ROLLBACK_COMPLETE 的空 stack**：

```bash
aws cloudformation delete-stack --stack-name okj-exchange-secret-stack-stage
aws cloudformation wait stack-delete-complete --stack-name okj-exchange-secret-stack-stage
```

3. **用 `cdk import` 将现有 secret 纳入 CDK 管理**（不会修改 secret 的值）：

```bash
cdk import okj-exchange-secret-stack-stage --context env=stage
```

CDK 会交互式询问每个资源的物理 ID，填入对应的完整 ARN。stack 最终进入 `IMPORT_COMPLETE` 状态，后续 `cdk deploy` 按正常 update 处理。

**ARN 格式**：`arn:aws:secretsmanager:<region>:<account>:secret:<name>-<suffix>`（注意末尾有 6 位随机后缀）。

**规律**：新环境 secret-stack 部署前，先确认这 4 个 secret 是否已存在。若已存在且有真实数据，直接走 import 而非重建，避免数据丢失。

---

### 坑 3：EKS AccessEntry 与 Cluster 并行创建导致 404

**Stack**：`okj-exchange-eks-stage`

**现象**：stack 创建时 `AWS::EKS::AccessEntry` 报错：

```
No cluster found for name: okj-exchange-stage.
(Service: Eks, Status Code: 404)
```

stack 回滚进入 `ROLLBACK_COMPLETE`。

**根本原因**：

`createNodeAccessEntry` 函数中 `ClusterName` 使用了硬编码字符串：

```go
// 错误写法：硬编码字符串，CloudFormation 不知道依赖关系
ClusterName: jsii.String(clusterName),
```

CloudFormation 无法从字面字符串推导出 `AccessEntry` 依赖 `Cluster`，于是并行创建两者，`AccessEntry` 先到时集群还不存在，报 404。

**修复**（`internal/stacks/infra/k8s/eks_nodegroups.go`）：

```go
// 正确写法：使用 s.cluster.Ref()，CDK 生成 {"Ref": "eks-cluster"}
// CloudFormation 识别 Ref 依赖，自动等集群创建完再执行 AccessEntry
ClusterName: s.cluster.Ref(),
```

`Ref()` 返回的是 CloudFormation intrinsic function `{ "Ref": "eks-cluster" }`，等价于集群名称，但同时建立了隐式依赖关系。

**处理步骤**：

1. 修复代码（如上）
2. 删除 ROLLBACK_COMPLETE 的 stack：

```bash
aws cloudformation delete-stack --stack-name okj-exchange-eks-stage
aws cloudformation wait stack-delete-complete --stack-name okj-exchange-eks-stage
```

3. 重新部署（约 13 分钟）：

```bash
cdk deploy okj-exchange-eks-stage --context env=stage --require-approval never
```

**规律**：CDK 中凡是用字面字符串引用同 stack 内其他资源名称的地方，都应改为 `resource.Ref()` 或 `resource.AttrXxx()`，避免 CloudFormation 并行创建导致的隐式依赖缺失。

---

### 坑 4：S3 bucket / ECR 仓库空孤儿导致 `already exists`（2026-06-10）

**Stack**：`okj-uno-api-stage`（S3）、`okj-log-clickhouse-stage` 等 10 个服务栈（ECR）

**现象**：change set 早期校验失败：

```
Resource of type 'AWS::S3::Bucket' with identifier 'okj-uno-api-bucket-stage' already exists.
Resource of type 'AWS::ECR::Repository' with identifier 'stage/okj-log-clickhouse' already exists.
```

新栈卡在 `REVIEW_IN_PROGRESS`（空壳）。

**根本原因**：之前某次部署（04-24 / 06-02）创建过这些资源后栈被删除，资源因 `RETAIN` 策略（或 ECR 非空保护习惯）被保留，成为不归任何 CFN 栈管理的「孤儿」。重新部署时 CFN 不会接管同名资源，直接报错。

**诊断流程（先查归属，再决定删还是 import）**：

```bash
# 1. 栈是否空壳
aws cloudformation describe-stacks --stack-name <stack> --query "Stacks[0].StackStatus"
# REVIEW_IN_PROGRESS = 空壳，可直接删

# 2. 资源里有没有真实数据
aws s3api list-objects-v2 --bucket <bucket> --max-items 5 --query "Contents[].Key"   # null = 空
aws ecr list-images --repository-name <repo> --query "length(imageIds)"             # 0 = 空

# 3. 摸清同批残留的全部范围（按创建日期聚类，一次性批量处理）
aws s3api list-buckets --query "Buckets[?ends_with(Name, '-bucket-stage')].[Name,CreationDate]"
aws ecr describe-repositories --query "sort_by(repositories[?starts_with(repositoryName, 'stage/')], &createdAt)[].[repositoryName,createdAt]"
```

**处理**：
- **空资源** → 删空壳栈 + 删资源（`aws s3 rb`；`aws ecr delete-repository` **不带 `--force`**，非空会自动拒删，是第二道保险）→ 重新 deploy。
- **有数据** → 走 `cdk import`（同坑 2 secret 思路），不要删。

**判孤儿三条件须同时成立**：① 不在 ecr-stack 等合法栈的输出名单里；② 对应服务栈未 `CREATE_COMPLETE`；③ 资源为空。**仅凭创建日期会误判**——同日期也可能是 ecr-stack 合法追加的仓库。

**规律**：新环境重跑曾部分失败的 `--all` 时，`already exists` 会连环出现。先按日期聚类圈出整批孤儿一次性清理，不要逐个打地鼠。「为什么仓库是空的」：CDK 只造货架不放货，镜像由 CI / Image Builder 流水线另行推入，新环境从未跑过构建所以全空。

---

### 坑 5：ECR 仓库所有权迁移漏改服务栈，双重定义冲突（2026-06-10）

**Stack**：`okj-monitor-alert-webhook-stage`

**现象**：与坑 4 相同的 `already exists`，但资源 `stage/okj-monitor-alert-webhook` **归 ecr-stack 的 CFN 管理**（`describe-stack-resources` 可查到 `CREATE_COMPLETE`），不是孤儿——**绝不能删**，删了 ecr-stack 会 drift。

**根本原因**：代码把该仓库所有权从服务栈迁到 ecr-stack（`ecr_stack.go` 新增 `MonitorAlertWebhookRepo`，`base_container_image_stack.go` 改为引用它），但服务栈侧没有退出机制——`EKSServiceBuilder` 给每个服务默认 `CreateECRRepository=true`，于是 ecr-stack 与服务栈两边都声明同名仓库。

**修复**（三处，沿用 `S3BucketProvider` 可选接口惯例）：
- `services/structs.go`：新增 `ExternalECRRepositoryProvider` 可选接口（`UsesExternalECRRepository() bool`）。
- `services/builder.go`：组装时类型断言，命中则 `Identity.CreateECRRepository = jsii.Bool(false)`。
- `services/middleware/okj_monitor_alert_webhook.go`：实现该接口返回 `true`。

验证：`cdk synth okj-exchange-okj-monitor-alert-webhook-stage | grep "AWS::ECR::Repository"` 应无输出。

**规律**：同样的 `already exists` 报错，根因可能完全不同——**先用 `describe-stack-resources` 查资源归属**：无主空孤儿→删；有主（别的栈合法持有）→改代码消除重复定义。跨栈迁移资源所有权时，必须同时处理「新主建」与「旧主退出」两端。

`okj-exchange-param-store-<env>` 每个应用服务创建三类参数：

| 参数路径 | 用途 | DataType |
|---------|------|---------|
| `/imagebuilder/okj/{env}/services/{app}/pipeline-ami-id` | Image Builder 流水线构建出的最新 AMI | `aws:ec2:image` |
| `/okj/{env}/services/{app}/deploy-ami-id` | 经人工验证、可用于实际部署的 AMI | `aws:ec2:image` |
| `/okj/{env}/services/{app}/artifact` | S3 制品路径（`.jar` 或 Go binary） | String |

初始占位值均为公共 AMI `ami-01f861bd163a0cbdf`，由 Image Builder 流水线实际运行后更新。

---

## 与其他环境的差异

| 项目 | test | stage | prod |
|------|------|-------|------|
| VPC | 共用 rehe VPC | **独立 VPC** | 独立 VPC |
| cidrPrefix | `10.131.` | `10.110.` | — |
| SSM 高吞吐 | 已开启（历史原因）| 2026-04-30 开启 | — |
| 首次部署 param-store | 已部署 | ✅ 2026-04-30 完成 | — |
