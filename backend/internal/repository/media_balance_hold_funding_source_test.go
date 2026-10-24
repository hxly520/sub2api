package repository

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const userBalanceMediaHoldPredicateForTest = "COALESCE(to_jsonb(media_balance_holds)->>'funding_source', 'user_balance') = 'user_balance'"

func mediaFundingSourceHoldCommand() *service.MediaBalanceHoldCommand {
	return &service.MediaBalanceHoldCommand{
		RequestID:          "media-funding-source-test",
		APIKeyID:           7,
		UserID:             42,
		RequestFingerprint: strings.Repeat("a", 64),
		HoldAmount:         2,
	}
}

func TestMediaBalanceHold_UsesSchemaCompatibleFundingSourceGuard(t *testing.T) {
	predicate := regexp.QuoteMeta(userBalanceMediaHoldPredicateForTest)
	ctx := context.Background()

	t.Run("reserve rejects retired key before any balance or hold mutation", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		cmd := mediaFundingSourceHoldCommand()
		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT COALESCE\(to_jsonb\(api_keys\)->>'key_type', 'standard'\).*FROM api_keys.*WHERE id = \$1.*FOR KEY SHARE`).
			WithArgs(cmd.APIKeyID).
			WillReturnRows(sqlmock.NewRows([]string{"key_type"}).AddRow("link"))
		mock.ExpectRollback()
		result, err := (&usageBillingRepository{db: db}).ReserveMediaBalance(ctx, cmd)
		require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
		require.Nil(t, result)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("dispatch excludes historical non-user holds", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		cmd := mediaFundingSourceHoldCommand()
		mock.ExpectExec(`(?s)UPDATE media_balance_holds.*`+predicate).
			WithArgs(cmd.RequestID, cmd.APIKeyID, cmd.UserID, cmd.RequestFingerprint, cmd.HoldAmount, cmd.ExpirySeconds()).
			WillReturnResult(sqlmock.NewResult(0, 0))
		_, err = (&usageBillingRepository{db: db}).MarkMediaBalanceDispatched(ctx, cmd)
		require.ErrorIs(t, err, service.ErrMediaBalanceHoldConflict)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("capture marker excludes historical non-user holds", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		cmd := mediaFundingSourceHoldCommand()
		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT user_id, request_fingerprint.*FROM media_balance_holds.*`+predicate).
			WithArgs(cmd.RequestID, cmd.APIKeyID).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()
		_, err = (&usageBillingRepository{db: db}).MarkMediaBalanceForCapture(ctx, cmd, 1)
		require.ErrorIs(t, err, sql.ErrNoRows)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("release never refunds historical non-user holds", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		cmd := mediaFundingSourceHoldCommand()
		mock.ExpectBegin()
		mock.ExpectQuery(`(?s)SELECT id, user_id, request_fingerprint.*FROM media_balance_holds.*`+predicate).
			WithArgs(cmd.RequestID, cmd.APIKeyID).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectCommit()
		result, err := (&usageBillingRepository{db: db}).ReleaseMediaBalance(ctx, cmd)
		require.NoError(t, err)
		require.False(t, result.Applied)
		require.Nil(t, result.NewBalance)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("billing capture never settles historical non-user holds", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		cmd := mediaFundingSourceHoldCommand()
		mock.ExpectBegin()
		tx, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		mock.ExpectQuery(`(?s)SELECT id, user_id, hold_amount.*FROM media_balance_holds.*`+predicate).
			WithArgs(cmd.RequestID, cmd.APIKeyID).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()
		_, err = (&usageBillingRepository{db: db}).captureMediaBalanceHold(ctx, tx, &service.UsageBillingCommand{
			APIKeyID: cmd.APIKeyID, UserID: cmd.UserID, MediaBalanceHoldRequestID: cmd.RequestID, MediaBalanceHoldActualCost: 1,
		})
		require.ErrorIs(t, err, service.ErrMediaBalanceHoldNotFound)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestMediaBalanceHold_ReconciliationExcludesHistoricalNonUserHolds(t *testing.T) {
	predicate := regexp.QuoteMeta(userBalanceMediaHoldPredicateForTest)
	ctx := context.Background()

	t.Run("global scan excludes audit-only holds", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectQuery(`(?s)SELECT user_id, MIN\(expires_at\).*FROM media_balance_holds.*`+predicate+`.*LIMIT \$3`).
			WithArgs(nil, int64(0), 10).
			WillReturnRows(sqlmock.NewRows([]string{"user_id", "first_expires_at"}))
		result, err := (&usageBillingRepository{db: db}).ReconcileExpiredMediaBalanceHolds(ctx, nil, 10)
		require.NoError(t, err)
		require.Zero(t, result.ScannedUsers)
		require.Empty(t, result.ReconciledUserIDs)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("settlement and task expiry both exclude audit-only holds", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		mock.ExpectBegin()
		mock.ExpectExec(`(?s)WITH expired_holds AS MATERIALIZED .*` + predicate + `.*UPDATE media_generation_tasks`).
			WithArgs(int64(42)).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`(?s)WITH expired_holds AS MATERIALIZED .*` + predicate + `.*UPDATE media_balance_holds.*UPDATE users`).
			WithArgs(int64(42)).
			WillReturnRows(sqlmock.NewRows([]string{"reconciled_count", "updated_user_count"}).AddRow(int64(0), int64(0)))
		mock.ExpectCommit()
		applied, err := (&usageBillingRepository{db: db}).reconcileExpiredMediaBalanceHoldsForUser(ctx, 42)
		require.NoError(t, err)
		require.False(t, applied)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
