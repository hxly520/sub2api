# Database Migrations

## Overview

> **候选源码边界（2026-10-24）**：提链/额度卡已从当前候选的运行时代码、路由、前端页面和部署入口移除；本次未切换生产容器，尚未核验生产运行态是否停用。下文旧版本的相关接口、配置、账本、验收和回滚步骤仅为历史或审计资料，不是当前候选的部署、启用或回滚指令。`194_link_cards.sql` 原文件、原 checksum 和既有迁移历史保持不变，当前候选迁移 runner 按完整文件名跳过它；不得借升级重新启用旧入口或把历史提链 Key 当作普通 Key 使用。

### 当前候选总览（2026-10-24）

- 当前工作树版本为 `0.2.18-52t.1`，官方升级目标为 `v0.2.18`；后端 `go.mod` 使用 Go `1.27.3`。当前测试、发布、镜像与缓存状态以 [v0.2.18 候选交接](../../docs/RELEASE_V0_2_18_52T_1_CN.md) 的逐项证据为准，不沿用下方 v0.2.15 的历史全绿结论。
- 当前候选完全移除提链/额度卡产品运行时，只保留原 `194_link_cards.sql`、checksum、迁移历史和审计数据；历史 Key/媒体冻结分类须失败关闭，禁止回落普通余额。
- 本次不切换或重启生产，不修改生产 PostgreSQL、Redis、Nginx 或其他容器。2026-10-24 已重新只读核验生产为 `0.2.15-52t.1`，healthy、restart 0；旧配置仍启用提链，尚未切换本候选，具体见本轮交接。



This directory contains SQL migration files for database schema changes. The migration system uses SHA256 checksums to ensure migration immutability and consistency across environments.

## Migration File Naming

Format: `NNN_description.sql`
- `NNN`: Sequential number (e.g., 001, 002, 003)
- `description`: Brief description in snake_case

Example: `017_add_gemini_tier_id.sql`

### `_notx.sql` 命名与执行语义（并发索引专用）

当迁移包含 `CREATE INDEX CONCURRENTLY` 或 `DROP INDEX CONCURRENTLY` 时，必须使用 `_notx.sql` 后缀，例如：

- `062_add_accounts_priority_indexes_notx.sql`
- `063_drop_legacy_indexes_notx.sql`

运行规则：

1. `*.sql`（不带 `_notx`）按事务执行。
2. `*_notx.sql` 按非事务执行，不会包裹在 `BEGIN/COMMIT` 中。
3. `*_notx.sql` 仅允许并发索引语句，不允许混入事务控制语句或其他 DDL/DML。

幂等要求（必须）：

- 创建索引：`CREATE INDEX CONCURRENTLY IF NOT EXISTS ...`
- 删除索引：`DROP INDEX CONCURRENTLY IF EXISTS ...`

这样可以保证灾备重放、重复执行时不会因对象已存在/不存在而失败。

## Migration File Structure

This project uses a custom migration runner (`internal/repository/migrations_runner.go`) that executes the full SQL file content as-is.

- Regular migrations (`*.sql`): executed in a transaction.
- Non-transactional migrations (`*_notx.sql`): split by statement and executed without transaction (for `CONCURRENTLY`).

```sql
-- Forward-only migration (recommended)
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS example_column VARCHAR(100);
```

> ⚠️ Do **not** place executable "Down" SQL in the same file. The runner does not parse goose Up/Down sections and will execute all SQL statements in the file.

## Private Migration Registry

- `193_points_balance_credit_ledger.sql` and the historical `194_link_cards.sql` are immutable private migrations recorded in `docs/PRODUCTION_OPERATIONS_CN.md`; the link-card migration is retained for audit only and is not used by the current runtime.
- Historical `194_link_cards.sql` entered production on `2026-08-08`; preserve the original file and SHA256 checksum over trimmed content (`7a40799ddd3379acda1a3f704f110d81278a8d38705cd965325880996a8d23b4`) for audit. Do not execute it for the current runtime.
- Upstream `v0.1.175` adds `194_add_usage_log_upstream_response_model.sql`, `194_channel_monitor_v2.sql`, `195_add_usage_log_upstream_model_mismatch_index_notx.sql`, `195_channel_monitor_mode.sql`, the Channel Monitor V2 sequence through `206_channel_monitor_v2_privacy_defaults.sql`, and pricing/config migrations `217_group_video_model_prices.sql` through `220_clear_non_grok_video_generation_config.sql`.
- Upstream `v0.1.176` adds `221_group_model_pricing.sql`, which adds the group-level long-context pricing switch and per-model JSONB pricing overrides. The migration is forward-only and must be applied by the image/Compose upgrade after a database backup; it is not eligible for binary hot update.
- Upstream `v0.1.183` adds these migrations: `222_group_usage_daily_rollups.sql`, `223_group_usage_rollup_timezone.sql`, `224_user_platform_quotas_add_cn_providers.sql`, `225_backfill_codex_fingerprint_seed.sql`, `225_channel_model_time_pricing.sql`, `226_add_usage_log_effective_model_indexes_notx.sql`, `226_channel_monitor_quota_mode.sql`, `227_composite_routes_add_cn_providers.sql`, `228_channel_pricing_multipliers.sql`, `229_plugins.sql`, and `230_plugin_artifacts.sql`. The set includes duplicate numeric prefixes (`225` and `226`); all files are independent migrations and must be tracked by complete filename and checksum.
- Upstream `v0.2.5` adds `235_group_model_allowlist.sql`, `236_group_model_allowlist_repair.sql`, `237_add_minimax_platform.sql`, `238_opencode_go_platform.sql`, and `238_purge_unlimited_user_platform_quotas.sql`. Both `238` files are independent and coexist by complete filename. Migration `235` promotes the legacy model-list setting into request admission; before switching the image, audit every group whose legacy list is enabled and confirm all production models used by standard keys and media requests are allowed; link-card keys are historical and disabled.
- The three `194` migrations coexist by complete filename; `194_link_cards.sql` is historical and audit-only: `194_add_usage_log_upstream_response_model.sql`, `194_channel_monitor_v2.sql`, and `194_link_cards.sql`. The runner identifies and records migrations by the complete filename plus checksum, never by the numeric prefix alone.
- Keep every filename and SQL body unchanged. Lexical ordering applies among duplicate numeric prefixes (`194_add...`, `194_channel...`, `194_link...`, then `195_add...`, `195_channel...`). The current runner explicitly skips the complete audit-only filename `194_link_cards.sql` before application; other already-applied migrations are checksum-validated and each missing upstream file is applied independently. Never generalize this exception to all `194` files.
- The `v0.2.5-52t.1` compatibility release is `image-update-required`. Back up PostgreSQL and audit `schema_migrations` by full filename/checksum, including the complete `235-238` set and both `238` files, before the maintainer performs the Compose image switch; do not install this migration set through binary hot update.
- Any correction must use a new, unique migration filename after the current migration set. Never edit, rename, delete, or renumber an applied migration.
- Historical rollback records do not remove link-card tables or ledger rows. The product runtime is removed from this candidate, while production has not been switched or reverified by this change. Its historical tables/rows are retained only for audit; do not re-enable link-card routes or keys, and never load legacy link-card keys as standard keys or charge the creator balance. Preserve operations and ledger rows when auditing old data.

