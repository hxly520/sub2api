package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTryAcquireMediaGenerationFinalizationRecoversPersistedSuccess(t *testing.T) {
	sql := strings.ToLower(tryAcquireMediaGenerationFinalizationSQL)
	require.Contains(t, sql, "usage_recorded_at is null")
	require.NotContains(t, sql, "finalized_at is null")
	require.Contains(t, sql, "lower(btrim(status)) in ('complete', 'completed', 'success', 'succeeded', 'done')")
	require.Contains(t, sql, "finalization_lease_until is null or finalization_lease_until <= now()")
}
