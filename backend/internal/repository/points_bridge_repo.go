package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type pointsBridgeRepository struct {
	db *sql.DB
}

func NewPointsBridgeRepository(db *sql.DB) service.PointsBridgeRepository {
	return &pointsBridgeRepository{db: db}
}

func (r *pointsBridgeRepository) ApplyPointsBalanceCredit(ctx context.Context, input service.PointsBalanceCreditInput) (*service.PointsBalanceCreditResult, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("points bridge repository is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	insertResult, err := tx.ExecContext(ctx, `
		INSERT INTO points_balance_credits (
			transaction_id, user_id, amount, kind, source_reference, reason,
			request_nonce, payload_hash, request_id, created_at
		) VALUES ($1, $2, $3::numeric, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (transaction_id) DO NOTHING
	`, input.TransactionID, input.UserID, input.Amount.StringFixed(2), input.Kind,
		input.SourceReference, input.Reason, input.Nonce, input.PayloadHash, input.RequestID)
	if err != nil {
		return nil, err
	}
	inserted, err := insertResult.RowsAffected()
	if err != nil {
		return nil, err
	}
	if inserted == 0 {
		result, err := readExistingPointsCredit(ctx, tx, input)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}

	var balanceAfterRaw string
	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = COALESCE(balance, 0) + $1::numeric, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING balance::text
	`, input.Amount.StringFixed(2), input.UserID).Scan(&balanceAfterRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPointsCreditUserMissing
	}
	if err != nil {
		return nil, err
	}
	balanceAfter, err := decimal.NewFromString(balanceAfterRaw)
	if err != nil {
		return nil, fmt.Errorf("parse points credit balance: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE points_balance_credits
		SET balance_after = $2::numeric, applied_at = NOW()
		WHERE transaction_id = $1
	`, input.TransactionID, balanceAfter.String()); err != nil {
		return nil, err
	}
	if err := insertPointsCreditAuditLog(ctx, tx, input, balanceAfter); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &service.PointsBalanceCreditResult{
		TransactionID: input.TransactionID,
		BalanceAfter:  balanceAfter,
	}, nil
}

func readExistingPointsCredit(ctx context.Context, tx *sql.Tx, input service.PointsBalanceCreditInput) (*service.PointsBalanceCreditResult, error) {
	var (
		userID          int64
		amountRaw       string
		kind            string
		sourceReference string
		payloadHash     string
		balanceAfterRaw sql.NullString
	)
	err := tx.QueryRowContext(ctx, `
		SELECT user_id, amount::text, kind, source_reference, payload_hash, balance_after::text
		FROM points_balance_credits
		WHERE transaction_id = $1
	`, input.TransactionID).Scan(&userID, &amountRaw, &kind, &sourceReference, &payloadHash, &balanceAfterRaw)
	if err != nil {
		return nil, err
	}
	amount, err := decimal.NewFromString(amountRaw)
	if err != nil {
		return nil, err
	}
	if userID != input.UserID || !amount.Equal(input.Amount) || kind != input.Kind ||
		sourceReference != input.SourceReference || payloadHash != input.PayloadHash || !balanceAfterRaw.Valid {
		return nil, service.ErrPointsCreditConflict
	}
	balanceAfter, err := decimal.NewFromString(balanceAfterRaw.String)
	if err != nil {
		return nil, err
	}
	return &service.PointsBalanceCreditResult{
		TransactionID: input.TransactionID,
		BalanceAfter:  balanceAfter,
		Idempotent:    true,
	}, nil
}

func insertPointsCreditAuditLog(ctx context.Context, tx *sql.Tx, input service.PointsBalanceCreditInput, balanceAfter decimal.Decimal) error {
	extra, err := json.Marshal(map[string]any{
		"transaction_id":   input.TransactionID.String(),
		"user_id":          input.UserID,
		"amount":           input.Amount.StringFixed(2),
		"kind":             input.Kind,
		"source_reference": input.SourceReference,
		"balance_after":    balanceAfter.String(),
	})
	if err != nil {
		return err
	}
	requestID := strings.TrimSpace(input.RequestID)
	if len(requestID) > 64 {
		requestID = requestID[:64]
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
			created_at, actor_user_id, actor_email, actor_role, auth_method,
			credential_masked, action, method, path, request_id, client_ip,
			user_agent, request_body, status_code, latency_ms, extra
		) VALUES ($1, NULL, '', 'points_system', 'hmac', '', 'points.balance_credit',
			'POST', '/api/internal/points/credits', $2, '', '', $3, 200, 0, $4::jsonb)
	`, time.Now().UTC(), requestID, "", string(extra))
	return err
}

var _ service.PointsBridgeRepository = (*pointsBridgeRepository)(nil)
var _ = uuid.Nil
