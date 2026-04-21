# 本地 k3s + ArgoCD GitOps 部署实践

## 背景

在本地 k3s 集群上验证 `okj-cdk8s-exchange` 生成的 Kubernetes 清单，
通过 ArgoCD 实现 GitOps 流程，以 `okj-aws-infra` 仓库作为 GitOps source。

涉及项目：

| 项目 | 路径 | 作用 |
| --- | --- | --- |
| `okj-cdk8s-exchange` | `~/Documents/Gitlab/okj-cdk8s-exchange` | CDK8s Go 项目，生成 K8s YAML |
| `okj-argo-manifests` | `~/Documents/Gitlab/okj-argo-manifests` | 生产 Kustomize overlays |
| `okj-aws-infra` | `~/Documents/Github/okj-aws-infra` | 本次用作本地 GitOps source |

---

## 环境信息

- k3s 版本：v1.34.6+k3s1
- k3s 节点：`k3s-server`（192.168.2.2:6443）
- ArgoCD：已预装，所有 Pod Running
- AWS 账号：097102939699（ap-northeast-1）

---

## 整体架构

```
okj-cdk8s-exchange
  └── make synth
        └── okj-charts/<service-name>/<service-name>.yaml   ← CDK8s 生成的原始 YAML

选取安全的 YAML 复制到 →

okj-aws-infra (GitHub)
  └── practice/okj-exchange-mini/manifests/
        ├── okj-namespaces.yaml
        └── okj-priority-classes.yaml

ArgoCD Application (指向该路径)
  └── kubectl apply -f manifests/*  →  k3s 集群
```

---

## 操作步骤

### Step 1：合成 YAML 清单

```bash
cd ~/Documents/Gitlab/okj-cdk8s-exchange
make synth
```

生成结果：`okj-charts/` 下约 100 个子目录，每个目录对应一个服务，
每个目录内有一个 YAML 文件包含该服务所有 K8s 资源。

**遇到的问题：**

```
!! This software has not been tested with node v25.8.1.
!! Supported: ^24.0.0, ^22.0.0, ^20.0.0
```

这是警告，不是错误。Node.js 版本高于 CDK8s 当前支持版本，但合成正常完成。
消除警告可在命令前加：

```bash
JSII_SILENCE_WARNING_UNTESTED_NODE_VERSION=1 make synth
```

### Step 2：选取可安全部署的 YAML

`okj-charts/` 中的服务按依赖关系分层：

| 分类 | 示例目录 | 能否直接部署 |
| --- | --- | --- |
| 纯基础资源（无镜像） | `okj-namespaces/`、`okj-priority-classes/` | ✅ 可以 |
| 依赖 ECR 镜像 | 所有业务服务、监控服务 | ⚠️ 需要 ECR 认证 |
| AWS 专用组件 | `okj-helm-aws-load-balancer-controller/` 等 | ❌ k3s 不适用 |
| 依赖自定义 CRD | `okj-crd-stateful-operator/` 的使用方 | ⚠️ 需先安装 CRD |

**本次选取：**

```bash
okj-charts/okj-namespaces/okj-namespaces.yaml        # 5 个 Namespace
okj-charts/okj-priority-classes/okj-priority-classes.yaml  # PriorityClass
```

### Step 3：将 YAML 放入 okj-aws-infra

```bash
cd ~/Documents/Github/okj-aws-infra
mkdir -p practice/okj-exchange-mini/manifests
mkdir -p practice/okj-exchange-mini/argocd

cp ~/Documents/Gitlab/okj-cdk8s-exchange/okj-charts/okj-namespaces/okj-namespaces.yaml \
   practice/okj-exchange-mini/manifests/

cp ~/Documents/Gitlab/okj-cdk8s-exchange/okj-charts/okj-priority-classes/okj-priority-classes.yaml \
   practice/okj-exchange-mini/manifests/
```

### Step 4：创建 ArgoCD Application CR

