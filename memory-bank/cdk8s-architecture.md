# CDK8s 架构速查（okj-cdk8s-exchange）

## CDK vs CDK8s 核心区别

| 维度 | okj-cdk-exchange (CDK) | okj-cdk8s-exchange (CDK8s) |
|------|----------------------|--------------------------|
| 目标 | AWS 基础设施（EC2/VPC/RDS） | Kubernetes 资源（Deployment/Service/Ingress） |
| 产物 | CloudFormation JSON/YAML | Kubernetes YAML manifests |
| 部署方式 | `cdk deploy` → CloudFormation Stack | `make synth` → okj-charts/ → ArgoCD 同步 |
| 层次叫法 | Construct / Stack / Props / main.go | Spec / Builder / Construct / App |

## 4 层架构

```
Layer 1: internal/spec/          — 服务契约接口（StatelessAppSpec、StatefulAppSpec）
Layer 2: services/{tier}/        — 每个服务实现接口（声明副本数、资源限制等）
Layer 3: services/{tier}/helpers.go — 桥接层，调用 Construct 生成 Chart
Layer 4: internal/constructs/    — 生成实际 K8s 对象（Deployment + Service + HPA + Ingress）
入口:    cmd/main.go             — 遍历 services.All()，调用 BuildFunc()，app.Synth()
注册:    services/registry.go   — All() 返回全部服务（唯一注册点，新增服务在此注册）
```

## 典型数据流

```
OKJWebChart（实现 StatelessAppSpec）
  → BuildFunc() 调用 buildExchangeService(app, c)
  → NewStatelessAppConstruct(chart, props)
  → 生成 Deployment + Service + HPA + Ingress YAML
  → 写入 okj-charts/okj-web/okj-web.yaml
  → ArgoCD 同步到 EKS
```

## 服务分层（services/ 5个子目录）

| 目录 | 用途 | 资源类型 |
|------|------|---------|
| infra/ | 集群基础设施（CRD/Namespace/StorageClass） | Namespace, StorageClass |
| helm/ | ArgoCD Helm Application（预装 operator） | ArgoCD Application CR |
| middleware/ | 可观测性与中间件（Prometheus/Kafka/ES/Mongo） | Deployment, StatefulService CR, DaemonSet |
| exchange/ | 无状态应用服务（~40个） | Deployment + Service |
| aircraft/ | 有状态交易服务（Aeron 集群） | StatefulService CR |
| uno/ | CI/CD 执行器服务（~16个） | Deployment |

## 关键文件

| 文件 | 说明 |
|------|------|
| `cmd/main.go` | 入口：遍历所有服务，每个服务创建独立 App，调用 Synth() |
| `services/registry.go` | 唯一注册点：All() 返回所有服务 |
| `internal/spec/base.go` | BaseServiceSpec（Name/Replicas/Strategy/BizType 默认实现） |
| `internal/spec/stateless.go` | StatelessAppSpec 接口 |
| `internal/spec/types.go` | DeploymentStrategy、BizType、ResourceRequirements |
| `services/exchange/helpers.go` | buildExchangeService()：连接 Spec 层和 Construct 层 |
| `internal/constructs/stateless_app_construct.go` | StatelessAppConstruct：生成 Deployment+Service+HPA+Ingress |
| `cdk8s.yaml` | CDK8s CLI 配置（输出目录、CRD 导入路径） |
| `crds/` | CRD 源文件（source of truth） |
| `imports/` | 由 `make import` 自动生成的 Go 类型绑定，勿手动编辑 |

## 编码约定

- `BaseServiceSpec` 提供默认实现，服务类型嵌入它并按需覆盖方法
- 服务文件只声明需求（副本数/资源/节点亲和），**不写业务逻辑**
- 每个服务实现 `BuildFunc() func(cdk8s.App)` 方法
- `cmd/main.go` 不需要改：只需在 registry.go 注册新服务
- 所有字符串参数用 `jsii.String()`，整数指针用 `ptr.To(n)`
- CPU:内存比例约定 **1 core : 4 GB**（requests 和 limits 独立设置）
- 三级探针：startup（吸收慢启动）→ liveness（检测死进程）→ readiness（控制流量）

## 常用命令

```bash
cd /Users/xiubao.li/Documents/Gitlab/okj-cdk8s-exchange
make fmt        # 格式化 Go 代码
make lint       # 运行 golangci-lint
make test       # 运行 Go 测试
make synth      # 合成 K8s manifests → okj-charts/
make import     # 从 crds/ 重新生成 imports/ 类型绑定
```

## 与 CDK 实践经验的对比

| CDK（cdk-mini 已掌握） | CDK8s（待练习） |
|----------------------|----------------|
| Construct 封装 AWS 资源 | Construct 封装 K8s 对象 |
| Stack 聚合 Construct + 工厂函数 | Builder 函数聚合 Construct |
| Props 总线传跨 Stack ARN | 无需 Props 总线（K8s 资源引用用名字即可） |
| `cdk deploy` 直接部署 | `make synth` 生成 YAML，ArgoCD 同步部署 |
| `jsii.String()` / `jsii.Bool()` | 同样使用 `jsii.String()`，整数用 `ptr.To()` |
