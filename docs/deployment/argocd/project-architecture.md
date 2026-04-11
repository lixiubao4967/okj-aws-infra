# okj-argo-manifests 架构详解

## 目录结构一览

```
okj-argo-manifests/
├── base/                          # cdk8s CI 自动生成（只读，不要手动改）
│   ├── okj-uno-api/
│   │   ├── kustomization.yaml
│   │   └── okj-uno-api.yaml      # Deployment + Service + Ingress + SecurityGroupPolicy
│   ├── okj-namespaces/           # Namespace 定义
│   ├── okj-helm-external-secrets/# ArgoCD Application CR（包 Helm chart）
│   └── 105+ 个其他服务目录/
│
├── overlays/
│   ├── test/                     # 测试集群差异配置
│   │   ├── okj-uno-api/
│   │   │   └── kustomization.yaml  # 引用 base，打 SG/Ingress/镜像 patch
│   │   └── 100+ 其他服务/
│   └── prod/                     # 生产集群差异配置
│       ├── okj-uno-api/
│       │   └── kustomization.yaml
│       └── 100+ 其他服务/
│
├── argocd/
│   ├── core/                     # ArgoCD 安装 manifest（已打补丁的版本）
│   ├── test/
│   │   ├── applicationset.yaml   # 自动发现 overlays/test/* 的 ApplicationSet
│   │   ├── project.yaml          # AppProject（RBAC）
│   │   └── repository.yaml       # GitLab 仓库凭证
│   └── prod/
│       ├── applicationset.yaml   # 自动发现 overlays/prod/*
│       ├── project.yaml
│       └── repository.yaml
│
├── configs/
│   ├── test.yaml                 # 测试环境服务配置（SG名、ingress主机名）
│   └── prod.yaml                 # 生产环境服务配置
│
├── scripts/                      # Python 帮助脚本
│   ├── create_overlay.py         # 创建 overlay 目录
│   ├── delete_app.py             # 删除服务
│   ├── check_overlay_coverage.py # 验证所有 base 都有 overlay
│   ├── check_overlay_sgp.py      # 验证 SG 配置一致性
│   └── update_test_configs.py    # 从 configs/*.yaml 生成 overlay
│
└── docs/                         # 项目内部文档
```

---

## Base 层：cdk8s 生成的内容

**原则：base/ 由 cdk8s CI 完全托管，任何手动修改都会被下次 CI 覆盖。**

每个服务在 base/ 下有一个目录，包含 cdk8s synth 生成的所有 K8s 资源：

```yaml
# base/okj-uno-api/okj-uno-api.yaml 包含的资源：

# 1. SecurityGroupPolicy（VPC 安全组绑定，groupIds 在 base 中为空，由 overlay 填充）
apiVersion: vpcresources.k8s.aws/v1beta1
kind: SecurityGroupPolicy
metadata:
  name: okj-uno-api-group-policy
  namespace: okj-uno
spec:
  podSelector:
    matchLabels:
      app: okj-uno-api
  securityGroups:
    groupIds: []    # ← overlay 会在这里注入实际的 SG ID

---
# 2. ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: okj-uno-api
  namespace: okj-uno
  automountServiceAccountToken: false

---
# 3. Deployment（1核/4GB/1副本/private节点）
apiVersion: apps/v1
kind: Deployment
metadata:
  name: okj-uno-api
  namespace: okj-uno
spec:
  replicas: 1
  template:
    spec:
      containers:
        - name: okj-uno-api
          image: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-uno-api:latest
          # ↑ "latest" 只是占位符，overlay 的 images: 会替换成实际 tag
          resources:
            requests: {cpu: "1", memory: "4Gi"}
            limits:   {cpu: "1", memory: "4Gi"}
      nodeSelector:
        okj.com/node-group-role: uno-private
      tolerations:
        - key: okj.com/uno-access
          value: "true"
          effect: NoSchedule

---
# 4. Service（ClusterIP）
apiVersion: v1
kind: Service
metadata:
  name: okj-uno-api
  namespace: okj-uno
spec:
  type: ClusterIP
  ports:
    - name: http
      port: 7001

---
# 5. Ingress（ALB，groupIds/证书/域名在 overlay 填充）
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: okj-uno-api
  # certificate-arn、group.name 等在 overlay 填充
```

