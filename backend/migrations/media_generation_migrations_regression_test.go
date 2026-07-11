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