`practice/okj-exchange-mini/argocd/application.yaml`：

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: okj-exchange-mini
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/lixiubao4967/okj-aws-infra.git
    targetRevision: develop
    path: practice/okj-exchange-mini/manifests
  destination:
    server: https://kubernetes.default.svc
    namespace: default
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=false
```

**path 的原理：** ArgoCD clone 仓库后，`path` 是从仓库根目录开始的相对路径。
目录内无 `kustomization.yaml` 时，ArgoCD 自动识别为 Plain YAML 模式，直接 apply 所有 `.yaml` 文件。

### Step 5：Push 并注册 Application

```bash
git add practice/okj-exchange-mini/
git commit -m "feat: add okj-exchange-mini ArgoCD application for k3s local deploy"
git push origin develop

# 在 k3s 中注册 Application
kubectl apply -f practice/okj-exchange-mini/argocd/application.yaml
```

### Step 6：验证

```bash
# 查看 Application 状态
kubectl get application -n argocd okj-exchange-mini

# 验证 Namespace 是否创建
kubectl get ns | grep okj
```

**成功输出：**

```
NAME                    STATUS   AGE
okj-exchange            Active   7m
okj-external-operator   Active   7m
okj-monitor             Active   7m
okj-tool                Active   7m
okj-uno                 Active   7m
```

---

## 部署 okj-monitor-alert-webhook 实际遇到的问题

以下问题按排查顺序记录，每个问题均已解决。

### 问题一：ECR 镜像认证 ✅

**现象：** Pod 无法拉取镜像。

**解决：** 在目标 namespace 创建 imagePullSecret，并 patch ServiceAccount：

```bash
# 创建 Secret（ECR Token 12小时过期，需定期刷新）
kubectl create secret docker-registry ecr-pull-secret \
  --docker-server=097102939699.dkr.ecr.ap-northeast-1.amazonaws.com \
  --docker-username=AWS \
  --docker-password=$(aws ecr get-login-password --region ap-northeast-1) \
  --namespace=okj-monitor

# 让该 namespace 下所有 Pod 自动使用此 Secret
kubectl patch serviceaccount default \
  -n okj-monitor \
  -p '{"imagePullSecrets": [{"name": "ecr-pull-secret"}]}'
```

**注意：** ECR Token 有效期 12 小时，长期使用需部署自动刷新 CronJob。

---

### 问题二：SecurityGroupPolicy CRD 不存在 ✅

**现象：** ArgoCD sync 失败，报错：

```
The Kubernetes API could not find vpcresources.k8s.aws/SecurityGroupPolicy
```

**原因：** CDK8s 自动为每个服务生成 `SecurityGroupPolicy`（AWS VPC Resource Controller 专属 CRD），
用于给 Pod 分配 AWS 安全组。k3s 上没有安装此 CRD。

**解决：** 在 `okj-aws-infra` 的副本 YAML 里删除 SecurityGroupPolicy 段（约 13 行）。
不要修改 `okj-charts/` 里的原始生成文件。

---

### 问题三：内存资源超出节点上限 ✅

**现象：** Pod 长期 `Pending`，describe 显示：

```
0/1 nodes are available: 1 Insufficient memory.
```

**原因：** 生产配置为 500m CPU / 2Gi 内存（遵循 1:4 比例规范），
但 k3s 本地 VM 总内存约 2Gi，已分配 460Mi，不够再分配 2048Mi。

**解决：** 在副本 YAML 里降低资源配置：

```yaml
resources:
  limits:
    cpu: "0.5"
    memory: 256Mi
  requests:
    cpu: "100m"
    memory: 128Mi
