package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestReconcileExpiredMediaBalanceHoldsForUser_SettlesAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_generation_tasks`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_balance_holds.*UPDATE users.*SELECT totals.reconciled_count`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"reconciled_count", "updated_user_count"}).AddRow(int64(2), int64(1)))
	mock.ExpectCommit()

	repo := &usageBillingRepository{db: db}
	applied, err := repo.reconcileExpiredMediaBalanceHoldsForUser(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReconcileExpiredMediaBalanceHoldsForUser_IsIdempotentWhenNoActiveHoldsRemain(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_generation_tasks`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_balance_holds`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"reconciled_count", "updated_user_count"}).AddRow(int64(0), int64(0)))
	mock.ExpectCommit()

	repo := &usageBillingRepository{db: db}
	applied, err := repo.reconcileExpiredMediaBalanceHoldsForUser(context.Background(), 42)
	require.NoError(t, err)
	require.False(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReconcileExpiredMediaBalanceHolds_ContinuesAfterOneUserFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	firstExpiry := time.Now().Add(-2 * time.Hour)
	secondExpiry := firstExpiry.Add(time.Minute)
	mock.ExpectQuery(`(?s)SELECT user_id, MIN\(expires_at\).*FROM media_balance_holds.*LIMIT \$3`).
		WithArgs(nil, int64(0), 10).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "first_expires_at"}).AddRow(int64(11), firstExpiry).AddRow(int64(22), secondExpiry))

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_generation_tasks`).
		WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_balance_holds`).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"reconciled_count", "updated_user_count"}).AddRow(int64(1), int64(1)))
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_generation_tasks`).
		WithArgs(int64(22)).
		WillReturnError(errors.New("temporary database error"))
	mock.ExpectRollback()

	repo := &usageBillingRepository{db: db}
	result, err := repo.ReconcileExpiredMediaBalanceHolds(context.Background(), nil, 10)
	require.Error(t, err)
	require.Equal(t, 2, result.ScannedUsers)
	require.Equal(t, []int64{11}, result.ReconciledUserIDs)
	require.Equal(t, &service.MediaBalanceHoldReconciliationCursor{ExpiresAt: secondExpiry, UserID: 22}, result.NextCursor)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReconcileExpiredMediaBalanceHolds_RejectsInconsistentFrozenBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_generation_tasks`).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)WITH expired_holds AS MATERIALIZED .*UPDATE media_balance_holds`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"reconciled_count", "updated_user_count"}).AddRow(int64(1), int64(0)))
	mock.ExpectRollback()

	repo := &usageBillingRepository{db: db}
	_, err = repo.reconcileExpiredMediaBalanceHoldsForUser(context.Background(), 42)
	require.ErrorContains(t, err, "frozen balance is inconsistent")
	require.NoError(t, mock.ExpectationsWereMet())
}
