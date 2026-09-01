# Aurora PostgreSQL 蓝绿部署大版本升级手册

> 场景：Aurora PostgreSQL 集群所在版本即将（或已经）结束标准支持，需要在**尽量短的写中断**内完成大版本升级。
> 通用流程见 §1~§11（以 `okj-sygna` 12.14 → 16.11 的实施复盘为底稿）。
> **下一次升级（16.11 → 17.7 LTS）的决策与待办清单见 [§12](#12-下一次升级计划1611--177lts-待办)。**
> 历史实施记录见文末「实施记录」。

---

## 0. 目标与验收标准

| 项 | 内容 |
|----|------|
| 目标 | 完成大版本升级，**写中断控制在 60 秒内**（Switchover 期间蓝绿双方都禁写） |
| 非目标 | 实例规格调整、Schema 变更、跨区迁移 |
| 验收 | 版本号正确 + 关键表行数一致 + 序列值不回退 + 扩展版本已更新 + `ANALYZE` 完成 + 慢查询无明显退化 |

> ⚠️ **不要把目标写成「不影响业务」**。蓝绿的 Switchover 会：停止双方写入 → 断开所有连接 → 等复制追平 → 改名切换。
> 应用会收到 `AdminShutdown: terminating connection due to administrator command`，必须确认连接池能自动重连、且写失败有重试。
> 正确说法是「写中断 < 60s，且不需要修改应用连接串」。

---

## 1. 版本选择与支持期（先算清楚再动手）

### 1.1 当前版本处境（截至 2026-09）

`okj-sygna` 当前为 **Aurora PostgreSQL 16.11**（2025-12 已完成 12.14 → 16.x 升级），集群 `RDS Extended Support: Enabled`。

**要分清两个不同的到期日**，别混着看：

| 维度 | 值 | 到期后会发生什么 |
|------|-----|----------------|
| **小版本** 16.11 标准支持 | 2027-05-31 | AWS 在维护窗口**自动小升级**到受支持的 16.x（不收费、不停服） |
| **大版本** PG 16 标准支持 | 2029-02-28 | 之后才转入 **付费** Extended Support |

→ 所以 `Extended Support: Enabled` 目前**不产生任何费用**，它只是"到期后自动转入付费延长支持而非被停服"的开关。当前**不紧急**。

> 历史对比：上次升级时的 PG 12 标准支持已于 2025-02-28 结束、Extended Support 到 2028-02-29，那时是**真在按小时多付费**，性质完全不同。

### 1.2 蓝绿部署支持的最低版本

| 大版本 | 支持蓝绿的最低版本 |
|--------|------------------|
| Aurora PG 11 | 11.21+ |
| Aurora PG 12 | **12.16+** |
| Aurora PG 13 | 13.12+ |
| Aurora PG 14 | 14.9+ |
| Aurora PG 15 | 15.4+ |
| Aurora PG 16 | 16.1+ |
| Aurora PG 17 | 17.4+ |

→ 12.14 **不满足**，必须先做小版本升级。上次选了 12.17，可行；但建议选 **12.22**（12 系列最后一个小版本，且是 Extended Support 合格小版本），避免中途被 RDS 强制小升级打断。

### 1.3 目标版本怎么选：看「一次动作买多久」

不要按「升到最新版本」来决策，要按**一次升级动作能买到多长的支持期**来决策。

大版本标准支持截止：

| 大版本 | 标准支持截止 |
|--------|------------|
| PG 16 | 2029-02-28 |
| PG 17 | 2030-02-28 |
| PG 18 | 2031-02-28 |

但**小版本各有自己的支持期**，这才是真正决定你多久要再动一次手的东西：

| 小版本 | Aurora 发布 | 小版本标准支持截止 |
|--------|-----------|------------------|
| 16.9 | 2025-06-30 | 2026-12-31 |
| 16.10 | 2025-11-25 | 2027-04-30 |
| 16.11 | 2025-12-18 | 2027-05-31 |
| 16.13 | 2026-04-06 | 2027-09-30 |
| 16.14 | 2026-08-21 | 2027-12-31 |
| **16.8（LTS）** | 2025-04-08 | **2029-02-28** |
| 17.9 | 2026-04-06 | 2027-09-30 |
| 17.10 | 2026-08-21 | 2027-12-31 |
| **17.7（LTS）** | 2025-12-18 | **2030-02-28** |
| 18.3 | 2026-06-11 | 2027-11-30 |
| 18.4 | 2026-08-21 | 2027-12-31 |

**结论**：普通小版本只买 ~1.3 年，**LTS 版本能买 3~4 年**。除非有特定补丁需求，否则应优先选 LTS。
注意 LTS 是**不可回退**的——一旦小版本超过了本系列的 LTS（如 16.11 > 16.8），该系列的 LTS 就再也用不上了，只能一直追小版本或跳大版本。

---

## 2. 前置检查（三类，缺一类就会中途翻车）

### 2.A 蓝绿部署前置（AWS 硬性要求）

1. **集群必须挂自定义 DB 集群参数组**，且 `rds.logical_replication = 1`
2. 参数改完**必须重启 writer 实例**——蓝绿创建时会校验 writer 与参数组是否 in-sync，不一致直接创建失败
3. **所有表必须有主键**（或 `REPLICA IDENTITY FULL`）
4. 蓝库**不能**是自建逻辑复制的 publisher / subscriber
5. 若用 **RDS Proxy**：必须**先**把蓝库注册进 Proxy，再创建蓝绿；蓝绿建好后无法再注册
6. 蓝绿**不支持**：跨区只读副本、Aurora Serverless v1、CloudFormation 管理
7. 若有 Redshift zero-ETL 集成：Switchover 前必须删除，切换后重建

#### 参数清单（注意静态/动态之分）

| 参数 | 建议值 | 类型 | 说明 |
|------|--------|------|------|
| `rds.logical_replication` | `1` | **静态·需重启** | 不开无法创建蓝绿；连带把 `wal_level` 变为 `logical` |
| `synchronous_commit` | `on` | 动态 | AWS 明确要求为 `on`（多数情况本来就是，属确认项非变更项） |
| `max_replication_slots` | ≥ 数据库数量 + 余量（如 20） | **静态·需重启** | 蓝绿里**每个 database 占一个槽** |
| `max_wal_senders` | ≥ 槽数 + 管理会话余量（如 20） | **静态·需重启** | 上次保持 10 未改，数据库数量多时会不够 |
| `max_logical_replication_workers` | ≥ 槽数（如 10） | **静态·需重启** | 从 `max_worker_processes` 池中取 |
| `autovacuum_max_workers` | 保持默认公式 | **静态·需重启** | 上次未实际变更，可从变更表移除 |
| `max_parallel_workers` | 8 | 动态 | |
| `max_worker_processes` | ≥ 上述三者之和（如 32） | **静态·需重启** | 写死 32 的风险：将来换实例规格后不再随内存自适应，需在变更记录里注明 |
| `rds.logically_replicate_unlogged_tables` | 有 unlogged 表才设 `1` | 静态 | 蓝绿**创建后禁止再改**，否则复制报错 |

> 上次文档第 10 条「MySQL 的参数组要修改 performance_schema 为 0」属于 MySQL 蓝绿的内容，**与本文档无关，应删除或拆到 Aurora MySQL 手册**。混在 PostgreSQL 手册里容易误操作。

参数生效确认（连 writer）：

```bash
psql -h <cluster-writer-endpoint> -U <user> -p 5432 -d <db>
```

```sql
SHOW rds.logical_replication;   -- 期望 on
SHOW wal_level;                 -- 期望 logical
SELECT name, setting, pending_restart
FROM pg_settings
WHERE name IN ('rds.logical_replication','synchronous_commit','max_replication_slots',
               'max_wal_senders','max_logical_replication_workers',
               'max_worker_processes','max_parallel_workers','autovacuum_max_workers');
```

**`pending_restart` 必须全为 `f`**，否则说明还没重启，蓝绿创建会失败。上次文档只 `SHOW` 了两个参数，没有校验 `pending_restart`，这是最容易漏的一步。

### 2.B 逻辑复制语义盲区（数据不一致的主要来源）

蓝绿用的是 PostgreSQL 原生逻辑复制，以下内容**不会**同步到绿环境。这一节在上次文档中完全缺失，是最大的风险敞口。

| 项目 | 行为 | 应对 |
|------|------|------|
| **DDL**（CREATE/ALTER/DROP TABLE、加分区） | 不复制；一旦在蓝库执行，绿库进入 **Replication degraded**，只能删除蓝绿重建 | **变更冻结**：蓝绿期间禁止任何 DDL、禁止 Flyway/Liquibase 迁移、禁止新建分区 |
| **DCL**（GRANT/REVOKE） | 不复制，仅告警 | 切换后手工重放权限变更 |
| **序列 NEXTVAL** | 期间不同步；**Switchover 时 AWS 会把绿库序列值对齐蓝库**。序列数量极大（数十万）可能导致切换超时 | 提高 `--switchover-timeout`；切换后核对 `pg_sequences` |
| **大对象 `pg_largeobject`** | 完全不复制；期间新增/修改大对象 → 绿库 degraded，必须删除重建 | 先查 `pg_largeobject_metadata` 计数；若在用，蓝绿方案不适用，改走 DMS |
| **物化视图 REFRESH** | 在蓝库 REFRESH 会**直接打断复制** | 期间停掉所有 MV 刷新任务（含 cron）；切换后手工 `REFRESH MATERIALIZED VIEW` |
| **无主键表的 UPDATE/DELETE** | 直接报错 | 补主键，或 `REPLICA IDENTITY FULL`（仅在无任何唯一键时用，会拖慢复制） |
| **unlogged 表** | 默认不复制 | 设 `rds.logically_replicate_unlogged_tables=1`，且创建蓝绿后不得再改 |
| **视图 / 外部表** | 不复制数据（结构随克隆带过去） | 切换后确认 FDW 连接串用的是**集群 endpoint 名而非 IP**，否则切换后失效 |
| **apply 进程单线程** | 绿库回放是单线程，蓝库高写入时会持续 lag 甚至失败 | 提前压测；写入量极大时改用 DMS 或自建逻辑复制 |

扩展相关的硬性约束：

| 扩展 | 要求 |
|------|------|
| `pg_partman` | 创建蓝绿时**必须禁用**（它会执行 DDL） |
| `pg_cron` | 绿库上**必须保持禁用**（后台 worker 以 superuser 绕过只读，造成复制冲突） |
| `pglogical` / `pgactive` | 创建蓝绿时必须禁用，切换后可重新启用 |
| `pgaudit` | 蓝、绿**两侧的 DB 参数组**都必须保留在 `shared_preload_libraries` 里 |
| `apg_plan_mgmt` | 绿库 `apg_plan_mgmt.capture_plan_baselines = off` |

> **对应检查**：`SELECT * FROM pg_extension;`。上次文档完全没有检查扩展，如果集群里装了 `pg_cron` 或 `pg_partman`，蓝绿会在中途 degraded。

### 2.C pg_upgrade 前置阻断项

绿环境的大版本升级底层仍是 `pg_upgrade`，以下任一项存在就会 precheck 失败：

```sql
-- 1) 未结束的 prepared transaction
SELECT count(*) FROM pg_catalog.pg_prepared_xacts;   -- 必须为 0

-- 2) 不支持的 reg* 类型（regtype/regclass 除外）
SELECT count(*) FROM pg_catalog.pg_class c, pg_catalog.pg_namespace n, pg_catalog.pg_attribute a
 WHERE c.oid = a.attrelid AND NOT a.attisdropped
   AND a.atttypid IN ('pg_catalog.regproc'::regtype,'pg_catalog.regprocedure'::regtype,
                      'pg_catalog.regoper'::regtype,'pg_catalog.regoperator'::regtype,
                      'pg_catalog.regconfig'::regtype,'pg_catalog.regdictionary'::regtype)
   AND c.relnamespace = n.oid
   AND n.nspname NOT IN ('pg_catalog','information_schema');   -- 必须为 0

-- 3) invalid database（DROP DATABASE 被中断留下的）
SELECT datname FROM pg_database WHERE datconnlimit = -2;       -- 必须为空

-- 4) 遗留逻辑复制槽（DMS / CDC / Debezium 用的）
SELECT slot_name, plugin, active FROM pg_replication_slots WHERE slot_type <> 'physical';

-- 5) 大对象数量（>2500 万时需要 ≥32GB 内存实例，否则升级 OOM）
SELECT count(*) FROM pg_largeobject_metadata;

-- 6) unknown 类型
SELECT DISTINCT data_type FROM information_schema.columns WHERE data_type ILIKE 'unknown';

-- 7) 依赖 pg_stat_activity 的视图 / 物化视图（precheck 会报错要求 DROP）
SELECT n.nspname, c.relname FROM pg_class c
  JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE c.relkind IN ('v','m')
   AND EXISTS (SELECT 1 FROM pg_depend d JOIN pg_rewrite r ON r.oid = d.objid
                WHERE r.ev_class = c.oid AND d.refobjid = 'pg_catalog.pg_stat_activity'::regclass);
```

**主键检查（改进版）**——上次文档的 SQL 只查了 `public` schema 和 `relkind='r'`，会漏掉：其他 schema、分区表（`relkind='p'`）、以及已设 `REPLICA IDENTITY FULL` 的表被误报：

```sql
SELECT n.nspname AS schema,
       c.relname AS table_name,
       c.relkind,
       CASE c.relreplident WHEN 'd' THEN 'default' WHEN 'n' THEN 'nothing'
                           WHEN 'f' THEN 'full'    WHEN 'i' THEN 'index' END AS replica_identity
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r','p')
  AND n.nspname NOT IN ('pg_catalog','information_schema')
  AND n.nspname NOT LIKE 'pg_%'
  AND NOT c.relispartition
  AND NOT EXISTS (SELECT 1 FROM pg_index i WHERE i.indrelid = c.oid AND i.indisprimary)
  AND c.relreplident <> 'f'
ORDER BY 1, 2;
```

同时记录基线，供切换后比对：

```sql
-- 序列基线
SELECT schemaname, sequencename, last_value FROM pg_sequences ORDER BY 1,2;
-- 关键表行数基线
SELECT relname, n_live_tup FROM pg_stat_user_tables ORDER BY n_live_tup DESC LIMIT 30;
-- 物化视图清单（切换后要手工 REFRESH）
SELECT schemaname, matviewname FROM pg_matviews;
-- 扩展清单与版本
SELECT extname, extversion FROM pg_extension ORDER BY 1;
```

### 2.D 应用侧不兼容（12 → 16 跨 4 个大版本，这一节上次是空的）

| 变更点 | 引入版本 | 影响 | 应对 |
|--------|---------|------|------|
| **`public` schema 默认不再允许非 owner 建对象**（撤销 PUBLIC 的 CREATE 权限） | **PG 15** | 应用用非 owner 账号 `CREATE TABLE`/临时表落在 public 会失败；Flyway/ORM 自动建表会挂 | 升级后按需 `GRANT CREATE ON SCHEMA public TO <role>;` |
| `password_encryption` 默认 `scram-sha-256` | PG 14 | 老驱动（libpq <10、PgJDBC 很旧版本）无法认证 | 确认 JDBC / psycopg / Go pgx 版本 |
| `wal_keep_segments` → `wal_keep_size` | PG 13 | 参数组里若显式设过会失效 | 检查自定义参数组里所有显式设值的参数在 16 里是否仍存在 |
| `force_parallel_query` → `debug_parallel_query`、`promote_trigger_file` / `vacuum_defer_cleanup_age` 移除 | PG 16 | 同上 | 同上 |
| `stats_temp_directory` 移除（统计走共享内存） | PG 15 | 参数组残留项 | 同上 |
| **优化器统计信息不随大版本升级迁移** | — | 切换后可能出现严重慢查询 | Switchover **前**在绿库 `ANALYZE`，切换**后**再全库 `ANALYZE VERBOSE` |
| **glibc / collation 版本变化** | OS 升级 | 文本列索引排序规则可能变化，产生无效索引项 | Aurora 12.13+ / 14.6+ 起内置了独立 collation 库以稳定排序；仍建议切换后对关键文本索引跑 `amcheck`，异常则 `REINDEX INDEX CONCURRENTLY` |
| 执行计划回归 | — | 4 个大版本的 planner 差异 | 在绿库用生产查询集做对比（`pg_stat_statements` top N 逐条 `EXPLAIN`） |

**参数组残留项自查**：

```sql
SELECT name, setting, source FROM pg_settings
WHERE source NOT IN ('default','override','client') ORDER BY name;
```
把结果与 16 系列参数组做差集，凡是 16 里不存在的参数在新参数组里必须去掉。

---

## 3. 参数组规划（上次文档这里有硬错误）

上次文档的表格把**目标参数组的模板也写成了 12 系列**：

| | 上次文档 | 应为 |
|---|---------|------|
| 目标集群参数组模板 | ~~`prod-okj-postgresql12-cluster-default`~~ | `default.aurora-postgresql16`（**family 必须是 aurora-postgresql16**） |
| 目标 DB 参数组模板 | ~~`default.aurora-postgresql12`~~ | `default.aurora-postgresql16` |

Aurora 的参数组绑定引擎 family，**12 family 的参数组无法挂到 16 集群上**，创建蓝绿时会直接报错。正确做法：

| 用途 | 名称 | family |
|------|------|--------|
| 源集群参数组 | `prod-okj-postgresql-cluster-sygna12` | aurora-postgresql12 |
| 源 DB 参数组 | `prod-okj-postgresql-db-sygna12` | aurora-postgresql12 |
| 目标集群参数组 | `prod-okj-postgresql-cluster-sygna16` | **aurora-postgresql16** |
| 目标 DB 参数组 | `prod-okj-postgresql-db-sygna16` | **aurora-postgresql16** |

目标参数组里要做的事：
1. 把源参数组里**所有自定义值**逐项搬过来（16 里已移除/改名的除外）
2. 保留 `shared_preload_libraries` 中的 `pgaudit`、`pg_stat_statements` 等
3. `apg_plan_mgmt.capture_plan_baselines = off`（若用该扩展）
4. 目标侧**不需要**开 `rds.logical_replication`（只有源侧需要）

---

## 4. 变更冻结与依赖方通知（上次文档缺失）

蓝绿创建后到 Switchover 完成之间，必须冻结：

- [ ] 所有 DDL / Schema 迁移（Flyway、Liquibase、手工 ALTER）
- [ ] 新建分区的定时任务（`pg_partman` 或自研 cron）
- [ ] 物化视图 REFRESH 任务
- [ ] 大对象写入
- [ ] `GRANT`/`REVOKE` 变更
- [ ] 涉及该库的业务发布（避免同时排查两个变更）

依赖方盘点（切换后需要动手的）：

| 依赖 | 切换后影响 | 处理 |
|------|-----------|------|
| DMS 任务 | checkpoint 在绿库无效，**任务无法续跑** | 切换后重建 DMS 任务 |
| Debezium / 自建 CDC | 槽在 `-old1` 上，不会跟过来 | 切换前记录位点，切换后重建订阅 |
| CloudWatch 告警 / Grafana | 按 DBInstanceIdentifier 或 resource ID 引用的会指错对象 | 切换后重新绑定；Performance Insights / CloudTrail 的 resource ID 也变了 |
| **Aurora Auto Scaling 策略** | **不会**复制到绿环境 | 切换后重新配置 |
| **关联的 IAM Role**（S3 导入导出等） | **不会**自动复制到绿环境 | 切换后重新关联 |
| IAM 数据库认证 | 策略 Resource 需同时包含蓝、绿两库 | 切换前先加 |
| Tag | 创建时从蓝复制一次；之后不同步；Switchover 时**蓝的 tag 覆盖绿的全部 tag** | 建蓝绿**之前**把 tag 打全 |
| Secrets Manager 托管主密码 | **蓝绿不支持** | 需先关闭托管 |
| 只读副本 / 自建 Reader 连接 | endpoint 名会跟过来，但连接会断 | 确认连接池重连 |
| Nacos 中的数据源配置 | endpoint 名不变则无需改 | 仍需确认 [Nacos](https://nacos-admin.okcoin.tokyo/) 里没有写死实例名 |
| DNS TTL | 客户端 DNS 缓存 > 5s 会继续写到蓝库 | 确认 JVM `networkaddress.cache.ttl` 等不超过 5s |

---

## 5. 实施步骤

| # | 步骤 | 预估耗时 | 校验点 |
|---|------|---------|--------|
| 1 | 打全 tag；关闭 Secrets Manager 托管主密码（如有）；RDS Proxy 先注册蓝库 | 10 min | — |
| 2 | 创建 4 个参数组（源 12 family × 2、目标 16 family × 2） | 10 min | family 正确 |
| 3 | 源集群挂上自定义参数组，按 §2.A 表格改参数 | 10 min | — |
| 4 | **重启 writer 实例** | 2~5 min | `pg_settings.pending_restart` 全为 `f` |
| 5 | 跑 §2.B / §2.C / §2.D 全部检查，记录基线 | 30 min | 无阻断项；基线已存档 |
| 6 | 小版本升级 12.14 → 12.22（顺带满足蓝绿最低版本） | 15~30 min | 版本号；应用无异常 |
| 7 | 观察 1 个业务日（可选但建议） | 1 天 | 无回归 |
| 8 | 创建蓝绿部署：目标版本 16.x + 目标集群参数组 | 30~90 min（视数据量） | 状态 `Available`；复制状态 healthy |
| 9 | 绿库验证：版本、扩展、行数、`EXPLAIN` 对比、`ANALYZE` | 2~4 h | 见 §6 |
| 10 | Go/No-Go 检查 | 10 min | 见 §6 |
| 11 | **Switchover**（`--switchover-timeout 600`） | 30 s ~ 数分钟 | 见 §7 |
| 12 | 切换后任务 | 1~2 h | 见 §8 |
| 13 | 观察 1~7 天后删除蓝绿部署资源 + 老集群（**先手工快照**） | — | 见 §8.4 |

> **上次文档缺少 dry run**。AWS 明确建议先用快照/PITR 恢复一套演练环境跑一遍大版本升级，验证 precheck 与执行计划。stage 环境先走一遍是成本最低的保险。

创建蓝绿（CLI）：

```bash
aws rds create-blue-green-deployment \
  --blue-green-deployment-name okj-sygna-bg-16 \
  --source arn:aws:rds:ap-northeast-1:<account-id>:cluster:okj-sygna \
  --target-engine-version 16.8 \
  --target-db-cluster-parameter-group-name prod-okj-postgresql-cluster-sygna16
```

---

## 6. Switchover 前 Go/No-Go 检查

AWS 自身会跑 switchover guardrails（绿库复制健康、复制延迟、绿库无活跃写；蓝库无外部复制、无长事务、无长 DDL、无不支持的 PostgreSQL 变更）。但**别只依赖 guardrail**，人工确认：

- [ ] 蓝、绿集群及全部实例状态均为 `Available`
- [ ] 复制延迟 ≈ 0：CloudWatch `OldestReplicationSlotLag`（蓝库侧），或
  ```sql
  SELECT slot_name, confirmed_flush_lsn AS flushed, pg_current_wal_lsn(),
         (pg_current_wal_lsn() - confirmed_flush_lsn) AS lsn_distance
  FROM pg_catalog.pg_replication_slots WHERE slot_type = 'logical';
  ```
  `lsn_distance = 0` 表示已追平
- [ ] 复制状态**不是** `Replication degraded`（若是，说明期间发生了 DDL / 大对象变更 → 只能「Delete with green databases」后重建）
- [ ] 蓝库无长事务 / 长 DDL：`SELECT pid, now()-xact_start AS dur, query FROM pg_stat_activity WHERE state<>'idle' ORDER BY 2 DESC NULLS LAST LIMIT 10;`
- [ ] `DatabaseConnections` / `DBLoad` 处于低位；必要时提前缩减连接数
- [ ] 已确认应用**没有**在会话级把 `default_transaction_read_only` 改成 `off`（否则切换期间可能写进绿库，一旦切换回滚就产生数据不一致，需手工修复）
- [ ] 绿库已跑过 `ANALYZE`
- [ ] 客户端 DNS 缓存 TTL ≤ 5s
- [ ] 已选好低峰时间窗；值班人员到位；回滚决策人明确
- [ ] **不要点 `Promote`**——那会打断复制并让蓝绿进入 `Invalid configuration`，只能点 `Switch over`

---

## 7. Switchover

```bash
aws rds switchover-blue-green-deployment \
  --blue-green-deployment-identifier bgd-xxxxxxxxxxxx \
  --switchover-timeout 600
```

- 超时范围 30~3600 秒，默认 300。**序列数量多、事务偏长的库建议设 600**，超时会整体回滚、两边都不变更
- 期间行为：停写 → 断连 → 等复制追平 → 改名（绿接过蓝的名字和 endpoint；蓝改名为 `-old1`）→ 放开连接 → 绿库放开写
- 切换后蓝库**强制只读**，直到删除蓝绿部署资源为止
- 可用 EventBridge 监听 `SWITCHOVER_IN_PROGRESS` 等事件做自动化（例如自动清理连接）

---

## 8. 切换后任务

### 8.1 立刻（写中断窗口内/刚结束）

- [ ] 版本确认：`SELECT version();`
- [ ] 应用写入恢复正常（[Grafana](https://grafana.okcoin.tokyo/) 错误率、[Log View](https://log-view.okcoin.tokyo/query) 报错）
- [ ] 必要时重启业务服务（连接池未自动重连的情况）

### 8.2 30 分钟内

- [ ] **全库 `ANALYZE VERBOSE;`**（每个 database 都要跑；统计信息不随大版本迁移，不跑必然出慢查询）
- [ ] 扩展升级：逐个 `ALTER EXTENSION <name> UPDATE;`，再核对 `SELECT extname, extversion FROM pg_extension;`
- [ ] 序列核对：与 §2.C 基线比对 `pg_sequences.last_value`，确认无回退
- [ ] 关键表行数与基线比对
- [ ] 物化视图 `REFRESH MATERIALIZED VIEW`
- [ ] 若用 `pg_cron` / `pg_partman` / `pglogical`：重新启用
- [ ] `public` schema 权限按 §2.D 补 `GRANT`（PG15 行为变化）
- [ ] 检查 `pg_upgrade_internal.log` / `pg_upgrade_server.log`（RDS 日志页）

### 8.3 当天

- [ ] 索引有效性：`SELECT indexrelid::regclass FROM pg_index WHERE NOT indisvalid;`
- [ ] 文本索引排序校验（collation 变化）：`amcheck` 的 `bt_index_check()`，异常则 `REINDEX INDEX CONCURRENTLY`
- [ ] 重建 DMS 任务 / CDC 订阅
- [ ] 重新配置 Auto Scaling 策略、重新关联 IAM Role
- [ ] 重新绑定 CloudWatch 告警 / Performance Insights / Grafana 数据源（resource ID 已变）
- [ ] 确认 tag 完整（Switchover 时蓝的 tag 覆盖了绿的）
- [ ] 确认 deletion protection、备份保留期、维护窗口在新集群上正确
- [ ] 清理残留逻辑复制槽：`SELECT * FROM pg_replication_slots;`，非活跃的 `pg_drop_replication_slot()`（**不清理会持续堆积 WAL**）
- [ ] 视需要把 `rds.logical_replication` 关回 `0`（静态参数，需安排一次重启；不关会持续产生额外 WAL 与 IO）
- [ ] 慢查询对比：`pg_stat_statements` top N 与升级前基线对比

### 8.4 删除旧集群（不要着急）

⚠️ **PITR 会重置**：新生产集群的 earliest restorable time 从绿环境创建时刻开始，**无法恢复到切换前任何时间点**。切换前的时间点只能从 `-old1` 蓝集群（用它的 `DbClusterResourceId`，不是名字）恢复。

所以：

1. 蓝集群至少保留到覆盖你的恢复窗口需求（如 7 天），期间照常计费
2. 删除前**先手工快照**并设保留期（`aws rds create-db-cluster-snapshot`）
3. 删除蓝绿部署资源（分离新旧集群，`-old{n}` / `-new{n}` 都会保留）
4. 再删除 `-old1` 集群
5. 注意：删掉蓝集群后，绿集群底层的 Aurora 克隆卷会**膨胀到完整大小**（存储费用上升）

---

## 9. 回滚方案（上次文档完全缺失，这是最关键的补充）

必须区分两个阶段：

| 阶段 | 能否回滚 | 做法 |
|------|---------|------|
| 蓝绿已创建，**未** Switchover | ✅ 完全可回滚 | 直接删除蓝绿部署（选「Delete with green databases」），生产库全程未受影响 |
| Switchover **进行中**失败/超时 | ✅ 自动回滚 | AWS 自动回滚，两边都不变更；排查后重试 |
| Switchover **已完成** | ❌ **不能点一下切回去** | 见下 |

Switchover 完成后的应急路径（都有代价，需事先定好选哪条）：

1. **就地修复**（首选）：绝大多数问题是慢查询/权限/统计信息 → `ANALYZE`、补 `GRANT`、`REINDEX`、加索引
2. **切回蓝库（有数据丢失）**：蓝集群被强制只读，需先删除蓝绿部署资源解除只读，再把应用指回 `-old1` endpoint。**切换后写入绿库的数据全部丢失**——只有在切换后极短时间内、且业务能接受这段数据丢失时才可行
3. **从蓝库快照/PITR 重建**：RTO 数小时，RPO 到切换时刻

**因此必须事先明确**：
- 回滚决策窗口（例：切换后 15 分钟内决定是否走路径 2，超过则只能走路径 1）
- 决策人是谁
- 判定标准（例：错误率 > X%、P99 > Y ms、出现数据不一致）

---

## 10. 风险与参数还原记录表

实施时填写实际值，便于还原：

| 参数 | 初始值 | 修改后 | 是否需重启 | 是否需还原 |
|------|--------|--------|-----------|-----------|
| `rds.logical_replication` | 0 | 1 | ✅ | 建议升级完成后还原为 0 |
| `synchronous_commit` | on | on | ❌ | 无变更 |
| `max_replication_slots` | 10 | 20 | ✅ | 可保留 |
| `max_wal_senders` | 10 | 20 | ✅ | 可保留 |
| `max_logical_replication_workers` | (默认) | 10 | ✅ | 可保留 |
| `autovacuum_max_workers` | 默认公式 | 默认公式 | ✅ | 无变更 |
| `max_parallel_workers` | (默认) | 8 | ❌ | 可保留 |
| `max_worker_processes` | `LEAST({DBInstanceClassMemory/2132104534},64)` | 32 | ✅ | ⚠️ 写死后不再随规格自适应，换规格时需复核 |

其他风险：

| 风险 | 影响 | 缓解 |
|------|------|------|
| 绿环境与生产同时运行 | 成本翻倍（实例 + 存储增量），可能触碰集群数量/子网 IP 配额 | 提前算成本、查配额；尽量缩短绿环境存续时间 |
| 单线程 apply 追不上 | 复制持续 lag，无法切换 | 提前压测；必要时改 DMS |
| 期间误执行 DDL | 绿库 degraded，前面工作全废 | 变更冻结（§4），并把冻结通知发到研发群 |
| 切换后执行计划回归 | 业务变慢 | 切换前后都 `ANALYZE`；保留 `pg_stat_statements` 基线 |

---

## 11. 相对上次文档的改进清单

| # | 上次的问题 | 本文档的处理 |
|---|-----------|-------------|
| 1 | 目标写成「不影响业务」 | 改为「写中断 < 60s」，并说明切换必然断连 |
| 2 | 目标参数组模板写成 12 family | 修正为 aurora-postgresql16 family（硬错误） |
| 3 | 没有回滚方案 | 新增 §9，区分三个阶段与三条应急路径 |
| 4 | 没有逻辑复制盲区清单 | 新增 §2.B（大对象/物化视图/序列/DDL/DCL/unlogged/扩展） |
| 5 | 没有 pg_upgrade 前置阻断项 | 新增 §2.C（prepared xacts / reg* / invalid db / 遗留槽 / 大对象 / unknown） |
| 6 | 没有 12→16 应用不兼容分析 | 新增 §2.D（PG15 public schema、scram、参数改名、统计信息、collation） |
| 7 | 没有 `ANALYZE` | 列为切换前后必做项 |
| 8 | 没有 dry run | §5 步骤 5 之前建议先在 stage 演练 |
| 9 | 参数未区分静态/动态、未校验 `pending_restart` | §2.A 表格补充；校验 SQL 补充 |
| 10 | 主键检查 SQL 只覆盖 public schema / 普通表 | §2.C 改进版 SQL |
| 11 | 「通知研发确认」过于笼统 | §8 拆成 立刻 / 30 分钟 / 当天 三档具体清单 |
| 12 | 直接「删除 old 集群」 | §8.4 说明 PITR 重置、先手工快照、保留恢复窗口 |
| 13 | 没有依赖方盘点 | §4 表格（DMS/CDC/告警/Auto Scaling/IAM Role/Tag/DNS TTL） |
| 14 | 混入 MySQL `performance_schema` 参数 | 标注应删除或拆分 |
| 15 | 缺少变更冻结说明 | §4 冻结清单 |
| 16 | 版本选择理由不足 | §1 支持期表格 + 16/17 与 LTS 对比 |
| 17 | 「替代解决方案」章节空白 | 见下 |

### 替代方案对比（补齐上次的空白章节）

| 方案 | 停机时间 | 复杂度 | 适用 |
|------|---------|--------|------|
| **蓝绿部署**（本次采用） | 写中断 30s~数分钟 | 中 | 大多数场景；AWS 托管，endpoint 不变 |
| 原地大版本升级 | 10~30+ 分钟不可用 | 低 | 能接受停机窗口的小库 |
| 自建逻辑复制 + Aurora 快速克隆 | 与蓝绿相近 | 高（全手工） | 写入量极大、需要精细控制切换时机 |
| DMS 迁移 | 秒级切换 | 高 | 有大对象、高写入量、或需要 Schema 变更 |
| `pg_dump`/`pg_restore` | 数小时 | 低 | 小库、或需要彻底重整（顺带解决 collation/膨胀） |

选择蓝绿的理由：endpoint 与连接串不变、AWS 托管复制与 guardrails、切换前可在绿环境完整验证、未切换前可零成本放弃。

---

## 12. 下一次升级计划：16.11 → 17.7（LTS）— 待办

> 状态：**已决策，待排期**｜决策日期：2026-09-01｜建议完成时间：**2027-Q1 前**（避开 16.11 小版本 2027-05-31 到期）｜不紧急

### 12.1 版本决策依据

| 目标 | 类型 | 支持到 | 一次动作买到多久 |
|------|------|--------|----------------|
| **17.7（LTS）** | 大版本 | **2030-02-28** | ⭐ **约 3.5 年** |
| 16.14（16 系列最新） | 小版本 | 2027-12-31 | 约 1.3 年 |
| 17.10（17 系列最新） | 大版本 | 2027-12-31 | 约 1.3 年 |
| 18.4 | 大版本 | 2027-12-31 | 约 1.3 年，且 18 太新（社区 2026-02 发布） |

**选 17.7 的理由**：

1. 唯一能一次买 ~3.5 年的选项。留在 16 里追小版本，2027-05-31、2027-12-31 要连做两次升级，明年一样躲不开动手
2. **16 系列的 LTS（16.8）已经错过**（当前 16.11 > 16.8），留在 16 就只能一直追小版本
3. 17.7 于 2025-12-18 发布，已运行约 9 个月，且为 LTS，成熟度足够
4. 16 → 17 的不兼容面比 12 → 16 小一个数量级，没有 PG 15 那种 `public` schema 权限翻转级别的破坏性变更
5. PG 17 起**逻辑复制槽可跨大版本升级保留**，利于后续再升级
6. 升 17 的 `rdkit` 前置约束要求源 ≥ 16.5，当前 16.11 已满足

### 12.2 待办清单

**阶段一：确认与评估（可随时做，无风险）**

- [ ] 确认 Aurora 允许 16.11 直接升到 17.7（不需要中间垫脚小版本）：
  ```bash
  aws rds describe-db-engine-versions \
    --engine aurora-postgresql --engine-version 16.11 \
    --query 'DBEngineVersions[0].ValidUpgradeTarget[?IsMajorVersionUpgrade==`true`].EngineVersion' \
    --output text
  ```
- [ ] 跑一遍前置检查脚本，确认无阻断项：`scripts/aws-cli/rds-pg-bluegreen-precheck.sh`
- [ ] 排查 §12.3 的 16 → 17 专项差异（6 项）
- [ ] 确认目标小版本仍为当时的 17 系列 LTS（LTS 可能已更新，届时复查发布日历）
- [ ] 评估绿环境并行运行的成本，确认集群数量 / 子网 IP 配额

**阶段二：演练（stage 环境）**

- [ ] 用生产快照恢复一套 stage 集群，完整走一遍蓝绿 + 17.7 升级
- [ ] 对比升级前后执行计划（`pg_stat_statements` top N 逐条 `EXPLAIN`）
- [ ] 记录各阶段实际耗时，用于确定生产变更窗口

**阶段三：生产实施**

- [ ] 按 §4 发出变更冻结通知（DDL / 分区任务 / MV 刷新 / 大对象写入）
- [ ] 按 §3 创建 `aurora-postgresql17` family 的目标参数组
- [ ] 按 §5 执行；按 §6 做 Go/No-Go；按 §7 Switchover
- [ ] 按 §8 完成切换后任务（重点：全库 `ANALYZE VERBOSE`、扩展 `ALTER EXTENSION UPDATE`、序列核对、清理残留复制槽）
- [ ] 更新 Grafana / 监控面板中受 §12.3 列名变更影响的查询
- [ ] 观察 7 天后手工快照 → 删除蓝绿部署资源 → 删除 `-old1` 集群
- [ ] 回填本文档「实施记录」表

### 12.3 16 → 17 专项差异（在 §2.D 的通用检查之外额外要查的）

按重要性排序：

| # | 变更 | 影响 | 检查方式 |
|---|------|------|---------|
| 1 | **维护操作使用安全 `search_path`** | 表达式索引 / 物化视图依赖的函数若引用非默认 schema 的对象、且建函数时未固定 `search_path`，则 `ANALYZE` / `CLUSTER` / `CREATE INDEX` / `CREATE MATERIALIZED VIEW` / `REFRESH MATERIALIZED VIEW` / `REINDEX` / `VACUUM` 会失败 | 见下方 SQL |
| 2 | **`pg_stat_statements` 列改名** | `blk_read_time` → `shared_blk_read_time`、`blk_write_time` → `shared_blk_write_time`。[Grafana](https://grafana.okcoin.tokyo/) 里查这两列的面板会直接坏 | 全局搜索面板 JSON / 监控 SQL |
| 3 | **`pg_stat_bgwriter` 删列** | 移除 `buffers_backend`、`buffers_backend_fsync`（并入 `pg_stat_io`），同样影响监控面板 | 同上 |
| 4 | **参数被移除** | `old_snapshot_threshold`、`trace_recovery_messages`、`db_user_namespace` 在 17 中已移除，自定义参数组显式设过就必须删掉 | `SELECT name FROM pg_settings WHERE name IN ('old_snapshot_threshold','trace_recovery_messages','db_user_namespace');` |
| 5 | **`adminpack` 扩展被移除** | 装了则升级失败，需先 `DROP EXTENSION adminpack;` | `SELECT * FROM pg_extension WHERE extname='adminpack';` |
| 6 | `interval` 中的 `ago` 只允许出现在末尾；空的 interval 单位不允许重复出现 | 仅影响手写 interval 字面量的 SQL | 代码搜索 `ago` |

其他改名（影响自研监控脚本，不影响业务 SQL）：`pg_stat_progress_vacuum` 的 `max_dead_tuples` → `max_dead_tuple_bytes`、`num_dead_tuples` → `num_dead_item_ids`；`pg_stat_slru` 列名；`pg_collation.colliculocale` → `colllocale`；`pg_database.daticulocale` → `datlocale`；`pg_attribute.attstattarget` 默认值改用 `NULL` 表示。

第 1 项的排查 SQL——列出所有未固定 `search_path` 的自定义函数：

```sql
SELECT n.nspname AS schema, p.proname AS function_name, p.proconfig
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname NOT IN ('pg_catalog','information_schema')
  AND (p.proconfig IS NULL
       OR NOT EXISTS (SELECT 1 FROM unnest(p.proconfig) c WHERE c LIKE 'search\_path=%'))
ORDER BY 1,2;
```

其中被表达式索引或物化视图引用的函数**必须**补上 `ALTER FUNCTION ... SET search_path = ...`。

---

## 参考

- [创建蓝绿部署（含 PostgreSQL 前置准备）](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/blue-green-deployments-creating.html)
- [蓝绿部署的限制与注意事项](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/blue-green-deployments-considerations.html)
- [Switchover（guardrails / 超时 / 最佳实践 / 切换后）](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/blue-green-deployments-switching.html)
- [蓝绿支持的 Aurora PostgreSQL 版本](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Concepts.Aurora_Fea_Regions_DB-eng.Feature.BlueGreenDeployments.html)
- [Aurora PostgreSQL 版本发布日历与支持期](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraPostgreSQLReleaseNotes/aurorapostgresql-release-calendar.html)
- [PostgreSQL 大版本升级前置检查清单（RDS）](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_UpgradeDBInstance.PostgreSQL.MajorVersion.Process.html)
- [Aurora PostgreSQL collation 与 glibc](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/PostgreSQL-Collations.html)

配套脚本：[`scripts/aws-cli/rds-pg-bluegreen-precheck.sh`](../../../scripts/aws-cli/rds-pg-bluegreen-precheck.sh)

---

## 实施记录

| 日期 | 集群 | 版本变化 | 实施人 | 备注 |
|------|------|---------|--------|------|
| 2025-12（待补准确日期） | okj-sygna / sygna_hub_db | 12.14 → 12.17 → 16.11 | xiubao.li | 首次蓝绿升级；本文档据此复盘整理 |
| 待排期（2027-Q1 前） | okj-sygna / sygna_hub_db | 16.11 → 17.7（LTS） | — | 计划见 §12 |
