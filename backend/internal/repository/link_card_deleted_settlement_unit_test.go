//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingApplyDeletedOrRefundedLinkCardRollsBack(t *testing.T) {
	for _, state := range []string{"deleted", service.LinkCardStateRefunded} {
		t.Run(state, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			cmd := &service.UsageBillingCommand{
				RequestID: "late-link-" + state, APIKeyID: 7, UserID: 42,
				PrepaidLinkCost: 10, APIKeyRateLimitCost: 10,
			}
			cmd.Normalize()
			mock.ExpectBegin()
			mock.ExpectQuery(`(?s)INSERT INTO usage_billing_dedup.*RETURNING id`).
				WithArgs(cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			mock.ExpectQuery(`(?s)SELECT request_fingerprint.*FROM usage_billing_dedup_archive`).
				WithArgs(cmd.RequestID, cmd.APIKeyID).WillReturnError(sql.ErrNoRows)
			query := mock.ExpectQuery(`(?s)SELECT quota, quota_used, COALESCE\(link_reserved_amount, 0\), user_id.*FOR UPDATE`).
				WithArgs(cmd.APIKeyID, service.APIKeyTypeLink)
			wantErr := service.ErrAPIKeyNotFound
			if state == "deleted" {
				query.WillReturnError(sql.ErrNoRows)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"quota", "quota_used", "reserved", "user_id", "state", "status"}).
					AddRow("20", "2", "0", cmd.UserID, state, service.StatusAPIKeyDisabled))
				wantErr = service.ErrLinkCardOperationNotAllowed
			}
			// No users UPDATE or key-counter UPDATE is permitted on this path.
			mock.ExpectRollback()
			result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), cmd)
			require.ErrorIs(t, err, wantErr)
			require.Nil(t, result)
			require.Zero(t, cmd.BalanceCost)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
