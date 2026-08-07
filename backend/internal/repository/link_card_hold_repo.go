package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/shopspring/decimal"
)

// Link-card holds share media_balance_holds with standard media holds, but
// move funds in api_keys.link_reserved_amount rather than users.frozen_balance.
// Keeping the implementation in a separate file makes the standard billing
// SQL paths easy to audit and leaves their query shape unchanged.

const linkCardHoldFundingSource = "link_card"

func (r *usageBillingRepository) ReserveLinkCardMediaBalance(ctx context.Context, cmd *service.MediaBalanceHoldCommand) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	amount, ok := linkCardHoldAmount(cmd.HoldAmount)
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || !ok {
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
	result, err := r.reserveLinkCardHoldTx(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.UserID,
		cmd.RequestFingerprint, amount, cmd.ExpirySeconds())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) MarkLinkCardMediaBalanceDispatched(ctx context.Context, cmd *service.MediaBalanceHoldCommand) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	amount, ok := linkCardHoldAmount(cmd.HoldAmount)
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || !ok {
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
	if err := lockLinkCardForHold(ctx, tx, cmd.APIKeyID, cmd.UserID); err != nil {
		return nil, err
	}
	var rows int64
	res, err := tx.ExecContext(ctx, `
		UPDATE media_balance_holds
		SET status='dispatched',
			expires_at=GREATEST(expires_at, NOW()+($6 * INTERVAL '1 second')),
			updated_at=NOW()
		WHERE request_id=$1 AND api_key_id=$2 AND user_id=$3
		  AND request_fingerprint=$4 AND hold_amount=$5
		  AND funding_source=$7 AND status IN ('reserved','dispatched')
	`, cmd.RequestID, cmd.APIKeyID, cmd.UserID, cmd.RequestFingerprint, amount.StringFixed(8), cmd.ExpirySeconds(), linkCardHoldFundingSource)
	if err != nil {
		return nil, err
	}
	rows, err = res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return &service.MediaBalanceHoldResult{Applied: true}, nil
}

