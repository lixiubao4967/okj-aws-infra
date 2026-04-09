# 基于现有 EC2 快速创建同配置实例

> 场景：需要快速复制一台正在运行的 EC2 实例（相同系统、软件环境、配置），用于扩容或搭建同步运行环境。

## 核心思路

```
现有 EC2 → 创建 AMI（镜像）→ 基于 AMI 启动新实例
```

AMI 会完整捕获：根卷快照、操作系统、已安装软件、系统配置。  
**不会**同步：实例的弹性 IP、安全组绑定（需手动指定）、EBS 数据卷（额外挂载的数据盘需单独处理）。

---

## 方式一：控制台操作（推荐）

### 第一步：为现有实例创建 AMI

1. 进入 **EC2 控制台** → **实例**，选中目标实例
2. 右键 → **镜像和模板** → **创建镜像**
3. 填写：
   - **镜像名称**：建议格式 `<实例名>-YYYYMMDD`，如 `app-server-20260409`
   - **描述**：简要说明用途
   - **无重启**：若实例不能停机，勾选「无需重启」（注意：可能导致文件系统不一致）
4. 点击 **创建镜像**，等待状态变为 `available`（通常 5~15 分钟）

> AMI 创建期间实例正常运行不受影响（除非未勾选「无需重启」时会短暂重启）。

### 第二步：用「Launch more like this」启动新实例

1. 回到 **实例列表**，选中源实例
2. 右键 → **镜像和模板** → **Launch more like this**
   > 此操作会将源实例的实例类型、VPC、安全组、密钥对、存储配置全部预填入启动向导
3. 在启动向导中只需修改以下几项：
   - **实例名称**：按可用区命名规范修改，如源实例名为 `app-c01`，新实例应改为 `app-a01`
   - **AMI**：替换为上一步刚创建的镜像（默认仍是原实例的 AMI）
   - **子网**：按需切换到目标可用区的子网
   - **安全组**：选择「选择现有安全组」，确认已勾选正确的安全组
4. 点击 **启动实例**

### 第三步：验证新实例

SSH 登录新实例后，依次确认以下服务状态：

```bash
# 1. 确认监控采集服务
systemctl status node-exporter.service
systemctl status process-exporter.service

# 2. 确认日志采集服务
systemctl status fluent-bit.service
```

所有服务应为 `active (running)` 状态。

**如果 fluent-bit 服务异常或日志未上报：**
1. 前往日志监控系统 [ClickHouse Log View](https://log-view.okcoin.tokyo/query)，确认是否有新机器的日志出现
2. 若没有，重启 fluent-bit：
   ```bash
   sudo systemctl restart fluent-bit.service
   systemctl status fluent-bit.service
   ```

**确认服务注册：**

前往 [Nacos 控制台](https://nacos-admin.okcoin.tokyo/)，在服务列表中确认新实例已完成注册。

**确认监控指标：**

前往 [Grafana](https://grafana.okcoin.tokyo/)，确认新实例的监控指标（CPU、内存、进程等）已正常上报。

---

## 方式二：AWS CLI 操作

### 第一步：创建 AMI

```bash
# 查找源实例 ID
aws ec2 describe-instances \
  --filters "Name=tag:Name,Values=<实例名称>" \
  --query "Reservations[].Instances[].InstanceId" \
  --output text

# 创建 AMI
aws ec2 create-image \
  --instance-id <源实例ID> \
  --name "app-server-$(date +%Y%m%d)" \
  --description "Clone of <源实例名> on $(date +%Y-%m-%d)" \
  --no-reboot

# 示例输出：{ "ImageId": "ami-0xxxxxxxxxxxxxxxx" }
```

### 第二步：等待 AMI 可用

```bash
# 轮询 AMI 状态，直到 available
aws ec2 wait image-available --image-ids <AMI_ID>

# 或手动查询状态
aws ec2 describe-images \
  --image-ids <AMI_ID> \
  --query "Images[].State" \
  --output text
```

### 第三步：查询原实例配置（复用参数）

```bash
# 获取原实例的子网、安全组、实例类型
aws ec2 describe-instances \
  --instance-ids <源实例ID> \
  --query "Reservations[].Instances[].[InstanceType,SubnetId,SecurityGroups[].GroupId]" \
  --output json
```

### 第四步：启动新实例

```bash
aws ec2 run-instances \
  --image-id <AMI_ID> \
  --instance-type <实例类型，如 t3.medium> \
  --subnet-id <子网ID> \
  --security-group-ids <安全组ID> \
  --key-name <密钥对名称> \
  --count 1 \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=Name,Value=app-server-clone}]'
```

---

## 注意事项

| 事项 | 说明 |
|------|------|
| 数据卷 | 额外挂载的 EBS 数据盘不会自动复制，需单独创建快照并挂载 |
| 弹性 IP | 新实例没有 EIP，需单独分配并绑定 |
| 内网 IP | 新实例会分配新的私有 IP，配置文件中如有写死 IP 需更新 |
| AMI 费用 | AMI 本身免费，但底层 EBS 快照按容量收费（约 $0.05/GB/月）|
| 清理 | 用完后记得注销 AMI 并删除对应快照，避免产生不必要费用 |

---

## 快速脚本

见 [`scripts/aws-cli/clone-ec2.sh`](../../../scripts/aws-cli/clone-ec2.sh)
