package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration192AddsGlobalMediaHoldExpiryIndexWithoutTransaction(t *testing.T) {
	content, err := FS.ReadFile("192_media_balance_hold_reconciliation_index_notx.sql")
	require.NoError(t, err)
	sql := strings.ToUpper(string(content))

	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS")
	require.Contains(t, sql, "ON MEDIA_BALANCE_HOLDS (EXPIRES_AT, USER_ID)")
	require.Contains(t, sql, "'RESERVED', 'DISPATCHED', 'CAPTURE_PENDING'")
}
