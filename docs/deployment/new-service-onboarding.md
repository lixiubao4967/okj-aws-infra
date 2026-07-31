# EKS 新服务上线全流程：四仓库协作原理与 Runbook

实例：`okj-vault` misc/evm/utxo 共 8 个分片服务（2026-07-31 完成，分支 `vault-meu`）。

本篇讲的是**跨仓库的端到端流程**。单个仓库内部怎么写代码，见：

- [okj-cdk-exchange 添加资源](cdk/add-new-resource.md)
- [okj-cdk8s-exchange 添加服务](cdk8s/add-new-service.md)
- [Stage 业务服务上线](argocd/stage-service-bringup.md)（Apollo / Nacos / SG 墙那一类应用层依赖）

---

## 一、四个仓库的分工

一个 EKS 服务上线要动四个仓库，各管一层，缺一层就有一种特定的故障：

| 仓库 | 职责 | 产出物 | 缺了会怎样 |
|------|------|--------|-----------|
| `okj-cdk-exchange` | AWS 侧身份 | ECR 仓库、IAM 角色、EKS Pod Identity 关联 | 无处推镜像；Pod 读不到 Secret |
| `okj-cdk8s-exchange` | 工作负载定义 | `base/<app>/` 下的 Deployment / Service / SA / SGP | ArgoCD 没有可同步的清单 |
| `okj-argo-manifests` | 环境差异 + 部署 | `overlays/{test,prod}/<app>/`，ArgoCD Application | 服务不会被部署 |
| `okj-uno` | 构建与发布 | `uno_release_app` 表记录 | 平台上看不到这个应用，无法构建 |

### 唯一契约：服务名

四个仓库之间只靠**服务名**一个字符串串联，且必须逐字节一致：

```
okj-cdk-exchange                okj-cdk8s-exchange           okj-argo-manifests          okj-uno
─────────────────               ──────────────────           ──────────────────          ───────
chains := []string{             chart 名                     overlays/<env>/<name>/      uno_release_app.name
  "misc-a",  ──────────────►    "okj-vault-misc-a"  ────►     okj-vault-misc-a/     ────►  okj-vault-misc-a
}                                     │                             │
      │                               │                             │
      ├── CDK 栈 okj-exchange-okj-vault-misc-a-test               ArgoCD Application
      ├── ECR 仓库 test/okj-vault-misc-a                          okj-{env}-okj-vault-misc-a
      ├── IAM 角色 okj-vault-misc-a-test-instance-role
      └── Pod Identity: ns=okj-exchange, sa=okj-vault-misc-a
                                      │
                                      ├── ServiceAccount okj-vault-misc-a
                                      ├── SecurityGroupPolicy okj-vault-misc-a-group-policy
                                      └── image .../test/okj-vault-misc-a
```

**为什么名字派生出这么多东西**（这决定了改动量为什么这么小）：

| 位置 | 代码 | 效果 |
|------|------|------|
| `okj-cdk-exchange/services/builder.go` | `ServiceAccount: svc.Name()`、`RepositoryName: svc.Name()` | 服务名 → SA 名 + ECR 仓库名 |
| `okj-cdk8s-exchange/internal/constructs/constructs.go` | `RBACConfig{}` 的 `ServiceAccountName` 为空时 `saName = appName` | chart 名 → SA 名 |
| `okj-cdk8s-exchange/services/exchange/helpers.go` | `exchangeImage(name)` = `ecrRegistry + "/" + name` | chart 名 → 镜像路径 |
| `okj-argo-manifests/argocd/{env}/applicationset.yaml` | git directories 生成器扫 `overlays/{env}/*`，模板 `okj-{env}-{{path.basename}}` | 目录名 → Application 名 |

推论：**名字拼错不会在任何一层报错**。CDK 照样建资源、kustomize 照样渲染、ArgoCD 照样 Synced —— 只是 Pod 拿不到 AWS 凭证。这是整条链路上最阴的失败模式，所以每次上线都要交叉核对名字。

---

## 二、base 与 overlay 的分层原则

这是 `okj-argo-manifests` 里最容易漏的地方。

`base/` 是 CI 从 `okj-cdk8s-exchange` **自动同步**的产物（commit 形如 `chore(manifests): sync okj-cdk8s-exchange@<sha>`），它是**环境无关**的，所以凡是跟 AWS 账号绑定的值，base 里一律留空或留 test 默认值，由 overlay 补：

