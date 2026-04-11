# 工具详解：aeron-toolkit & jvm-toolkit

两个工具都用 Rust 编写，编译为 **musl 静态二进制**（不依赖任何系统库），支持 amd64 和 arm64。

---

## aeron-toolkit：Aeron 集群健康探针

### 解决什么问题

Aeron 是一个高性能消息中间件，OKJ 的撮合引擎（Jove 系列）使用 Aeron Cluster 实现高可用。
传统的健康检查（HTTP `/health`）只能知道进程还活着，无法判断 **Aeron 集群状态是否正常**（是否加入了集群？是否是 Leader？）。

aeron-toolkit 通过**直接读取 Aeron 的内存映射文件**（`cluster-mark.dat`、`cnc.dat`）获取集群状态，
无需侵入 JVM 进程，也不需要 Aeron 暴露任何 API。

### 工作原理

```
Aeron 进程（JVM）
    ↓ 写入
/data/.../consensus-module/cluster-mark.dat  ← SBE 格式二进制文件
/dev/shm/aeron-*/cnc.dat                     ← Agrona 计数器文件
    ↑ 只读 mmap
aeron-toolkit（Rust 进程）
    ↓ 解析
集群状态（成员 ID、角色、任期号、是否健康）
```

### 子命令

| 命令 | 作用 | 典型用途 |
|------|------|---------|
| `status` | 健康检查，输出 JSON/text | K8s liveness/readiness 探针 |
| `serve` | HTTP sidecar（`/health` `/leader` `/metrics`） | 持续运行，供探针轮询 |
| `describe` | 打印 cluster-mark.dat 元数据 | 调试集群配置 |
| `errors` | 解析并显示 Aeron 错误日志 | 排查集群异常 |
| `recording-log` | 显示 snapshot/term 历史 | 排查日志回放问题 |
| `pid` | 提取进程 PID | 脚本自动化 |

### 退出码约定

| 退出码 | 含义 |
|-------|------|
| 0 | 健康 |
| 10 | 不健康（集群状态异常） |
| 11 | 文件未找到（Aeron 还未启动） |
| 12 | 解析错误（文件损坏或版本不匹配） |
| 13 | 配置/参数错误 |

### 使用方式

**作为 K8s sidecar（生产用法）：**

```yaml
# Pod spec 中
containers:
  - name: jove-order
    image: okj-jove-order:latest
    # ...

  - name: aeron-probe          # ← sidecar
    image: aeron-toolkit:latest
    command: ["aeron-toolkit", "serve"]
    args:
      - --cluster-dir=/data/okcoin/jove-order/consensus-module
      - --port=8080

livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 60
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /health
    port: 8080
  periodSeconds: 3
```

**`serve` 的 HTTP 端点：**

| 路径 | 说明 |
|------|------|
| `GET /health` | 集群是否健康（exit 0 = 200，exit 10 = 503） |
| `GET /leader` | 是否是 Leader（非 Leader 返回 503） |
| `GET /status` | 完整 JSON 状态信息 |
| `GET /metrics` | Prometheus 格式指标 |

`serve` 模式还会**动态修改 Pod 的 label**（通过 K8s API）：
- Leader → `aeron-cluster-role: leader`
- Follower → `aeron-cluster-role: follower`

这样 K8s Service 可以用 label selector 把流量只路由到 Leader。

**命令行直接使用（调试时）：**

```bash
# 检查健康状态，输出可读文本
aeron-toolkit status \
  --cluster-dir /data/okcoin/okcoin-jove-order/39/consensus-module \
  --probe cluster \
  --output text

# 只有 Leader 才健康（用于 Leader 专属探针）
aeron-toolkit status \
  --cluster-dir /data/... \
  --probe leader

# 查看错误日志
aeron-toolkit errors --cluster-dir /data/...

# 查看快照历史
aeron-toolkit recording-log --cluster-dir /data/...
```

---

## jvm-toolkit：JVM Pod 诊断工具

### 解决什么问题

Java 服务出现内存问题、连接堆积、GC 停顿时，传统方法是 `jmap`、`jstat`、`jstack`，
但这些工具需要 JDK（生产镜像通常只有 JRE），而且会短暂 Stop-the-World。

jvm-toolkit 通过读取 **Linux `/proc` 文件系统**获取信息，完全不侵入 JVM：
- 内存：读 `/proc/<pid>/status` 和 `/proc/<pid>/smaps`
- 连接：读 `/proc/<pid>/fd` 和 `/proc/<pid>/net/tcp`
- GC：读 GC 日志文件（或 Spring Boot Actuator HTTP API）

### 子命令

#### `mem`：内存分布分析

```bash
jvm-toolkit mem              # 自动检测 Java 进程
jvm-toolkit mem --pid 12345  # 指定 PID
```

