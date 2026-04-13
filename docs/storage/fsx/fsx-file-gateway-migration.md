# 从 Samba+LDAP 迁移到 FSx File Gateway 方案

## 1. 现状与目标

### 现有架构

```
Windows/Mac 客户端 ──SMB──▶ Samba 文件服务器 ──认证──▶ OpenLDAP (smbldap-tools)
```

### 目标架构

```
                        办公室                          │           AWS VPC
                                                       │
Windows/Mac 客户端                                     │   ┌─────────────────────────┐
       │                                               │   │ FSx for Windows         │
       ├──SMB──▶ FSx File Gateway ──VPN Tunnel─────────┼──▶│ File Server             │
       │         (本地 VM，缓存层)                      │   │ (文件存储+SMB 服务)      │
       │                                               │   └──────────┬──────────────┘
       │                                               │              │
       └──(可选：VPN 直连)──────────────────────────────┼──────────────┘
                                                       │              │
                  办公室路由器/防火墙                    │   ┌──────────▼──────────────┐
                       │                               │   │ AWS Managed             │
                       └──IPsec Tunnel─────────────────┼──▶│ Microsoft AD            │
                                                       │   │ (用户认证+权限)          │
                                                       │   └─────────────────────────┘
```

## 2. 网络方案选择（VPN）

FSx 是 VPC 内部服务，没有公网端点，**必须建立网络通道**。

| 方案 | 月费用 (估算) | 办公室要求 | 适合场景 |
|------|-------------|-----------|---------|
| **AWS Site-to-Site VPN** (推荐) | ~$36 | 支持 IPsec 的路由器/防火墙 | 全办公室共享，稳定 |
| EC2 自建 VPN (WireGuard/SoftEther) | ~$8–15 (EC2 费) | 办公室路由器或客户端软件 | 预算极低，可接受自运维 |
| AWS Client VPN | ~$160 (10 人) | 每台电脑装 VPN 客户端 | 远程办公为主 |
| AWS Direct Connect | $200+ | 运营商专线 | 大带宽需求，**不推荐** |

### 推荐：AWS Site-to-Site VPN

**费用明细：**

| 计费项 | 单价 | 月费用 |
|--------|------|--------|
| VPN 连接 (1 条) | $0.05/小时 | ~$36.5 |
| 数据传出 AWS | $0.09/GB (前 10TB) | 按量 |
| **合计（不含流量）** | | **~$37/月** |

**办公室端要求（任一即可）：**

- 企业路由器/防火墙（大多支持 IPsec：Yamaha, FortiGate, Cisco, Juniper, pfSense 等）
- Linux 服务器 + strongSwan（软件方案，零硬件成本）

## 3. 各组件成本估算

以 **Single-AZ、200GB SSD 存储、10 人团队**为例：

| 组件 | 规格 | 月费用 (东京区) |
|------|------|----------------|
| AWS Managed Microsoft AD | Standard (2 DC) | ~$175 |
| FSx for Windows File Server | Single-AZ, 200GB SSD, 32MB/s | ~$30 |
| Site-to-Site VPN | 1 条连接 | ~$37 |
| FSx File Gateway (本地) | 办公室已有服务器上的 VM | $0 (用现有硬件) |
| 数据传出 | 按实际用量 | ~$10–50 |
| **合计** | | **~$252–302/月** |

> **注意**：AD 是最大开销。如想降低成本，可用 **Simple AD**（~$73/月）替代，
> 但会失去 MFA、细粒度密码策略等功能。对于文件共享场景 Simple AD 通常够用。
>
> 使用 Simple AD 的总费用约 **~$150–200/月**。

## 4. 实施步骤

### 第一阶段：AWS 基础设施搭建

#### 4.1 创建 VPC 及子网

```bash
# 创建 VPC
aws ec2 create-vpc \
  --cidr-block 10.0.0.0/16 \
  --tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=fileserver-vpc}]'

# 创建私有子网（至少两个 AZ，AD 需要）
aws ec2 create-subnet \
  --vpc-id vpc-xxxxxxxx \
  --cidr-block 10.0.1.0/24 \
  --availability-zone ap-northeast-1a \
  --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=private-1a}]'

aws ec2 create-subnet \
  --vpc-id vpc-xxxxxxxx \
  --cidr-block 10.0.2.0/24 \
  --availability-zone ap-northeast-1c \
  --tag-specifications 'ResourceType=subnet,Tags=[{Key=Name,Value=private-1c}]'
```