| 字段 | base 里的值 | overlay 必须补什么 | 漏了的后果 |
|------|-----------|------------------|-----------|
| `SecurityGroupPolicy.groupIds` | `[]`（144 个 app 里 131 个都是空） | 两个环境各自的 sg ID | SGP 不分配安全组，Pod 连不上 Aurora/Redis/MSK，全部超时 |
| 容器 `image` | 硬编码 test 账号地址 `097102939699.../test/<app>:latest` | prod 侧用 `images:` 改写成 `738595724739.../prod/<app>` | prod 跨账号拉 test 镜像，`ImagePullBackOff` |
| `replicas` | `1` | 新服务先写 `0`（见第五节） | prod 立刻起 Pod 并失败 |

**判断某个值该不该写在 overlay 的标准**：它在 test 和 prod 是不是同一个值。不是，就必须在 overlay 补，base 帮不了你。

### 生成器只做一半

| 工具 | 会生成 | 不会生成 |
|------|-------|---------|
| `make create-overlay APP=<app> ENV=all` | `resources:` 指向 base | SGP patch、`images:`、`replicas:` |
| `make update-test-configs ENV=<env>` | SGP patch 里的 sg ID（从 `configs/<env>.yaml` 的 `sgs` 解析） | `images:`、`replicas:` |
| 手工 | — | `images:`、`replicas:`、SkyWalking 注入 |

sg ID 的拼接顺序是**按位置**的（`scripts/update_test_configs.py`）：

```python
new_ids = [sg_cache[name] for name in app_cfg["sgs"]] + [ec2_common_id]
```

即 `configs` 里 `sgs` 列的组 SG 在前、全局 `ec2_common_sg` 固定追加在最后。且脚本**只做首次 bootstrap**：已有 `sg-*` 的 overlay 会被 skip，不做持续对账。要改 SG，得先把 overlay 里的 ID 删掉再跑。

---

## 三、Runbook

### Step 0：前置判断

| 判断 | 说明 |
|------|------|
| 是否属于已有服务家族 | `okj-vault-*` 家族已抽象成泛型（`services/stateless/okj_vault_chain.go`），加一行标识符即可，不用新建文件。其他服务仍是一文件一服务 |
| SG 用哪个组 | 复用现有组 SG（如 vault/wallet/asset 类用 `service-group-asset`），**不要新建 SG**；组 SG 定义在 `internal/stacks/network/security_group_stack.go` |
| 上游代码目录是否存在 | 不存在也能先上线（见第五节 0 副本策略） |

### Step 1：okj-cdk-exchange

```go
// internal/stacks/association/okj_eks_services.go
chains := []string{
    "substrate", "aptos", "sui", "canton", "solana", "tron", "ripple",
    "misc-a", "misc-b", "misc-c",
    "evm-a", "evm-b", "evm-c",
    "utxo-a", "utxo-b",
}
```

```bash
make check && make validate
cdk synth --context env=test          # 确认新增了预期数量的栈
cdk deploy --context env=test         # ← 必须实际部署，合并 MR 不等于资源已创建
```

⚠️ **合 MR ≠ 资源存在**。必须跑 `cdk deploy` 才会真正创建 ECR 仓库和 Pod Identity 关联。

### Step 2：okj-cdk8s-exchange

新增 chart + 注册到 `services/registry.go`，然后 `make check`。合并后 CI 自动同步进 `okj-argo-manifests` 的 `base/`。

### Step 3：okj-argo-manifests

顺序不能反 —— `create-overlay` 依赖 base 存在，`update-test-configs` 依赖 configs 已登记：

```bash
# ① 手工编辑 configs/test.yaml 和 configs/prod.yaml，加 services 条目
#   okj-vault-misc-a:
#     sgs:
#       - service-group-asset

# ② 生成 overlay 骨架（先 dry-run）
for app in misc-a misc-b misc-c evm-a evm-b evm-c utxo-a utxo-b; do
  make create-overlay APP=okj-vault-$app ENV=all DRY_RUN=1
done

# ③ 填充 SGP patch 的 sg ID
make update-test-configs ENV=test
AWS_PROFILE=prod-sre make update-test-configs ENV=prod   # prod 是独立账号，要换 profile

# ④ 手工补 images: 和 replicas:（脚本不生成）
#    最省事的办法：整份复制同家族已有服务的 overlay，再全局替换服务名
#    同家族的 sg ID 相同，复制不会出错
for app in misc-a misc-b ...; do
  for env in test prod; do
    sed "s/okj-vault-ripple/okj-vault-$app/g" \
      overlays/$env/okj-vault-ripple/kustomization.yaml \
      > overlays/$env/okj-vault-$app/kustomization.yaml
  done
done

# ⑤ 验证
make fmt
make check                       # ← CI 门禁只有这个
make check-overlay-coverage      # 报告型，见下方说明
make check-overlay-sgp           # 报告型
kubectl kustomize overlays/test/okj-vault-misc-a    # ← 最强验证，见第六节
```

