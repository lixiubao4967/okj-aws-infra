# Stage 业务服务上线：原理、排查方法与 Runbook

> 记录 stage EKS 集群业务服务从老 EC2 世界迁移上线的完整原理与实战排查过程。
> 已上线：okj-egress-gateway（2026-07-10，全新 Apollo 应用）、okj-auth（2026-07-16，复用 EC2 存量 Apollo 应用）。
> 前传见 okj-argo-manifests 仓库 `docs/stage-egress-gateway-bringup.md`。

## 一、两个世界并存的大背景

stage 环境正处于「EC2 老世界」和「EKS 新世界」并存期，很多坑都源于此：

| | 老世界（EC2） | 新世界（EKS） |
|---|---|---|
| VPC | vpc-01063e4472e3d8da1（10.110.0.0/16） | vpc-0cb040cb4fe3fd7bb |
| 服务形态 | EC2 实例 + 内部 ALB | Pod（branch ENI + SGP） |
| 网络身份 | `stage_client_*` / `stage_service_*` SG | 统一 `ec2-common`（sg-03010e2870464d144）+ 各自 `service-group-*` |
| Secret 命名 | `stage-{app}`（如 `stage-okj-auth`） | `{app}-stage-secret`（CDK 预建，Pod Identity 只授权这个） |
| Nacos 入口 | `nacos-server.internal.okcoin.japan`（Alias → 内部 ALB，仅 8848） | 必须用 `nacos-cluster.internal.okcoin.japan`（直指三台，8848+9848） |

Apollo / Nacos / RDS / Redis 等中间件全部还在老世界，EKS pod 跨 VPC 访问它们，
每个新依赖都可能是一堵 SG 墙。

## 二、业务服务的启动依赖链（业务逻辑过程）

所有 Java 业务服务启动时按以下顺序拉起依赖，**任何一环断掉都起不来**，
且错误往往表现在下一环：

```text
① GitOps        overlay 推分支 → ApplicationSet 自动建 Application → ArgoCD 同步
② SGP/ENI       pod 拿到 branch ENI，网络身份 = SGP 里的 SG（不是节点 SG）
③ 镜像拉取       当前直接引用 test/ ECR（长期方案挂账中）
④ Pod Identity  CDK 预建的 association 自动生效，pod 拿到 IAM 角色
⑤ Apollo        连配置中心拉全部应用配置（含 aws.secrets.secret-name）
⑥ Secrets Mgr   按 ⑤ 里的 secret 名去读真实凭证（DB 密码等）
⑦ 中间件         按 ⑤⑥ 的配置连 RDS / Redis / 其他（每个服务不同！）
⑧ Nacos         服务注册，2.x 客户端双通道 8848(http) + 9848(grpc)
⑨ Started       健康检查通过，ArgoCD 转 Healthy
```

两个关键机制：

- **Apollo 兜底陷阱**：Apollo 连不上时，应用掉进 jar 内打包的兜底默认值，
  secret 名会变成 `rehe-{app}`（test 老命名）。**看到 `rehe-*` 的 AccessDenied
  = Apollo 没连上或没发布，不是 IAM 问题。**
- **Nacos 2.x 端口约定**：gRPC 端口 = 主机名相同、端口 +1000（8848 → 9848），
  **写死在客户端协议里**，无法单独配置。所以 Nacos 地址绝不能填只有 8848
  监听器的 ALB 域名（见下文 okj-auth 案例）。

## 三、上线分两种场景（先判断再动手）

上线前先去 stage Apollo Portal（<https://stage-apollo.okqa.work>，登录后搜 appId）确认：

### 场景 A：Apollo 无存量应用（例：okj-egress-gateway）

纯新建，从 test 导出配置整贴 → 改 secret 名 → 发布。见前传 runbook。

### 场景 B：Apollo 已有 EC2 存量应用（例：okj-auth）

**绝对不能整贴覆盖** —— EC2 实例正在消费这份配置（Portal 的 Instance List
能看到连接数），而且存量配置里已经是真 stage 值（RDS 地址等），比 test 导出的更对。
正确姿势是 **diff 增补**：

```bash
# test 导出（冒充客户端，比抄 Portal UI 准）
kubectl --context <test集群> -n okj-exchange exec deploy/{app} -- \
  curl -s 'http://10.130.7.13:8080/configs/{app}/default/application' \
  | jq -r '.configurations | to_entries[] | "\(.key)=\(.value)"' > /tmp/{app}.properties

# stage 存量：Portal Text 模式全选复制 → pbpaste > /tmp/{app}-stage-current.properties

# 找出 test 有、stage 没有的 key（注意存量是 "key = value" 带空格格式，
# 必须用 sed 去空格提 key，直接 cut -d= 会全部对不上）
comm -13 \
  <(grep -v '^#' /tmp/{app}-stage-current.properties | sed -E 's/[[:space:]]*=.*//' | sort -u) \
  <(grep -v '^#' /tmp/{app}.properties | cut -d= -f1 | sort -u)
```