func (r *usageBillingRepository) MarkLinkCardMediaBalanceForCapture(ctx context.Context, cmd *service.MediaBalanceHoldCommand, actualCost float64) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	holdAmount, ok := linkCardHoldAmount(cmd.HoldAmount)
	actual, actualOK := linkCardHoldAmount(actualCost)
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || !ok || (!actualOK && actualCost > 0) || (actualCost > 0 && actual.GreaterThan(holdAmount)) {
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
	if err := lockLinkCardForHold(ctx, tx, cmd.APIKeyID, cmd.UserID); err != nil {
		return nil, err
	}
	var holdUser int64
	var holdFingerprint, source, status string
	var storedAmount decimal.Decimal
	var storedCapture sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT user_id,request_fingerprint,hold_amount,capture_amount,funding_source,status
		FROM media_balance_holds
		WHERE request_id=$1 AND api_key_id=$2
		FOR UPDATE
	`, cmd.RequestID, cmd.APIKeyID).Scan(&holdUser, &holdFingerprint, &storedAmount, &storedCapture, &source, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMediaBalanceHoldNotFound
	}
	if err != nil {
		return nil, err
	}
	if holdUser != cmd.UserID || strings.TrimSpace(holdFingerprint) != strings.TrimSpace(cmd.RequestFingerprint) || source != linkCardHoldFundingSource || !storedAmount.Equal(holdAmount) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if status == "captured" {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		tx = nil
		return &service.MediaBalanceHoldResult{Applied: false}, nil
	}
	if status != "reserved" && status != "dispatched" && status != "capture_pending" {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if storedCapture.Valid {
		stored, parseErr := decimal.NewFromString(storedCapture.String)
		if parseErr != nil || !stored.Equal(actual) {
			return nil, service.ErrMediaBalanceHoldConflict
		}
	}
	updated, err := tx.ExecContext(ctx, `
		UPDATE media_balance_holds
		SET status='capture_pending',capture_amount=$1,
			expires_at=GREATEST(expires_at,NOW()+($2 * INTERVAL '1 second')),updated_at=NOW()
		WHERE request_id=$3 AND api_key_id=$4 AND funding_source=$5
		  AND status IN ('reserved','dispatched','capture_pending')
	`, actual.StringFixed(8), cmd.ExpirySeconds(), cmd.RequestID, cmd.APIKeyID, linkCardHoldFundingSource)
	if err != nil {
		return nil, err
	}
	if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
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

func (r *usageBillingRepository) ReleaseLinkCardMediaBalance(ctx context.Context, cmd *service.MediaBalanceHoldCommand) (_ *service.MediaBalanceHoldResult, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if cmd == nil {
		return &service.MediaBalanceHoldResult{}, nil
	}
	cmd.Normalize()
	holdAmount, ok := linkCardHoldAmount(cmd.HoldAmount)
	if cmd.RequestID == "" || cmd.APIKeyID <= 0 || cmd.UserID <= 0 || !ok {
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
	result, err := r.releaseLinkCardHoldTx(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.UserID,
		cmd.RequestFingerprint, holdAmount)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

// reserveLinkCardHoldTx is shared by synchronous media and batch-image holds.
// The caller owns the transaction. Card rows are always locked before hold
// rows, which gives reserve/capture/release a single lock order.
func (r *usageBillingRepository) reserveLinkCardHoldTx(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID, userID int64, fingerprint string, amount decimal.Decimal, expirySeconds int64) (*service.MediaBalanceHoldResult, error) {
	linkState, apiStatus, err := lockLinkCardForHoldState(ctx, tx, apiKeyID, userID)
	if err != nil {
		return nil, err
	}
	if err := r.settleExpiredLinkCardHoldsTx(ctx, tx, apiKeyID, userID); err != nil {
		return nil, err
	}
	var existingUser int64
	var existingFingerprint, existingSource, existingStatus string
	var existingAmount decimal.Decimal
	err = tx.QueryRowContext(ctx, `
		SELECT user_id,request_fingerprint,hold_amount,funding_source,status
		FROM media_balance_holds
		WHERE request_id=$1 AND api_key_id=$2
		FOR UPDATE
	`, requestID, apiKeyID).Scan(&existingUser, &existingFingerprint, &existingAmount, &existingSource, &existingStatus)
	if err == nil {
		if existingUser != userID || strings.TrimSpace(existingFingerprint) != strings.TrimSpace(fingerprint) || existingSource != linkCardHoldFundingSource || !existingAmount.Equal(amount) {
			return nil, service.ErrMediaBalanceHoldConflict
		}
		if isActiveLinkCardHoldStatus(existingStatus) {
			return &service.MediaBalanceHoldResult{Applied: false}, nil
		}
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var quota, used, reserved decimal.Decimal
	if err := tx.QueryRowContext(ctx, `
		SELECT quota,quota_used,COALESCE(link_reserved_amount,0)
		FROM api_keys WHERE id=$1 AND key_type=$2 AND deleted_at IS NULL
		FOR UPDATE
	`, apiKeyID, service.APIKeyTypeLink).Scan(&quota, &used, &reserved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	if linkState != service.LinkCardStateActive || apiStatus != service.StatusAPIKeyActive {
		return nil, service.ErrLinkCardOperationNotAllowed
	}
	if quota.IsNegative() || quota.IsZero() || quota.Sub(used).Sub(reserved).LessThan(amount) {
		return nil, service.ErrLinkCardPrepaidExhausted
	}
	newReserved := reserved.Add(amount)
	updated, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET link_reserved_amount=$1,updated_at=NOW()
		WHERE id=$2 AND key_type=$3
		  AND quota - quota_used - link_reserved_amount >= $1 - link_reserved_amount
	`, newReserved.StringFixed(8), apiKeyID, service.APIKeyTypeLink)
	if err != nil {
		return nil, err
	}
	if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
		if err != nil {
			return nil, err
		}
		return nil, service.ErrLinkCardPrepaidExhausted
	}
	if expirySeconds <= 0 {
		expirySeconds = int64((24 * time.Hour) / time.Second)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO media_balance_holds
			(request_id,api_key_id,user_id,request_fingerprint,hold_amount,status,expires_at,funding_source)
		VALUES($1,$2,$3,$4,$5,'reserved',NOW()+($6 * INTERVAL '1 second'),$7)
	`, requestID, apiKeyID, userID, fingerprint, amount.StringFixed(8), expirySeconds, linkCardHoldFundingSource); err != nil {
		return nil, err
	}
	return &service.MediaBalanceHoldResult{Applied: true}, nil
}

func (r *usageBillingRepository) captureLinkCardHoldTx(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID, userID int64, fingerprint string, holdAmount, actual decimal.Decimal) (*service.MediaBalanceHoldResult, error) {
	if err := lockLinkCardForHold(ctx, tx, apiKeyID, userID); err != nil {
		return nil, err
	}
	var holdUser int64
	var holdFingerprint, source, status string
	var storedAmount decimal.Decimal
	var captureAmount sql.NullString
	var holdID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id,user_id,request_fingerprint,hold_amount,capture_amount,funding_source,status
		FROM media_balance_holds
		WHERE request_id=$1 AND api_key_id=$2
		FOR UPDATE
	`, requestID, apiKeyID).Scan(&holdID, &holdUser, &holdFingerprint, &storedAmount, &captureAmount, &source, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMediaBalanceHoldNotFound
	}
	if err != nil {
		return nil, err
	}
	if holdUser != userID || (strings.TrimSpace(fingerprint) != "" && strings.TrimSpace(holdFingerprint) != strings.TrimSpace(fingerprint)) || source != linkCardHoldFundingSource || !storedAmount.Equal(holdAmount) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if status == "captured" {
		return &service.MediaBalanceHoldResult{Applied: false}, nil
	}
	if status != "reserved" && status != "dispatched" && status != "capture_pending" {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if captureAmount.Valid {
		storedCapture, parseErr := decimal.NewFromString(captureAmount.String)
		if parseErr != nil || !storedCapture.Equal(actual) {
			return nil, service.ErrMediaBalanceHoldConflict
		}
	}
	var quota, used, reserved decimal.Decimal
	if err := tx.QueryRowContext(ctx, `
		SELECT quota,quota_used,COALESCE(link_reserved_amount,0)
		FROM api_keys WHERE id=$1 AND key_type=$2 AND deleted_at IS NULL
		FOR UPDATE
	`, apiKeyID, service.APIKeyTypeLink).Scan(&quota, &used, &reserved); err != nil {
		return nil, err
	}
	if reserved.LessThan(holdAmount) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	newUsed := used.Add(actual)
	if quota.IsPositive() && newUsed.GreaterThan(quota) {
		return nil, service.ErrMediaBalanceCostExceedsHold
	}
	newReserved := reserved.Sub(holdAmount)
	exhausted := quota.IsPositive() && newUsed.GreaterThanOrEqual(quota)
	var currentState, newStatus string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(link_state,''),status FROM api_keys WHERE id=$1 FOR UPDATE`, apiKeyID).Scan(&currentState, &newStatus); err != nil {
		return nil, err
	}
	newState := currentState
	if exhausted {
		newState = service.LinkCardStateDepleted
		newStatus = service.StatusAPIKeyQuotaExhausted
	} else if currentState == service.LinkCardStateActive {
		newStatus = service.StatusAPIKeyActive
	}
	if currentState == service.LinkCardStateFrozen {
		newState = service.LinkCardStateFrozen
		newStatus = service.StatusAPIKeyDisabled
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET quota_used=$1,link_reserved_amount=$2,link_state=$3,status=$4,updated_at=NOW()
		WHERE id=$5 AND key_type=$6
	`, newUsed.StringFixed(8), newReserved.StringFixed(8), newState, newStatus, apiKeyID, service.APIKeyTypeLink); err != nil {
		return nil, err
	}
	if actual.IsPositive() {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO link_card_ledger(api_key_id,creator_user_id,entry_type,reserve_delta,creator_balance_delta,
				quota_before,quota_after,quota_used_before,quota_used_after,request_id,reason,metadata)
			VALUES($1,$2,'usage',-($3::numeric),0,$4,$4,$5,$6,$7,'media hold settlement',
				jsonb_build_object('hold_amount',$8,'actual_cost',$3))
		`, apiKeyID, userID, actual.StringFixed(8), quota.StringFixed(8), used.StringFixed(8), newUsed.StringFixed(8), requestID, holdAmount.StringFixed(8)); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE media_balance_holds
		SET status='captured',capture_amount=$1,settled_amount=$1,settled_at=NOW(),updated_at=NOW()
		WHERE id=$2
	`, actual.StringFixed(8), holdID); err != nil {
		return nil, err
	}
	return &service.MediaBalanceHoldResult{Applied: true, LinkCardQuotaExhausted: exhausted}, nil
}

