//go:build unit

package repository

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

const usageBillingKeyTypeSQL = `(?s)SELECT COALESCE\(to_jsonb\(api_keys\)->>'key_type', 'standard'\)\s+FROM api_keys\s+WHERE id = \$1\s+FOR KEY SHARE`

func TestStandardAPIKeyPredicate_ReadsOptionalRetiredColumnWithoutSelectingIt(t *testing.T) {
	for _, alias := range []string{"", "auth_key"} {
		t.Run(alias, func(t *testing.T) {
			table := entsql.Table("api_keys")
			rowRef := "api_keys"
			if alias != "" {
				table.As(alias)
				rowRef = alias
			}
			selector := entsql.Dialect(dialect.Postgres).Select("id").From(table)
			standardAPIKeyPredicate(selector)
			query, args := selector.Query()
			require.Contains(t, query, `COALESCE(to_jsonb("`+rowRef+`")->>'key_type', 'standard') = 'standard'`)
			require.Empty(t, args)
		})
	}
}

func TestRequireStandardUsageBillingAPIKey(t *testing.T) {
	for _, tc := range []struct {
		name    string
		keyType string
		missing bool
		wantErr bool
	}{
		{name: "ordinary", keyType: "standard"},
		{name: "already removed ordinary", missing: true},
		{name: "historical prepaid", keyType: "link", wantErr: true},
		{name: "unknown discriminator", keyType: "unknown", wantErr: true},
		{name: "invalid empty discriminator", keyType: "", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			tx, err := db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			rows := sqlmock.NewRows([]string{"key_type"})
			if !tc.missing {
				rows.AddRow(tc.keyType)
			}
			mock.ExpectQuery(usageBillingKeyTypeSQL).WithArgs(int64(7)).WillReturnRows(rows)
			mock.ExpectRollback()

			err = requireStandardUsageBillingAPIKey(context.Background(), tx, 7)
			if tc.wantErr {
				require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, tx.Rollback())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUsageBillingApply_RetiredKeyStopsBeforeDedupAndOwnerBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery(usageBillingKeyTypeSQL).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"key_type"}).AddRow("link"))
	mock.ExpectRollback()

	result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), &service.UsageBillingCommand{
		RequestID:   "retired-key-inflight",
		APIKeyID:    7,
		UserID:      42,
		BalanceCost: 10,
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingApply_KeyClassificationReadErrorFailsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery(usageBillingKeyTypeSQL).WithArgs(int64(7)).WillReturnError(sql.ErrConnDone)
	mock.ExpectRollback()

	result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), &service.UsageBillingCommand{
		RequestID: "retired-key-read-error", APIKeyID: 7, UserID: 42, BalanceCost: 10,
	})
	require.Nil(t, result)
	require.ErrorIs(t, err, sql.ErrConnDone)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchImageBalance_RetiredKeyCannotReachOwnerFunds(t *testing.T) {
	for _, action := range []string{"reserve", "capture", "release"} {
		t.Run(action, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectBegin()
			mock.ExpectQuery(usageBillingKeyTypeSQL).WithArgs(int64(7)).
				WillReturnRows(sqlmock.NewRows([]string{"key_type"}).AddRow("link"))
			mock.ExpectRollback()
			repo := &usageBillingRepository{db: db}
			cmd := &service.BatchImageBalanceHoldCommand{RequestID: "retired-batch-" + action, APIKeyID: 7, UserID: 42, HoldAmount: 10}
			var result *service.BatchImageBalanceHoldResult
			switch action {
			case "reserve":
				result, err = repo.ReserveBatchImageBalance(context.Background(), cmd)
			case "capture":
				result, err = repo.CaptureBatchImageBalance(context.Background(), cmd)
			case "release":
				result, err = repo.ReleaseBatchImageBalance(context.Background(), cmd)
			}
			require.Nil(t, result)
			require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestUsageBillingApply_StandardKeyStillSettlesOwnerBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery(usageBillingKeyTypeSQL).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"key_type"}).AddRow("standard"))
	mock.ExpectQuery(`INSERT INTO usage_billing_dedup`).WithArgs("ordinary-key-inflight", int64(7), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT request_fingerprint FROM usage_billing_dedup_archive WHERE request_id = $1 AND api_key_id = $2")).
		WithArgs("ordinary-key-inflight", int64(7)).WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
	mock.ExpectQuery(conditionalBalanceDeductSQL).WithArgs(10.0, int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(90.0))
	mock.ExpectCommit()

	result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), &service.UsageBillingCommand{
		RequestID: "ordinary-key-inflight", APIKeyID: 7, UserID: 42, BalanceCost: 10,
	})
	require.NoError(t, err)
	require.True(t, result.Applied)
	require.InDelta(t, 90, *result.NewBalance, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())
}
