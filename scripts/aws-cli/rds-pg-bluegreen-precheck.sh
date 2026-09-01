#!/usr/bin/env bash
#
# Aurora PostgreSQL 蓝绿部署 / 大版本升级前置检查
#
# 用法:
#   ./rds-pg-bluegreen-precheck.sh <host> <port> <user> <database> [cluster-id]
#
# 示例:
#   ./rds-pg-bluegreen-precheck.sh \
#     okj-sygna.cluster-xxxxxxxx.ap-northeast-1.rds.amazonaws.com 5432 okj_db_user sygna_hub_db okj-sygna
#
# 说明:
#   - 密码通过环境变量 PGPASSWORD 或 ~/.pgpass 提供，不在脚本中硬编码
#   - 必须连接【writer 实例 / 集群写入端点】
#   - 只做只读检查，不修改任何数据；输出「阻断项」需全部清零才能创建蓝绿
#   - 传入 cluster-id 时会额外用 aws cli 检查引擎版本与参数组
#
# 对应文档: docs/database/aurora-postgresql/blue-green-major-upgrade.md

set -euo pipefail

if [[ $# -lt 4 ]]; then
  echo "用法: $0 <host> <port> <user> <database> [cluster-id]" >&2
  exit 1
fi

HOST="$1"
PORT="$2"
USER="$3"
DB="$4"
CLUSTER_ID="${5:-}"

PSQL=(psql -h "$HOST" -p "$PORT" -U "$USER" -d "$DB" -X -q -v ON_ERROR_STOP=1)

section() {
  echo
  echo "=============================================================="
  echo "  $1"
  echo "=============================================================="
}

# ---------- 1. 参数状态（重点看 pending_restart 是否全为 f） ----------
section "1. 逻辑复制相关参数（pending_restart 必须全为 f）"
"${PSQL[@]}" -c "
SELECT name, setting, boot_val, pending_restart
FROM pg_settings
WHERE name IN ('rds.logical_replication','wal_level','synchronous_commit',
               'max_replication_slots','max_wal_senders','max_logical_replication_workers',
               'max_worker_processes','max_parallel_workers','autovacuum_max_workers')
ORDER BY name;"

# ---------- 2. 阻断项：无主键且非 REPLICA IDENTITY FULL 的表 ----------
section "2. 阻断项 — 无主键的表（含所有 schema 与分区表父表）"
"${PSQL[@]}" -c "
SELECT n.nspname AS schema, c.relname AS table_name, c.relkind,
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
ORDER BY 1,2;"

# ---------- 3. 阻断项：pg_upgrade precheck 会失败的项 ----------
section "3. 阻断项 — pg_upgrade precheck（下列计数应全为 0 / 结果应为空）"
"${PSQL[@]}" -c "
SELECT 'prepared_xacts' AS item, count(*)::text AS value FROM pg_catalog.pg_prepared_xacts
UNION ALL
SELECT 'unsupported_reg_types', count(*)::text
  FROM pg_catalog.pg_class c, pg_catalog.pg_namespace n, pg_catalog.pg_attribute a
 WHERE c.oid = a.attrelid AND NOT a.attisdropped
   AND a.atttypid IN ('pg_catalog.regproc'::regtype,'pg_catalog.regprocedure'::regtype,
                      'pg_catalog.regoper'::regtype,'pg_catalog.regoperator'::regtype,
                      'pg_catalog.regconfig'::regtype,'pg_catalog.regdictionary'::regtype)
   AND c.relnamespace = n.oid
   AND n.nspname NOT IN ('pg_catalog','information_schema')
UNION ALL
SELECT 'invalid_databases', count(*)::text FROM pg_database WHERE datconnlimit = -2
UNION ALL
SELECT 'unknown_type_columns', count(*)::text
  FROM information_schema.columns WHERE data_type ILIKE 'unknown'
UNION ALL
SELECT 'large_objects', count(*)::text FROM pg_largeobject_metadata
UNION ALL
SELECT 'unlogged_tables', count(*)::text FROM pg_class WHERE relpersistence = 'u'
UNION ALL
SELECT 'logical_replication_slots', count(*)::text
  FROM pg_replication_slots WHERE slot_type <> 'physical';"

# ---------- 4. 警告项：逻辑复制不同步的对象 ----------
section "4. 警告项 — 逻辑复制不会同步的对象（需在切换后手工处理）"
"${PSQL[@]}" -c "SELECT schemaname, matviewname AS materialized_view FROM pg_matviews ORDER BY 1,2;"
"${PSQL[@]}" -c "SELECT extname, extversion FROM pg_extension ORDER BY 1;"
echo "→ 注意：pg_partman / pglogical / pgactive 需在创建蓝绿前禁用；pg_cron 在绿库须保持禁用；pgaudit 须保留在两侧 shared_preload_libraries"

# ---------- 5. 基线快照（切换后用于比对） ----------
section "5. 基线 — 序列值（切换后需比对，确认未回退）"
"${PSQL[@]}" -c "SELECT schemaname, sequencename, last_value FROM pg_sequences ORDER BY 1,2;"

section "5b. 基线 — 行数 Top 30"
"${PSQL[@]}" -c "
SELECT schemaname, relname, n_live_tup
FROM pg_stat_user_tables ORDER BY n_live_tup DESC LIMIT 30;"

section "5c. 基线 — 非默认参数（用于比对 16 系列参数组是否有已移除/改名参数）"
"${PSQL[@]}" -c "
SELECT name, setting, source FROM pg_settings
WHERE source NOT IN ('default','override','client') ORDER BY name;"

# ---------- 6. 集群侧信息 ----------
if [[ -n "$CLUSTER_ID" ]]; then
  section "6. 集群侧 — 引擎版本与参数组"
  aws rds describe-db-clusters \
    --db-cluster-identifier "$CLUSTER_ID" \
    --query 'DBClusters[0].{Engine:Engine,Version:EngineVersion,ClusterParamGroup:DBClusterParameterGroup,DeletionProtection:DeletionProtection,BackupRetention:BackupRetentionPeriod}' \
    --output table

  section "6b. 集群侧 — 可升级的目标版本"
  aws rds describe-db-engine-versions \
    --engine aurora-postgresql \
    --engine-version "$(aws rds describe-db-clusters --db-cluster-identifier "$CLUSTER_ID" \
        --query 'DBClusters[0].EngineVersion' --output text)" \
    --query 'DBEngineVersions[0].ValidUpgradeTarget[?IsMajorVersionUpgrade==`true`].EngineVersion' \
    --output text
fi

# ---------- 7. Go/No-Go 判定 ----------
# TODO(需要业务判断)：实现 go_no_go_gate()，在 Switchover 前调用。
#
# 这一段故意留空，因为阈值是业务决策而非技术默认值：
#   - 可接受的写中断时长（决定 --switchover-timeout，30~3600s，默认 300）
#   - 允许切换的复制延迟上限（lsn_distance / OldestReplicationSlotLag，通常要求 0）
#   - 允许切换的活跃连接数 / 长事务时长上限
#   - 切换后多久内还允许走「切回蓝库（丢数据）」这条回滚路径
#
# 参考实现骨架（在下方补全判定逻辑，非 0 退出即 No-Go）：
#
# go_no_go_gate() {
#   local lsn_distance
#   lsn_distance=$("${PSQL[@]}" -t -A -c \
#     "SELECT coalesce(max(pg_current_wal_lsn() - confirmed_flush_lsn),0)
#        FROM pg_replication_slots WHERE slot_type='logical';")
#   # TODO: 在此判断 lsn_distance、长事务、连接数是否满足切换条件
# }

section "检查完成"
echo "阻断项（第 2、3 节）必须全部清零后才能创建蓝绿部署。"
echo "第 5 节基线请存档，切换后用于比对。"
