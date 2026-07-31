package store

import (
	"context"
	"time"
)

const (
	defaultAdminUserPointsLimit = 50
	maxAdminUserPointsLimit     = 200
)

// AdminUserPoints is the deliberately small administrator projection used by
// the points user directory. Accounting internals stay behind the store API.
type AdminUserPoints struct {
	UserID                    int64
	LoginEmail                string
	TotalPointsHundredths     int64
	YesterdayPointsHundredths int64
	TotalSpendMicroUSD        int64
	YesterdaySpendMicroUSD    int64
	SnapshotBusinessDate      *time.Time
	SnapshotStatus            string
}

type AdminUserPointsPage struct {
	Items        []AdminUserPoints
	Total        int64
	Limit        int
	Offset       int
	BusinessDate time.Time
}

func (s *Store) ListAdminUserPoints(ctx context.Context, businessDate time.Time, limit, offset int) (AdminUserPointsPage, error) {
	if limit <= 0 || limit > maxAdminUserPointsLimit {
		limit = defaultAdminUserPointsLimit
	}
	if offset < 0 {
		offset = 0
	}
	businessDate = s.BusinessDate(businessDate)
	page := AdminUserPointsPage{
		Items:        make([]AdminUserPoints, 0, limit),
		Limit:        limit,
		Offset:       offset,
		BusinessDate: businessDate,
	}
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*)::bigint FROM users
		WHERE deleted_at IS NULL`).Scan(&page.Total); err != nil {
		return AdminUserPointsPage{}, err
	}
	rows, err := s.DB.Query(ctx, `SELECT site_user.id,COALESCE(site_user.email,''),
		COALESCE(account.total_points_hundredths,0)::bigint,
		COALESCE(snapshot.awarded_points_hundredths,0)::bigint,
		COALESCE(account.total_spend_microusd,0)::bigint,
		COALESCE(snapshot.actual_cost_microusd,0)::bigint,snapshot.business_date,snapshot.status
		FROM users site_user
		LEFT JOIN points_accounts account ON account.user_id=site_user.id
		LEFT JOIN points_daily_snapshots snapshot
			ON snapshot.user_id=site_user.id AND snapshot.business_date=$1
		WHERE site_user.deleted_at IS NULL
		ORDER BY COALESCE(account.total_points_hundredths,0) DESC,site_user.id ASC
		LIMIT $2 OFFSET $3`, dateString(businessDate), limit, offset)
	if err != nil {
		return AdminUserPointsPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item AdminUserPoints
		var snapshotStatus *string
		if err := rows.Scan(&item.UserID, &item.LoginEmail, &item.TotalPointsHundredths,
			&item.YesterdayPointsHundredths, &item.TotalSpendMicroUSD,
			&item.YesterdaySpendMicroUSD, &item.SnapshotBusinessDate, &snapshotStatus); err != nil {
			return AdminUserPointsPage{}, err
		}
		item.SnapshotStatus = "missing"
		if snapshotStatus != nil {
			item.SnapshotStatus = *snapshotStatus
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return AdminUserPointsPage{}, err
	}
	return page, nil
}