---

## Overlay 层：环境差异配置

Overlay 的核心是 `kustomization.yaml`，它做三件事：

### 1. 引用 base

```yaml
resources:
  - ../../../base/okj-uno-api   # 相对路径指向 base 目录
```

### 2. 打 Patch（JSON Patch 格式）

```yaml
patches:
  # 填充 SecurityGroupPolicy 的 SG ID（每个环境不同）
  - patch: |-
      - op: replace
        path: /spec/securityGroups/groupIds
        value:
          - sg-0e928c807cbdbe4f0   # ← 测试环境的实际 SG ID
          - sg-0ac9968ecd92ec11a
    target:
      group: vpcresources.k8s.aws
      kind: SecurityGroupPolicy
      name: okj-uno-api-group-policy
      version: v1beta1

  # 填充 Ingress 的证书、ALB 分组、域名
  - patch: |-
      apiVersion: networking.k8s.io/v1
      kind: Ingress
      metadata:
        name: okj-uno-api
        annotations:
          alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:...:certificate/test-cert
          alb.ingress.kubernetes.io/group.name: okj-test
          external-dns.alpha.kubernetes.io/hostname: test-uno-api.okqa.work
    target:
      kind: Ingress
      name: okj-uno-api
```

### 3. 覆盖镜像 tag（Deployer 服务负责更新这里）

```yaml
images:
  - name: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-uno-api
    newName: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-uno-api
    newTag: 8e540803   # ← Deployer 每次部署只改这一行
```

### 测试 vs 生产的关键差异

| 配置项 | 测试环境 | 生产环境 |
|-------|---------|---------|
| 镜像 registry | 097102939699.dkr.ecr... /test/ | 738595724739.dkr.ecr... /prod/ |
| 域名后缀 | .okqa.work | .okcoin.tokyo |
| ALB group | okj-test | okj-prod |
| 副本数 | 通常更少 | 通常更多 |
| Sidecar | 有 debugger 调试容器 | 无 |
| SG ID | 测试环境 VPC 中的 SG | 生产环境 VPC 中的 SG |

---

## ArgoCD ApplicationSet：自动发现机制

这是整个架构最关键的设计。**一个 ApplicationSet 可以管理 100+ 个服务**，不需要手动创建每个 Application。

```yaml
# argocd/test/applicationset.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: okj-test-apps
  namespace: argocd
spec:
  generators:
    - git:
        repoURL: https://gitlab.okcoin.tokyo/devops/.../okj-argo-manifests.git
        revision: HEAD
        directories:
          - path: overlays/test/*    # ← 监听这个路径下的所有子目录
  template:
    metadata:
      name: okj-test-{{path.basename}}   # okj-test-okj-uno-api
      annotations:
        # 告诉 ArgoCD：渲染时除了 overlay 还要包含 base 的路径
        argocd.argoproj.io/manifest-generate-paths: ".;/base/{{path.basename}}"
    spec:
      source:
        path: "{{path}}"           # 使用 overlays/test/okj-xxx 目录
      destination:
        server: https://kubernetes.default.svc
      syncPolicy:
        automated:
          prune: true              # Git 删了资源 → 集群也删
          selfHeal: true           # 集群状态漂移 → 自动修复
```

**效果**：在 `overlays/test/` 下新建一个目录，ArgoCD 就自动创建对应的 Application 并开始同步，无需任何额外配置。

---

## Sync Wave：部署顺序控制

ArgoCD 按 annotation 中的 wave 顺序部署，数字越小越先：

