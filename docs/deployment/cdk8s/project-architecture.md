# okj-cdk8s-exchange 架构详解

## 目录结构一览

```
okj-cdk8s-exchange/
├── cmd/main.go                     # 入口：遍历所有服务，依次 Synth
├── cdk8s.yaml                      # CDK8s 配置（语言: go，CRD 导入列表）
├── Makefile                        # 所有开发工作流（setup/synth/check/publish）
│
├── internal/
│   ├── constructs/                 # 可复用的 CDK8s Construct 库
│   │   ├── constructs.go           # 共享类型、工具函数
│   │   ├── stateless_app_construct.go     # Deployment + Service + HPA + Ingress
│   │   ├── stateful_service_construct.go  # StatefulService CR（operator 托管）
│   │   ├── argocd_application_construct.go # ArgoCD Application CR
│   │   ├── cron_job_construct.go          # CronJob
│   │   ├── daemon_set_construct.go        # DaemonSet
│   │   ├── gitlab_runner_construct.go     # GitLab Runner CR
│   │   └── scheduling.go                  # 节点选择器 & 容忍度工具函数
│   └── spec/
│       ├── stateless.go            # StatelessAppSpec 接口（无状态服务规范）
│       ├── stateful.go             # StatefulAppSpec 接口（有状态服务规范）
│       ├── base.go                 # BaseServiceSpec（默认实现）
│       └── types.go                # 共享类型（DeploymentStrategy、BizType）
│
├── services/
│   ├── registry.go                 # 总注册表：All() 返回所有服务列表
│   ├── consts/                     # 常量（命名空间、标签、sync-wave 顺序）
│   ├── infra/                      # 集群基础：命名空间、StorageClass、CRD、Operator
│   ├── helm/                       # Helm 托管的集群 Operator（通过 ArgoCD 部署）
│   ├── middleware/                 # 可观测性与基础中间件（日志/监控）
│   ├── exchange/                   # 交易所业务服务（42 个）
│   ├── aircraft/                   # 撮合引擎服务（27 个）
│   └── uno/                        # CI/CD 执行平台（15 个）
│
├── crds/                           # 原始 CRD YAML（源文件，不要直接编辑）
├── imports/                        # cdk8s import 生成的 Go 类型绑定（不要手动改）
├── docs/                           # 项目内部文档
└── okj-charts/                     # 生成的 YAML 输出目录（git-ignored）
```

---

## 数据流全景

```
cmd/main.go
    ↓ services.All()
    遍历每个 ServiceChart
    ↓ 对每个服务：
    cdk8s.NewApp(outdir: "okj-charts/<name>/")
    chart.BuildFunc()(app)         ← 实例化 Construct，拼装 K8s 资源
    app.Synth()                    ← 写出 YAML 文件
    ↓
okj-charts/<service-name>/<chart>.yaml
    ↓ make publish-manifests
okj-argo-manifests/base/<service>/
    ↓ Kustomize overlay（各环境）
    ↓ ArgoCD sync
EKS 集群
```

---

## Construct 库详解（`internal/constructs/`）

这是整个项目的"积木库"，每种 Construct 对应一种 Kubernetes 工作负载模式。

### StatelessAppConstruct（最常用）

生成：`Deployment + Service + 可选 HPA + 可选 Ingress`

适用场景：所有无状态 HTTP/gRPC 服务

```go
NewStatelessAppConstruct(chart, "okj-auth", &StatelessAppConstructProps{
    Name:      "okj-auth",
    Namespace: "okj-exchange",
    Image:     "097102939699.dkr.ecr.ap-northeast-1.amazonaws.com/test/okj-auth:latest",
    Replicas:  ptr.To(2),

    // 端口配置
    Ports: []ContainerPort{
        {Name: "http", ContainerPort: 7001},
        {Name: "health", ContainerPort: 7002},
    },

    // 资源：遵守 1核:4GB 比例
    Resources: &ResourceRequirements{
        CPURequest: "1",    CPULimit: "1",
        MemoryRequest: "4Gi", MemoryLimit: "4Gi",
    },

    // 健康检查
    LivenessProbe: &ProbeConfig{
        Path: "/health", Port: 7002,
        InitialDelaySeconds: 30, PeriodSeconds: 15,
    },

    // Service
    Service: &ServiceConfig{
        Type: "ClusterIP",
        Ports: map[int]int{7001: 7001, 7002: 7002},
    },

    // 可选：ALB Ingress（公网服务才需要）
    Ingress: &IngressConfig{
        ClassName: "alb",
        Annotations: map[string]string{
            "alb.ingress.kubernetes.io/scheme": "internet-facing",
            // ...
        },
        Rules: []IngressRuleConfig{{Path: "/", ServicePort: 7001}},
    },

    // 节点调度（不同服务用不同节点组）
    NodeSelector: cdkConstructs.EgressNodeSelector(),
    Tolerations:  cdkConstructs.EgressTolerations(),

    // 安全上下文
    PodRunAsUser: ptr.To(int64(1000)),
    PodFsGroup:   ptr.To(int64(1000)),
})
```

### StatefulServiceConstruct

