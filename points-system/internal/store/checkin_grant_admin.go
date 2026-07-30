package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/domain"
)

// ListCheckinBalanceGrants excludes the retired points-system manual grant
// type while preserving historical rows for accounting and audit purposes.
func (s *Store) ListCheckinBalanceGrants(ctx context.Context, userID int64, admin bool, limit int) ([]BalanceGrant, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := balanceGrantSelect + ` FROM points_balance_grants WHERE kind='checkin'`
	args := []any{}
	if !admin {
		query += ` AND user_id=$1`
		args = append(args, userID)
	}
	query += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprint(len(args)+1)
	args = append(args, limit)
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BalanceGrant, 0, limit)
	for rows.Next() {
		var item BalanceGrant
		if err := scanBalanceGrant(rows, &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// RequireCheckinBalanceGrant prevents administrator retry and reversal routes
// from operating on legacy manual-grant rows even when an ID is supplied.
func (s *Store) RequireCheckinBalanceGrant(ctx context.Context, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return domain.ErrNotFound
	}
	var exists bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM points_balance_grants WHERE id=$1 AND kind='checkin'
	)`, id).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrNotFound
	}
	return nil
}
