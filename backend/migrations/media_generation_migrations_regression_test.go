package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration176LetsSuccessfulMediaTasksRecoverIncompleteBilling(t *testing.T) {
	content, err := FS.ReadFile("176_media_generation_finalization_recovery.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "WHERE usage_recorded_at IS NULL")
	require.NotContains(t, sql, "finalized_at IS NULL")
}

func TestMigration177AddsNullableMediaPricingSnapshot(t *testing.T) {
	content, err := FS.ReadFile("177_media_generation_pricing_snapshot.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "billing_unit_price NUMERIC(20, 10)")
	require.Contains(t, sql, "billing_rate_multiplier NUMERIC(20, 10)")
	require.NotContains(t, sql, "UPDATE media_generation_tasks")
}

func TestMigration178AddsAtomicMediaBalanceHolds(t *testing.T) {
	content, err := FS.ReadFile("178_media_balance_holds.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS media_balance_holds")
	require.Contains(t, sql, "UNIQUE (request_id, api_key_id)")
	require.Contains(t, sql, "status VARCHAR(16) NOT NULL DEFAULT 'reserved'")
	require.Contains(t, sql, "capture_amount NUMERIC(20, 8) NULL")
	require.Contains(t, sql, "ON media_balance_holds (user_id, expires_at)")
	require.Contains(t, sql, "status IN ('reserved', 'capture_pending')")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS request_count")
	require.NotContains(t, sql, "UPDATE media_generation_tasks")
}
