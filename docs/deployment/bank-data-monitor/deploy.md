# BankInfoUpdate（bank-data-monitor）脚本机部署

日本银行信息变更监控与通知工具的脚本机部署手册。

- **部署目录**：`/data/okcoin/bank-data-monitor`
- **运行方式**：venv 隔离环境 + cron 每周一 09:00 执行 `run.sh`
- **不污染系统 Python**：所有第三方依赖只装在项目内 venv，不执行任何系统级 `pip3 install`

## 前置条件

| 项目 | 要求 |
|------|------|
| Python | 3.9 及以上（`python3 --version` 确认）。Amazon Linux 2 自带 3.7、官方源最高 3.8，需用 uv 装托管 Python（见第 2 步） |
| 网络 | 脚本机可访问 FSA / MUFG 公网页面、Google Chat Webhook |
| 文件 | `bankInfo.csv`（系统内银行快照，放项目根目录） |
| 配置 | `config.py` 已填好 Webhook URL 等配置项 |

## 部署步骤

### 1. 创建项目目录并放置代码

```bash
sudo mkdir -p /data/okcoin
sudo chown "$(whoami)" /data/okcoin

# 将 BankInfoUpdate 代码放到目标目录（git clone 或 scp 上传）
# 目录名固定为 bank-data-monitor
cd /data/okcoin/bank-data-monitor
```

### 2. 创建 venv 并安装依赖

venv 建在项目目录内，随项目一起管理。

**系统 Python >= 3.9 时**：

```bash
cd /data/okcoin/bank-data-monitor
python3 -m venv venv
./venv/bin/pip install --upgrade pip
./venv/bin/pip install requests beautifulsoup4
```

**系统 Python < 3.9 时（如 Amazon Linux 2，实际部署即此路径）**：venv 不能凭空提供更高版本解释器（它只是链接创建它的那个 python），先用 uv 装托管 Python：

```bash
# 装 uv（到 ~/.local/bin，无需 sudo；老 glibc 会自动落到 musl 静态版）
curl -LsSf https://astral.sh/uv/install.sh | sh
source ~/.local/bin/env

# 下载预编译 Python 3.11（到 ~/.local/share/uv/python/）
uv python install 3.11

cd /data/okcoin/bank-data-monitor
# --seed 必须加：往 venv 塞 pip，run.sh 的 pip3 自动装依赖逻辑才能用
uv venv --python 3.11 --seed venv
./venv/bin/pip install requests beautifulsoup4
```

建议在项目根目录固化一份 `requirements.txt`，锁定版本便于将来迁移/重建：

```bash
./venv/bin/pip freeze > requirements.txt
# 之后重建环境只需：./venv/bin/pip install -r requirements.txt
```

### 3. 修改 run.sh 接入 venv

在 `run.sh` **切换到项目目录之后、依赖检查之前**，加入 venv 激活逻辑：

```bash
# ===== venv =====
VENV_DIR="${PROJECT_DIR:-$(cd "$(dirname "$0")" && pwd)}/venv"
if [ ! -x "${VENV_DIR}/bin/python3" ]; then
    echo "ERROR: venv 不存在，请先执行: python3 -m venv ${VENV_DIR} && ${VENV_DIR}/bin/pip install -r requirements.txt" >&2
    exit 1
fi
# shellcheck disable=SC1091
source "${VENV_DIR}/bin/activate"
```

激活后 `run.sh` 内原有的 `python3` / `pip3` 命令都会解析到 venv 内的版本：

- 原「缺失依赖自动 `pip3 install`」逻辑无需改动，装的是 venv 而非系统
- `python3 main.py` 跑的也是 venv 解释器

> venv 不存在时选择**报错退出**而非自动创建：cron 静默环境下自动建环境容易掩盖磁盘/权限问题，环境初始化应是一次性的人工部署动作。

### 4. 放置数据与配置

