package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageBillingRepository) ReserveMediaBalance(ctx context.Context, cmd *service.MediaBalanceHoldCommand) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || cmd.HoldAmount <= 0 {
		return &service.MediaBalanceHoldResult{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	if err := releaseExpiredMediaBalanceHolds(ctx, tx, cmd.UserID); err != nil {
		return nil, err
	}

	var holdID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO media_balance_holds (request_id, api_key_id, user_id, request_fingerprint, hold_amount, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, 'reserved', NOW() + INTERVAL '24 hours')
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id
	`, cmd.RequestID, cmd.APIKeyID, cmd.UserID, cmd.RequestFingerprint, cmd.HoldAmount).Scan(&holdID)
	if errors.Is(err, sql.ErrNoRows) {
		var existingUserID int64
		var fingerprint, status string
		var existingAmount float64
		if scanErr := tx.QueryRowContext(ctx, `
				SELECT user_id, request_fingerprint, hold_amount, status
				FROM media_balance_holds
				WHERE request_id = $1 AND api_key_id = $2
			`, cmd.RequestID, cmd.APIKeyID).Scan(&existingUserID, &fingerprint, &existingAmount, &status); scanErr != nil {
			return nil, scanErr
		}
		if existingUserID != cmd.UserID || strings.TrimSpace(fingerprint) != strings.TrimSpace(cmd.RequestFingerprint) || !mediaBalanceAmountsEqual(existingAmount, cmd.HoldAmount) {
			return nil, service.ErrMediaBalanceHoldConflict
		}
		if strings.EqualFold(status, "reserved") || strings.EqualFold(status, "dispatched") || strings.EqualFold(status, "capture_pending") {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			tx = nil
			return &service.MediaBalanceHoldResult{Applied: false}, nil
		}
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if err != nil {
		return nil, err
	}

	var balance, frozen float64
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			frozen_balance = COALESCE(frozen_balance, 0) + $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance >= $1
		RETURNING balance, frozen_balance
	`, cmd.HoldAmount, cmd.UserID).Scan(&balance, &frozen)
	if errors.Is(err, sql.ErrNoRows) {
		if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
			return nil, existsErr
		} else if !exists {
			return nil, service.ErrUserNotFound
		}
		return nil, service.ErrMediaInsufficientBalance
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return &service.MediaBalanceHoldResult{Applied: true, NewBalance: &balance, FrozenBalance: &frozen}, nil
}

