package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration193CreatesIdempotentPointsBalanceCreditLedger(t *testing.T) {
	content, err := FS.ReadFile("193_points_balance_credit_ledger.sql")
	require.NoError(t, err)
	sql := strings.ToUpper(string(content))

	require.Contains(t, sql, "TRANSACTION_ID UUID PRIMARY KEY")
	require.Contains(t, sql, "USER_ID BIGINT NOT NULL REFERENCES USERS(ID)")
	require.Contains(t, sql, "CHECK (AMOUNT <> 0)")
	require.Contains(t, sql, "KIND IN ('CHECKIN', 'MANUAL_GRANT', 'REVERSAL')")
	require.Contains(t, sql, "PAYLOAD_HASH CHAR(64) NOT NULL")
	require.Contains(t, sql, "BALANCE_AFTER NUMERIC(20, 10)")
}