func (r *usageBillingRepository) releaseLinkCardHoldTx(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID, userID int64, fingerprint string, holdAmount decimal.Decimal) (*service.MediaBalanceHoldResult, error) {
	if err := lockLinkCardForHold(ctx, tx, apiKeyID, userID); err != nil {
		return nil, err
	}
	var holdID, holdUser int64
	var storedFingerprint, source, status string
	var storedAmount decimal.Decimal
	err := tx.QueryRowContext(ctx, `
		SELECT id,user_id,request_fingerprint,hold_amount,funding_source,status
		FROM media_balance_holds
		WHERE request_id=$1 AND api_key_id=$2
		FOR UPDATE
	`, requestID, apiKeyID).Scan(&holdID, &holdUser, &storedFingerprint, &storedAmount, &source, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return &service.MediaBalanceHoldResult{Applied: false}, nil
	}
	if err != nil {
		return nil, err
	}
	if holdUser != userID || source != linkCardHoldFundingSource || (strings.TrimSpace(fingerprint) != "" && strings.TrimSpace(storedFingerprint) != strings.TrimSpace(fingerprint)) || !storedAmount.Equal(holdAmount) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	if status == "captured" || status == "released" || status == "capture_pending" {
		return &service.MediaBalanceHoldResult{Applied: false}, nil
	}
	var reserved decimal.Decimal
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(link_reserved_amount,0) FROM api_keys WHERE id=$1 FOR UPDATE`, apiKeyID).Scan(&reserved); err != nil {
		return nil, err
	}
	if reserved.LessThan(holdAmount) {
		return nil, service.ErrMediaBalanceHoldConflict
	}
	newReserved := reserved.Sub(holdAmount)
	if _, err := tx.ExecContext(ctx, `UPDATE api_keys SET link_reserved_amount=$1,updated_at=NOW() WHERE id=$2 AND key_type=$3`, newReserved.StringFixed(8), apiKeyID, service.APIKeyTypeLink); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_balance_holds SET status='released',settled_amount=0,settled_at=NOW(),updated_at=NOW() WHERE id=$1`, holdID); err != nil {
		return nil, err
	}
	return &service.MediaBalanceHoldResult{Applied: true}, nil
}

