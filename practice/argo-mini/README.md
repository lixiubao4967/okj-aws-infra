# argo-mini 练习项目

复刻 `okj-argo-manifests` 的三层 GitOps 结构，使用 nginx 和 redis 两个服务。

## 目录结构

```
practice/argo-mini/
├── base/                          # cdk8s synth 输出物（禁止手动编辑）
│   ├── nginx/
│   │   ├── nginx.yaml             # Deployment + Service
│   │   └── kustomization.yaml
│   └── redis/
│       ├── redis.yaml
│       └── kustomization.yaml
├── overlays/                      # 环境特化层
│   ├── dev/
│   │   ├── nginx/kustomization.yaml   # 覆盖 image tag / replicas
│   │   └── redis/kustomization.yaml
│   └── prod/
│       ├── nginx/kustomization.yaml   # 多副本 + 更高资源限制
│       └── redis/kustomization.yaml
└── argocd/                        # ArgoCD 引导层（apply once）
    ├── dev/
    │   ├── project.yaml
    │   └── applicationset.yaml    # 监听 overlays/dev/*，自动发现服务
    └── prod/
        ├── project.yaml
        └── applicationset.yaml    # 监听 overlays/prod/*
```

## 核心概念

### 数据流

```
base/{app}/{app}.yaml        ← cdk8s 生成，只读
  ↓ Kustomize resources
overlays/{env}/{app}/kustomization.yaml   ← 环境差异（replicas / image tag / resources）
  ↓ ApplicationSet git-directory 生成器
ArgoCD Application: argo-mini-{env}-{app}
  ↓ automated sync (prune + selfHeal)
K8s 集群
```

### ApplicationSet 自动发现机制

ApplicationSet 使用 `git directory` 生成器扫描 `overlays/{env}/*`：
- 每个子目录 = 一个服务 = 自动创建一个 ArgoCD Application
- **新增服务**：在 `overlays/{env}/` 下创建目录，ApplicationSet 自动发现，**无需改 ArgoCD 配置**

## 本地验证（无需集群）

```bash
# 验证 dev/nginx overlay 渲染结果
kubectl kustomize practice/argo-mini/overlays/dev/nginx

# 验证 prod/nginx overlay 渲染结果（应看到 replicas:2 和更高资源）
kubectl kustomize practice/argo-mini/overlays/prod/nginx

# 对比 dev 和 prod 的差异
diff \
  <(kubectl kustomize practice/argo-mini/overlays/dev/nginx) \
  <(kubectl kustomize practice/argo-mini/overlays/prod/nginx)
```

## 部署到真实集群（k3s 已验证，2026-04-20）

前提：已有可访问的 K8s 集群 + ArgoCD 已安装

### 安装 ArgoCD

```bash
kubectl create namespace argocd

# 首次安装必须用 --server-side，避免 CRD annotation 超限（262144 字节）
kubectl apply -n argocd \
  -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml \
  --server-side --force-conflicts

# 等待所有 pod 就绪
kubectl get pods -n argocd -w
```

### 获取初始密码

```bash
# 用户名固定为 admin
kubectl get secret argocd-initial-admin-secret \
  -n argocd \
  -o jsonpath='{.data.password}' | base64 -d
```

### apply argo-mini 引导层

```bash
# 1. 创建 AppProject（每个环境执行一次）
kubectl apply -f practice/argo-mini/argocd/dev/project.yaml

# 2. 创建 ApplicationSet（之后 ArgoCD 自动接管）
kubectl apply -f practice/argo-mini/argocd/dev/applicationset.yaml

# 3. 查看自动创建的 Applications
kubectl get applications -n argocd
```

### 手动触发立即同步（默认 3 分钟轮询）

```bash
kubectl annotate application argo-mini-dev-nginx \
  -n argocd argocd.argoproj.io/refresh=normal
kubectl annotate application argo-mini-dev-redis \
  -n argocd argocd.argoproj.io/refresh=normal
```

## 已知坑（k3s 部署，2026-04-20）

| 问题 | 原因 | 解法 |
|------|------|------|
| `kubectl apply` 报 CRD annotation too long | ArgoCD ApplicationSet CRD 超过 262144 字节限制 | 改用 `--server-side --force-conflicts` |
| ApplicationSet 报 `no matches for kind` | 上一步 CRD 没装成功 | 重新 `--server-side` apply |
| `authentication required: Repository not found` | GitHub 私有仓库，ArgoCD 无法拉取 | 仓库改为 public，或配置 PAT Secret |
| nginx pod `CreateContainerConfigError: runAsNonRoot` | base/nginx.yaml 镜像未同步 cdk8s-mini 修复 | 换 `nginxinc/nginx-unprivileged:1.27-alpine`，端口 80→8080 |
| redis pod `CreateContainerConfigError: runAsNonRoot` | base/redis.yaml 缺少 `runAsUser: 999` | container securityContext 加 `runAsUser: 999` |
| selfHeal 无 Event 记录 | ArgoCD 静默修复漂移，不写 Event | 用 `kubectl get deployment nginx -o jsonpath='{.spec.replicas}'` 验证 |

## 练习任务

### 任务 1：理解 Kustomize 渲染
运行 `kubectl kustomize overlays/dev/nginx`，观察 base 与 overlay 的合并结果。
重点观察：`replicas` 和 `progressDeadlineSeconds` 是否被 overlay 正确覆盖。

### 任务 2：新增服务（无需改 ArgoCD）
在 `overlays/dev/` 下新建目录 `postgres/kustomization.yaml`：
```yaml
resources:
  - ../../../base/nginx   # 复用 nginx base 作为占位
replicas:
  - name: nginx
    count: 1
```
思考：ApplicationSet 如何发现这个新服务？命名规则是什么？

### 任务 3：模拟 deployer 更新镜像 tag
在 `overlays/dev/nginx/kustomization.yaml` 中把 `newTag` 改成 `1.28-alpine`：
```yaml
images:
  - name: nginx
    newTag: "1.28-alpine"
```
这模拟了 okj-argo-manifests 中 deployer 服务自动更新 tag 的行为。

### 任务 4：理解 selfHeal
`selfHeal: true` 意味着：如果有人手动 `kubectl edit deployment nginx` 修改了副本数，
ArgoCD 会在下次同步时自动回滚到 Git 中定义的状态。
这是 GitOps "Git 是单一事实来源"原则的具体实现。

## 与 okj-argo-manifests 的对应关系

| argo-mini | okj-argo-manifests |
|-----------|-------------------|
| base/nginx/ | base/uno/ |
| overlays/dev/ | overlays/test/ |
| overlays/prod/ | overlays/prod/ |
| argocd/dev/applicationset.yaml | argocd/test/applicationset.yaml |
| argo-mini-dev-nginx | okj-test-uno |
| develop 分支 | develop 分支 (HEAD) |
| main 分支 | prod 分支 |