输出示例：
```
=== VM Memory (PID 12345) ===
RSS:           2048 MB   # 实际物理内存
Peak RSS:      2200 MB
Swap:             0 MB

=== Memory Regions ===
Heap:           800 MB   # Java 堆
Code Cache:      64 MB   # JIT 编译代码
Aeron mmap:     256 MB   # Aeron 共享内存
Anon rw:        128 MB   # 其他匿名内存
Mapped files:    32 MB
```

#### `conn`：网络连接统计

```bash
jvm-toolkit conn              # 自动检测 Java 进程
jvm-toolkit conn --pid 12345
```

输出示例：
```
=== Connections by Remote Host ===
10.130.1.50:3306  ESTABLISHED: 10  CLOSE_WAIT: 2
10.130.2.100:6379 ESTABLISHED: 5
```

`CLOSE_WAIT` 积累是连接泄漏的典型信号。

#### `gc`：GC 停顿分析

```bash
jvm-toolkit gc --log-path /data/okcoin/logs/gc.log
jvm-toolkit gc  # 从 Spring Boot Actuator 获取
```

输出示例：
```
=== GC Analysis ===
Pauses:    p50=12ms  p95=45ms  p99=120ms  max=280ms
Alloc:     234 MB/s
Frequency: 8.5 /min
Last GC:   3s ago
```

### 注入到容器的方式

通常在 Dockerfile 中从 S3 拉取二进制，或通过 initContainer 注入：

```dockerfile
# 方式一：Dockerfile 直接安装
RUN curl -o /usr/local/bin/jvm-toolkit \
  https://s3.../okj-ops-kit/jvm-toolkit/latest/jvm-toolkit-linux-amd64 && \
  chmod +x /usr/local/bin/jvm-toolkit
```

```yaml
# 方式二：initContainer 注入到 emptyDir
initContainers:
  - name: install-tools
    image: okj-ops-kit-installer:latest
    command: [cp, /tools/jvm-toolkit, /shared/jvm-toolkit]
    volumeMounts:
      - name: tools
        mountPath: /shared
volumes:
  - name: tools
    emptyDir: {}
```

---

## 构建与发布流程

### Rust 代码结构

```
tools/
├── aeron-toolkit/
│   ├── Cargo.toml         # 版本号在这里（改版本触发 S3 上传）
│   ├── build.rs           # 构建时自动下载 Aeron SBE schema，生成类型偏移量
│   └── src/
│       ├── main.rs        # CLI 入口（clap derive）
│       ├── mark_file.rs   # cluster-mark.dat 解析（SBE 格式）
│       ├── cnc.rs         # cnc.dat 计数器解析
│       ├── probe.rs       # 健康状态状态机
│       ├── serve.rs       # HTTP sidecar（tiny_http + mmap volatile 轮询）
│       ├── error_log.rs   # Aeron 错误日志解析
│       └── recording_log.rs # 快照/term 历史解析
│
└── jvm-toolkit/
    ├── Cargo.toml
    └── src/
        ├── main.rs        # CLI 入口
        ├── mem.rs         # /proc/<pid>/status + smaps 解析
        ├── conn.rs        # /proc/<pid>/net/tcp 解析 + socket inode 匹配
        ├── gc.rs          # GC 日志解析（G1GC 格式）+ Actuator fallback
        ├── pid.rs         # 自动检测 Java 进程 PID
        └── actuator.rs    # Spring Boot Actuator HTTP 客户端
```

### CI 发布流程

```
代码 push / MR
  ↓
check（fmt + lint + test）并行运行
  ↓
build（musl 静态编译，amd64 和 arm64）
  ↓
smoke（在 Corretto 镜像中验证退出码）
  ↓
version 变更检测（对比 Cargo.toml 和 S3 的 version.txt）
  ↓ 版本不同时
upload 到 S3：
  s3://bucket/okj-ops-kit/aeron-toolkit/latest/aeron-toolkit-linux-amd64
  s3://bucket/okj-ops-kit/aeron-toolkit/latest/aeron-toolkit-linux-arm64
  s3://bucket/okj-ops-kit/aeron-toolkit/latest/version.txt（写入新版本号）
```

### 版本升级步骤

```bash
# 1. 修改 Cargo.toml 中的 version
vim tools/aeron-toolkit/Cargo.toml
# version = "0.1.0" → "0.2.0"

# 2. 本地验证
make check

# 3. 提交
git add tools/aeron-toolkit/Cargo.toml
git commit -m "feat(aeron-toolkit): 添加新功能描述"
git push

# 4. CI 自动构建并上传到 S3（无需手动操作）
# amd64 build 是 manual job，需要在 GitLab CI 页面手动触发
```

---

## 设计原则

- **无 async**：HTTP 服务用 `tiny_http`，轮询用 `std::thread`，避免引入 tokio 的复杂性
- **musl 静态链接**：二进制在任何 Linux 容器里都能直接运行，无依赖
- **零侵入**：通过文件系统读取状态，不发送信号、不修改目标进程
- **退出码语义**：0=成功，10=探针失败，11=文件不存在，12=解析错误，13=参数错误
- **最小依赖**：只引入必要的 crate（clap、memmap2、serde_json、ureq、tiny_http）