func lockLinkCardForHold(ctx context.Context, tx *sql.Tx, apiKeyID, userID int64) error {
	_, _, err := lockLinkCardForHoldState(ctx, tx, apiKeyID, userID)
	return err
}

func lockLinkCardForHoldState(ctx context.Context, tx *sql.Tx, apiKeyID, userID int64) (string, string, error) {
	var owner int64
	var state, status, keyType string
	err := tx.QueryRowContext(ctx, `
		SELECT user_id,key_type,COALESCE(link_state,''),status
		FROM api_keys WHERE id=$1 AND deleted_at IS NULL FOR UPDATE
	`, apiKeyID).Scan(&owner, &keyType, &state, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", service.ErrAPIKeyNotFound
	}
	if err != nil {
		return "", "", err
	}
	if keyType != service.APIKeyTypeLink {
		return "", "", service.ErrLinkCardOperationNotAllowed
	}
	if owner != userID {
		return "", "", service.ErrLinkCardOperationNotAllowed
	}
	if state == service.LinkCardStateRefunded || state == service.LinkCardStateRevoked || state == service.LinkCardStatePendingActivation || status == service.StatusAPIKeyDisabled && state != service.LinkCardStateFrozen {
		return "", "", service.ErrLinkCardOperationNotAllowed
	}
	return state, status, nil
}

func isActiveLinkCardHoldStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "reserved", "dispatched", "capture_pending":
		return true
	default:
		return false
	}
}

func linkCardHoldAmount(v float64) (decimal.Decimal, bool) {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return decimal.Zero, false
	}
	d := decimal.NewFromFloat(v).Round(8)
	return d, d.IsPositive()
}

