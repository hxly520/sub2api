package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestLinkCardCreateBatchRollsBackEveryWriteWhenOneKeyFails(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO link_card_operations.*ON CONFLICT.*RETURNING id`).
		WithArgs("create", int64(1), int64(1), nil, "idem-hash", "fingerprint").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(501)))
	mock.ExpectQuery(`(?s)SELECT g\.id, g\.name, g\.platform.*FOR SHARE OF a, g`).
		WithArgs(int64(8), int64(1), service.SubscriptionTypeStandard).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "platform", "description", "rate_multiplier",
			"enabled", "sort_order", "created_at", "updated_at",
		}).AddRow(int64(8), "group-8", service.PlatformOpenAI, "", 0.08, true, 0, now, now))
	mock.ExpectQuery(`(?s)UPDATE users SET balance=balance-\$1.*RETURNING balance`).
		WithArgs("20.00000000", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("80.00000000"))
	mock.ExpectQuery(`(?s)INSERT INTO api_keys.*RETURNING id, created_at, updated_at`).
		WithArgs(
			int64(1), "sk-card-batch-first", sqlmock.AnyArg(), int64(8),
			service.StatusAPIKeyDisabled, service.APIKeyTypeLink,
			service.LinkCardStatePendingActivation, 0.08, "10.00000000", 5, 0,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(601), now, now))
	mock.ExpectQuery(`(?s)INSERT INTO link_card_ledger.*RETURNING id`).
		WithArgs(int64(501), int64(601), int64(1), "10.00000000", int64(8), 0.08, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(701)))
	mock.ExpectQuery(`(?s)INSERT INTO api_keys.*RETURNING id, created_at, updated_at`).
		WithArgs(
			int64(1), "sk-card-batch-second", sqlmock.AnyArg(), int64(8),
			service.StatusAPIKeyDisabled, service.APIKeyTypeLink,
			service.LinkCardStatePendingActivation, 0.08, "10.00000000", 5, 0,
		).
		WillReturnError(errors.New("forced second key failure"))
	mock.ExpectRollback()

	repo := NewLinkCardRepository(nil, db)
	_, err = repo.CreateCards(context.Background(), service.CreateLinkCardsCommand{
		CreatorUserID: 1,
		Group: service.LinkCardGroup{
			GroupID:        8,
			RateMultiplier: 0.08,
		},
		Quantity:           2,
		AmountPerCard:      decimal.NewFromInt(10),
		TotalDebit:         decimal.NewFromInt(20),
		Keys:               []string{"sk-card-batch-first", "sk-card-batch-second"},
		Concurrency:        5,
		RPMLimit:           0,
		IdempotencyKeyHash: "idem-hash",
		RequestFingerprint: "fingerprint",
	})
	require.EqualError(t, err, "forced second key failure")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLinkCardCreateSnapshotsNativeUserRateOverride(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO link_card_operations.*ON CONFLICT.*RETURNING id`).
		WithArgs("create", int64(1), int64(1), nil, "rate-idem", "rate-fingerprint").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(801)))
	mock.ExpectQuery(`(?s)SELECT g\.id, g\.name, g\.platform.*COALESCE\(ugr\.rate_multiplier, g\.rate_multiplier\).*LEFT JOIN user_allowed_groups uag.*LEFT JOIN user_group_rate_multipliers ugr.*\(g\.is_exclusive=FALSE OR uag\.user_id IS NOT NULL\).*FOR SHARE OF a, g`).
		WithArgs(int64(8), int64(1), service.SubscriptionTypeStandard).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "platform", "description", "rate_multiplier",
			"enabled", "sort_order", "created_at", "updated_at",
		}).AddRow(int64(8), "group-8", service.PlatformOpenAI, "", 0.07, true, 0, now, now))
	mock.ExpectQuery(`(?s)UPDATE users SET balance=balance-\$1.*RETURNING balance`).
		WithArgs("10.00000000", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("90.00000000"))
	mock.ExpectQuery(`(?s)INSERT INTO api_keys.*RETURNING id, created_at, updated_at`).
		WithArgs(
			int64(1), "sk-card-native-rate", sqlmock.AnyArg(), int64(8),
			service.StatusAPIKeyDisabled, service.APIKeyTypeLink,
			service.LinkCardStatePendingActivation, 0.07, "10.00000000", 5, 0,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(901), now, now))
	mock.ExpectQuery(`(?s)INSERT INTO link_card_ledger.*RETURNING id`).
		WithArgs(int64(801), int64(901), int64(1), "10.00000000", int64(8), 0.07, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1001)))
	mock.ExpectExec(`UPDATE link_card_operations SET response_body=\$1::jsonb WHERE id=\$2`).
		WithArgs(sqlmock.AnyArg(), int64(801)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewLinkCardRepository(nil, db)
	result, err := repo.CreateCards(context.Background(), service.CreateLinkCardsCommand{
		CreatorUserID: 1,
		Group: service.LinkCardGroup{
			GroupID:        8,
			RateMultiplier: 0.08,
		},
		Quantity:           1,
		AmountPerCard:      decimal.NewFromInt(10),
		TotalDebit:         decimal.NewFromInt(10),
		Keys:               []string{"sk-card-native-rate"},
		Concurrency:        5,
		RPMLimit:           0,
		IdempotencyKeyHash: "rate-idem",
		RequestFingerprint: "rate-fingerprint",
	})
	require.NoError(t, err)
	require.Len(t, result.Cards, 1)
	require.InDelta(t, 0.07, result.Cards[0].IssueRateMultiplier, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLinkCardCreateRejectsExclusiveGroupWithoutNativeUserGrant(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO link_card_operations.*ON CONFLICT.*RETURNING id`).
		WithArgs("create", int64(2), int64(2), nil, "exclusive-idem", "exclusive-fingerprint").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1101)))
	mock.ExpectQuery(`(?s)SELECT g\.id, g\.name, g\.platform.*LEFT JOIN user_allowed_groups uag.*LEFT JOIN user_group_rate_multipliers ugr.*\(g\.is_exclusive=FALSE OR uag\.user_id IS NOT NULL\).*FOR SHARE OF a, g`).
		WithArgs(int64(50), int64(2), service.SubscriptionTypeStandard).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "platform", "description", "rate_multiplier",
			"enabled", "sort_order", "created_at", "updated_at",
		}))
	mock.ExpectRollback()

	repo := NewLinkCardRepository(nil, db)
	_, err = repo.CreateCards(context.Background(), service.CreateLinkCardsCommand{
		CreatorUserID:      2,
		Group:              service.LinkCardGroup{GroupID: 50, RateMultiplier: 0.01},
		Quantity:           1,
		AmountPerCard:      decimal.NewFromInt(10),
		TotalDebit:         decimal.NewFromInt(10),
		Keys:               []string{"sk-card-exclusive-denied"},
		Concurrency:        5,
		IdempotencyKeyHash: "exclusive-idem",
		RequestFingerprint: "exclusive-fingerprint",
	})
	require.ErrorIs(t, err, service.ErrLinkCardGroupNotAuthorized)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLinkCardMutationRejectsNonOwnerBeforeAnyFinancialWrite(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT ak\.id,ak\.user_id.*FOR UPDATE OF ak`).
		WithArgs(service.APIKeyTypeLink, int64(91)).
		WillReturnRows(linkCardSecurityRows().AddRow(
			int64(91), int64(1), "owner@example.test", "sk-card-owner-only",
			int64(8), "group-8", service.PlatformOpenAI, 0.08,
			service.LinkCardStatePendingActivation, "100.00000000", "100.00000000",
			"0.00000000", "0.00000000", "0.00000000", 5, 0, nil, nil, "", now, now, int64(0),
		))
	mock.ExpectRollback()

	repo := NewLinkCardRepository(nil, db)
	_, err = repo.Refund(context.Background(), service.LinkCardMutationCommand{
		APIKeyID:           91,
		ActorUserID:        2,
		Admin:              false,
		Scope:              "refund",
		IdempotencyKeyHash: "idem-hash",
		RequestFingerprint: "fingerprint",
	})
	require.ErrorIs(t, err, service.ErrLinkCardNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLinkCardRechargeRestoresDepletedCardAtomically(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT ak\.id,ak\.user_id.*FOR UPDATE OF ak`).
		WithArgs(service.APIKeyTypeLink, int64(91)).
		WillReturnRows(linkCardSecurityRows().AddRow(
			int64(91), int64(1), "owner@example.test", "sk-card-depleted",
			int64(8), "group-8", service.PlatformOpenAI, 0.08,
			service.LinkCardStateDepleted, "10.00000000", "10.00000000",
			"0.00000000", "10.00000000", "0.00000000", 5, 0, now, nil, "", now, now, int64(1),
		))
	mock.ExpectQuery(`(?s)INSERT INTO link_card_operations.*ON CONFLICT.*RETURNING id`).
		WithArgs("recharge", int64(1), int64(1), int64(91), "recharge-idem", "recharge-fingerprint").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1201)))
	mock.ExpectQuery(`UPDATE users SET balance=balance-\$1.*RETURNING balance`).
		WithArgs("5.00000000", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow("15.00000000"))
	mock.ExpectExec(`UPDATE api_keys SET quota=quota\+\$1,link_total_funded=link_total_funded\+\$1,link_state=\$2,status=\$3`).
		WithArgs("5.00000000", service.LinkCardStateActive, service.StatusAPIKeyActive, int64(91), service.APIKeyTypeLink).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)INSERT INTO link_card_ledger.*'recharge'.*RETURNING id`).
		WithArgs(
			int64(1201), int64(91), int64(1), "5.00000000", "10.00000000",
			"15.00000000", "10.00000000", int64(1), "creator top up",
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1301)))
	mock.ExpectExec(`UPDATE link_card_operations SET response_body=\$1::jsonb WHERE id=\$2`).
		WithArgs(sqlmock.AnyArg(), int64(1201)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewLinkCardRepository(nil, db)
	result, err := repo.Recharge(context.Background(), service.LinkCardMutationCommand{
		APIKeyID:           91,
		ActorUserID:        1,
		Scope:              "recharge",
		Amount:             decimal.NewFromInt(5),
		Reason:             "creator top up",
		IdempotencyKeyHash: "recharge-idem",
		RequestFingerprint: "recharge-fingerprint",
	})
	require.NoError(t, err)
	require.Equal(t, service.LinkCardStateActive, result.Card.Status)
	require.True(t, result.Card.TotalDepositAmount.Equal(decimal.NewFromInt(15)))
	require.True(t, result.RemainingUserBalance.Equal(decimal.NewFromInt(15)))
	require.NoError(t, mock.ExpectationsWereMet())
}

func linkCardSecurityRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "user_id", "email", "key", "group_id", "group_name", "platform",
		"link_rate_multiplier", "link_state", "link_original_debit", "link_total_funded",
		"link_total_refunded", "quota_used", "link_reserved_amount", "link_concurrency", "link_rpm_limit",
		"link_activated_at", "link_revoked_at", "link_frozen_reason", "created_at",
		"updated_at", "request_count",
	})
}

