//go:build integration

package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/security"
	pointsstore "github.com/hxly520/sub2api/points-system/internal/store"
)

func TestPostgresAdminUserPointsArePagedAndJoinedToYesterday(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	yesterday := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -1)
	if _, err := fixture.db.Exec(ctx, `CREATE TABLE users(
		id BIGINT PRIMARY KEY,username TEXT NOT NULL DEFAULT '',deleted_at TIMESTAMPTZ
	)`); err != nil {
		t.Fatalf("create isolated user directory: %v", err)
	}
	for _, row := range []struct {
		userID, points, spend int64
		username              string
	}{
		{userID: 101, username: "alice", points: 100, spend: 1_000_000},
		{userID: 202, username: "bob", points: 900, spend: 9_000_000},
		{userID: 303, username: "carol", points: 500, spend: 5_000_000},
	} {
		if _, err := fixture.db.Exec(ctx, `INSERT INTO users(id,username) VALUES($1,$2)`, row.userID, row.username); err != nil {
			t.Fatalf("insert user %d: %v", row.userID, err)
		}
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_accounts(
			user_id,total_points_hundredths,total_spend_microusd) VALUES($1,$2,$3)`,
			row.userID, row.points, row.spend); err != nil {
			t.Fatalf("insert points account %d: %v", row.userID, err)
		}
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO users(id,username) VALUES(404,'zero')`); err != nil {
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
	if page.Items[0].UserID != 202 || page.Items[0].Username != "bob" || page.Items[0].YesterdayPointsHundredths != 200 ||
		page.Items[0].YesterdaySpendMicroUSD != 2_000_000 || page.Items[0].SnapshotStatus != "ready" ||
		page.Items[0].SnapshotBusinessDate == nil {
		t.Fatalf("unexpected joined user points: %+v", page.Items[0])
	}
	if page.Items[1].UserID != 303 || page.Items[1].Username != "carol" || page.Items[1].SnapshotStatus != "missing" ||
		page.Items[1].SnapshotBusinessDate != nil {
		t.Fatalf("unexpected missing snapshot projection: %+v", page.Items[1])
	}

	last, err := fixture.store.ListAdminUserPoints(ctx, yesterday, 2, 2)
	if err != nil {
		t.Fatalf("list final administrator user points page: %v", err)
	}
	if last.Total != 4 || len(last.Items) != 2 || last.Items[0].UserID != 101 || last.Items[0].Username != "alice" ||
		last.Items[1].UserID != 404 || last.Items[1].Username != "zero" || last.Items[1].TotalPointsHundredths != 0 ||
		last.Items[1].TotalSpendMicroUSD != 0 {
		t.Fatalf("unexpected final page: %+v", last)
	}

	username, err := fixture.store.Username(ctx, 202)
	if err != nil || username != "bob" {
		t.Fatalf("lookup username = %q, err=%v", username, err)
	}
	const activeSessionToken = "active-user-session"
	activeClaims := security.LaunchClaims{Subject: 202, Role: "user", Theme: "light", Language: "zh-CN",
		Nonce: "active-user-ticket", ExpiresAt: fixture.now.Add(time.Minute).Unix()}
	if _, err := fixture.store.ConsumeLaunchTicket(ctx, activeClaims, activeSessionToken, time.Hour, fixture.now); err != nil {
		t.Fatalf("consume active user launch ticket: %v", err)
	}
	if _, err := fixture.store.Session(ctx, activeSessionToken, fixture.now); err != nil {
		t.Fatalf("active user session was rejected: %v", err)
	}

	var policyVersion int64
	if err := fixture.db.QueryRow(ctx, `INSERT INTO points_policy_versions(
		effective_date,enabled,created_by) VALUES($1,FALSE,1) RETURNING version_no`,
		yesterday.Format("2006-01-02")).Scan(&policyVersion); err != nil {
		t.Fatalf("insert grant policy: %v", err)
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_balance_grants(
		id,user_id,amount_microusd,kind,status,external_event_id,request_fingerprint,policy_version
	) VALUES($1,202,100000,'checkin','settled',$2,$3,$4)`, uuid.New(), uuid.NewString(),
		strings.Repeat("c", 64), policyVersion); err != nil {
		t.Fatalf("insert check-in balance grant: %v", err)
	}
	grants, err := fixture.store.ListAdminCheckinBalanceGrants(ctx, 10)
	if err != nil {
		t.Fatalf("list administrator check-in balance grants: %v", err)
	}
	if len(grants) != 1 || grants[0].Username != "bob" || grants[0].Grant.UserID != 202 {
		t.Fatalf("unexpected administrator grant projection: %+v", grants)
	}
	if _, err := fixture.db.Exec(ctx, `UPDATE users SET deleted_at=$1 WHERE id=202`, fixture.now); err != nil {
		t.Fatalf("soft delete user: %v", err)
	}
	if _, err := fixture.store.Username(ctx, 202); err == nil {
		t.Fatal("soft-deleted user still resolved to a public username")
	}
	var softDeletedSession pointsstore.Session
	err = fixture.db.QueryRow(ctx, `SELECT user_id,role,theme,language,expires_at FROM points_sessions
		WHERE token_hash=$1`, security.HashToken(activeSessionToken)).Scan(&softDeletedSession.UserID, &softDeletedSession.Role,
		&softDeletedSession.Theme, &softDeletedSession.Language, &softDeletedSession.ExpiresAt)
	if err != nil {
		t.Fatalf("soft-deleted user's stored session disappeared unexpectedly: %v", err)
	}
	if _, err := fixture.store.Session(ctx, activeSessionToken, fixture.now); err == nil {
		t.Fatal("soft-deleted user's session remained usable")
	}
	deletedClaims := activeClaims
	deletedClaims.Nonce = "soft-deleted-user-ticket"
	if _, err := fixture.store.ConsumeLaunchTicket(ctx, deletedClaims, "deleted-user-session", time.Hour, fixture.now); !errors.Is(err, security.ErrInvalidTicket) {
		t.Fatalf("soft-deleted user launch error = %v, want invalid ticket", err)
	}
	var deletedTicketRows int
	if err := fixture.db.QueryRow(ctx, `SELECT COUNT(*) FROM points_launch_ticket_nonces
		WHERE jti_hash=$1`, security.HashToken(deletedClaims.Nonce)).Scan(&deletedTicketRows); err != nil {
		t.Fatalf("count rejected deleted-user ticket: %v", err)
	}
	if deletedTicketRows != 0 {
		t.Fatalf("rejected deleted-user ticket persisted %d nonce rows", deletedTicketRows)
	}
}
