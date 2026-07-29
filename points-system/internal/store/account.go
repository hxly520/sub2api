package store

import (
	"context"
	"encoding/json"

	"github.com/hxly520/sub2api/points-system/internal/domain"
)

func (s *Store) Account(ctx context.Context, userID int64) (domain.Account, error) {
	_, _ = s.DB.Exec(ctx, `INSERT INTO points_accounts(user_id) VALUES($1) ON CONFLICT(user_id) DO NOTHING`, userID)
	var account domain.Account
	err := s.DB.QueryRow(ctx, `SELECT a.user_id,a.total_points_hundredths,a.total_spend_microusd,
		COALESCE((SELECT SUM(g.amount_microusd) FROM points_balance_grants g
			WHERE g.user_id=a.user_id AND g.kind='checkin'
				AND g.settled_at IS NOT NULL AND g.reversed_at IS NULL),0)::bigint,
		a.created_at,a.updated_at
		FROM points_accounts a WHERE a.user_id=$1`, userID).Scan(&account.UserID,
		&account.TotalPointsHundredths, &account.TotalSpendMicroUSD,
		&account.SettledCheckinRewardMicroUSD, &account.CreatedAt, &account.UpdatedAt)
	return account, translateNotFound(err)
}

func (s *Store) Ledger(ctx context.Context, userID int64, limit int) ([]domain.LedgerEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.DB.Query(ctx, `SELECT id,user_id,kind,delta_points_hundredths,total_after_hundredths,
		source,external_event_id,policy_version,business_date,reference_id,metadata,created_at
		FROM points_ledger WHERE user_id=$1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.LedgerEntry
	for rows.Next() {
		var entry domain.LedgerEntry
		var metadata []byte
		if err := rows.Scan(&entry.ID, &entry.UserID, &entry.Kind, &entry.DeltaPointsHundredths,
			&entry.TotalAfterHundredths, &entry.Source, &entry.ExternalEventID, &entry.PolicyVersion,
			&entry.BusinessDate, &entry.ReferenceID, &metadata, &entry.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &entry.Metadata)
		result = append(result, entry)
	}
	return result, rows.Err()
}