生成：`StatefulService CR`（由 okj-stateful-operator 管理）

适用场景：有状态服务（ES、MongoDB、Etcd、Zookeeper 等）

### ArgocdApplicationConstruct

生成：`Argo CD Application CR`

适用场景：Helm Chart 安装的集群 Operator（ALB Controller、External Secrets 等）

### CronJobConstruct / DaemonSetConstruct

生成：`CronJob / DaemonSet`

---

## Service 接口体系

每个服务必须实现 `ServiceChart` 接口（注册用）+ 选择一种 Spec 接口（描述规格）。

### ServiceChart 接口（注册用）

```go
type ServiceChart interface {
    Name() string                        // 服务名，也是 okj-charts 子目录名
    BuildFunc() func(cdk8s.App)          // 生成 K8s 资源的函数
}
```

### StatelessAppSpec 接口（无状态服务规范）

```go
type StatelessAppSpec interface {
    Name() string
    Replicas() *int                      // nil = 由 HPA 控制
    Resources() *ResourceRequirements    // CPU/内存 requests/limits
    NodeSelector() map[string]string     // 目标节点组
    Tolerations() []Toleration           // 允许调度到哪些有 taint 的节点
    Strategy() DeploymentStrategy        // RollingUpdate | Recreate
    BizType() BizType                    // rest | job
}
```

### BaseServiceSpec（默认实现）

大多数服务继承 `BaseServiceSpec`，只覆盖需要定制的方法：

```go
// BaseServiceSpec 默认值：
// - Replicas() → nil（使用全局默认或 HPA）
// - Resources() → 1核/4GB
// - NodeSelector() → private 节点组
// - Strategy() → RollingUpdate
// - BizType() → rest
```

---

## 节点组与调度

EKS 集群有多种节点组，每个组有不同的 taint，服务需要声明对应的 toleration 才能调度上去。

| 节点组 | 标签 | 适用服务 |
|-------|------|---------|
| `private` | `okj.com/node-group-role: private` | 大多数业务服务 |
| `egress` | `okj.com/egress-access` | 需要出口访问的服务 |
| `uno-private` | `okj.com/uno-access` | UNO 平台服务 |
| `trading` | `okj.com/trading-access` | Aircraft 交易服务 |
| `trading-jove` | `okj.com/trading-jove-access` | Jove 系列（4xlarge，Aeron） |
| `gitlab-runner` | `okj.com/gitlab-runner` | CI/CD Runner |
| `monitor-private` | `okj.com/monitor-access` | 监控服务 |

**调度工具函数**（`internal/constructs/scheduling.go`）：

```go
cdkConstructs.EgressNodeSelector()   // 出口节点
cdkConstructs.PrivateNodeSelector()  // 私有节点（默认）
cdkConstructs.TradingNodeSelector()  // 交易节点
cdkConstructs.UnoNodeSelector()      // UNO 节点
```

---

## ArgoCD Sync-Wave 顺序

ArgoCD 按 sync-wave 顺序部署，wave 越小越先部署：

```
Wave -30  → Namespace、StorageClass（其他所有资源的前提）
Wave -20  → Helm Operator（ALB Controller、External Secrets、ExternalDNS）
Wave -15  → Helm CRD 的 CR（ClusterSecretStore 等，依赖上面的 CRD）
Wave -10  → 自定义 Operator CRD 和 Workload
Wave   0  → 普通业务服务（默认）
```

---

## 资源规格约定

**CPU:内存 = 1核:4GB**，全项目强制执行：

| CPU Request | 内存 Request |
|-------------|-------------|
| `100m` | `400Mi` |
| `500m` | `2Gi` |
| `1` | `4Gi` |
| `2` | `8Gi` |
| `4` | `16Gi` |

Request 和 Limit 独立计算（可以 Request 500m，Limit 2，但各自都遵守 1:4 比例）。

---

## CRD 管理流程

项目自己维护两个 CRD（`crds/` 目录）：
- `okj-stateful-operator.yaml`：StatefulService 自定义资源
- `gitlab-runner-operator.yaml`：GitLab Runner 自定义资源

更新 CRD 后需要重新生成 Go 类型绑定：

```bash
# 1. 更新 crds/*.yaml（放入新版 CRD）
# 2. 重新生成 Go 绑定
make import
# 3. imports/ 目录下的文件自动更新
# 4. 之后代码里可以用类型安全的方式操作 CRD
```

---

## 与 okj-cdk-exchange 的关系

| 维度 | okj-cdk-exchange | okj-cdk8s-exchange |
|------|------------------|--------------------|
| 语言/框架 | Go + AWS CDK | Go + CDK8s |
| 创建什么 | AWS 资源（EKS、VPC、RDS、S3...） | K8s 资源（Deployment、Service、Ingress...） |
| 输出 | CloudFormation 模板 | Kubernetes YAML |
| 部署方式 | `cdk deploy` | `make publish-manifests` → ArgoCD |
| 依赖关系 | 先执行（建集群） | 后执行（往集群里放东西） |

CDK 创建了节点组的 taint，CDK8s 里的 toleration 必须与之匹配，两个仓库需要保持一致。
