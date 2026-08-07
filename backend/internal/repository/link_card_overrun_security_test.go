package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestConsumeLinkCardCapsFinalRequestCostAtAvailableQuota(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT quota, quota_used, COALESCE\(link_reserved_amount, 0\), user_id.*FOR UPDATE`).
		WithArgs(int64(91), service.APIKeyTypeLink).
		WillReturnRows(sqlmock.NewRows([]string{"quota", "quota_used", "link_reserved_amount", "user_id", "link_state", "status"}).
			AddRow("1.00000000", "0.90000000", "0.00000000", int64(1), service.LinkCardStateActive, service.StatusAPIKeyActive))
	mock.ExpectExec(`(?s)UPDATE api_keys SET quota_used=\$1,status=\$2,link_state=\$3.*COALESCE\(link_reserved_amount,0\) >= \$6`).
		WithArgs(exactDecimalSQLArg("1.00000000"), service.StatusAPIKeyQuotaExhausted, service.LinkCardStateDepleted, int64(91), service.APIKeyTypeLink, exactDecimalSQLArg("0.10000000")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO link_card_ledger.*jsonb_build_object`).
		WithArgs(
			int64(91), int64(1), exactDecimalSQLArg("0.10000000"), exactDecimalSQLArg("1.00000000"),
			exactDecimalSQLArg("0.90000000"), exactDecimalSQLArg("1.00000000"), "request-overrun",
			exactDecimalSQLArg("0.50000000"), exactDecimalSQLArg("0.40000000"),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	exhausted, err := consumeUsageBillingLinkCard(context.Background(), tx, &service.UsageBillingCommand{
		RequestID: "request-overrun", APIKeyID: 91, UserID: 1, PrepaidLinkCost: 0.5,
	})
	require.NoError(t, err)
	require.True(t, exhausted)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConsumeLinkCardRecordsZeroChargeLateSettlement(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT quota, quota_used, COALESCE\(link_reserved_amount, 0\), user_id.*FOR UPDATE`).
		WithArgs(int64(91), service.APIKeyTypeLink).
		WillReturnRows(sqlmock.NewRows([]string{"quota", "quota_used", "link_reserved_amount", "user_id", "link_state", "status"}).
			AddRow("1.00000000", "1.00000000", "0.00000000", int64(1), service.LinkCardStateDepleted, service.StatusAPIKeyQuotaExhausted))
	mock.ExpectExec(`(?s)UPDATE api_keys SET quota_used=\$1,status=\$2,link_state=\$3.*COALESCE\(link_reserved_amount,0\) >= \$6`).
		WithArgs(exactDecimalSQLArg("1.00000000"), service.StatusAPIKeyQuotaExhausted, service.LinkCardStateDepleted, int64(91), service.APIKeyTypeLink, exactDecimalSQLArg("0.00000000")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO link_card_ledger.*jsonb_build_object`).
		WithArgs(
			int64(91), int64(1), exactDecimalSQLArg("0.00000000"), exactDecimalSQLArg("1.00000000"),
			exactDecimalSQLArg("1.00000000"), exactDecimalSQLArg("1.00000000"), "request-late-settlement",
			exactDecimalSQLArg("0.20000000"), exactDecimalSQLArg("0.20000000"),
		).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	exhausted, err := consumeUsageBillingLinkCard(context.Background(), tx, &service.UsageBillingCommand{
		RequestID: "request-late-settlement", APIKeyID: 91, UserID: 1, PrepaidLinkCost: 0.2,
	})
	require.NoError(t, err)
	require.True(t, exhausted)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
