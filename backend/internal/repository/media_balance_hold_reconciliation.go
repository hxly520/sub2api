package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const maxMediaBalanceHoldReconciliationBatchSize = 1000

func (r *usageBillingRepository) ReconcileExpiredMediaBalanceHolds(ctx context.Context, after *service.MediaBalanceHoldReconciliationCursor, limit int) (*service.MediaBalanceHoldReconciliationResult, error) {
	result := &service.MediaBalanceHoldReconciliationResult{}
	if r == nil || r.db == nil {
		return result, errors.New("usage billing repository db is nil")
	}
	if limit <= 0 {
		return result, nil
	}
	if limit > maxMediaBalanceHoldReconciliationBatchSize {
		limit = maxMediaBalanceHoldReconciliationBatchSize
	}

	var afterExpiry any
	var afterUserID int64
	if after != nil {
		afterExpiry = after.ExpiresAt
		afterUserID = after.UserID
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT user_id, MIN(expires_at) AS first_expires_at
		FROM media_balance_holds
		WHERE funding_source = 'user_balance'
			AND status IN ('reserved', 'dispatched', 'capture_pending')
			AND expires_at <= NOW()
		GROUP BY user_id
		HAVING $1::timestamptz IS NULL
			OR MIN(expires_at) > $1
			OR (MIN(expires_at) = $1 AND user_id > $2)
		ORDER BY MIN(expires_at), user_id
		LIMIT $3
	`, afterExpiry, afterUserID, limit)
	if err != nil {
		return result, err
	}
	type expiredUser struct {
		userID    int64
		expiresAt time.Time
	}
	expiredUsers := make([]expiredUser, 0, limit)
	for rows.Next() {
		var item expiredUser
		if err := rows.Scan(&item.userID, &item.expiresAt); err != nil {
			_ = rows.Close()
			return result, err
		}
		expiredUsers = append(expiredUsers, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	result.ScannedUsers = len(expiredUsers)
	if len(expiredUsers) > 0 {
		last := expiredUsers[len(expiredUsers)-1]
		result.NextCursor = &service.MediaBalanceHoldReconciliationCursor{ExpiresAt: last.expiresAt, UserID: last.userID}
	}

	var reconciliationErrors []error
	for _, item := range expiredUsers {
		userID := item.userID
		if err := ctx.Err(); err != nil {
			reconciliationErrors = append(reconciliationErrors, err)
			break
		}
		applied, err := r.reconcileExpiredMediaBalanceHoldsForUser(ctx, userID)
		if err != nil {
			reconciliationErrors = append(reconciliationErrors, fmt.Errorf("reconcile expired media holds for user %d: %w", userID, err))
			continue
		}
		if applied {
			result.ReconciledUserIDs = append(result.ReconciledUserIDs, userID)
		}
	}
	return result, errors.Join(reconciliationErrors...)
}

func (r *usageBillingRepository) reconcileExpiredMediaBalanceHoldsForUser(ctx context.Context, userID int64) (_ bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	applied, err := reconcileExpiredMediaBalanceHolds(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	tx = nil
	return applied, nil
}
