package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestReserveLinkCardMediaBalanceUsesOnlyCardReserve(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	cmd := &service.MediaBalanceHoldCommand{
		RequestID:          "media_balance_hold:reserve-1",
		APIKeyID:           91,
		UserID:             1,
		RequestFingerprint: "fingerprint-1",
		HoldAmount:         0.8,
		LinkCard:           true,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT user_id,key_type,COALESCE(link_state,''),status
		FROM api_keys WHERE id=$1 AND deleted_at IS NULL FOR UPDATE
	`)).WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "key_type", "link_state", "status"}).
		AddRow(int64(1), service.APIKeyTypeLink, service.LinkCardStateActive, service.StatusAPIKeyActive))
	mock.ExpectQuery(`(?s)SELECT id,request_id,hold_amount,capture_amount,status.*FROM media_balance_holds.*funding_source=\$3`).
		WithArgs(int64(91), int64(1), linkCardHoldFundingSource).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "hold_amount", "capture_amount", "status"}))
	mock.ExpectQuery(`(?s)SELECT user_id,request_fingerprint,hold_amount,funding_source,status.*FROM media_balance_holds`).
		WithArgs(cmd.RequestID, cmd.APIKeyID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`(?s)SELECT quota,quota_used,COALESCE\(link_reserved_amount,0\).*FROM api_keys.*FOR UPDATE`).
		WithArgs(cmd.APIKeyID, service.APIKeyTypeLink).
		WillReturnRows(sqlmock.NewRows([]string{"quota", "quota_used", "link_reserved_amount"}).AddRow("10.00000000", "1.00000000", "2.00000000"))
	mock.ExpectExec(`(?s)UPDATE api_keys SET link_reserved_amount=\$1`).
		WithArgs("2.80000000", cmd.APIKeyID, service.APIKeyTypeLink).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO media_balance_holds.*funding_source`).
		WithArgs(cmd.RequestID, cmd.APIKeyID, cmd.UserID, cmd.RequestFingerprint, "0.80000000", int64(86400), linkCardHoldFundingSource).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	repo := &usageBillingRepository{db: db}
	result, err := repo.ReserveLinkCardMediaBalance(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReserveLinkCardMediaBalanceRejectsInsufficientAvailableQuota(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	cmd := &service.MediaBalanceHoldCommand{
		RequestID:          "media_balance_hold:reserve-2",
		APIKeyID:           92,
		UserID:             1,
		RequestFingerprint: "fingerprint-2",
		HoldAmount:         0.5,
		LinkCard:           true,
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT user_id,key_type.*FROM api_keys.*FOR UPDATE`).
		WithArgs(int64(92)).WillReturnRows(sqlmock.NewRows([]string{"user_id", "key_type", "link_state", "status"}).
		AddRow(int64(1), service.APIKeyTypeLink, service.LinkCardStateActive, service.StatusAPIKeyActive))
	mock.ExpectQuery(`(?s)SELECT id,request_id,hold_amount,capture_amount,status.*FROM media_balance_holds`).
		WithArgs(int64(92), int64(1), linkCardHoldFundingSource).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "hold_amount", "capture_amount", "status"}))
	mock.ExpectQuery(`(?s)SELECT user_id,request_fingerprint,hold_amount,funding_source,status.*FROM media_balance_holds`).
		WithArgs(cmd.RequestID, cmd.APIKeyID).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`(?s)SELECT quota,quota_used,COALESCE\(link_reserved_amount,0\).*FROM api_keys.*FOR UPDATE`).
		WithArgs(cmd.APIKeyID, service.APIKeyTypeLink).
		WillReturnRows(sqlmock.NewRows([]string{"quota", "quota_used", "link_reserved_amount"}).AddRow("1.00000000", "0.80000000", "0.10000000"))
	mock.ExpectRollback()

	repo := &usageBillingRepository{db: db}
	_, err = repo.ReserveLinkCardMediaBalance(context.Background(), cmd)
	require.ErrorIs(t, err, service.ErrLinkCardPrepaidExhausted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStandardMediaReconciliationIsFundingSourceScoped(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`(?s)SELECT user_id, MIN\(expires_at\).*FROM media_balance_holds.*funding_source = 'user_balance'`).
		WithArgs(nil, int64(0), 10).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "first_expires_at"}))

	repo := &usageBillingRepository{db: db}
	result, err := repo.ReconcileExpiredMediaBalanceHolds(context.Background(), nil, 10)
	require.NoError(t, err)
	require.Zero(t, result.ScannedUsers)
	require.NoError(t, mock.ExpectationsWereMet())
}