The former link-card product checklist is retired. Historical production application status is recorded in [`../../docs/PRODUCTION_OPERATIONS_CN.md`](../../docs/PRODUCTION_OPERATIONS_CN.md) and [`../../docs/LINK_CARDS_ACCEPTANCE_20260808_CN.md`](../../docs/LINK_CARDS_ACCEPTANCE_20260808_CN.md).

## Important Rules

### ⚠️ Immutability Principle

**Once a migration is applied to ANY environment (dev, staging, production), it MUST NOT be modified.**

Why?
- Each migration has a SHA256 checksum stored in the `schema_migrations` table
- Modifying an applied migration causes checksum mismatch errors
- Different environments would have inconsistent database states
- Breaks audit trail and reproducibility

### ✅ Correct Workflow

1. **Create new migration**
   ```bash
   # Create new file with next sequential number
   touch migrations/018_your_change.sql
   ```

2. **Write forward-only migration SQL**
   - Put only the intended schema change in the file
   - If rollback is needed, create a new migration file to revert

3. **Test locally**
   ```bash
   # Apply migration
   make migrate-up

   # Test rollback
   make migrate-down
   ```

4. **Commit and deploy**
   ```bash
   git add migrations/018_your_change.sql
   git commit -m "feat(db): add your change"
   ```

### ❌ What NOT to Do

- ❌ Modify an already-applied migration file
- ❌ Delete migration files
- ❌ Change migration file names
- ❌ Reorder migration numbers

### 🔧 If You Accidentally Modified an Applied Migration

**Error message:**
```
migration 017_add_gemini_tier_id.sql checksum mismatch (db=abc123... file=def456...)
```

**Solution:**
```bash
# 1. Find the original version
git log --oneline -- migrations/017_add_gemini_tier_id.sql

# 2. Revert to the commit when it was first applied
git checkout <commit-hash> -- migrations/017_add_gemini_tier_id.sql

# 3. Create a NEW migration for your changes
touch migrations/018_your_new_change.sql
```

## Migration System Details

- **Checksum Algorithm**: SHA256 of trimmed file content
- **Tracking Table**: `schema_migrations` (filename, checksum, applied_at)
- **Runner**: `internal/repository/migrations_runner.go`
- **Auto-run**: Migrations run automatically on service startup

## Best Practices

1. **Keep migrations small and focused**
   - One logical change per migration
   - Easier to review and rollback

2. **Plan reversibility without inline Down SQL**
   - Keep the applied migration forward-only and immutable
   - Use a new numbered migration for schema reversal when that is safe
   - Document application rollback compatibility separately from database restoration

3. **Use transactions**
   - Wrap DDL statements in transactions when possible
   - Ensures atomicity

4. **Add comments**
   - Explain WHY the change is needed
   - Document any special considerations

5. **Test in development first**
   - Apply migration locally
   - Verify data integrity
   - Test rollback

## Example Migration

```sql
-- Add tier_id field to Gemini OAuth accounts for quota tracking
UPDATE accounts
SET credentials = jsonb_set(
    credentials,
    '{tier_id}',
    '"LEGACY"',
    true
)
WHERE platform = 'gemini'
  AND type = 'oauth'
  AND credentials->>'tier_id' IS NULL;
```

## Troubleshooting

### Checksum Mismatch
See "If You Accidentally Modified an Applied Migration" above.

### Migration Failed
```bash
# Check migration status
psql -d sub2api -c "SELECT * FROM schema_migrations ORDER BY applied_at DESC;"

# Manually rollback if needed (use with caution)
# Better to fix the migration and create a new one
```

### Need to Skip a Migration (Emergency Only)
```sql
-- DANGEROUS: Only use in development or with extreme caution
INSERT INTO schema_migrations (filename, checksum, applied_at)
VALUES ('NNN_migration.sql', 'calculated_checksum', NOW());
```

## References

- Migration runner: `internal/repository/migrations_runner.go`
- PostgreSQL docs: https://www.postgresql.org/docs/
