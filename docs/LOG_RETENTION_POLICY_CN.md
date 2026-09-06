# 日志保留策略（52Token 二开）

## 目标

- **使用记录**：`public.usage_logs` 默认保留最近 **30 天**。
- **操作日志**：`public.audit_logs` 默认保留最近 **30 天**。
- **运维系统日志**：启用运维清理任务后，`ops_system_logs`、`ops_error_logs`、`ops_alert_events`、入口拒绝聚合和清理审计按 `error_log_retention_days` 保留；建议值为 **30 天**。

清理任务按时间索引分批删除，不执行 `TRUNCATE`，不会中断请求处理。保留窗口按 `created_at`（聚合表按自身时间列）计算。

## 明确不清理的数据

以下数据不属于本策略，保留期限和内容不随日志策略变化：

- `usage_billing_dedup`（计费幂等/追溯记录，默认保留 365 天）；
- `payment_orders`、`payment_audit_logs`、充值/退款及兑换码记录；
- `users`、`settings`、`groups`、`api_keys`、余额、积分账本与其他业务配置数据。

不得用日志清理脚本删除上述表，也不得通过覆盖 `settings` 整行来修改配置。

## 配置入口与优先级

1. `dashboard_aggregation.retention.usage_logs_days`（环境变量
   `DASHBOARD_AGGREGATION_RETENTION_USAGE_LOGS_DAYS`）控制 `usage_logs`；默认值为 30。
2. 管理后台系统设置中的 `audit_log_retention_days` 控制 `audit_logs`；空值/非法值回退 30，`0` 仍表示永久保留。
3. `ops_advanced_settings.data_retention` 覆盖 `ops.cleanup` 配置。要自动清理运维系统日志，需将 `cleanup_enabled` 设为 `true`，并将 `error_log_retention_days` 设为 30；保存后由调度器按原 cron 计划运行。其他字段保持原值。

数据库中已有的显式设置优先于代码默认值。升级镜像不会强制覆盖已有设置；生产变更前应先备份相关 JSON，仅更新目标字段，确认最近 30 天记录仍可查询后再观察分批清理结果。

## 验证清单

- 查询 `usage_logs`、`audit_logs` 和（启用后）`ops_system_logs` 的最早/最新时间及 30 天外行数；
- 确认 `usage_billing_dedup`、充值表和 `settings` 非目标字段行数与内容不变；
- 检查清理心跳/审计记录成功，再在低峰期执行普通 `VACUUM (ANALYZE)`；不要在生产高峰执行 `VACUUM FULL`。

## 2026-09-06 生产执行记录

- 生产 Compose 已显式传入 `DASHBOARD_AGGREGATION_RETENTION_USAGE_LOGS_DAYS=30`
  与 `DASHBOARD_AGGREGATION_RETENTION_USAGE_BILLING_DEDUP_DAYS=365`，并在健康检查
  通过后保持 `sub2api`、PostgreSQL、Redis、积分和生图工作台运行。
- 执行前快照与可恢复备份位于
  `/home/api/sub2api-deploy/backups/log-retention-20260906-024727/`，包括
  `target-logs.dump`、`settings-before.sql`、`rollups-before.sql`、配置副本和
  `SHA256SUMS.*`。
- `audit_log_retention_days` 仅从 `180` 调整为 `30`；非目标 `settings` 行的
  `id/key/value` 集合在变更前后完全一致。
- 按固定 30 天截止时间分批清理 `usage_logs` 与 `audit_logs`，并重建受影响的
  `usage_group_daily_rollups`；固定截止时间复核的超期行数均为 `0`。随后执行
  `VACUUM (ANALYZE)`，未执行高锁定风险的 `VACUUM FULL`。
- 本次 SQL 未写入 `payment_orders`、`payment_audit_logs`、`usage_billing_dedup` 或
  其他业务表；生产容器镜像摘要未变化，重建后健康状态为 `healthy`、重启次数为 `0`。
- `ops_system_logs` 等运维日志仍由 `ops_advanced_settings.data_retention` 单独控制，
  当前 `cleanup_enabled=false`，因此本次不删除该表；如需纳入 30 天策略，先单独确认
  运维日志范围并在后台开启清理。
