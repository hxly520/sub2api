//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingApplyDeletedOrRefundedLinkCardPreservesMoneyAndDedup(t *testing.T) {
	for _, state := range []string{"deleted", service.LinkCardStateRefunded} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			client := testEntClient(t)
			user := mustCreateUser(t, client, &service.User{
				Email: "late-link-" + uuid.NewString() + "@example.com", PasswordHash: "hash", Balance: 100,
			})
			key, err := client.APIKey.Create().SetUserID(user.ID).
				SetKey("sk-late-link-" + uuid.NewString()).SetName("late-settlement-compat").
				SetKeyType(service.APIKeyTypeLink).SetLinkState(service.LinkCardStateActive).
				SetStatus(service.StatusAPIKeyActive).SetLinkRateMultiplier(0.08).
				SetLinkOriginalDebit(10).SetLinkTotalFunded(10).SetLinkReservedAmount(0).
				SetQuota(10).SetQuotaUsed(2.5).Save(ctx)
			require.NoError(t, err)
			t.Cleanup(func() {
				cleanupCtx := context.Background()
				_, cleanupErr := integrationDB.ExecContext(cleanupCtx, "DELETE FROM usage_billing_dedup WHERE api_key_id=$1", key.ID)
				require.NoError(t, cleanupErr)
				_, cleanupErr = integrationDB.ExecContext(cleanupCtx, "DELETE FROM api_keys WHERE id=$1", key.ID)
				require.NoError(t, cleanupErr)
				_, cleanupErr = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id=$1", user.ID)
				require.NoError(t, cleanupErr)
			})
			update := client.APIKey.UpdateOneID(key.ID).SetStatus(service.StatusAPIKeyDisabled)
			if state == "deleted" {
				update.SetDeletedAt(time.Now())
			} else {
				update.SetLinkState(service.LinkCardStateRefunded).SetLinkTotalRefunded(7.5)
			}
			require.NoError(t, update.Exec(ctx))
			cmd := &service.UsageBillingCommand{
				RequestID: "late-link-" + uuid.NewString(), APIKeyID: key.ID, UserID: user.ID,
				PrepaidLinkCost: 1.25, APIKeyRateLimitCost: 1.25,
			}
			wantErr := service.ErrAPIKeyNotFound
			if state == service.LinkCardStateRefunded {
				wantErr = service.ErrLinkCardOperationNotAllowed
			}
			repo := NewUsageBillingRepository(client, integrationDB)
			// A repeated attempt must remain an error, never a successful dedup no-op.
			for range 2 {
				result, err := repo.Apply(ctx, cmd)
				require.ErrorIs(t, err, wantErr)
				require.Nil(t, result)
				require.Zero(t, cmd.BalanceCost)
			}
			var balance, quotaUsed, usage5h float64
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id=$1", user.ID).Scan(&balance))
			require.InDelta(t, 100, balance, 1e-8, "late link usage must not fall back to creator balance")
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT quota_used,usage_5h FROM api_keys WHERE id=$1", key.ID).Scan(&quotaUsed, &usage5h))
			require.InDelta(t, 2.5, quotaUsed, 1e-8)
			require.Zero(t, usage5h)
			var dedupCount, ledgerCount int
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id=$1 AND request_id=$2", key.ID, cmd.RequestID).Scan(&dedupCount))
			require.Zero(t, dedupCount, "the failed settlement must roll back its dedup claim")
			require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM link_card_ledger WHERE api_key_id=$1", key.ID).Scan(&ledgerCount))
			require.Zero(t, ledgerCount)
		})
	}
}