```bash
cd /data/okcoin/bank-data-monitor
ls bankInfo.csv          # 银行快照，必须存在
vi config.py             # 确认 Webhook URL、NOTIFY_INTERVAL、MIN_EVENT_YEAR/MONTH 等
```

### 5. 手动验证

先把 `config.py` 中 `GOOGLE_CHAT_WEBHOOK_URL` 留空跑一轮**开发模式**（只打日志不真发）：

```bash
cd /data/okcoin/bank-data-monitor
cp config.py config.py.bak
sed -i 's#^GOOGLE_CHAT_WEBHOOK_URL = .*#GOOGLE_CHAT_WEBHOOK_URL = ""#' config.py
./run.sh
cat "$(ls -t logs/run_*.log | head -1)"   # 人工审通知内容（四行日文格式）
```

> ⚠️ **开发模式也会写 `sent_events.json`**（notifier 打印日志后返回成功）。演习完必须删状态再恢复配置，否则正式跑会把所有事件当成「已通知」而静默跳过：

```bash
rm -f sent_events.json      # 重置演习产生的状态
mv config.py.bak config.py  # 恢复真实 Webhook
```

确认日志链路正常（抓取 → 过滤 → 命中 → 去重 → 通知条数），再填回真实 Webhook URL 跑一次正式验证。正常日志形如：

```
INFO __main__: 共抓取解析 22 条原始事件
INFO matcher: Matcher: 年月过滤 10/22 条（>= 2026 年 5 月）
INFO matcher: Matcher: 命中系统内银行 9/10 条
INFO dedupe: Dedupe: 跨源合并 9 → 7 条
INFO dedupe: Dedupe: 来源集合去重 7 → 2 条（剔除 5）
INFO __main__: 本轮新通知 2 条
```

### 6. 配置 crontab

以运行用户（与手动验证同一用户）配置，每周一上午 9 点执行：

```bash
crontab -e
```

```cron
0 9 * * 1 /data/okcoin/bank-data-monitor/run.sh >> /data/okcoin/bank-data-monitor/logs/cron.log 2>&1
```

venv 已在 `run.sh` 内部激活，crontab 里**不需要**额外 source activate 或指定 PATH。`>> cron.log` 是兜底：run.sh 若在创建日志文件之前失败（如 venv 被误删），错误会落在 cron.log 而不是丢失。

## 注意事项

| 事项 | 说明 |
|------|------|
| 敏感信息 | Webhook URL 硬编码在 `config.py`，该目录不要提交到公共 git 仓库；如需入库，先剥离真实 URL |
| 用户一致性 | 手动验证与 cron 必须用同一用户，否则 `sent_events.json`、`logs/` 会出现权限冲突 |
| 状态文件 | `sent_events.json` 是去重依据，**删除即重置通知历史**，下轮会全量重发；迁移机器时记得带走 |
| 通知失败重试 | 发送失败（网络错误、429）的事件不写状态，下次运行自动重试，无需人工补发 |
| Python 升级 | 系统 Python 大版本升级后 venv 会失效，需删除 `venv/` 重建并 `pip install -r requirements.txt` |
| uv 托管 Python | venv 内的解释器软链到 `~okj-admin/.local/share/uv/python/`，清理该用户 home 或 uv 缓存会弄坏 venv |
| 日志 | `logs/run_YYYYmmdd_HHMMSS.log` 保留 30 天，由 `run.sh` 自动清理 |
| 首轮通知量 | 首次运行没有历史状态，符合条件的事件会全部通知一遍，属预期行为 |

## 目录布局（部署完成后）

```
/data/okcoin/bank-data-monitor/
├── venv/                  # 项目独立 venv（不进 git）
├── requirements.txt       # 锁定的依赖版本
├── bankInfo.csv           # 系统内银行快照
├── config.py              # 集中配置（含敏感 Webhook URL）
├── sent_events.json       # 已通知状态（首次运行自动生成）
├── logs/                  # 运行日志（保留 30 天）
├── main.py / run.sh / ...
├── fetchers/  parsers/
└── 技术方案.md
```
