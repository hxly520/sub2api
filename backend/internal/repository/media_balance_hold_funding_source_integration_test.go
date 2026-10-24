//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMediaBalanceHoldFundingSourceCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rowSQL string
		want   int
	}{
		{name: "fresh schema without retired column", rowSQL: "SELECT 1::bigint AS id", want: 1},
		{name: "historical user balance", rowSQL: "SELECT 'user_balance'::text AS funding_source", want: 1},
		{name: "historical retired funding source", rowSQL: "SELECT 'link_card'::text AS funding_source", want: 0},
		{name: "unknown funding source fails closed", rowSQL: "SELECT 'unknown'::text AS funding_source", want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count int
			err := integrationDB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM ("+tc.rowSQL+") media_balance_holds WHERE "+userBalanceMediaHoldPredicateForTest).Scan(&count)
			require.NoError(t, err)
			require.Equal(t, tc.want, count)
		})
	}
}