// settleExpiredLinkCardHoldsTx releases expired uncertain holds and captures
// holds that already have a durable success marker. It is called lazily while
// the card row is locked; the periodic standard reconciliation path also calls
// this helper for users with expired link holds (see reconciliation patch).
func (r *usageBillingRepository) settleExpiredLinkCardHoldsTx(ctx context.Context, tx *sql.Tx, apiKeyID, userID int64) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id,request_id,hold_amount,capture_amount,status
		FROM media_balance_holds
		WHERE api_key_id=$1 AND user_id=$2 AND funding_source=$3
		  AND status IN ('reserved','dispatched','capture_pending') AND expires_at <= NOW()
		FOR UPDATE
	`, apiKeyID, userID, linkCardHoldFundingSource)
	if err != nil {
		return err
	}
	type expiredHold struct {
		id            int64
		requestID     string
		hold          decimal.Decimal
		capture       decimal.Decimal
		captureValid  bool
		status        string
		actual        decimal.Decimal
		shouldCapture bool
	}
	var expired []expiredHold
	for rows.Next() {
		var item expiredHold
		var capture sql.NullString
		if err := rows.Scan(&item.id, &item.requestID, &item.hold, &capture, &item.status); err != nil {
			_ = rows.Close()
			return err
		}
		if capture.Valid {
			item.capture, _ = decimal.NewFromString(capture.String)
			item.captureValid = true
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}
	var reservedTotal, capturedTotal decimal.Decimal
	for i := range expired {
		item := &expired[i]
		var actual decimal.Decimal
		captureSuccess := item.status == "capture_pending" && item.captureValid
		if !captureSuccess {
			var taskErr error
			actual, captureSuccess, taskErr = r.linkHoldSuccessfulTaskCost(ctx, tx, apiKeyID, item.requestID)
			if taskErr != nil {
				return taskErr
			}
		} else {
			actual = item.capture
		}
		if captureSuccess && (actual.IsNegative() || actual.GreaterThan(item.hold)) {
			return service.ErrMediaBalanceCostExceedsHold
		}
		item.actual = actual
		item.shouldCapture = captureSuccess
		reservedTotal = reservedTotal.Add(item.hold)
		if captureSuccess {
			capturedTotal = capturedTotal.Add(actual)
		}
	}
	var quota, used, reserved decimal.Decimal
	var linkState, apiStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT quota,quota_used,COALESCE(link_reserved_amount,0),COALESCE(link_state,''),status
		FROM api_keys WHERE id=$1 AND key_type=$2 AND user_id=$3 AND deleted_at IS NULL
		FOR UPDATE
	`, apiKeyID, service.APIKeyTypeLink, userID).Scan(&quota, &used, &reserved, &linkState, &apiStatus); err != nil {
		return err
	}
	if reserved.LessThan(reservedTotal) || (quota.IsPositive() && used.Add(capturedTotal).GreaterThan(quota)) {
		return service.ErrMediaBalanceHoldConflict
	}
	runningUsed := used
	for _, item := range expired {
		if item.shouldCapture {
			before := runningUsed
			runningUsed = runningUsed.Add(item.actual)
			if item.actual.IsPositive() {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO link_card_ledger(api_key_id,creator_user_id,entry_type,reserve_delta,creator_balance_delta,
						quota_before,quota_after,quota_used_before,quota_used_after,request_id,reason,metadata)
					VALUES($1,$2,'usage',-($3::numeric),0,$4,$4,$5,$6,$7,'expired media hold capture',
						jsonb_build_object('hold_amount',$8,'actual_cost',$3))
				`, apiKeyID, userID, item.actual.StringFixed(8), quota.StringFixed(8), before.StringFixed(8), runningUsed.StringFixed(8), item.requestID, item.hold.StringFixed(8)); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE media_balance_holds SET status='captured',capture_amount=$1,settled_amount=$1,settled_at=NOW(),updated_at=NOW() WHERE id=$2`, item.actual.StringFixed(8), item.id); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `UPDATE media_balance_holds SET status='released',settled_amount=0,settled_at=NOW(),updated_at=NOW() WHERE id=$1`, item.id); err != nil {
			return err
		}
	}
	newReserved := reserved.Sub(reservedTotal)
	exhausted := quota.IsPositive() && runningUsed.GreaterThanOrEqual(quota)
	if exhausted && linkState != service.LinkCardStateFrozen {
		linkState = service.LinkCardStateDepleted
		apiStatus = service.StatusAPIKeyQuotaExhausted
	}
	updated, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET link_reserved_amount=$1,quota_used=$2,link_state=$3,status=$4,updated_at=NOW()
		WHERE id=$5 AND key_type=$6
	`, newReserved.StringFixed(8), runningUsed.StringFixed(8), linkState, apiStatus, apiKeyID, service.APIKeyTypeLink)
	if err != nil {
		return err
	}
	if affected, err := updated.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return err
		}
		return service.ErrAPIKeyNotFound
	}
	return nil
}

func (r *usageBillingRepository) ReconcileExpiredLinkCardHolds(ctx context.Context, limit int) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	if limit <= 0 {
		return []int64{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT api_key_id,user_id
		FROM media_balance_holds
		WHERE funding_source=$1 AND status IN ('reserved','dispatched','capture_pending')
		  AND expires_at <= NOW()
		ORDER BY api_key_id
		LIMIT $2
	`, linkCardHoldFundingSource, limit)
	if err != nil {
		return nil, err
	}
	type candidate struct{ apiKeyID, userID int64 }
	candidates := make([]candidate, 0, limit)
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.apiKeyID, &c.userID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	userIDs := make([]int64, 0, len(candidates))
	seenUsers := make(map[int64]struct{}, len(candidates))
	for _, c := range candidates {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return userIDs, err
		}
		if err := lockLinkCardForHold(ctx, tx, c.apiKeyID, c.userID); err != nil {
			_ = tx.Rollback()
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				continue
			}
			return userIDs, err
		}
		if err := r.settleExpiredLinkCardHoldsTx(ctx, tx, c.apiKeyID, c.userID); err != nil {
			_ = tx.Rollback()
			return userIDs, err
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return userIDs, err
		}
		if _, ok := seenUsers[c.userID]; !ok {
			seenUsers[c.userID] = struct{}{}
			userIDs = append(userIDs, c.userID)
		}
	}
	return userIDs, nil
}

