package store

import (
	"context"
	"encoding/json"
	"time"

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
	return s.LedgerPage(ctx, userID, limit, nil)
}

func (s *Store) LedgerPage(ctx context.Context, userID int64, limit int, beforeID *int64) ([]domain.LedgerEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT ledger.id,ledger.user_id,ledger.kind,
		ledger.delta_points_hundredths,ledger.total_after_hundredths,ledger.source,
		ledger.external_event_id,ledger.policy_version,ledger.business_date,
		ledger.reference_id,ledger.metadata,ledger.created_at,
		COALESCE(schedule_policy.refresh_minute,ledger_policy.refresh_minute)
		FROM points_ledger ledger
		LEFT JOIN points_policy_versions ledger_policy
			ON ledger_policy.version_no=ledger.policy_version
		LEFT JOIN LATERAL (
			SELECT policy.refresh_minute
			FROM points_policy_versions policy
			WHERE policy.effective_date <= ledger.business_date + 1
			ORDER BY policy.effective_date DESC,policy.version_no DESC
			LIMIT 1
		) schedule_policy ON ledger.business_date IS NOT NULL
		WHERE ledger.user_id=$1`
	args := []any{userID, limit}
	if beforeID != nil {
		query += ` AND ledger.id<$3`
		args = append(args, *beforeID)
	}
	query += ` ORDER BY ledger.id DESC LIMIT $2`
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.LedgerEntry
	for rows.Next() {
		var entry domain.LedgerEntry
		var metadata []byte
		var refreshMinute *int
		if err := rows.Scan(&entry.ID, &entry.UserID, &entry.Kind, &entry.DeltaPointsHundredths,
			&entry.TotalAfterHundredths, &entry.Source, &entry.ExternalEventID, &entry.PolicyVersion,
			&entry.BusinessDate, &entry.ReferenceID, &metadata, &entry.CreatedAt, &refreshMinute); err != nil {
			return nil, err
		}
		entry.AwardedAt = ledgerAwardedAt(entry.Kind, entry.BusinessDate, refreshMinute,
			entry.CreatedAt, s.Location)
		_ = json.Unmarshal(metadata, &entry.Metadata)
		result = append(result, entry)
	}
	return result, rows.Err()
}

func ledgerAwardedAt(kind string, businessDate *time.Time, refreshMinute *int,
	createdAt time.Time, location *time.Location) time.Time {
	if kind != "usage_points" || businessDate == nil || refreshMinute == nil ||
		*refreshMinute < 0 || *refreshMinute >= 24*60 {
		return createdAt
	}
	if location == nil {
		location = time.UTC
	}
	year, month, day := businessDate.Date()
	return time.Date(year, month, day+1, 0, *refreshMinute, 0, 0, location)
}