### Step 4：okj-uno

一个 Flyway 迁移文件，命名 `V{YYYYMMDD}.{HHMMSS}__{description}.sql`：

```sql
-- flyway-definition/V20260731.100000__insert_vault_misc_evm_utxo_apps.sql
INSERT INTO uno_release_app (app_type, language_runtime, app_num, name, resource_kind, description, git_url, default_git_branch, build_path, build_spec_id, creator_id, last_updater_id, restart_policy, create_time, update_time, artifact_last_version, deploy_type, dockerfile_id) VALUES
('JAVA','JAVA_8',1,'okj-vault-misc-a','DEPLOYMENT','okj-vault-misc-a','https://gitlab.okcoin.tokyo/okj-main/wallet/hot-wallet/okj-vault.git','develop','vault-misc-a',1,1,1,'STOP_FIRST','2026-07-31 10:00:00','2026-07-31 10:00:00',NULL,'K8S',4),
('JAVA','JAVA_8',1,'okj-vault-misc-b',...),
...;
```

要点：

| 项 | 说明 |
|----|------|
| 一批服务合成**一条**多行 INSERT | 原子性：8 个独立迁移是 8 个事务，中途失败会留下"注册了 3 个"的半吊子状态，Flyway 不自动回滚 |
| `build_path` | 指向上游代码仓库的**目录名**，是全表唯一不跟服务名走的字段 —— 历史上就在这栽过（有过一条专门的 `UPDATE` 修正迁移）。必须逐个核对 |
| `dockerfile_id` | Java 8 用 `4`，Java 17 用 `1`；Flink 类用 `(SELECT id FROM uno_dockerfile WHERE name = 'Java-17-flink')` |
| Flink 应用额外需要 `uno_config_source` | 普通服务不需要 |
| 迁移一旦执行就锁 checksum | CI 在 build 阶段**之前**跑 flyway，所以 MR 合并即执行，之后改不了文件，只能再补 `UPDATE` 迁移 —— **合并前必须核对完** |

### Step 5：验证（见第六节）

### Step 6：收尾（见第五节）

---

## 四、部署触发机制：合 MR 就是上线

`okj-argo-manifests` 的两个环境都是 `automated: prune + selfHeal`，**没有人工 gate**：

```yaml
# argocd/{test,prod}/applicationset.yaml
generators:
  - git:
      directories:
        - path: overlays/{env}/*        # ← 目录即 Application
template:
  metadata:
    name: okj-{env}-{{path.basename}}
  spec:
    syncPolicy:
      automated:
        prune: true
        selfHeal: true
```

推论：

- **建目录 = 建 Application**，不需要写 ArgoCD Application、不需要动 applicationset.yaml
- **提交前的文件状态就是上线状态**，没有二次确认机会
- `selfHeal: true` 意味着 `kubectl scale` 会在几分钟内被拽回 —— **扩缩容必须走 overlay 提交**

各 Application 的 REVISION 可能停在不同 commit，这是正常的：模板里有 `argocd.argoproj.io/manifest-generate-paths: ".;/base/{{path.basename}}"`，每个 Application 只在自己的路径变动时重新同步。

---

## 五、新服务的 0 副本策略

新服务上线时 ECR 里还没有镜像，直接起会 `ImagePullBackOff`。做法是 overlay 里先写 `replicas: count: 0`：

```yaml
replicas:
- name: okj-vault-misc-a
  count: 0
```

好处：

- 0 副本的 Deployment 在 ArgoCD 里算 **Healthy**，上线后一片绿，比一片红干净
- prod 侧尤其重要 —— Application 建好待命，不会真的去起 Pod
- Application 存在后，uno 的部署流水线才有回写 `newTag` 的落点

这是团队既有做法，历史数据可以印证 —— 上游代码目录是否存在与 test 副本数完全相关：

| 服务 | 上游有代码目录 | test overlay |
|------|--------------|-------------|
| vault-aptos / ripple / substrate | 有 | `count: 1` |
| vault-sui / canton / solana / tron | 没有 | `count: 0` |

