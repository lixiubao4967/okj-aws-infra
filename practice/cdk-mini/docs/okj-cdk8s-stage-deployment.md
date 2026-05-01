# okj-cdk8s-exchange Stage 环境部署分析

> 记录 cdk8s stage 环境部署的整体流程、现状与待完成工作。

---

## 整体部署流水线

cdk8s 不直接 apply 到 K8s，走的是 GitOps 路径：

```
okj-cdk8s-exchange
  └─ make synth
       └─ okj-charts/<service>/*.yaml
            └─ CI publish-manifests.sh
                 └─ okj-argo-manifests/base/<service>/
                      └─ ArgoCD ApplicationSet
                           └─ overlays/<env>/<service>/  (环境 patch)
                                └─ EKS 集群
```

**关键脚本**：`ci/publish-manifests.sh`

- Phase 2：运行 `go run cmd/main.go`（即 `make synth`）生成 YAML 到 `okj-charts/`
- Phase 4：把 `okj-charts/<service>/` 复制到 `okj-argo-manifests/base/<service>/` 并自动生成 `kustomization.yaml`
- Phase 5：develop 分支直接 push，prod 分支创建 MR

**分支映射**（硬编码在 `publish-manifests.sh`）：

| cdk8s 分支 | argo-manifests 分支 |
|-----------|-------------------|
| `develop` | `develop` |
| `prod` | `prod` |
| 其他（含 stage）| ❌ 报错退出 |

---

## okj-argo-manifests 结构

```
okj-argo-manifests/
├── base/<service>/          # cdk8s 生成的基础 YAML（CI 自动同步）
│   ├── <service>.yaml
│   └── kustomization.yaml
├── overlays/
│   ├── test/<service>/      # test 环境 patch（主要是 SG ID）
│   │   └── kustomization.yaml
│   └── prod/<service>/      # prod 环境 patch
└── argocd/
    ├── test/
    │   ├── applicationset.yaml   # 监听 overlays/test/*
    │   ├── project.yaml
    │   └── repository.yaml
    └── prod/
        └── ...
```

**overlay patch 示例**（`overlays/test/okj-auth/kustomization.yaml`）：

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- ../../../base/okj-auth
patches:
- patch: |-
    - op: replace
      path: /spec/securityGroups/groupIds
      value:
        - sg-057d6fef80d40b0db   # test 环境的 SG ID
        - sg-0ac9968ecd92ec11a
  target:
    group: vpcresources.k8s.aws
    kind: SecurityGroupPolicy
    name: okj-auth-group-policy
    version: v1beta1
```

**ArgoCD ApplicationSet 逻辑**（`argocd/test/applicationset.yaml`）：

- 监听 `overlays/test/*` 下所有目录，自动为每个目录创建一个 ArgoCD Application
- Application 名称：`okj-test-<service-name>`
- 目标 namespace：`okj-exchange`
- 开启 `automated.prune + selfHeal`

---

## Stage 环境现状（2026-05-01）

### 已完成

- [x] CDK 基础设施部署（VPC、EKS、SSM 参数等）
- [x] `okj-argo-manifests/base/` 已有所有服务的基础 YAML（develop CI 自动同步）

### 待完成

| 工作项 | 位置 | 说明 |
|-------|------|------|
| 添加 stage 分支映射 | `okj-cdk8s-exchange/ci/publish-manifests.sh` | 在 `case` 语句加 `stage) PUBLISH_BRANCH="stage"` |
| 创建 stage overlays | `okj-argo-manifests/overlays/stage/<service>/` | 每个服务一个 kustomization.yaml，patch stage 的 SG ID |
| 创建 stage ArgoCD 配置 | `okj-argo-manifests/argocd/stage/` | applicationset.yaml、project.yaml、repository.yaml |
| 在 stage EKS 上应用 | `kubectl apply -f argocd/stage/` | 创建 ApplicationSet，ArgoCD 接管后续 |

---

## Stage Overlay 创建步骤（待执行）

### 1. 查出 stage 的 Security Group IDs

```bash
# 从 CDK 输出或 AWS 控制台查询
aws ec2 describe-security-groups \
  --filters "Name=vpc-id,Values=vpc-0cb040cb4fe3fd7bb" \
  --query "SecurityGroups[*].{Name:GroupName,Id:GroupId}" \
  --output table
```

### 2. 创建 argocd/stage/ 配置（参考 argocd/test/ 复制改写）

主要差异：
- `metadata.name`：`okj-test-apps` → `okj-stage-apps`
- `directories.path`：`overlays/test/*` → `overlays/stage/*`
- Application 名称模板：`okj-test-{{path.basename}}` → `okj-stage-{{path.basename}}`

### 3. 批量创建 overlays/stage/

可以用脚本批量生成，每个服务的 kustomization.yaml 只差 SG ID：

```bash
for svc in $(ls overlays/test/); do
  mkdir -p overlays/stage/$svc
  # 从 test 复制后替换 SG ID
  cp overlays/test/$svc/kustomization.yaml overlays/stage/$svc/
  # sed 替换 SG ID 为 stage 的值
done
```

### 4. 在 stage EKS 上部署 ArgoCD 配置

```bash
kubectl apply -f argocd/stage/
```

---

## 关键理解

- **`base/` 是环境无关的**：所有环境共用同一份 cdk8s 生成的 YAML，差异全靠 overlay patch
- **overlay patch 的核心**：SecurityGroupPolicy 里的 SG ID（不同环境安全组不同）
- **ArgoCD ApplicationSet 的威力**：只要 `overlays/stage/` 下新增一个目录，ArgoCD 自动为它创建一个 Application，无需手动操作
- **stage 没有 CI 自动同步路径**：publish-manifests.sh 未映射 stage 分支，需要手动或扩展 CI
