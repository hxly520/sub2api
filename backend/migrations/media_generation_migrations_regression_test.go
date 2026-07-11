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