然后逐条判断：缺的 key 补进去（值抄 test），已有的 key **只审不动**，
重点审这三个（k8s pod 能不能起来就看它们）：

| key | 要求 |
|---|---|
| `aws.secrets.secret-name` | 必须与 Pod Identity 角色 policy 授权的 secret 一致（见第四节） |
| `spring.cloud.nacos.discovery.server-addr` | 必须是 `nacos-cluster.internal.okcoin.japan:8848` |
| DB / Redis 等中间件地址 | 记下来，去查对应 SG 是否放行 ec2-common（见第五节） |

改完**每个 namespace 都要点 Release**（未发布 = 客户端拉到 404）。

## 四、Secret 命名冲突及决策（okj-auth 案例）

存量应用的 `aws.secrets.secret-name` 指向老命名 `stage-{app}`，
但 CDK 给 pod 预建的 Pod Identity 角色（`{app}-stage-instance-role`）
的 policy **只授权** `{app}-stage-secret`。确认方法：

```bash
# 找到 pod 的角色
aws eks list-pod-identity-associations --cluster-name okj-exchange-stage \
  --region ap-northeast-1 --query "associations[?serviceAccount=='{app}']"
aws eks describe-pod-identity-association --cluster-name okj-exchange-stage \
  --association-id <上面的 id> --query 'association.roleArn'

# 看角色 policy 授权的 secret ARN
aws iam get-role-policy --role-name {app}-stage-instance-role \
  --policy-name <list-role-policies 查到的名字> \
  --query 'PolicyDocument.Statement'
```

**okj-auth 的决策**：把老 secret 内容复制进 CDK secret，Apollo 改指 CDK secret
（保持 IAM 零改动，命名向新世界收敛）：

```bash
aws secretsmanager get-secret-value --secret-id stage-okj-auth \
  --query SecretString --output text > /tmp/s.json
aws secretsmanager put-secret-value --secret-id okj-auth-stage-secret \
  --secret-string file:///tmp/s.json && rm -P /tmp/s.json
# Portal: aws.secrets.secret-name → okj-auth-stage-secret，Release
```

⚠️ 遗留风险：EC2 实例下次重启也会改读新 secret，其实例角色若无授权会起不来。
两份 secret 内容当前一致，纯 IAM 问题，待给 EC2 实例角色补授权（挂账）。

## 五、SG 墙：每个服务的中间件依赖都要预检

egress-gateway 拆掉了 Apollo/Nacos 四堵墙（全放行 ec2-common，惠及后续所有服务），
但 **DB / Redis 是每个服务自己的依赖**，上线前主动查、不要等 pod 报错：

```bash
# RDS：集群挂的 SG → 入站规则里找 ec2-common
aws rds describe-db-clusters --db-cluster-identifier <集群名> \
  --query 'DBClusters[0].VpcSecurityGroups'
aws ec2 describe-security-group-rules \
  --filters Name=group-id,Values=<sg-id>,... \
  --query 'SecurityGroupRules[?IsEgress==`false`].[GroupId,FromPort,ToPort,ReferencedGroupInfo.GroupId,CidrIpv4]' \
  --output text

# ElastiCache：replication group → member → SG，同上查规则
aws elasticache describe-replication-groups \
  --replication-group-id <名字> --query 'ReplicationGroups[0].MemberClusters[0]'
aws elasticache describe-cache-clusters --cache-cluster-id <member> \
  --query 'CacheClusters[0].SecurityGroups'

# 缺了就补（放行 ec2-common 一次，覆盖后续所有 EKS 服务）
aws ec2 authorize-security-group-ingress --group-id <目标sg> \
  --ip-permissions 'IpProtocol=tcp,FromPort=<端口>,ToPort=<端口>,UserIdGroupPairs=[{GroupId=sg-03010e2870464d144,Description="EKS pods via ec2-common"}]'
```

## 六、Nacos 双入口陷阱（okj-auth 最大的一堵墙）

现象：pod 日志报 `Server check fail ... port 9848 ... Connection refused: nacos-server.internal.okcoin.japan/10.110.11.17:9848`。

排查链（每步都有通用价值）：

1. **refused ≠ timeout**：timeout 是网络层（SG/路由）被丢包；refused 是 TCP
   已通、对端没有进程听这个端口。看到 refused 就不用再查 SG 了。