### ⚠️ 副本数不会自动变回 1

k8s-deploy 流水线的提交**只改 `newTag` 一行**，不碰 `replicas`：

```
overlays/test/okj-jove-replay/kustomization.yaml | 2 +-
-  newTag: 0.0.9
+  newTag: 0.0.8
```

所以镜像构建、部署、tag 回写全部完成之后，副本数**依然是 0**，需要再提一次 commit 改成 1。

失败模式静默：ArgoCD 显示 Synced + Healthy、镜像 tag 正确、无任何报错 —— 只是没有 Pod。建议在 overlay 里加注释标明是哪种 0：

```yaml
replicas:
# count: 0 until the first image is built (new service, upstream code not landed yet)
- name: okj-vault-misc-a
  count: 0
```

`replicas: 0` 在这个仓库里同时表示"已登记但未启用"和"临时停机"，文件里长得一样，靠数值本身分辨不出来。

---

## 六、验证清单

### 提交前（本地）

| 检查 | 命令 | 性质 |
|------|------|------|
| 格式 / lint / 全量渲染 | `make check` | **CI 门禁**（`.gitlab-ci.yml` 只有 `fmt-check` / `lint` / `validate` 三个 job） |
| base ↔ overlay 目录覆盖 | `make check-overlay-coverage` | 报告型，长期带着别人的欠账 |
| overlay SGP ↔ configs | `make check-overlay-sgp` | 报告型 |
| **单个 overlay 实际渲染** | `kubectl kustomize overlays/<env>/<app>` | 最强，见下 |

**为什么渲染是最强的验证**：JSON Patch 有路径依赖 —— `op: add` 到 `/spec/template/spec/containers/0/env/-` 要求 base 里已存在 `env` 数组，`op: replace` 到 `groupIds` 要求那个键存在。这两条只有真正渲染才验得出，文件级 grep 和 `check-overlay-sgp`（只比对值）都发现不了。

**报告型检查怎么看**：判断标准不是"红不红"，而是"**missing 列表里有没有我的 app**"。

- `check-overlay-coverage`：`missing` 里有自己的 app 才是问题。有些服务在 prod 缺 overlay 是**故意的**（如 `okj-chaos-executor` 只在 test 注册，prod 独立账号不建这个高权限 executor 就是刻意的爆炸半径边界）
- `check-overlay-sgp`：`missing` 必须是 0（否则 Pod 拿不到安全组）；`extra` 只是慢性漂移（该 app 的 SG 脱离 configs 管理），严重性完全不同但脚本用同一个退出码

⚠️ 别用 `&&` 串这几个检查 —— 报告型工具的非零退出码会挡住后面真正相关的检查。用 `;`。

### 上线后（集群 + AWS）

k9s 里切 context 后按 `:<对象>`，用正则一次筛全部：`/vault-(misc|evm|utxo)`

| 层 | 对象 | 期望 |
|----|------|------|
| ArgoCD | `applications`（ns `argocd`） | 全部 Synced / Healthy，名字前缀 `okj-{env}-` |
| K8s | `deployments`（ns `okj-exchange`） | 新服务应为 `0/0` |
| K8s | `pods` | 新服务应为 **0 个** |
| K8s | `serviceaccounts` | 数量齐全 |
| K8s | `securitygrouppolicies` | 数量齐全，且 sg ID 是**该环境**的 |
| AWS | CloudFormation 栈 `okj-exchange-<app>-<env>` | ← 判断 `cdk deploy` 有没有跑过，最直接 |
| AWS | ECR 仓库 `<env>/<app>` | 存在 |
| AWS | IAM 角色 `<app>-<env>-instance-role` | 存在，且挂了预期的策略 |
| AWS | Pod Identity 关联 | SA → 角色 |
| uno | `uno_release_app` | 记录存在（Portal: Release → App） |

命令形式：

```bash
kubectl get applications -n argocd | grep <app>
kubectl get deploy,sa,sgp -n okj-exchange | grep <app>

# Pod Identity：--namespace 必填，否则报 "Service account is set, but namespace is not"
aws eks list-pod-identity-associations --cluster-name okj-exchange-test \
  --namespace okj-exchange --service-account <app>
# list 接口不返回 roleArn，要拿角色得再 describe
aws eks describe-pod-identity-association --cluster-name okj-exchange-test --association-id <id>

aws iam list-attached-role-policies --role-name <app>-test-instance-role
aws ecr describe-repositories --repository-names test/<app>
```

