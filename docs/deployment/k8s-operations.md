# K8s 运维操作手册

## 从 Pod 导出文件

### 方案一：kubectl cp（需要容器内有 tar）

```bash
kubectl cp <namespace>/<pod-name>:/path/to/file ./local-file

# 示例
kubectl cp okj-exchange/my-pod:/tmp/recording.jfr ./recording.jfr
```

### 方案二：base64 传输（容器内无 tar 时使用）

适用于二进制文件（JFR、dump 等），几乎所有镜像都有 `base64`。

```bash
kubectl exec -n <namespace> <pod-name> -- \
  base64 /tmp/recording.jfr | base64 --decode > ./recording.jfr
```

验证文件完整性：

```bash
xxd ./recording.jfr | head -2
```

> **注意**：base64 传输会使文件体积增大约 33%，适合几十 MB 以内的文件。

---

## Java Flight Recorder（JFR）采集

### 1. 在 Pod 内启动录制

```bash
kubectl exec -it -n <namespace> <pod-name> -- bash

# 查找 Java 进程 PID（通常为 1）
jcmd

# 启动录制（写入 /tmp，根目录通常只读）
jcmd 1 JFR.start name=mem duration=180s filename=/tmp/recording.jfr
```

### 2. 等待完成后导出

```bash
# 若容器有 tar
kubectl cp <namespace>/<pod-name>:/tmp/recording.jfr ./recording.jfr

# 若容器无 tar（如 okj-report-job 等精简镜像）
kubectl exec -n <namespace> <pod-name> -- \
  base64 /tmp/recording.jfr | base64 --decode > ./recording.jfr
```

### 3. 提前停止录制

```bash
# Pod 内执行
jcmd 1 JFR.stop name=mem
```

> **注意**：Pod 重启会丢失 /tmp 文件，180s 内务必及时导出。
