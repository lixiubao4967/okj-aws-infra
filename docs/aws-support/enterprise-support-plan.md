# AWS Enterprise Support 推荐服务方案（OKJ · 2026 H2）

> 来源：AWS TAM（Stanley Pan）面向 OKJ 的主动服务方案分享会（2026）。整理自幻灯片。

基于 OKJ 当前架构和运维现状，AWS 精选 **5 项高价值服务**，由 **TAM 全程协调**。

| 服务 | 计费 | 备注 |
|------|------|------|
| SHIP 安全评估 | 🆓 免费（ES 权益） | |
| Well-Architected Review | 🆓 免费（ES 权益） | |
| Cost Optimization Workshop | 🆓 免费（ES 权益） | |
| Database Operational Review | 🆓 免费（ES 权益） | |
| AWS Security Agent | 💳 按需付费 | 独立付费服务，新客户 2 个月免费试用 |

---

## 1. SHIP 安全评估

Security Hub Improvement Program（安全改进计划），交付极轻量。

**评估流程**（从启动到交付仅需 2–3 周，客户投入约 2 小时）：

| 步骤 | 阶段 | 说明 |
|------|------|------|
| 01 | Discovery | 45 分钟 · 了解安全优先级 |
| 02 | Assessment | 自动化 · 无需客户操作 |
| 03 | Delivery | 60 分钟 · 交付定制报告 |
| 04 | Follow-up | 持续 · TAM 跟踪进度 |

**特点**：✅ 无需安装工具 ｜ ✅ 无需开放额外权限 ｜ ✅ 不影响生产环境 ｜ ✅ 支持多账号

---

## 2. AWS Security Agent（按需付费）

AI 驱动的持续应用安全验证。**2026 GA**。关键词：自主渗透测试、上下文感知、业务逻辑漏洞。

### 核心能力

| 能力 | 内容 |
|------|------|
| 静态分析 | 源代码扫描、SQL 注入 / XSS / SSRF、硬编码凭据检测、配置安全审计 |
| 动态测试 | 运行时漏洞发现、API 端点模糊测试、认证绕过检测、权限提升尝试 |
| 依赖审计 | 第三方库 CVE 扫描、供应链风险评估、License 合规检查、过时版本告警 |
| 自主渗透 | AI 构造攻击链、业务逻辑漏洞、多步骤组合攻击、上下文感知利用 |

### OKJ 适用场景

- 交易所核心系统（撮合引擎 / 资金划转）的**业务逻辑安全验证**
- EKS 微服务间 API 调用的认证与授权边界测试
- 替代或补充年度外部渗透测试，降低安全合规成本
- CI/CD 集成，每次代码提交自动安全验证

### 定价说明

按需付费，无预付费、无最低消费。

| 项 | 值 |
|----|----|
| 费率 | **$50 / task-hour** |
| 计费精度 | 按秒计费 |
| Task-hour 定义 | Agent 并行任务的累计时间（> 实际等待时间） |
| 预付 / 长期合约 | 无 |

**免费试用**：新客户 2 个月免费 ｜ 每月 400 task-hours 免费额度 ｜ 含完整报告 + 可操作修复代码 ｜ 试用期后自动转为按需付费。

**典型场景费用参考**：

| 场景 | 实际耗时 | 计费 Task-hours | 费用 |
|------|---------|----------------|------|
| 简单 API 渗透测试 | ~1 小时 | 3.5h | $173 |
| 月度电商应用渗透 | ~4 小时 | 24h | $1,200 |
| 复杂企业 SaaS 全面渗透 | ~9.5 小时 | 31.3h | $1,563 |

> 对比传统年度渗透测试（$30K–$100K/次），Security Agent 持续运行且按用付费，整体 TCO 通常更低。

---

## 3. Well-Architected Review

6 支柱最佳实践评审，**2–4h / workload**。

| 支柱 | 关注点 |
|------|--------|
| 安全性 | 身份管理、检测控制、基础设施保护、数据保护、事件响应 |
| 可靠性 | 故障恢复、高可用设计、变更管理、基础架构弹性、灾难恢复 |
| 性能效率 | 计算资源选型、存储优化、网络配置、数据库选择、性能监控 |
| 成本优化 | 支出意识、高性价比资源、供需匹配、持续优化、计费模型 |
| 卓越运营 | 组织准备、运行准备、日常运营、事件管理、持续改进 |
| 可持续性 | 区域选择、用户行为模式、软件与架构模式、数据模式、硬件与服务 |

---

## 4. Cost Optimization Workshop

以数据为驱动的成本优化研讨会。TAM 基于 **Cost Optimization Hub (COH)** 的真实数据，与客户技术/财务团队共同分析当前支出，制定可执行的优化行动计划。

| 阶段 | 内容 |
|------|------|
| 数据准备 | TAM 提前拉取 COH 建议、分类整理优化机会 |
| 研讨会 | 2–4 小时深入讨论、逐项确认可行性与优先级 |
| 行动计划 | 明确责任人与时间表、TAM 月度跟踪进度 |

**覆盖领域**：Savings Plans / Reserved Instances / Graviton 迁移 / EBS 优化 / Idle Resources / Right-sizing / MAP Credits 最大化。