#### 4.2 搭建 AWS Managed Microsoft AD

> **TODO: AD 选型待定**
> - **Managed AD Standard**（~$175/月）：功能全，支持 MFA、细粒度密码策略、AD 信任关系
> - **Simple AD**（~$73/月）：基于 Samba 4，文件共享场景通常够用，但不支持 MFA 和信任关系
> - 决定后相应调整下方命令中的 `--edition` 参数和第 3 节成本估算

**控制台方式（推荐）：**

1. 进入 **AWS Directory Service** 控制台
2. 点击 **Set up directory** → 选择 **AWS Managed Microsoft AD**
3. 选择 **Standard Edition**（Small 场景够用）
4. 设置参数：

| 参数 | 值 | 说明 |
|------|----|------|
| Directory DNS name | `corp.okcoin.internal` | 内部域名，不要用公网域名 |
| Admin password | （强密码） | AD 管理员密码，妥善保管 |
| VPC | `fileserver-vpc` | 上一步创建的 VPC |
| Subnets | `private-1a`, `private-1c` | 两个 AZ 各一个子网 |

5. 等待约 20-30 分钟，状态变为 **Active**
6. 记录输出的 **DNS 地址**（两个 IP），后续配置需要

**CLI 方式：**

```bash
aws ds create-microsoft-ad \
  --name "corp.okcoin.internal" \
  --short-name "CORP" \
  --password "YourStr0ngP@ssw0rd!" \
  --edition Standard \
  --vpc-settings "VpcId=vpc-xxxxxxxx,SubnetIds=[subnet-aaaa,subnet-bbbb]"
```

#### 4.3 创建 FSx for Windows File Server

**控制台方式（推荐）：**

1. 进入 **Amazon FSx** 控制台 → **Create file system**
2. 选择 **Amazon FSx for Windows File Server**
3. 设置参数：

| 参数 | 值 | 说明 |
|------|----|------|
| Deployment type | **Single-AZ 2** | 省成本；生产环境选 Multi-AZ |
| Storage type | SSD | 日常办公 IOPS 需求低，SSD 够用 |
| Storage capacity | 200 GB | 根据实际需求调整，最低 32 GB |
| Throughput capacity | 32 MB/s | 10 人团队足够 |
| VPC | `fileserver-vpc` | 与 AD 同一个 VPC |
| Subnet | `private-1a` | Single-AZ 选一个子网 |
| Microsoft AD directory | `corp.okcoin.internal` | 关联上一步创建的 AD |
| Security Group | 允许 SMB (445)、DNS (53) | 从办公室 CIDR 入站 |

4. 等待约 15-20 分钟，状态变为 **Available**
5. 记录 **DNS Name**（如 `amznfsxABCDEFGH.corp.okcoin.internal`）

**CLI 方式：**

```bash
aws fsx create-file-system \
  --file-system-type WINDOWS \
  --storage-capacity 200 \
  --storage-type SSD \
  --subnet-ids subnet-aaaa \
  --security-group-ids sg-xxxxxxxx \
  --windows-configuration '{
    "ActiveDirectoryId": "d-xxxxxxxxxx",
    "ThroughputCapacity": 32,
    "DeploymentType": "SINGLE_AZ_2",
    "PreferredSubnetId": "subnet-aaaa"
  }'
```

#### 4.4 配置 Site-to-Site VPN

**控制台操作步骤：**

1. **创建 Customer Gateway（办公室端）：**
   - 进入 VPC 控制台 → Customer Gateways → Create
   - BGP ASN: `65000`（或你路由器的 ASN）
   - IP Address: 办公室的**公网 IP**

2. **创建 Virtual Private Gateway（AWS 端）：**
   - VPC 控制台 → Virtual Private Gateways → Create
   - 关联到 `fileserver-vpc`

3. **创建 VPN Connection：**
   - VPC 控制台 → Site-to-Site VPN Connections → Create
   - Virtual Private Gateway: 上一步创建的
   - Customer Gateway: 第 1 步创建的
   - Routing: Static（填办公室内网 CIDR，如 `192.168.1.0/24`）

4. **下载配置文件：**
   - 选择你的路由器品牌/型号
   - 按配置文件设置办公室路由器的 IPsec 隧道

5. **更新路由表：**
   - VPC 路由表添加：`192.168.1.0/24` → Virtual Private Gateway
   - 办公室路由器添加：`10.0.0.0/16` → VPN Tunnel

6. **Security Group 规则：**

