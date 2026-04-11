# 如何添加新服务

本文演示三种最常见的场景：

- **场景 A**：添加普通交易所业务服务（无状态，HTTP）
- **场景 B**：添加需要定制配置的服务（不同资源/节点组/ingress）
- **场景 C**：添加 UNO gRPC 执行器服务

---

## 前置理解：两步走

```
① 创建服务文件（services/<分类>/okj_xxx.go）
② 注册到总表（services/registry.go）
```

就这两步，然后 `make synth` 就能生成 YAML。

---

## 场景 A：普通 Exchange 无状态服务

适用于大多数 REST API 服务，使用 `buildExchangeService` 帮助函数，只需几行代码。

### Step 1：创建服务文件

**文件：** `services/exchange/okj_my_service.go`

```go
package exchange

import (
    "okj-cdk8s-exchange/internal/ptr"
    "okj-cdk8s-exchange/internal/spec"

    "github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

const MyServiceName = "okj-my-service"

// BuildOKJMyServiceChart 是注册时传入的 BuildFunc
func BuildOKJMyServiceChart(app cdk8s.App) {
    buildExchangeService(app, NewOKJMyServiceChart())
}

// OKJMyServiceChart 实现 StatelessAppSpec + ServiceChart 接口
type OKJMyServiceChart struct {
    BaseStatelessAppSpec  // 继承默认实现（1核/4GB/private节点/RollingUpdate）
}

func NewOKJMyServiceChart() OKJMyServiceChart {
    return OKJMyServiceChart{
        BaseStatelessAppSpec{
            spec.BaseServiceSpec{ServiceName: MyServiceName},
        },
    }
}

// ServiceChart 接口
func (c OKJMyServiceChart) Name() string { return MyServiceName }

func (c OKJMyServiceChart) BuildFunc() func(cdk8s.App) {
    return func(app cdk8s.App) { buildExchangeService(app, c) }
}

// 只覆盖需要定制的方法，其他用默认值
func (OKJMyServiceChart) Replicas() *int {
    return ptr.To(2)
}
```

这样会生成：
- `Deployment`：2 副本，1核/4GB，private 节点
- `Service`：ClusterIP（exchange 默认无 Ingress）
- `ServiceAccount`：自动创建

### Step 2：注册到总表

**文件：** `services/registry.go`

```go
func ExchangeCharts() []ServiceChart {
    return []ServiceChart{
        // ... 现有服务（按字母顺序排列）...
        exchange.NewOKJMyServiceChart(),   // ← 加在对应字母位置
        // ...
    }
}
```

### Step 3：生成并验证

```bash
make synth
# 检查生成的文件
cat okj-charts/okj-my-service/okj-my-service.yaml
```

---

## 场景 B：需要定制配置的服务

假设服务需要：4核/16GB、需要公网 Ingress、使用 egress 节点。

```go
package exchange

import (
    cdkConstructs "okj-cdk8s-exchange/internal/constructs"
    "okj-cdk8s-exchange/internal/ptr"
    "okj-cdk8s-exchange/internal/spec"

    "github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

const PublicAPIName = "okj-public-api"

type OKJPublicAPIChart struct {
    BaseStatelessAppSpec
}

func NewOKJPublicAPIChart() OKJPublicAPIChart {
    return OKJPublicAPIChart{
        BaseStatelessAppSpec{spec.BaseServiceSpec{ServiceName: PublicAPIName}},
    }
}

func (c OKJPublicAPIChart) Name() string { return PublicAPIName }

func (c OKJPublicAPIChart) BuildFunc() func(cdk8s.App) {
    return func(app cdk8s.App) { buildExchangeService(app, c) }
}

// 覆盖：更多资源
func (OKJPublicAPIChart) Resources() *cdkConstructs.ResourceRequirements {
    return &cdkConstructs.ResourceRequirements{
        CPURequest:    "4",
        CPULimit:      "4",
        MemoryRequest: "16Gi",   // 保持 1核:4GB 比例
        MemoryLimit:   "16Gi",
    }
}

// 覆盖：使用 egress 节点（需要访问外部网络）
func (OKJPublicAPIChart) NodeSelector() map[string]string {
    return cdkConstructs.EgressNodeSelector()
}

func (OKJPublicAPIChart) Tolerations() []cdkConstructs.Toleration {
    return cdkConstructs.EgressTolerations()
}

// 覆盖：HPA 控制副本数（不固定）
func (OKJPublicAPIChart) Replicas() *int {
    return nil  // HPA 管理
}
```

如果还需要 Ingress（公网访问），在 `buildExchangeService` 的帮助函数里或直接用 `StatelessAppConstruct` 手工配置：