func TestLinkCardUsageWhereAppliesRequestTypeFilter(t *testing.T) {
	tests := []struct {
		name        string
		requestType string
		wantLegacy  string
		wantArg     int16
	}{
		{
			name:        "sync",
			requestType: "sync",
			wantLegacy:  "(ul.request_type = 0 AND ul.stream = FALSE AND ul.openai_ws_mode = FALSE)",
			wantArg:     int16(service.RequestTypeSync),
		},
		{
			name:        "stream",
			requestType: "stream",
			wantLegacy:  "(ul.request_type = 0 AND ul.stream = TRUE AND ul.openai_ws_mode = FALSE)",
			wantArg:     int16(service.RequestTypeStream),
		},
		{
			name:        "websocket",
			requestType: "ws_v2",
			wantLegacy:  "(ul.request_type = 0 AND ul.openai_ws_mode = TRUE)",
			wantArg:     int16(service.RequestTypeWSV2),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			where, args := buildLinkCardUsageWhere(nil, service.LinkCardUsageFilters{RequestType: test.requestType})

			require.Contains(t, where, "ul.request_type = $1")
			require.Contains(t, where, test.wantLegacy, "link-card usage filters must include historical rows")
			require.Equal(t, []any{test.wantArg}, args)
		})
	}
}