```bash
# FSx Security Group — 允许办公室访问
aws ec2 authorize-security-group-ingress \
  --group-id sg-xxxxxxxx \
  --ip-permissions '[
    {"IpProtocol":"tcp","FromPort":445,"ToPort":445,"IpRanges":[{"CidrIp":"192.168.1.0/24"}]},
    {"IpProtocol":"tcp","FromPort":53,"ToPort":53,"IpRanges":[{"CidrIp":"192.168.1.0/24"}]},
    {"IpProtocol":"udp","FromPort":53,"ToPort":53,"IpRanges":[{"CidrIp":"192.168.1.0/24"}]},
    {"IpProtocol":"tcp","FromPort":88,"ToPort":88,"IpRanges":[{"CidrIp":"192.168.1.0/24"}]},
    {"IpProtocol":"tcp","FromPort":389,"ToPort":389,"IpRanges":[{"CidrIp":"192.168.1.0/24"}]},
    {"IpProtocol":"tcp","FromPort":636,"ToPort":636,"IpRanges":[{"CidrIp":"192.168.1.0/24"}]}
  ]'
```

### 第二阶段：LDAP 用户迁移到 AD

#### 4.5 从 OpenLDAP 导出用户

```bash
# 导出所有用户（排除系统账户）
ldapsearch -x -H ldap://your-ldap-server \
  -b "ou=People,dc=okcoin,dc=internal" \
  -D "cn=admin,dc=okcoin,dc=internal" \
  -W \
  "(objectClass=posixAccount)" \
  uid cn sn givenName mail uidNumber gidNumber \
  > ldap_users.ldif
```

导出的 LDIF 大致格式：

```ldif
dn: uid=tanaka,ou=People,dc=okcoin,dc=internal
uid: tanaka
cn: Tanaka Taro
sn: Tanaka
givenName: Taro
mail: tanaka@okcoin.jp
uidNumber: 10001
gidNumber: 10001
```

#### 4.6 转换格式 — LDIF → CSV

创建转换脚本：

```bash
#!/bin/bash
# scripts/aws-cli/ldap-to-ad-csv.sh
# 用法: ./ldap-to-ad-csv.sh ldap_users.ldif > ad_users.csv
# 示例: ./ldap-to-ad-csv.sh ldap_users.ldif > ad_users.csv
set -euo pipefail

INPUT="${1:?用法: $0 <ldap_users.ldif>}"

echo "SamAccountName,GivenName,Surname,DisplayName,EmailAddress,Password"

awk '
BEGIN { OFS="," }
/^dn:/ { uid=""; cn=""; sn=""; given=""; mail="" }
/^uid:/ { gsub(/^uid: */, ""); uid=$0 }
/^cn:/ { gsub(/^cn: */, ""); cn=$0 }
/^sn:/ { gsub(/^sn: */, ""); sn=$0 }
/^givenName:/ { gsub(/^givenName: */, ""); given=$0 }
/^mail:/ { gsub(/^mail: */, ""); mail=$0 }
/^$/ {
  if (uid != "") {
    if (sn == "") sn = cn
    if (given == "") given = cn
    # 生成临时密码（用户首次登录时强制修改）
    pass = "TempP@ss" uid "2024!"
    print uid, given, sn, cn, mail, pass
  }
}
END {
  if (uid != "") {
    if (sn == "") sn = cn
    if (given == "") given = cn
    pass = "TempP@ss" uid "2024!"
    print uid, given, sn, cn, mail, pass
  }
}
' "$INPUT"
```

#### 4.7 导出 LDAP 组信息

```bash
# 导出所有组
ldapsearch -x -H ldap://your-ldap-server \
  -b "ou=Groups,dc=okcoin,dc=internal" \
  -D "cn=admin,dc=okcoin,dc=internal" \
  -W \
  "(objectClass=posixGroup)" \
  cn memberUid \
  > ldap_groups.ldif
```

#### 4.8 批量导入用户到 AWS Managed Microsoft AD

需要一台 **Windows EC2（域加入）** 作为管理跳板机来执行 PowerShell 命令。

**启动管理用 EC2：**

1. 启动 Windows Server 2022 实例（t3.small 足够）
2. 将其加入域 `corp.okcoin.internal`（通过 SSM 或手动）
3. 安装 RSAT 工具（Active Directory 管理工具）

> **TODO: PowerShell 脚本是否需要单独建文件？**
> - 目前内联在文档中，实施时可提取到 `scripts/ad/Import-ADUsers.ps1` 和 `scripts/ad/Import-ADGroups.ps1`
> - 视实施阶段决定

