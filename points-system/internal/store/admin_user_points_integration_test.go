//go:build integration

package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresAdminUserPointsArePagedAndJoinedToYesterday(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	yesterday := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -1)
	if _, err := fixture.db.Exec(ctx, `CREATE TABLE users(
		id BIGINT PRIMARY KEY,deleted_at TIMESTAMPTZ
	)`); err != nil {
		t.Fatalf("create isolated user directory: %v", err)
	}
	for _, row := range []struct {
		userID, points, spend int64
	}{
		{userID: 101, points: 100, spend: 1_000_000},
		{userID: 202, points: 900, spend: 9_000_000},
		{userID: 303, points: 500, spend: 5_000_000},
	} {
		if _, err := fixture.db.Exec(ctx, `INSERT INTO users(id) VALUES($1)`, row.userID); err != nil {
			t.Fatalf("insert user %d: %v", row.userID, err)
		}
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_accounts(
			user_id,total_points_hundredths,total_spend_microusd) VALUES($1,$2,$3)`,
			row.userID, row.points, row.spend); err != nil {
			t.Fatalf("insert points account %d: %v", row.userID, err)
		}
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO users(id) VALUES(404)`); err != nil {
		t.Fatalf("insert zero-points user: %v", err)
	}

	runID := uuid.New()
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_snapshot_refresh_runs(
		id,business_date,trigger,source_window_start,source_window_end,source_fingerprint,
		source_users,source_rows,changed_users,delta_spend_microusd,delta_points_hundredths,
		status,completed_at) VALUES($1,$2,'scheduled',$3,$4,$5,1,1,1,2000000,200,'succeeded',$4)`,
		runID, yesterday.Format("2006-01-02"), yesterday, yesterday.AddDate(0, 0, 1),
		strings.Repeat("a", 64)); err != nil {
		t.Fatalf("insert snapshot refresh run: %v", err)
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_daily_snapshots(
		user_id,business_date,actual_cost_microusd,accounted_spend_microusd,
		points_per_usd_hundredths,target_points_hundredths,awarded_points_hundredths,
		status,source_row_count,source_max_usage_log_id,source_fingerprint,last_refresh_run_id)
		VALUES(202,$1,2000000,2000000,1000,200,200,'ready',1,1,$2,$3)`,
		yesterday.Format("2006-01-02"), strings.Repeat("b", 64), runID); err != nil {
		t.Fatalf("insert yesterday snapshot: %v", err)
	}

	page, err := fixture.store.ListAdminUserPoints(ctx, yesterday, 2, 0)
	if err != nil {
		t.Fatalf("list first administrator user points page: %v", err)
	}
	if page.Total != 4 || page.Limit != 2 || page.Offset != 0 || len(page.Items) != 2 {
		t.Fatalf("unexpected first page: %+v", page)
	}
	if page.Items[0].UserID != 202 || page.Items[0].YesterdayPointsHundredths != 200 ||
		page.Items[0].YesterdaySpendMicroUSD != 2_000_000 || page.Items[0].SnapshotStatus != "ready" ||
		page.Items[0].SnapshotBusinessDate == nil {
		t.Fatalf("unexpected joined user points: %+v", page.Items[0])
	}
	if page.Items[1].UserID != 303 || page.Items[1].SnapshotStatus != "missing" ||
		page.Items[1].SnapshotBusinessDate != nil {
		t.Fatalf("unexpected missing snapshot projection: %+v", page.Items[1])
	}

	last, err := fixture.store.ListAdminUserPoints(ctx, yesterday, 2, 2)
	if err != nil {
		t.Fatalf("list final administrator user points page: %v", err)
	}
	if last.Total != 4 || len(last.Items) != 2 || last.Items[0].UserID != 101 ||
		last.Items[1].UserID != 404 || last.Items[1].TotalPointsHundredths != 0 ||
		last.Items[1].TotalSpendMicroUSD != 0 {
		t.Fatalf("unexpected final page: %+v", last)
	}
}
