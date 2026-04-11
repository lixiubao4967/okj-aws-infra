# CI 镜像管理：okj-k8s-ci

## 这个镜像是什么

`okj-k8s-ci` 是所有 OKJ 仓库 CI Job 使用的**统一基础镜像**，避免每个仓库各自维护工具链。

| 仓库 | 使用的工具 |
|------|-----------|
| okj-cdk-exchange | Go 1.26、aws-cdk、aws-cli、golangci-lint |
| okj-cdk8s-exchange | Go 1.26、cdk8s-cli、golangci-lint、pre-commit |
| okj-argo-manifests | kustomize、yamlfmt、yamllint、Python/ruff、uv、prettier、shfmt |

## 镜像内容（截至 2026-04-11）

### 基础环境
- 基础镜像：Debian 12 slim
- Node.js 24 LTS

### Go 生态
- Go 1.26.0
- golangci-lint 2.10.1

### K8s 工具
- kustomize 5.8.1
- aws-cdk 2.1108.0
- cdk8s 2.204.9

### AWS 工具
- aws-cli v2

### 代码质量工具
- ruff 0.15.2（Python lint/format）
- shfmt 3.12.0（Shell format）
- yamlfmt 0.21.0
- yamllint 1.38.0
- pre-commit 4.2.0
- markdownlint-cli2 0.21.0
- prettier（JS/YAML/MD format）

## 镜像标签策略

| 标签 | 格式 | 用途 |
|------|------|------|
| 不可变标签 | `<YYYYMMDD>-<git-sha>` | 生产 CI Job 锁定版本 |
| 滚动标签 | `latest` | 开发时方便拉取 |

示例：`097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/okj-k8s-ci:20260411-a1b2c3d4`

## 升级工具版本

### 常规版本升级

```bash
# 1. 编辑 Dockerfile，修改对应工具的版本 ARG
vim ci/Dockerfile.okj-k8s-ci

# 修改示例：升级 Go 版本
# ARG GO_VERSION=1.26.0
# 改为
# ARG GO_VERSION=1.27.0

# 2. 更新 ci/images.yaml 中的 smoke_test_cmd（如果版本检测命令输出变化）
vim ci/images.yaml

# 3. 本地验证
IMAGE=okj-k8s-ci make ci-build
docker run --rm okj-k8s-ci:local go version  # 验证工具版本

# 4. 提交
git add ci/
git commit -m "chore(ci): bump Go to 1.27.0"
git push

# 5. GitLab CI 自动构建并推送新镜像
# 各仓库下次 CI 运行时拉取 latest，或手动改 image tag 使用新版
```

### images.yaml 结构

```yaml
# ci/images.yaml
images:
  - name: okj-k8s-ci
    dockerfile: Dockerfile.okj-k8s-ci
    owners:
      - devops-platform           # 变更需要这个组 approve MR
    review_policy: devops-platform
    refresh_cadence: monthly      # 提醒：每月更新一次
    smoke_tests:
      # 每个工具都有一个 smoke test，验证安装成功
      - cmd: go version
        expect_contains: "go1.26"
      - cmd: kustomize version
        expect_contains: "5.8.1"
      - cmd: aws --version
        expect_contains: "aws-cli/2"
      # ... 共 14 个验证
```

## 本地构建镜像

```bash
# 构建单平台镜像（本地测试用，不推送）
IMAGE=okj-k8s-ci make ci-build

# 构建并推送（需要有 registry 权限）
IMAGE=okj-k8s-ci REGISTRY=097102939699.dkr.ecr.ap-northeast-1.amazonaws.com make ci-push

# 查看生成的镜像标签
IMAGE=okj-k8s-ci make ci-tags
# 输出：
# IMMUTABLE_TAG=20260411-a1b2c3d4
# LATEST_TAG=latest
```

## CI Pipeline 构建流程

```
push 到 develop 或 tag
  ↓
check stage（fmt-check + lint）
  ↓
ci-build stage
  - buildx 多架构构建（linux/amd64 + linux/arm64）
  - push 到 ECR：
    okj-k8s-ci:20260411-a1b2c3d4（不可变）
    okj-k8s-ci:latest（移动标签）
  ↓
smoke stage（在新镜像中验证所有工具版本）
```

## 各仓库如何使用

在各仓库的 `.gitlab-ci.yml` 中：

```yaml
# okj-argo-manifests 示例
default:
  image: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/okj-k8s-ci:latest

check:lint:
  script:
    - make lint   # 用镜像中的 yamllint、ruff 等工具
```

如果需要锁定版本（防止 latest 更新导致 CI 行为变化）：

```yaml
default:
  image: 097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/okj-k8s-ci:20260411-a1b2c3d4
```

## 维护建议

- **每月更新**：检查各工具是否有安全更新或重大版本
- **变更须 approve**：修改 Dockerfile 需要 `devops-platform` 组成员 review
- **不可变标签不能复用**：同一天的提交用 git sha 区分
- **smoke 测试必须全过**：任何工具安装失败都会在 CI 暴露