type exactDecimalSQLArg string

func (want exactDecimalSQLArg) Match(value driver.Value) bool {
	actual, err := decimal.NewFromString(fmt.Sprint(value))
	if err != nil {
		return false
	}
	return actual.Equal(decimal.RequireFromString(string(want)))
}

func TestConsumeLinkCardPreservesExactDecimalBoundary(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT quota, quota_used, COALESCE\(link_reserved_amount, 0\), user_id.*FOR UPDATE`).
		WithArgs(int64(91), service.APIKeyTypeLink).
		WillReturnRows(sqlmock.NewRows([]string{"quota", "quota_used", "link_reserved_amount", "user_id", "link_state", "status"}).
			AddRow("0.3000000000", "0.2000000000", "0.0000000000", int64(1), service.LinkCardStateActive, service.StatusAPIKeyActive))
	mock.ExpectExec(`(?s)UPDATE api_keys SET quota_used=\$1,status=\$2,link_state=\$3.*COALESCE\(link_reserved_amount,0\) >= \$6`).
		WithArgs(exactDecimalSQLArg("0.30000000"), service.StatusAPIKeyQuotaExhausted, service.LinkCardStateDepleted, int64(91), service.APIKeyTypeLink, exactDecimalSQLArg("0.10000000")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO link_card_ledger.*jsonb_build_object`).
		WithArgs(
			int64(91), int64(1), exactDecimalSQLArg("0.10000000"), exactDecimalSQLArg("0.30000000"),
			exactDecimalSQLArg("0.20000000"), exactDecimalSQLArg("0.30000000"), "request-decimal-boundary",
			exactDecimalSQLArg("0.10000000"), exactDecimalSQLArg("0.00000000"),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	exhausted, err := consumeUsageBillingLinkCard(context.Background(), tx, &service.UsageBillingCommand{
		RequestID:       "request-decimal-boundary",
		APIKeyID:        91,
		UserID:          1,
		PrepaidLinkCost: 0.1,
	})
	require.NoError(t, err)
	require.True(t, exhausted)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}
