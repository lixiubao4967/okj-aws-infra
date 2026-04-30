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

## SSM Parameter Store 参数结构说明

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