func (r *usageBillingRepository) MarkMediaBalanceDispatched(ctx context.Context, cmd *service.MediaBalanceHoldCommand) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || cmd.HoldAmount <= 0 {
		return &service.MediaBalanceHoldResult{}, nil
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE media_balance_holds
		SET status = 'dispatched',
			expires_at = GREATEST(expires_at, NOW() + INTERVAL '24 hours'),
			updated_at = NOW()
		WHERE request_id = $1 AND api_key_id = $2 AND user_id = $3
			AND request_fingerprint = $4 AND hold_amount = $5
			AND status IN ('reserved', 'dispatched')
	`, cmd.RequestID, cmd.APIKeyID, cmd.UserID, cmd.RequestFingerprint, cmd.HoldAmount)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	return &service.MediaBalanceHoldResult{Applied: true}, nil
}

func (r *usageBillingRepository) MarkMediaBalanceForCapture(ctx context.Context, cmd *service.MediaBalanceHoldCommand, actualCost float64) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	actualCost = normalizeMediaBalanceRepoAmount(actualCost)
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || cmd.HoldAmount <= 0 || actualCost <= 0 || actualCost > cmd.HoldAmount+0.00000001 {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	var holdUserID int64
	var fingerprint, status string
	var holdAmount float64
	var existingCapture sql.NullFloat64
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, request_fingerprint, hold_amount, capture_amount, status
		FROM media_balance_holds
		WHERE request_id = $1 AND api_key_id = $2
		FOR UPDATE
	`, cmd.RequestID, cmd.APIKeyID).Scan(&holdUserID, &fingerprint, &holdAmount, &existingCapture, &status)
	if err != nil {
		return nil, err
	}
	if holdUserID != cmd.UserID || strings.TrimSpace(fingerprint) != strings.TrimSpace(cmd.RequestFingerprint) || !mediaBalanceAmountsEqual(holdAmount, cmd.HoldAmount) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if strings.EqualFold(status, "captured") {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		tx = nil
		return &service.MediaBalanceHoldResult{Applied: false}, nil
	}
	if !strings.EqualFold(status, "reserved") && !strings.EqualFold(status, "dispatched") && !strings.EqualFold(status, "capture_pending") {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if existingCapture.Valid && !mediaBalanceAmountsEqual(existingCapture.Float64, actualCost) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE media_balance_holds
		SET status = 'capture_pending',
			capture_amount = $2,
			expires_at = GREATEST(expires_at, NOW() + INTERVAL '24 hours'),
			updated_at = NOW()
		WHERE request_id = $1 AND api_key_id = $3
		  AND status IN ('reserved', 'dispatched', 'capture_pending')
	`, cmd.RequestID, actualCost, cmd.APIKeyID)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		if err != nil {
			return nil, err
		}
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return &service.MediaBalanceHoldResult{Applied: true}, nil
}

func (r *usageBillingRepository) ReleaseMediaBalance(ctx context.Context, cmd *service.MediaBalanceHoldCommand) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || cmd.HoldAmount <= 0 {
		return &service.MediaBalanceHoldResult{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	var holdID int64
	var holdUserID int64
	var fingerprint, status string
	var holdAmount float64
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, request_fingerprint, hold_amount, status
		FROM media_balance_holds
		WHERE request_id = $1 AND api_key_id = $2
		FOR UPDATE
	`, cmd.RequestID, cmd.APIKeyID).Scan(&holdID, &holdUserID, &fingerprint, &holdAmount, &status)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		tx = nil
		return &service.MediaBalanceHoldResult{Applied: false}, nil
	}
	if err != nil {
		return nil, err
	}
	if holdUserID != cmd.UserID || strings.TrimSpace(fingerprint) != strings.TrimSpace(cmd.RequestFingerprint) || !mediaBalanceAmountsEqual(holdAmount, cmd.HoldAmount) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if !strings.EqualFold(status, "reserved") && !strings.EqualFold(status, "dispatched") {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		tx = nil
		return &service.MediaBalanceHoldResult{Applied: false}, nil
	}

	var balance, frozen float64
	if err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance + $1,
			frozen_balance = COALESCE(frozen_balance, 0) - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= $1
		RETURNING balance, frozen_balance
	`, holdAmount, cmd.UserID).Scan(&balance, &frozen); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE media_balance_holds
		SET status = 'released', settled_amount = 0, settled_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, holdID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return &service.MediaBalanceHoldResult{Applied: true, NewBalance: &balance, FrozenBalance: &frozen}, nil
}

// captureMediaBalanceHold is called inside UsageBillingRepository.Apply, so
// hold settlement and the normal usage/quota updates commit atomically.
func (r *usageBillingRepository) captureMediaBalanceHold(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) (float64, error) {
	if strings.TrimSpace(cmd.MediaBalanceHoldRequestID) == "" {
		return 0, nil
	}
	var holdID int64
	var userID int64
	var holdAmount float64
	var status string
	var captureAmount sql.NullFloat64
	err := tx.QueryRowContext(ctx, `
		SELECT id, user_id, hold_amount, capture_amount, status
		FROM media_balance_holds
		WHERE request_id = $1 AND api_key_id = $2
		FOR UPDATE
	`, cmd.MediaBalanceHoldRequestID, cmd.APIKeyID).Scan(&holdID, &userID, &holdAmount, &captureAmount, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrMediaBalanceHoldNotFound
	}
	if err != nil {
		return 0, err
	}
	if userID != cmd.UserID {
		return 0, service.ErrMediaBalanceHoldConflict
	}
	if !strings.EqualFold(status, "reserved") && !strings.EqualFold(status, "dispatched") && !strings.EqualFold(status, "capture_pending") {
		return 0, service.ErrMediaBalanceHoldConflict
	}
	actual := cmd.MediaBalanceHoldActualCost
	if captureAmount.Valid && !mediaBalanceAmountsEqual(captureAmount.Float64, actual) {
		return 0, service.ErrMediaBalanceHoldConflict
	}
	if actual < 0 || actual > holdAmount+0.00000001 {
		return 0, service.ErrMediaBalanceCostExceedsHold
	}
	var balance float64
	if err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance + $1 - $2,
			frozen_balance = COALESCE(frozen_balance, 0) - $1,
			updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= $1
		RETURNING balance
	`, holdAmount, actual, userID).Scan(&balance); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE media_balance_holds
		SET status = 'captured', settled_amount = $2, settled_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, holdID, actual); err != nil {
		return 0, err
	}
	return balance, nil
}

