# 操作手册

## 场景一：添加新服务到 GitOps 流水线

通常你不需要手动操作 base/，因为 cdk8s CI 会自动提交。但如果需要手动补 overlay，步骤如下。

### Step 1：确认 base/ 已存在

```bash
ls base/okj-my-service/
# 应该看到：kustomization.yaml  okj-my-service.yaml
```

如果不存在，先在 okj-cdk8s-exchange 运行 `make synth` 并让 CI 提交。

### Step 2：创建 overlay（脚本自动生成框架）

```bash
# 创建测试和生产两个 overlay
make create-overlay APP=okj-my-service

# 或指定单个环境
make create-overlay APP=okj-my-service ENV=test
make create-overlay APP=okj-my-service ENV=prod
```

生成的文件结构：
```
overlays/test/okj-my-service/kustomization.yaml
overlays/prod/okj-my-service/kustomization.yaml
```

### Step 3：补充环境差异配置

编辑 `overlays/test/okj-my-service/kustomization.yaml`，添加必要的 patch：

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../../../base/okj-my-service

patches:
  # 1. 填充安全组（必须）
  - patch: |-
      - op: replace
        path: /spec/securityGroups/groupIds
        value:
          - sg-xxxxxxxxxxxxxxxxx   # 从 configs/test.yaml 或 AWS Console 获取
          - sg-0ac9968ecd92ec11a   # 通用 EC2 SG
    target:
      group: vpcresources.k8s.aws
      kind: SecurityGroupPolicy
      name: okj-my-service-group-policy
      version: v1beta1

  # 2. 如果有 Ingress，填充证书和域名
  - patch: |-
      apiVersion: networking.k8s.io/v1
      kind: Ingress
      metadata:
        name: okj-my-service
        annotations:
          alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:ap-northeast-1:097102939699:certificate/test-cert-id
          alb.ingress.kubernetes.io/group.name: okj-test
          alb.ingress.kubernetes.io/security-groups: sg-03f926aa195f9e7e6
          external-dns.alpha.kubernetes.io/hostname: test-my-service.okqa.work
    target:
      kind: Ingress
      name: okj-my-service

  - patch: |-
      - op: add
        path: /spec/rules/0/host
        value: test-my-service.okqa.work
    target:
      kind: Ingress
      name: okj-my-service

# 3. 初始镜像 tag（Deployer 之后会自动更新这里）
images:
  - name: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-my-service
    newName: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-my-service
    newTag: latest

replicas:
  - count: 1
    name: okj-my-service
```

**生产 overlay** 同理，但：
- SG ID 换成 prod VPC 的
- 证书 ARN 换成 prod 的（`738595724739` 账号下）
- 域名换 `.okcoin.tokyo`
- `newName` 中的账号换成 `738595724739`，`/test/` 换成 `/prod/`
- group.name 改为 `okj-prod`

### Step 4：注册到 configs/

在 `configs/test.yaml` 和 `configs/prod.yaml` 中添加服务配置（用于脚本校验和 SG 生成）：

```yaml
# configs/test.yaml
services:
  okj-my-service:
    sgs:
      - service-group-business   # 服务属于哪个安全组分组
    replicas: 1
    # 如果有公网 Ingress
    ingress:
      name: okj-my-service
      type: public
      sg: alb-office-ingress-limited
      hostname: test-my-service.okqa.work
```

### Step 5：验证 + 提交

```bash
# 验证 overlay 语法正确
make build OVERLAY=overlays/test/okj-my-service
make build OVERLAY=overlays/prod/okj-my-service

# 全量检查
make check

# 提交
git add base/okj-my-service overlays/test/okj-my-service overlays/prod/okj-my-service configs/
git commit -m "feat: add okj-my-service to GitOps pipeline"
git push
```

### Step 6：ArgoCD 自动创建 Application

Push 后 1~2 分钟内，ArgoCD 自动发现新目录并创建：
- `okj-test-okj-my-service`（测试集群）
- `okj-prod-okj-my-service`（生产集群）

无需手动操作 ArgoCD。

---

## 场景二：手动更新镜像 tag（紧急情况）

正常情况由 Deployer 服务自动更新，紧急时可手动：

```bash
cd overlays/test/okj-my-service

# 方法1：直接编辑 kustomization.yaml 中的 newTag 字段
vim kustomization.yaml

# 方法2：用 kustomize CLI
kustomize edit set image \
  097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-my-service=097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-my-service:abc123