### OKJ 成本优化数据（COH 实测）

| 指标 | 值 |
|------|----|
| 月度技术优化空间 | **$14,749** |
| 叠加 MAP 后月度节省 | **$33–45K** |
| EBS 孤立卷（097 账号） | 324 个 |

**COH 建议明细（双账号合计，738 = prod / 097 = stage）**：

| 优化类别 | 账号 | 月度节省 | 说明 |
|---------|------|---------|------|
| EC2 Graviton 迁移 | 738 | $3,528 | m5 系列 → m8g / c7g |
| Compute SP 增购 | 738 | $3,270 | 覆盖 On-Demand 暴露 |
| RDS RI 购买 | 738+097 | $3,002 | 16x db.r8g.large 等 |
| EBS 孤立卷清理 | 738+097 | $2,445 | 324 个未挂载卷（097） |
| Idle Stage DB 关停 | 097 | $205 | 3 个 Stage 数据库 |
| Compute SP 增购 | 097 | $925 | SP 覆盖率仅 55% |
| 其他（ElastiCache / Redshift RI 等） | 097 | $1,374 | 多项小额优化 |

> **MAP Credits 叠加效应**：OKJ 拥有 Migration Acceleration Program credits，优化后的 On-Demand 支出可进一步用 MAP 覆盖，实现月省 $33–45K 的组合效果。

---

## 5. Database Operational Review（数据库运维评估）

**GSE Senior Engineer** 专家交付，深度评估数据库运营就绪程度。约 **2 周**（GSE 交付）。

- **覆盖引擎**：Aurora MySQL ✓ / RDS MySQL ✓ / Aurora PostgreSQL ✓ / RDS PostgreSQL ✓
- **5 维度评估**：高可用与灾备（HA/DR）、安全配置、引擎参数配置、监控与可观测性、资源使用趋势（2 周）
- **前提条件**：建议启用 Performance Insights + Enhanced Monitoring

**对 OKJ 的价值**：

- 738 prod — 22 个 Aurora 集群（含核心交易集群）
- 097 stage — 49 个集群（含 **2 个 EXPIRED 3.08.2 需升级**）
- 可选择核心 prod 集群做首轮全面健康评估

**流程概要**：TAM 提交 Intake → SE Manager 审批 → GSE 工程师分配 → 内部对齐范围 → 审查实例 → 1h 客户交付通话 → 跟进会议（可选）

> PostgreSQL 还可使用自动化工具：18 维度评估 / 3–4 分钟完成 / HTML 报告。

---

## 附：Unified Operations Team（按需专家）

参考：https://aws.amazon.com/premiumsupport/unified-operations-team/

Enterprise Support 下可按需调用的专家团队：

| 角色 | 职责 |
|------|------|
| Migrations and Events Engineer | 迁移、产品发布等关键节点的按需工程师支持：Launch & Migration Planning、Event Support & Real-time Monitoring、Post-event Analysis & Optimization、Readiness Reviews |
| Security Incident Response Engineers | 按需安全专家：安全发现的自动化监控与分诊、分钟级专家引导响应 |
| Cost Optimization Management | 指定财务顾问，协助优化云支出、Financial Business Reviews |

---

## 附：AWS Case 案例级别（严重程度 Severity）

提交 Support Case 时，请根据问题的严重性与紧急程度选择对应级别：

| 严重级别 | 响应时间 | Case 参考 |
|---------|---------|-----------|
| 🔴 严重 Business-critical system down | 15 min | 严重损坏——无法控制整个企业或客户的基础环境，最关键业务受到严重影响，或有严重的业务损失 |
| 🟠 紧急 Production system down | 1 hr | 没有替代方案且业务受到严重影响；重要业务功能、生产系统无法使用 |
| 🟠 高优先 Production system impaired | 4 hrs | 没有替代方案且业务受到严重影响；重要业务功能、生产系统受损或降级 |
| 🟢 普通 System impaired | 12 hrs | 有替代方案；非重要业务功能无法正常工作，或有紧急的开发需求 |
| 🔵 一般指南 General guidance | 24 hrs | 一般的问题咨询、开发需求或新功能需求 |

> 响应时间为 Enterprise Support 的 SLA。选择级别在提交 Case 时通过 **Severity** 下拉框设定。

---

## 与 OKJ 现状的关联

- **成本优化**：COH 明细里的 EBS 孤立卷清理，正对应 stage 部署时踩过的「空孤儿资源」坑（见 git 历史 `坑4`）；324 个未挂载卷值得纳入清理。
- **数据库升级**：097 stage 的 2 个 EXPIRED 3.08.2 集群需升级——可结合 DB Ops Review 一并处理。
- **Graviton 迁移**：m5 → m8g/c7g 月省 $3,528，是单项最大优化空间。

## 待跟进

- [ ] 确认 Security Agent 2 个月免费试用的启用时间与范围（哪些应用先接入）。
- [ ] 选定 DB Ops Review 首轮评估的核心 prod 集群。
- [ ] 与 TAM 约定 Cost Optimization Workshop 时间，落实 Graviton / SP 增购行动计划。