```

---

### 问题四：NodeSelector 不匹配 ✅

**现象：** Pod 无法调度（Pending），describe 显示 NodeSelector 不匹配。

**原因：** CDK8s 生成的 Deployment 包含 `okj.com/node-group-role: monitor-egress`，
k3s 节点没有此 label。

**解决：**

```bash
kubectl label node k3s-server okj.com/node-group-role=monitor-egress
```

---

### 问题五：runAsNonRoot 与镜像冲突 ✅

**现象：** `CreateContainerConfigError`，describe 显示：

```
container has runAsNonRoot and image will run as root
```

**原因：** CDK8s 生成的 securityContext 要求 `runAsNonRoot: true`，
但 test 镜像的 Dockerfile 没有 `USER` 指令，默认以 root 启动。

**解决（本地测试）：** 在副本 YAML 里将两处 `runAsNonRoot: true` 改为 `false`：
- `spec.template.spec.securityContext.runAsNonRoot`
- `spec.template.spec.containers[].securityContext.runAsNonRoot`

**根本解法：** 在服务 Dockerfile 里加 `USER nonroot`。

---

### 问题六：config 文件挂载路径错误 ✅

**现象：** CrashLoopBackOff，容器日志：

```
open ./conf/config.yaml: no such file or directory
```

**原因：** CDK8s 将 ConfigMap 挂载到 `/data/okcoin/conf/config.yaml`，
但容器镜像的 WORKDIR 是 `/`，应用实际查找的路径是 `/conf/config.yaml`。

**排查方法：** 用调试 Pod 覆盖 ENTRYPOINT 检查工作目录：

```bash
kubectl run debug-webhook -n okj-monitor \
  --image=<ecr-image> \
  --restart=Never \
  --command \
  --overrides='{"spec":{"imagePullSecrets":[{"name":"ecr-pull-secret"}],"securityContext":{"runAsNonRoot":false}}}' \
  -- sh -c "pwd && ls -la && sleep 60"
# 输出：WORKDIR = /
```

**解决：** 修改副本 YAML 的 volumeMounts：

```yaml
# 修改前（CDK8s 生成）
mountPath: /data/okcoin/conf/config.yaml

# 修改后（k3s 本地测试）
mountPath: /conf/config.yaml
```

同理修改 `alert-card.json` 的挂载路径。

---

### 问题七：alert-webhook-config ConfigMap 缺失 ✅

**原因：** CDK8s 只生成 `alert-webhook-card`（通用 Lark 模板），
`alert-webhook-config`（含 `config.yaml`）是环境特定配置，
在生产环境由 `okj-argo-manifests/overlays/` 提供。

**解决：** 手动创建占位 ConfigMap：

```bash
kubectl create configmap alert-webhook-config \
  --from-literal=config.yaml="server:
  port: 8080
" \
  -n okj-monitor
```

---

## 最终结果

`okj-monitor-alert-webhook` 成功运行，Pod 日志：

```
Route registered: POST /alert → alertmanager-lark-intergration/handler.HandleAlert
Route registered: POST /proxy/alerts → alertmanager-lark-intergration/handler.ProxyAlert
[GIN-debug] POST   /alert      --> handler.HandleAlert (3 handlers)
[GIN-debug] POST   /proxy/alerts --> handler.ProxyAlert (3 handlers)
```

k3s 节点架构（arm64）与 ECR 镜像兼容，镜像为多架构构建。

---

## k3s 本地部署与生产 EKS 的差异对照

| 配置项 | 生产 EKS | k3s 本地 |
| --- | --- | --- |
| 镜像认证 | IRSA / 节点 IAM Role | imagePullSecret（手动刷新） |
| SecurityGroupPolicy | 自动分配 AWS SG | 删除（不适用） |
| 资源配置 | 500m CPU / 2Gi Mem | 100m CPU / 128Mi Mem |
| NodeSelector | AWS 节点组 label | 手动给 k3s-server 打 label |
| runAsNonRoot | 强制开启 | 关闭（镜像未配置非 root 用户） |
| config 挂载路径 | `/data/okcoin/conf/` | `/conf/`（WORKDIR 不同） |
| ConfigMap 来源 | ArgoCD overlay 自动创建 | 手动创建占位 ConfigMap |

---

## 下一步

- [ ] 验证 readiness probe 是否通过（READY 1/1）
- [ ] 部署自动刷新 ECR Token 的 CronJob（12 小时 Token 过期问题）
- [ ] 调查 `runAsNonRoot` 根本解法（Dockerfile 加 USER 指令）
- [ ] 创建 overlay 结构统一管理 k3s 环境差异（避免每次手改副本 YAML）