git commit -m "deploy: okj-my-service test → abc123"
git push
# ArgoCD 自动同步，几秒后生效
```

---

## 场景三：删除服务

```bash
# 预览（不实际删除）
make delete-app APP=okj-old-service DRY_RUN=1

# 实际删除 base/ + 所有 overlays/
make delete-app APP=okj-old-service

git add -A
git commit -m "chore: remove okj-old-service"
git push
# ArgoCD 检测到 overlays/ 目录消失，自动删除对应 Application 和 K8s 资源
```

---

## 场景四：验证所有服务都有 overlay

```bash
# 检查 base/ 下的所有服务是否都有 test + prod 两个 overlay
make check-overlay-coverage

# 检查 SG 配置是否与 configs/*.yaml 一致
make check-overlay-sgp
```

---

## 场景五：渲染并查看完整的 K8s YAML（不部署）

```bash
# 预览 ArgoCD 实际部署到集群的内容
make build OVERLAY=overlays/test/okj-uno-api

# 或直接用 kustomize
kustomize build overlays/test/okj-uno-api
```

---

## 场景六：添加只在某个环境存在的额外资源

有时需要在某个环境的 overlay 里附加额外的 K8s 资源（如 ExternalSecret、ConfigMap）：

```bash
# 1. 在 overlay 目录下创建额外资源文件
cat > overlays/test/okj-log-kafka/external-secret.yaml << 'EOF'
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: okj-log-kafka-cluster-secret
  namespace: okj-monitor
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: ClusterSecretStore
  target:
    name: okj-log-kafka-cluster-secret
  data:
    - secretKey: cluster-id
      remoteRef:
        key: okj-exchange-test-uno-global
        property: log.kafka.cluster-id
EOF

# 2. 在 kustomization.yaml 的 resources 中引用它
# （在 - ../../../base/okj-log-kafka 之后添加）
#   - external-secret.yaml
```

---

## 常用命令速查

```bash
# 开发验证
make check                                    # 完整检查（必须通过才能提交）
make fmt                                      # 自动修复格式问题
make validate                                 # kustomize build 所有 overlay

# 单个 overlay 操作
make build OVERLAY=overlays/test/okj-auth     # 渲染预览
make create-overlay APP=okj-xxx               # 创建新 overlay
make create-overlay APP=okj-xxx ENV=test      # 只创建测试 overlay
make delete-app APP=okj-xxx                   # 删除服务（所有环境）

# 检查工具
make check-overlay-coverage                   # 验证 base/overlay 覆盖
make check-overlay-sgp                        # 验证 SG 配置

# ArgoCD bootstrap（首次部署集群时用）
make argocd-core-apply                        # 安装 ArgoCD
kubectl apply -f argocd/test/project.yaml     # 创建 AppProject
kubectl apply -f argocd/test/repository.yaml  # 添加仓库凭证
kubectl apply -f argocd/test/applicationset.yaml # 启动自动发现
```

---

## 常见问题

### Q：修改了 base/ 但 ArgoCD 没有更新？

ArgoCD 默认轮询间隔几分钟。可以在 ArgoCD UI 手动点 "Refresh"，或等轮询触发。

### Q：overlay kustomize build 报错怎么办？

```bash
# 本地复现
kustomize build overlays/test/okj-xxx
# 看具体报错，通常是：
# - 引用了 base 中不存在的资源名
# - JSON Patch 的 path 写错
# - YAML 格式错误（用 make fmt 修复）
```

### Q：新服务 ArgoCD 已创建 Application 但一直显示 Unknown/Error？

检查：
1. `kustomize build overlays/test/okj-xxx` 本地能否成功
2. SecurityGroupPolicy 中的 SG ID 是否真实存在于该 VPC
3. Namespace 是否已创建（`base/okj-namespaces/` 里是否有对应 namespace）
4. 镜像是否存在于 ECR（`newTag` 指向的镜像 tag）

### Q：如何区分哪些 overlay 需要 Ingress patch？

只有对外提供 HTTP 服务的才需要 Ingress。判断依据：
- 看 `base/<服务名>/<服务名>.yaml` 里是否有 `kind: Ingress` 资源
- 如果有，overlay 就需要打 Ingress patch 填充证书和域名

### Q：生产镜像 tag 和测试不同，怎么管理？

Deployer 服务独立管理测试和生产的 tag。两个环境的 overlay 中 `images.newTag` 字段独立，互不影响。