**PowerShell 批量创建用户脚本：**

```powershell
# Import-ADUsers.ps1
# 用法: .\Import-ADUsers.ps1 -CsvPath .\ad_users.csv
param(
    [Parameter(Mandatory=$true)]
    [string]$CsvPath
)

Import-Module ActiveDirectory

# 创建 OU（组织单位）用于存放迁移用户
$domain = "corp.okcoin.internal"
$ouPath = "OU=FileUsers,OU=corp,$((Get-ADDomain).DistinguishedName)"

try {
    Get-ADOrganizationalUnit -Identity $ouPath
    Write-Host "OU already exists: $ouPath"
} catch {
    New-ADOrganizationalUnit -Name "FileUsers" -Path "OU=corp,$((Get-ADDomain).DistinguishedName)"
    Write-Host "Created OU: $ouPath"
}

# 读取 CSV 并批量创建用户
$users = Import-Csv -Path $CsvPath
$successCount = 0
$failCount = 0

foreach ($user in $users) {
    try {
        New-ADUser `
            -SamAccountName $user.SamAccountName `
            -GivenName $user.GivenName `
            -Surname $user.Surname `
            -Name $user.DisplayName `
            -DisplayName $user.DisplayName `
            -EmailAddress $user.EmailAddress `
            -UserPrincipalName "$($user.SamAccountName)@$domain" `
            -AccountPassword (ConvertTo-SecureString $user.Password -AsPlainText -Force) `
            -ChangePasswordAtLogon $true `
            -Enabled $true `
            -Path $ouPath

        Write-Host "[OK] Created: $($user.SamAccountName)" -ForegroundColor Green
        $successCount++
    } catch {
        Write-Host "[FAIL] $($user.SamAccountName): $_" -ForegroundColor Red
        $failCount++
    }
}

Write-Host "`n=== Import Complete ==="
Write-Host "Success: $successCount / Failed: $failCount"
```

**PowerShell 批量创建组脚本：**

```powershell
# Import-ADGroups.ps1
# 用法: .\Import-ADGroups.ps1 -GroupName "engineering" -Members "tanaka,suzuki,yamada"
param(
    [Parameter(Mandatory=$true)]
    [string]$GroupName,

    [Parameter(Mandatory=$true)]
    [string]$Members  # 逗号分隔的用户名
)

Import-Module ActiveDirectory

$ouPath = "OU=FileUsers,OU=corp,$((Get-ADDomain).DistinguishedName)"

# 创建安全组
New-ADGroup -Name $GroupName `
    -GroupScope Global `
    -GroupCategory Security `
    -Path $ouPath `
    -Description "Migrated from LDAP group: $GroupName"

# 添加成员
$memberList = $Members -split ","
foreach ($member in $memberList) {
    $member = $member.Trim()
    try {
        Add-ADGroupMember -Identity $GroupName -Members $member
        Write-Host "[OK] Added $member to $GroupName" -ForegroundColor Green
    } catch {
        Write-Host "[FAIL] $member: $_" -ForegroundColor Red
    }
}
```

### 第三阶段：FSx File Gateway 部署（可选）

> **TODO: 是否部署 Gateway 待定**
> - 如果办公室网络带宽良好（50Mbps+）且文件不大，可先跳过，直接通过 VPN 访问 FSx
> - Gateway 主要价值是本地缓存加速 + VPN 断开时缓存数据仍可读
> - 建议：先不部署，上线后观察延迟和体验，按需追加

如需部署 Gateway：

#### 4.9 下载并部署 Gateway VM

1. 进入 **Storage Gateway 控制台** → Create Gateway
2. 选择 **Amazon FSx File Gateway**
3. 下载 VM 镜像（支持 VMware ESXi / Hyper-V / KVM）
4. 在办公室服务器上部署 VM：

| 资源 | 最低要求 |
|------|---------|
| vCPU | 4 核 |
| 内存 | 16 GB |
| 系统盘 | 80 GB |
| 缓存盘 | 150 GB+（SSD 推荐，缓存热数据） |
| 网络 | 能通过 VPN 访问 AWS VPC |

5. 激活 Gateway：在 AWS 控制台输入 Gateway VM 的 IP
6. 配置缓存盘
7. 将 FSx 文件系统关联到 Gateway

### 第四阶段：配置文件共享与权限

#### 4.10 在 FSx 上创建共享文件夹

通过管理跳板机（Windows EC2）操作：

```powershell
# 映射 FSx 管理共享
net use Z: \\amznfsxABCDEFGH.corp.okcoin.internal\share

