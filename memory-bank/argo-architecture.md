# okj-argo-manifests 架构速查

## 仓库定位

GitOps 仓库，是 cdk8s-exchange 生成 YAML 的**部署终点**。
ArgoCD 监听本仓库，自动同步到 EKS 集群。

## 整体结构

```
okj-argo-manifests/
├── base/                    # cdk8s synth 输出物（只读，CI 写入，禁止手动编辑）
│   └── {appName}/           # 每个服务一个目录
│       ├── {appName}.yaml   # Deployment + Service + HPA 等
│       └── kustomization.yaml
├── overlays/                # 环境特化层（Kustomize patches）
│   ├── test/
│   │   └── {appName}/kustomization.yaml
│   └── prod/
│       └── {appName}/kustomization.yaml
├── argocd/                  # ArgoCD 引导层（apply once，不自动更新）
│   ├── test/
│   │   ├── applicationset.yaml  # 监听 overlays/test/*，自动发现应用
│   │   └── project.yaml
│   └── prod/
│       ├── applicationset.yaml
│       └── project.yaml
└── scripts/                 # Python 辅助脚本（生成 AWS SG policy）
```

## 部署模式：ApplicationSet + Git 目录生成器

```
argocd/{env}/applicationset.yaml
  └─ generator: git directories overlays/{env}/*
  └─ 自动为每个目录创建一个 ArgoCD Application
  └─ 命名规则: okj-{env}-{dirname}
```

**新增服务**：在 `overlays/{env}/` 下创建目录 → ApplicationSet 自动发现 → 无需改 ArgoCD 配置。

## 完整数据流

```
okj-cdk8s-exchange
  ↓ make synth / CI
base/{appName}/{appName}.yaml   ← cdk8s 生成，只读
  ↓ Kustomize resources
overlays/{env}/{appName}/kustomization.yaml  ← 环境差异（image tag / replicas / patches）
  ↓ ApplicationSet 监听
ArgoCD Application: okj-{env}-{appName}
  ↓ automated sync
EKS 集群
```

## 多环境区分

| 环境 | 监听路径 | Git 分支 | 命名前缀 |
|------|---------|---------|---------|
| test | overlays/test/* | develop (HEAD) | okj-test-* |
| prod | overlays/prod/* | prod 分支 | okj-prod-* |

## overlay kustomization.yaml 典型结构

```yaml
resources:
  - ../../../base/{appName}   # 引用 cdk8s 生成的 base

patches:
  - op: replace
    path: /spec/securityGroups/groupIds
    value: [sg-xxxx]          # 环境特定的 AWS SG

images:
  - name: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/{appName}
    newTag: 0.0.4             # deployer 服务更新 image tag

replicas:
  - name: {appName}
    count: 2
```

## 自动同步配置

```yaml
syncPolicy:
  automated:
    prune: true      # 删除 Git 中已移除的资源
    selfHeal: true   # 自动修复集群漂移
  syncOptions:
    - CreateNamespace=true
```

## 工具栈

| 工具 | 用途 |
|------|------|
| Kustomize | overlays 渲染（patches + images） |
| Helm | 第三方应用（aws-load-balancer-controller, external-secrets 等） |
| Python scripts | AWS SecurityGroupPolicy / ServiceAccount 自动生成 |
| yamlfmt / yamllint | CI 格式检查（2 空格缩进） |

## 与 cdk8s-exchange 的关系

- cdk8s-exchange 的 `make synth` → 写入本仓库的 `base/`
- cdk8s-exchange 的 `okj-charts/` 对应本仓库的 `base/`（CI 同步，非 git submodule）
- overlay 的 `image.newTag` 由独立 deployer 服务更新（不是 cdk8s 管的）

## 关键设计决策

- base 目录**禁止手动编辑**，所有业务逻辑改动通过 cdk8s 生成
- overlay 只允许改环境差异（镜像 tag、副本数、AWS 资源 ID）
- ApplicationSet git-directory 生成器实现"服务即目录"，零 API 操作