```go
func (c OKJPublicAPIChart) BuildFunc() func(cdk8s.App) {
    return func(app cdk8s.App) {
        chart := cdk8s.NewChart(app, jsii.String(PublicAPIName), nil)
        cdkConstructs.NewStatelessAppConstruct(chart, PublicAPIName, &cdkConstructs.StatelessAppConstructProps{
            Name:      PublicAPIName,
            Namespace: "okj-exchange",
            Image:     buildECRImage(PublicAPIName),
            Replicas:  nil,
            Resources: c.Resources(),
            NodeSelector: c.NodeSelector(),
            Tolerations: c.Tolerations(),
            // 新增 Ingress 配置
            Ingress: &cdkConstructs.IngressConfig{
                ClassName: "alb",
                Annotations: map[string]string{
                    "alb.ingress.kubernetes.io/scheme":           "internet-facing",
                    "alb.ingress.kubernetes.io/target-type":      "ip",
                    "alb.ingress.kubernetes.io/certificate-arn":  "<your-cert-arn>",
                },
                Rules: []cdkConstructs.IngressRuleConfig{
                    {Path: "/", ServicePort: 8080},
                },
            },
        })
    }
}
```

---

## 场景 C：UNO gRPC 执行器服务

UNO 服务用 `buildSimpleGRPCExecutor` 帮助函数：

**文件：** `services/uno/okj_my_executor.go`

```go
package uno

import (
    "github.com/cdk8s-team/cdk8s-core-go/cdk8s/v2"
)

const MyExecutorName = "okj-my-executor"

type OKJMyExecutorChart struct{}

func NewOKJMyExecutorChart() OKJMyExecutorChart {
    return OKJMyExecutorChart{}
}

func (c OKJMyExecutorChart) Name() string { return MyExecutorName }

func (c OKJMyExecutorChart) BuildFunc() func(cdk8s.App) {
    return func(app cdk8s.App) {
        buildSimpleGRPCExecutor(app, MyExecutorName)
        // buildSimpleGRPCExecutor 内部封装了：
        // - uno-private 节点调度
        // - gRPC 服务端口（默认 50051）
        // - 1核/4GB 资源
        // - UNO namespace
    }
}
```

注册到 UNO 分组：

```go
// services/registry.go
func UnoCharts() []ServiceChart {
    return []ServiceChart{
        // ...
        uno.NewOKJMyExecutorChart(),
    }
}
```

---

## 常见问题

### Q：如何知道该用哪个节点组？

| 服务特征 | 节点组 | 选择器函数 |
|---------|-------|-----------|
| 普通业务服务（无出口需求） | private | `PrivateNodeSelector()` |
| 需要访问外部 API/下载 | egress | `EgressNodeSelector()` |
| UNO CI/CD 任务 | uno-private / uno-egress | `UnoNodeSelector()` |
| Aircraft 撮合核心 | trading | `TradingNodeSelector()` |
| Aeron 高性能交易 | trading-jove | `TradingJoveNodeSelector()` |
| 监控服务 | monitor-private | `MonitorNodeSelector()` |

### Q：如何设置资源规格？

必须遵守 **1核:4GB** 比例，否则会被 code review 拒：

```go
// 正确
CPURequest: "500m", MemoryRequest: "2Gi"   // 0.5核:2GB ✅
CPURequest: "2",    MemoryRequest: "8Gi"   // 2核:8GB ✅

// 错误
CPURequest: "1",    MemoryRequest: "2Gi"   // 1核:2GB ❌（内存太少）
CPURequest: "500m", MemoryRequest: "4Gi"   // 0.5核:4GB ❌（CPU太少）
```

### Q：什么时候需要 Ingress？

| 访问模式 | 是否需要 Ingress |
|---------|----------------|
| 集群内服务互相调用（gRPC/HTTP） | 不需要，用 ClusterIP Service |
| 需要从 EKS 集群外访问（公网或 VPN） | 需要，用 ALB Ingress |
| UNO 执行器（只被 Supervisor 调用） | 不需要 |

### Q：副本数写 nil 还是具体数字？

```go
Replicas() *int {
    return nil           // 由 HPA 控制（流量型服务推荐）
    return ptr.To(2)     // 固定副本（稳定性要求高，或无 HPA 配置）
    return ptr.To(1)     // 单副本（批处理 Job 类型，避免重复处理）
}
```

---

## 完整工作流

```bash
# 1. 创建服务文件
vim services/exchange/okj_my_service.go

# 2. 注册到 registry.go
vim services/registry.go

# 3. 检查代码质量
make check

# 4. 预览生成的 YAML
make synth
cat okj-charts/okj-my-service/okj-my-service.yaml

# 5. 提交
git add services/exchange/okj_my_service.go services/registry.go
git commit -m "feat: add okj-my-service deployment"

# 6. 发布到 okj-argo-manifests（触发 ArgoCD 部署）
make publish-manifests
```