# 创建部门文件夹
New-Item -ItemType Directory -Path "Z:\Engineering"
New-Item -ItemType Directory -Path "Z:\Finance"
New-Item -ItemType Directory -Path "Z:\Common"

# 设置 NTFS 权限（示例：Engineering 文件夹只允许 engineering 组访问）
$acl = Get-Acl "Z:\Engineering"
$rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
    "CORP\engineering", "FullControl", "ContainerInherit,ObjectInherit", "None", "Allow"
)
$acl.AddAccessRule($rule)
Set-Acl "Z:\Engineering" $acl

# Common 文件夹：所有域用户可读写
$acl = Get-Acl "Z:\Common"
$rule = New-Object System.Security.AccessControl.FileSystemAccessRule(
    "CORP\Domain Users", "Modify", "ContainerInherit,ObjectInherit", "None", "Allow"
)
$acl.AddAccessRule($rule)
Set-Acl "Z:\Common" $acl
```

### 第五阶段：客户端配置

#### 4.11 Windows 客户端

1. **配置 DNS**：将客户端 DNS 指向 AD 的 DNS IP（或在办公室 DNS 添加条件转发）

```
# 办公室 DNS 服务器添加条件转发
# corp.okcoin.internal → AD DNS IP (10.0.1.x, 10.0.2.x)
```

2. **映射网络驱动器**：

```cmd
:: 方式一：直接连接 FSx（无 Gateway 场景）
net use S: \\amznfsxABCDEFGH.corp.okcoin.internal\share /user:CORP\tanaka

:: 方式二：通过 FSx File Gateway（有 Gateway 场景）
net use S: \\gateway-ip\share /user:CORP\tanaka
```

3. 或者通过文件资源管理器 → 「此电脑」→ 「映射网络驱动器」→ 输入 UNC 路径

#### 4.12 Mac 客户端

1. **Finder** → 菜单栏 **前往** → **连接服务器**（⌘K）
2. 输入地址：

```
smb://amznfsxABCDEFGH.corp.okcoin.internal/share
```

3. 输入凭据：
   - 用户名：`CORP\tanaka`（或 `tanaka@corp.okcoin.internal`）
   - 密码：AD 密码

## 5. 数据迁移（旧文件搬到 FSx）

```bash
# 方案一：使用 robocopy（Windows，推荐）
# 在能同时访问旧 Samba 和新 FSx 的机器上执行
robocopy \\old-samba-server\share \\amznfsxABCDEFGH.corp.okcoin.internal\share /E /COPYALL /R:3 /W:5 /LOG:migration.log

# 方案二：使用 rsync（Linux/Mac）
rsync -avz --progress /mnt/old-samba/ /mnt/new-fsx/
```

| 注意事项 | 说明 |
|---------|------|
| 权限迁移 | POSIX 权限无法直接映射到 NTFS ACL，需迁移后重新设置 |
| 中文文件名 | 确保编码一致（UTF-8），通常无问题 |
| 大文件 | 网络带宽是瓶颈，建议非工作时间执行 |
| 验证 | 迁移后对比文件数和大小：`robocopy /E /L`（仅列出不复制） |

## 6. 整体时间线

| 阶段 | 任务 | 预计时间 |
|------|------|---------|
| 1 | VPC + AD + FSx + VPN 搭建 | 1-2 天 |
| 2 | LDAP 导出 + AD 导入 + 测试 | 1 天 |
| 3 | FSx File Gateway 部署（如需） | 半天 |
| 4 | 文件共享权限配置 | 半天 |
| 5 | 数据迁移 | 视数据量（几小时到几天） |
| 6 | 客户端配置 + 测试 | 1 天 |
| **合计** | | **约 4-5 个工作日** |

## 7. 注意事项

| 项目 | 说明 |
|------|------|
| AD 管理员密码 | 妥善保管，不写入任何项目文件 |
| 备份策略 | FSx 支持自动每日备份，建议开启 |
| 并行运行期 | 建议新旧系统并行运行 1-2 周，确认无问题后再关闭旧服务器 |
| Simple AD 替代 | 如预算有限，可用 Simple AD（$73/月）替代 Managed AD（$175/月），但不支持 MFA 和信任关系 |
| 网络中断 | VPN 断开时无法访问 FSx；如部署了 Gateway 则缓存数据仍可读 |