func mediaBalanceAmountsEqual(left, right float64) bool {
	return math.Abs(left-right) <= 0.00000001
}

func normalizeMediaBalanceRepoAmount(value float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Ceil(value*1e8) / 1e8
}

func releaseExpiredMediaBalanceHolds(ctx context.Context, tx *sql.Tx, userID int64) error {
	if tx == nil || userID <= 0 {
		return nil
	}
	// Only a captured success marker or a successful async task can settle an
	// expired hold. Unsent, failed, and unresolved requests are refunded.
	var releaseAmount float64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(hold_amount), 0)
		FROM (
			SELECT holds.hold_amount
			FROM media_balance_holds holds
			LEFT JOIN media_generation_tasks tasks
				ON tasks.api_key_id = holds.api_key_id
				AND holds.request_id = 'media_balance_hold:' || tasks.public_task_id
			WHERE holds.user_id = $1
				AND holds.status IN ('reserved', 'dispatched')
				AND holds.expires_at <= NOW()
				AND (
					tasks.id IS NULL
					OR LOWER(BTRIM(tasks.status)) NOT IN (
						'complete', 'completed', 'success', 'succeeded', 'done'
					)
				)
			FOR UPDATE OF holds
		) expired_holds
	`, userID).Scan(&releaseAmount); err != nil {
		return err
	}
	var captureHoldAmount, captureAmount float64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(hold_amount), 0), COALESCE(SUM(actual_amount), 0)
		FROM (
			SELECT holds.hold_amount, COALESCE(holds.capture_amount, holds.hold_amount) AS actual_amount
			FROM media_balance_holds holds
				LEFT JOIN media_generation_tasks tasks
					ON tasks.api_key_id = holds.api_key_id
					AND holds.request_id = 'media_balance_hold:' || tasks.public_task_id
				WHERE holds.user_id = $1
					AND (
						holds.status = 'capture_pending'
						OR (
							holds.status IN ('reserved', 'dispatched')
							AND LOWER(BTRIM(tasks.status)) IN (
								'complete', 'completed', 'success', 'succeeded', 'done'
							)
						)
				)
				AND holds.expires_at <= NOW()
			FOR UPDATE OF holds
		) capture_holds
	`, userID).Scan(&captureHoldAmount, &captureAmount); err != nil {
		return err
	}
	totalExpired := releaseAmount + captureHoldAmount
	if totalExpired <= 0 {
		return nil
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1,
			frozen_balance = COALESCE(frozen_balance, 0) - $2,
			updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= $2
	`, releaseAmount+captureHoldAmount-captureAmount, totalExpired, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("expired media frozen balance is inconsistent")
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE media_balance_holds holds
		SET status = 'released', settled_amount = 0, settled_at = NOW(), updated_at = NOW()
		WHERE holds.user_id = $1
			AND holds.status IN ('reserved', 'dispatched')
			AND holds.expires_at <= NOW()
			AND NOT EXISTS (
				SELECT 1
				FROM media_generation_tasks tasks
				WHERE tasks.api_key_id = holds.api_key_id
					AND holds.request_id = 'media_balance_hold:' || tasks.public_task_id
					AND LOWER(BTRIM(tasks.status)) IN (
						'complete', 'completed', 'success', 'succeeded', 'done'
					)
			)
	`, userID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE media_balance_holds holds
		SET status = 'captured', settled_amount = COALESCE(holds.capture_amount, holds.hold_amount), settled_at = NOW(), updated_at = NOW()
		WHERE holds.user_id = $1
			AND holds.expires_at <= NOW()
			AND (
				holds.status = 'capture_pending'
				OR (
					holds.status IN ('reserved', 'dispatched')
					AND EXISTS (
						SELECT 1
						FROM media_generation_tasks tasks
						WHERE tasks.api_key_id = holds.api_key_id
							AND holds.request_id = 'media_balance_hold:' || tasks.public_task_id
							AND LOWER(BTRIM(tasks.status)) IN (
								'complete', 'completed', 'success', 'succeeded', 'done'
						)
					)
				)
			)
	`, userID)
	return err
}