⚠️ **绿灯 ≠ 服务可用**。0 副本时上面所有检查都能全绿，而服务从未运行过 —— 没有任何一项能证明镜像可拉取、进程能启动、Secret 能读到。真正的验证要等副本数改成 1 之后看 Pod 日志。

---

## 七、坑位台账

| 坑 | 表现 | 原因 | 规避 |
|----|------|------|------|
| prod overlay 缺 `images:` | prod `ImagePullBackOff` | base 硬编码 test 账号 ECR，靠 overlay 改写 | 所有真实应用的 prod overlay 都必须有 `images:`；缺的只应该是集群级基础设施（`okj-namespaces` / `okj-helm-*` 等） |
| SGP `groupIds: []` | Pod 起来但连不上所有中间件 | base 永远留空，靠 `configs` + `update-test-configs` 填 | 上线前跑 `make check-overlay-sgp`，`missing` 必须为 0 |
| prod 侧 AWS 命令失败 | `No cluster found for name: okj-exchange-prod` | prod 是独立账号（`738595724739`） | 加 `AWS_PROFILE=prod-sre`；该 profile 的 `source_profile` 是 `prod-mfa`，临时凭证会过期 |
| 副本数停在 0 | ArgoCD 全绿但没有 Pod | 流水线只回写 `newTag` | 镜像就绪后手工改 `count: 1` 并提交 |
| SA 有、Pod Identity 没有 | Pod 正常启动，日志里报 AWS 权限错误 | 只合了 MR，没跑 `cdk deploy` | 扩容前查 CloudFormation 栈是否存在 |
| `build_path` 写错 | 镜像构建成功但跑的是别的服务的代码 | 该字段指向上游仓库目录，是唯一不跟服务名走的字段 | 合 MR 前逐条核对；Flyway 执行后只能补 `UPDATE` 迁移 |
| Flyway 版本冲突 / checksum 锁 | CI flyway 阶段失败 | 迁移在 build 之前执行，合并即落库 | 新文件时间戳取当天、晚于现有最新版本；合并前核对完 |
| 目录里混入无扩展名垃圾文件 | 无（静默） | yamllint 按 `**/*.yaml` 筛选、kustomize 只读 `kustomization.yaml`，都跳过 | `find overlays base -type f ! -name "*.yaml" ! -name "*.yml"` 应恒为空 |
| Go 复合字面量缺尾逗号 | 编译报 `unexpected newline in composite literal` | 换行等价于分号 | 多行字面量最后一项也要逗号（gofmt 会强制加） |

---

## 八、命令速查

```bash
# ── okj-cdk-exchange ──
make check && make validate
cdk synth  --context env=test
cdk deploy --context env=test

# ── okj-cdk8s-exchange ──
make check

# ── okj-argo-manifests ──
make create-overlay APP=<app> ENV=all [DRY_RUN=1]
make update-test-configs ENV=test
AWS_PROFILE=prod-sre make update-test-configs ENV=prod
make fmt
make check                                  # CI 门禁
make check-overlay-coverage ; make check-overlay-sgp
make build OVERLAY=overlays/test/<app>      # 渲染单个 overlay
kubectl kustomize overlays/test/<app>       # 同上，不依赖仓库工具链

# ── 验证 ──
kubectl config get-contexts
kubectl get applications -n argocd | grep <app>
kubectl get deploy,sa,sgp,pods -n okj-exchange | grep <app>
```

k9s 常用：`:ctx` 切集群 → `:applications` / `:deploy` / `:sgp` → `/<正则>` 过滤。

---

## 九、上线顺序总结

依赖关系决定顺序，不能反：

```
1. okj-cdk-exchange    合并 + cdk deploy   → ECR 仓库、IAM 角色、Pod Identity 就位
2. okj-cdk8s-exchange  合并               → CI 同步 base/ 到 argo 仓库
3. okj-argo-manifests  合并               → Application 自动创建，0 副本待命
4. okj-uno             合并               → 平台可见该应用（flyway 在 build 前执行）
5. 上游代码就绪 → uno 构建镜像 → 流水线回写 newTag
6. 手工把 test 副本数 0 → 1 并提交        ← 唯一没有自动化的一步
7. prod 发布前：确认 prod 侧 cdk deploy 已执行，再改 prod 副本数
```

第 1 步必须先于第 3 步 —— ServiceAccount 名必须两边一致，而 Pod Identity 关联要先存在。