2. 10.110.11.17 查 EC2 查不到 → 查 ENI：
   `aws ec2 describe-network-interfaces --filters Name=addresses.private-ip-address,Values=10.110.11.17`
   → 是 `stage-exchange-internal-alb` 的 ENI。
3. 查 Route53：`internal.okcoin.japan` 是**脑裂 DNS**，同名 zone 有三个
   （okj/test/stage 各挂各的 VPC）。stage zone 里 `nacos-server` 是
   **Alias 记录指向内部 ALB**（list-resource-record-sets 里 TTL/records
   显示 null 就是 Alias 特征），而 `nacos-cluster` 直指三台 Nacos 实例。
4. 根因：ALB 只有 8848 监听器；Nacos 2.x 客户端在同一主机名上 +1000 连 gRPC
   → 打到 ALB 的 9848 → refused。EC2 老 jar 是 Nacos 1.x 客户端只用 http
   8848，走 ALB 没事——所以老世界一直没炸。
5. 修复前最后确认：ALB 8848 的 target group 成员 == `nacos-cluster` 直指的
   同三台实例 → 两个域名是**同一注册中心的两个入口**，把 Apollo 的
   `server-addr` 改成 `nacos-cluster.internal.okcoin.japan:8848` 对 EC2
   零影响（重启后只是不再绕 ALB）。

## 七、排查方法论（从两次上线沉淀）

| 原则 | 说明 |
|---|---|
| 错误信息变形 = 前进 | 改完环境必须删 pod 拿新鲜日志再判断，别对着旧日志排查 |
| timeout / refused / 4xx 分层 | timeout=SG/路由；refused=对端无监听；4xx/5xx=应用层数据/配置 |
| `rehe-*` secret 报错 | = Apollo 没连上/没发布，不是 IAM 问题 |
| SGP 渲染必须 grep 验证 | patch target 拼错时 kustomize 静默跳过，`kubectl kustomize ... \| grep sg-` |
| 存量配置先 diff 再动 | Portal Instance List 有连接数 = 有活的消费者，整贴会杀死老服务 |
| 主动预检代替等报错 | 服务的 DB/Redis 依赖上线前用 AWS API 查 SG，比等 pod 崩快得多 |

## 八、下一个服务上线 Runbook（v2，取代前传第 5 节）

1. **Overlay**：`configs/stage.yaml` 查该服务的 SG 组名 → 在 stage VPC 解析成
   ID（`aws ec2 describe-security-groups --filters Name=vpc-id,... Name=group-name,...`）
   → SGP patch = `[组SG, ec2-common]`；镜像 tag 取 **test 实跑版本**
   （`kubectl get deploy -o jsonpath`，overlay 文件里的可能过时）；
   test overlay 若带 skywalking 三件套,去掉 `-javaagent` 和两个 `SW_*`,
   保留 `-XX:MaxRAMPercentage=75.0`。本地 `kubectl kustomize | grep -E 'sg-|image:'`
   验证后推 `rehe-test21`。
2. **Apollo**:先判断场景 A（新建整贴）还是场景 B（存量 diff 增补），见第三节；
   三个必审 key:secret-name / nacos server-addr / 中间件地址。每个 namespace 都 Release。
3. **Secret**:确认 Pod Identity policy 授权的名字（第四节），把真值灌进去。
4. **SG 预检**:按第五节把该服务依赖的 RDS/Redis/其他中间件 SG 全查一遍，缺则补。
5. **验证**:删 pod，盯四行日志:`found main secrets config: [{app}-stage-secret]`
   → `Get AWS secrets`（无 ERROR）→ Nacos `register finished` → `Started`。

## 九、带外手工改动台账（待收编 IaC）

egress-gateway 的 4 条 SG 规则见前传；okj-auth 新增：

| 类型 | 对象 | 改动 | ID |
|---|---|---|---|
| SG | stage_db_auth_sg（sg-0be667342e6847246） | 放行 ec2-common:3306 | sgr-04854892854bc1eca |
| SG | stage redis SG（sg-021cdfb99fed33ad2） | 放行 ec2-common:6379 | sgr-0f458e7ec3f29bf9f |
| Apollo | okj-auth / application | `aws.secrets.secret-name` → `okj-auth-stage-secret`；`server-addr` → `nacos-cluster...:8848`；新增 metrics percentiles key | 2026-07-16 两次 Release |
| Secret | okj-auth-stage-secret | 灌入 `stage-okj-auth` 内容（原为 CDK 占位壳） | — |
| 待办 | EC2 okj-auth 实例角色 | 补 `okj-auth-stage-secret` 读授权（防 EC2 重启后起不来） | 未做 |