```
Wave -30  命名空间（所有资源的前提）
  ↓
Wave -20  Helm Operator（external-secrets、external-dns、ALB Controller）
  ↓
Wave -15  依赖 Helm CRD 的 CR（ClusterSecretStore）
  ↓
Wave -10  自定义 Operator CRD
  ↓
Wave   0  普通业务服务（默认，绝大多数服务）
```

wave 是在 `base/` 里的 YAML 中通过 annotation 设置的（由 cdk8s 控制）：
```yaml
metadata:
  annotations:
    argocd.argoproj.io/sync-wave: "-30"
```

---

## 完整部署链路

```
① 开发者在 okj-cdk8s-exchange 修改服务定义
       ↓
② cdk8s CI 运行 make synth，生成 YAML
       ↓
③ CI 把生成的 YAML commit 到 okj-argo-manifests/base/<服务名>/
       ↓
④ 新镜像构建完成，Deployer 服务读取镜像 tag
       ↓
⑤ Deployer 更新 overlays/{env}/{服务名}/kustomization.yaml 中的 images.newTag
       ↓
⑥ Deployer git push 到 okj-argo-manifests
       ↓
⑦ ArgoCD 检测到 Git 变化（webhook 或轮询）
       ↓
⑧ ArgoCD 执行 kustomize build overlays/{env}/{服务名}/
   （自动合并 base + overlay 的 patch + 镜像 tag 替换）
       ↓
⑨ ArgoCD 对比渲染结果与集群现状
       ↓
⑩ ArgoCD 应用差异（create/update/delete）
       ↓
⑪ Kubernetes 拉取新镜像，滚动更新 Pod
```

---

## AWS 集成机制

### SecurityGroupPolicy

EKS 使用 VPC CNI 的 SecurityGroupPolicy 把 AWS 安全组直接绑定到 Pod 网卡（不是节点级别）：

```yaml
# 每个服务都有这个 CR，控制 Pod 的网络访问
kind: SecurityGroupPolicy
spec:
  securityGroups:
    groupIds:
      - sg-xxx   # Pod 被分配这个 SG，决定进出网络规则
```

SG ID 在 overlay 中注入，测试和生产的 VPC 里 SG ID 不同。

### EKS Pod Identity（不用 IRSA）

服务不需要在 ServiceAccount 上注解 IAM Role ARN。AWS 侧的 Pod Identity Association（由 okj-cdk-exchange 创建）直接把 IAM Role 绑定到 ServiceAccount，Kubernetes 侧无感知。

### External Secrets Operator

从 AWS Secrets Manager 自动同步 Secret 到 K8s：

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
spec:
  secretStoreRef:
    name: aws-secrets-manager
    kind: ClusterSecretStore
  target:
    name: my-k8s-secret      # 在 K8s 中创建的 Secret 名
  data:
    - secretKey: db-password
      remoteRef:
        key: okj-exchange-test-my-service   # AWS Secrets Manager 中的 key
        property: database.password
```

---

## AppProject RBAC

三种角色，对应不同职责：

| 角色 | GitLab 组 | 权限 |
|------|---------|------|
| admin | okj-admins | 完全控制（create/delete/sync） |
| developer | okj-developers | 读取 + 手动同步 |
| viewer | okj-viewers | 只读 |

---

## 服务分类（100+ 服务）

| 分类 | 节点组 | 代表服务 |
|------|-------|---------|
| UNO 平台 | uno-private | okj-uno-api、okj-uno-gateway、okj-*-executor |
| 业务服务 | private/egress | okj-auth、okj-spot-rest、okj-wallet-* |
| 撮合引擎 | trading/trading-jove | okj-aircraft-*、okj-jove-* |
| 监控 | monitor-private | okj-monitor-grafana、okj-monitor-vm* |
| 日志 | monitor-private | okj-log-kafka、okj-log-clickhouse |
| 基础设施 | egress | external-secrets、external-dns、aws-lbc |
