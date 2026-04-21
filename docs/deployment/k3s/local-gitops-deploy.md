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

## 已知问题与待办

### 问题一：ECR 镜像认证（待解决）

所有业务服务镜像均在 `097102939699.dkr.ecr.ap-northeast-1.amazonaws.com`，
k3s 节点拉取前需要认证。解决方案待配置后补充。

### 问题二：NodeSelector 不匹配

服务代码中有 AWS 专用节点选择器（如 `MonitorEgressNodeSelector()`），
k3s 节点没有对应 label，Pod 会无法调度（Pending）。

解决方法：给 k3s 节点打上对应 label：

```bash
kubectl label node k3s-server okj.com/node-group-role=<role-name>
```

具体 role 值需查各服务的 Go 源码。

### 问题三：ConfigMap overlay 缺失

部分服务（如 `okj-monitor-alert-webhook`）依赖 overlay 里的 `config.yaml` ConfigMap，
CDK8s 只生成通用部分，环境特定配置在 `okj-argo-manifests/overlays/` 中管理。
直接 apply CDK8s 生成的 YAML 时这部分 ConfigMap 不存在。

---

## 下一步

- [ ] 配置 k3s ECR 镜像认证（imagePullSecret 或 registries.yaml）
- [ ] 部署一个带镜像的服务（计划：`okj-monitor-alert-webhook`）
- [ ] 补充 ECR 认证配置步骤到本文档
