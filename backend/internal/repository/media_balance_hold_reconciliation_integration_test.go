//go:build integration

package repository

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type mediaReconciliationFixture struct {
	user *service.User
	key  *service.APIKey
	hold *service.MediaBalanceHoldCommand
}

func TestMediaBalanceHoldReconciliation_AllUsersAndSettlementOutcomes(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	mediaRepo := repo.(service.MediaBalanceHoldRepository)
	reconciliationRepo := repo.(service.MediaBalanceHoldReconciliationRepository)

	newFixture := func(name, requestID string) mediaReconciliationFixture {
		user := mustCreateUser(t, client, &service.User{
			Email:        fmt.Sprintf("media-reconciliation-%s-%d@example.com", name, time.Now().UnixNano()),
			PasswordHash: "hash",
			Balance:      10,
		})
		key := mustCreateApiKey(t, client, &service.APIKey{
			UserID: user.ID,
			Key:    "sk-media-reconciliation-" + uuid.NewString(),
			Name:   name,
		})
		if requestID == "" {
			requestID = service.NewMediaBalanceHoldRequestID()
		}
		hold := &service.MediaBalanceHoldCommand{
			RequestID:          requestID,
			APIKeyID:           key.ID,
			UserID:             user.ID,
			RequestFingerprint: strings.Repeat(name[:1], 64),
			HoldAmount:         2,
		}
		_, err := mediaRepo.ReserveMediaBalance(ctx, hold)
		require.NoError(t, err)
		return mediaReconciliationFixture{user: user, key: key, hold: hold}
	}

	releasedTaskID := "reconciliation-expired-task-" + uuid.NewString()
	released := newFixture("released", service.MediaBalanceHoldRequestID(releasedTaskID))
	_, err := mediaRepo.MarkMediaBalanceDispatched(ctx, released.hold)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO media_generation_tasks (
			task_id, public_task_id, api_key_id, user_id, account_id, model,
			request_fingerprint, status, media_type, created_at, updated_at
		) VALUES ($1, $1, $2, $3, 0, 'image-model', $4, 'pending', 'image', NOW(), NOW())
	`, releasedTaskID, released.key.ID, released.user.ID, released.hold.RequestFingerprint)
	require.NoError(t, err)
	require.NoError(t, expireMediaHold(ctx, released.hold))

	capturePending := newFixture("capture", "")
	_, err = mediaRepo.MarkMediaBalanceForCapture(ctx, capturePending.hold, 0.75)
	require.NoError(t, err)
	require.NoError(t, expireMediaHold(ctx, capturePending.hold))

	taskID := "reconciliation-success-task-" + uuid.NewString()
	successfulTask := newFixture("success", service.MediaBalanceHoldRequestID(taskID))
	_, err = mediaRepo.MarkMediaBalanceDispatched(ctx, successfulTask.hold)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO media_generation_tasks (
			task_id, public_task_id, api_key_id, user_id, account_id, model,
			request_fingerprint, status, media_type, created_at, updated_at
		) VALUES ($1, $1, $2, $3, 0, 'video-model', $4, 'completed', 'video', NOW(), NOW())
	`, taskID, successfulTask.key.ID, successfulTask.user.ID, successfulTask.hold.RequestFingerprint)
	require.NoError(t, err)
	require.NoError(t, expireMediaHold(ctx, successfulTask.hold))

	result, err := reconciliationRepo.ReconcileExpiredMediaBalanceHolds(ctx, nil, 1000)
	require.NoError(t, err)
	for _, userID := range []int64{released.user.ID, capturePending.user.ID, successfulTask.user.ID} {
		require.True(t, slices.Contains(result.ReconciledUserIDs, userID), "user %d was not reconciled", userID)
	}

	assertMediaReconciliationBalance(t, ctx, released, 10, 0, "released", 0)
	assertMediaReconciliationBalance(t, ctx, capturePending, 9.25, 0, "captured", 0.75)
	assertMediaReconciliationBalance(t, ctx, successfulTask, 8, 0, "captured", 2)
	var releasedTaskStatus, releasedTaskError string
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status, COALESCE(finalization_error, '') FROM media_generation_tasks
		WHERE api_key_id = $1 AND public_task_id = $2
	`, released.key.ID, releasedTaskID).Scan(&releasedTaskStatus, &releasedTaskError))
	require.Equal(t, service.MediaGenerationStatusExpired, releasedTaskStatus)
	require.Equal(t, "balance_hold_expired", releasedTaskError)

	second, err := reconciliationRepo.ReconcileExpiredMediaBalanceHolds(ctx, nil, 1000)
	require.NoError(t, err)
	for _, userID := range []int64{released.user.ID, capturePending.user.ID, successfulTask.user.ID} {
		require.False(t, slices.Contains(second.ReconciledUserIDs, userID), "user %d was reconciled twice", userID)
	}
}

func expireMediaHold(ctx context.Context, hold *service.MediaBalanceHoldCommand) error {
	_, err := integrationDB.ExecContext(ctx, `
		UPDATE media_balance_holds SET expires_at = NOW() - INTERVAL '1 minute'
		WHERE request_id = $1 AND api_key_id = $2
	`, hold.RequestID, hold.APIKeyID)
	return err
}

func assertMediaReconciliationBalance(t *testing.T, ctx context.Context, fixture mediaReconciliationFixture, wantBalance, wantFrozen float64, wantStatus string, wantSettled float64) {
	t.Helper()
	var balance, frozen float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance, frozen_balance FROM users WHERE id = $1", fixture.user.ID).Scan(&balance, &frozen))
	require.InDelta(t, wantBalance, balance, 0.00000001)
	require.InDelta(t, wantFrozen, frozen, 0.00000001)
	var status string
	var settled float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT status, settled_amount FROM media_balance_holds
		WHERE request_id = $1 AND api_key_id = $2
	`, fixture.hold.RequestID, fixture.key.ID).Scan(&status, &settled))
	require.Equal(t, wantStatus, status)
	require.InDelta(t, wantSettled, settled, 0.00000001)
}
