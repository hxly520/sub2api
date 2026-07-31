package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/domain"
)

// ListCheckinBalanceGrants returns one user's check-in grants while excluding
// the retired manual grant type.
func (s *Store) ListCheckinBalanceGrants(ctx context.Context, userID int64, limit int) ([]BalanceGrant, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := balanceGrantSelect + ` FROM points_balance_grants
		WHERE kind='checkin' AND user_id=$1 ORDER BY created_at DESC LIMIT $2`
	rows, err := s.DB.Query(ctx, query, userID, limit)
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

type AdminCheckinBalanceGrant struct {
	Grant    BalanceGrant
	Username string
}

// ListAdminCheckinBalanceGrants joins the public username for display while
// retaining the numeric account key only inside the accounting projection.
func (s *Store) ListAdminCheckinBalanceGrants(ctx context.Context, limit int) ([]AdminCheckinBalanceGrant, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.Query(ctx, `SELECT grant_row.id,grant_row.user_id,grant_row.amount_microusd,
		grant_row.kind,grant_row.status,grant_row.external_event_id,grant_row.policy_version,
		grant_row.attempts,grant_row.next_attempt_at,COALESCE(grant_row.last_error,''),
		grant_row.reason,grant_row.created_at,grant_row.updated_at,COALESCE(site_user.username,'')
		FROM points_balance_grants grant_row
		LEFT JOIN users site_user ON site_user.id=grant_row.user_id
		WHERE grant_row.kind='checkin'
		ORDER BY grant_row.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AdminCheckinBalanceGrant, 0, limit)
	for rows.Next() {
		var item AdminCheckinBalanceGrant
		targets := append(balanceGrantScanTargets(&item.Grant), &item.Username)
		if err := rows.Scan(targets...); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// SummarizeCheckinBalanceGrants returns full-table counts for the administrator
// overview without mixing retired manual-grant records into check-in operations.
func (s *Store) SummarizeCheckinBalanceGrants(ctx context.Context) (map[string]int64, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT status, COUNT(*)
		FROM points_balance_grants
		WHERE kind='checkin'
		GROUP BY status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
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
