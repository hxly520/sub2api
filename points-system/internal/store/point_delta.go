package store

import (
	"context"
	"errors"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/jackc/pgx/v5"
)

func claimIdempotency(ctx context.Context, tx pgx.Tx, scope, eventID, fingerprint string) (bool, error) {
	var inserted string
	err := tx.QueryRow(ctx, `INSERT INTO points_idempotency(scope,external_event_id,request_fingerprint)
		VALUES($1,$2,$3) ON CONFLICT(scope,external_event_id) DO NOTHING RETURNING external_event_id`,
		scope, eventID, fingerprint).Scan(&inserted)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var existing string
	if err := tx.QueryRow(ctx, `SELECT request_fingerprint FROM points_idempotency
		WHERE scope=$1 AND external_event_id=$2`, scope, eventID).Scan(&existing); err != nil {
		return false, err
	}
	if existing != fingerprint {
		return false, domain.ErrIdempotencyConflict
	}
	return false, nil
}

func min64(values ...int64) int64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