func (r *usageBillingRepository) linkHoldSuccessfulTaskCost(ctx context.Context, tx *sql.Tx, apiKeyID int64, requestID string) (decimal.Decimal, bool, error) {
	requestID = strings.TrimSpace(requestID)
	if strings.HasPrefix(requestID, "media_balance_hold:") {
		publicID := strings.TrimPrefix(requestID, "media_balance_hold:")
		var status, mediaType string
		var unitRaw, rateRaw sql.NullString
		var requestCount, durationSeconds int
		err := tx.QueryRowContext(ctx, `
			SELECT status,COALESCE(media_type,''),billing_unit_price,billing_rate_multiplier,
			       COALESCE(request_count,1),COALESCE(duration_seconds,0)
			FROM media_generation_tasks WHERE api_key_id=$1 AND public_task_id=$2
		`, apiKeyID, publicID).Scan(&status, &mediaType, &unitRaw, &rateRaw, &requestCount, &durationSeconds)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !service.IsMediaGenerationSuccessStatus(status)) {
			return decimal.Zero, false, nil
		}
		if err != nil {
			return decimal.Zero, false, err
		}
		if !unitRaw.Valid || !rateRaw.Valid {
			return decimal.Zero, false, errors.New("successful media task is missing its pricing snapshot")
		}
		unit, unitErr := decimal.NewFromString(unitRaw.String)
		rate, rateErr := decimal.NewFromString(rateRaw.String)
		if unitErr != nil || rateErr != nil || unit.IsNegative() || rate.IsNegative() {
			return decimal.Zero, false, errors.New("successful media task has an invalid pricing snapshot")
		}
		if requestCount <= 0 {
			requestCount = 1
		}
		units := decimal.NewFromInt(int64(requestCount))
		if strings.EqualFold(strings.TrimSpace(mediaType), "video") {
			if durationSeconds <= 0 {
				durationSeconds = 1
			}
			units = units.Mul(decimal.NewFromInt(int64(durationSeconds)))
		}
		return unit.Mul(rate).Mul(units).Round(8), true, nil
	}
	if strings.HasPrefix(requestID, "batch_image_hold:") {
		batchID := strings.TrimPrefix(requestID, "batch_image_hold:")
		var status string
		var actualRaw sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT status,actual_cost FROM batch_image_jobs WHERE api_key_id=$1 AND batch_id=$2`, apiKeyID, batchID).Scan(&status, &actualRaw)
		if errors.Is(err, sql.ErrNoRows) {
			return decimal.Zero, false, nil
		}
		if err != nil {
			return decimal.Zero, false, err
		}
		if !strings.EqualFold(status, service.BatchImageJobStatusCompleted) && !strings.EqualFold(status, service.BatchImageJobStatusOutputDeleted) {
			return decimal.Zero, false, nil
		}
		if !actualRaw.Valid {
			return decimal.Zero, false, errors.New("completed batch-image task is missing actual cost")
		}
		actual, parseErr := decimal.NewFromString(actualRaw.String)
		if parseErr != nil || actual.IsNegative() {
			return decimal.Zero, false, errors.New("completed batch-image task has an invalid actual cost")
		}
		return actual.Round(8), true, nil
	}
	return decimal.Zero, false, nil
}
