package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMediaBalanceHoldCommandExpirySeconds(t *testing.T) {
	t.Parallel()

	require.Equal(t, int64((24*time.Hour)/time.Second), (&MediaBalanceHoldCommand{}).ExpirySeconds())
	require.Equal(t, int64((30*time.Minute)/time.Second), (&MediaBalanceHoldCommand{
		ExpiresAfter: SynchronousMediaBalanceHoldTTL,
	}).ExpirySeconds())
	require.Equal(t, int64(2), (&MediaBalanceHoldCommand{ExpiresAfter: 1500 * time.Millisecond}).ExpirySeconds())
	require.Equal(t, int64((24*time.Hour)/time.Second), (&MediaBalanceHoldCommand{
		ExpiresAfter: 48 * time.Hour,
	}).ExpirySeconds())
}

func TestMediaGenerationPricingSnapshotEstimatedCost(t *testing.T) {
	tests := []struct {
		name     string
		snapshot *MediaGenerationPricingSnapshot
		count    int
		duration int
		want     float64
	}{
		{
			name:     "image per request",
			snapshot: &MediaGenerationPricingSnapshot{Mode: BillingModeImage, UnitPrice: 0.25, RateMultiplier: 1.2},
			count:    2,
			want:     0.6,
		},
		{
			name:     "video per second",
			snapshot: &MediaGenerationPricingSnapshot{Mode: BillingModeVideo, UnitPrice: 0.1, RateMultiplier: 0.8},
			count:    1,
			duration: 10,
			want:     0.8,
		},
		{
			name:     "video per request ignores duration",
			snapshot: &MediaGenerationPricingSnapshot{Mode: BillingModePerRequest, UnitPrice: 2.5, RateMultiplier: 1},
			count:    1,
			duration: 15,
			want:     2.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.InDelta(t, tt.want, tt.snapshot.EstimatedCost(tt.count, tt.duration), 1e-12)
		})
	}
}

func TestUsageBillingFingerprintIncludesMediaSettlement(t *testing.T) {
	base := &UsageBillingCommand{
		RequestID:                  "request-1",
		APIKeyID:                   10,
		UserID:                     20,
		MediaBalanceHoldRequestID:  "media_balance_hold:task-1",
		MediaBalanceHoldActualCost: 1.25,
	}
	first := buildUsageBillingFingerprint(base)

	changedHold := *base
	changedHold.MediaBalanceHoldRequestID = "media_balance_hold:task-2"
	require.NotEqual(t, first, buildUsageBillingFingerprint(&changedHold))

	changedCost := *base
	changedCost.MediaBalanceHoldActualCost = 1.5
	require.NotEqual(t, first, buildUsageBillingFingerprint(&changedCost))
}

func TestCapMediaBalanceHoldActualCost(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 0.05, capMediaBalanceHoldActualCost(0.10, "media_balance_hold:test", 0.05), 0.00000001)
	require.InDelta(t, 0.04, capMediaBalanceHoldActualCost(0.04, "media_balance_hold:test", 0.05), 0.00000001)
	require.InDelta(t, 0.10, capMediaBalanceHoldActualCost(0.10, "", 0.05), 0.00000001)
}
