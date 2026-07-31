//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/migrate"
	pointsstore "github.com/hxly520/sub2api/points-system/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pointsTestDatabaseEnv = "POINTS_TEST_DATABASE_URL"

type postgresFixture struct {
	db       *pgxpool.Pool
	store    *pointsstore.Store
	location *time.Location
	now      time.Time
}

func newPostgresFixture(t *testing.T) *postgresFixture {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv(pointsTestDatabaseEnv))
	if databaseURL == "" {
		t.Skip(pointsTestDatabaseEnv + " is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	schema := "points_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		admin.Close()
		t.Fatalf("create integration schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
		t.Fatalf("parse PostgreSQL URL: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	config.MaxConns = 32
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
		t.Fatalf("open schema-scoped PostgreSQL pool: %v", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
		t.Fatalf("ping schema-scoped PostgreSQL pool: %v", err)
	}
	if err := migrate.Run(ctx, db); err != nil {
		db.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
		t.Fatalf("run points migrations: %v", err)
	}

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		db.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		admin.Close()
		t.Fatalf("load points timezone: %v", err)
	}
	fixture := &postgresFixture{
		db: db, store: pointsstore.New(db, location), location: location,
		now: time.Now().In(location).Truncate(time.Second),
	}
	t.Cleanup(func() {
		db.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop integration schema: %v", err)
		}
		admin.Close()
	})
	return fixture
}

func TestPostgresMigrationsApplyAndRemainIdempotent(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrate.Run(ctx, fixture.db); err != nil {
		t.Fatalf("rerun points migrations: %v", err)
	}
	for _, filename := range []string{
		"migrations/001_init.sql",
		"migrations/002_balance_grant_outbox.sql",
		"migrations/003_usage_history_backfill.sql",
	} {
		var applied bool
		if err := fixture.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM points_schema_migrations WHERE filename=$1)`, filename,
		).Scan(&applied); err != nil {
			t.Fatalf("query migration %s: %v", filename, err)
		}
		if !applied {
			t.Fatalf("migration %s was not recorded", filename)
		}
	}
	var balanceGrantsExists bool
	if err := fixture.db.QueryRow(ctx,
		`SELECT to_regclass('points_balance_grants') IS NOT NULL`,
	).Scan(&balanceGrantsExists); err != nil {
		t.Fatal(err)
	}
	if !balanceGrantsExists {
		t.Fatal("points_balance_grants was not created")
	}
}

func TestPostgresLedgerAwardedAtUsesBusinessDateWhenCreatedAtMatches(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	userID := int64(9201)
	createdAt := fixture.now.UTC()
	firstBusinessDate := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -2)
	secondBusinessDate := firstBusinessDate.AddDate(0, 0, 1)

	var policyVersion int64
	if err := fixture.db.QueryRow(ctx, `INSERT INTO points_policy_versions(
		effective_date,enabled,refresh_minute,created_by
	) VALUES($1,FALSE,5,$2) RETURNING version_no`,
		firstBusinessDate.Format("2006-01-02"), userID).Scan(&policyVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_policy_versions(
		effective_date,enabled,refresh_minute,created_by
	) VALUES($1,FALSE,65,$2)`,
		secondBusinessDate.AddDate(0, 0, 1).Format("2006-01-02"), userID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_accounts(user_id) VALUES($1)`, userID); err != nil {
		t.Fatal(err)
	}
	for index, businessDate := range []time.Time{firstBusinessDate, secondBusinessDate} {
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_ledger(
			user_id,kind,delta_points_hundredths,total_after_hundredths,source,
			external_event_id,request_fingerprint,policy_version,business_date,created_at
		) VALUES($1,'usage_points',100,$2,'usage_snapshot',$3,$4,$5,$6,$7)`,
			userID, int64((index+1)*100), fmt.Sprintf("same-created-at-%d", index),
			strings.Repeat(string(rune('a'+index)), 64), policyVersion,
			businessDate.Format("2006-01-02"), createdAt); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := fixture.store.Ledger(ctx, userID, 10)
	if err != nil {
		t.Fatalf("list ledger: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ledger entries = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if !entry.CreatedAt.Equal(createdAt) || entry.BusinessDate == nil {
			t.Fatalf("unexpected ledger timestamps: %+v", entry)
		}
		year, month, day := entry.BusinessDate.Date()
		refreshMinute := 5
		if entry.BusinessDate.Format("2006-01-02") == secondBusinessDate.Format("2006-01-02") {
			refreshMinute = 65
		}
		expected := time.Date(year, month, day+1, 0, refreshMinute, 0, 0, fixture.location)
		if !entry.AwardedAt.Equal(expected) {
			t.Fatalf("business date %s awarded_at = %s, want %s",
				entry.BusinessDate.Format("2006-01-02"), entry.AwardedAt, expected)
		}
	}
	if entries[0].AwardedAt.Equal(entries[1].AwardedAt) {
		t.Fatalf("different business dates shared awarded_at %s", entries[0].AwardedAt)
	}
	firstPage, err := fixture.store.LedgerPage(ctx, userID, 1, nil)
	if err != nil || len(firstPage) != 1 {
		t.Fatalf("first ledger page = %+v, err=%v", firstPage, err)
	}
	beforeID := firstPage[0].ID
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_ledger(
		user_id,kind,delta_points_hundredths,total_after_hundredths,source,
		external_event_id,request_fingerprint,policy_version,business_date,created_at
	) VALUES($1,'usage_points',100,300,'usage_snapshot',$2,$3,$4,$5,$6)`,
		userID, "inserted-between-pages", strings.Repeat("c", 64), policyVersion,
		secondBusinessDate.Format("2006-01-02"), createdAt); err != nil {
		t.Fatal(err)
	}
	secondPage, err := fixture.store.LedgerPage(ctx, userID, 1, &beforeID)
	if err != nil || len(secondPage) != 1 {
		t.Fatalf("second ledger page = %+v, err=%v", secondPage, err)
	}
	if secondPage[0].ID >= beforeID {
		t.Fatalf("ledger cursor returned duplicate or newly inserted row: before=%d got=%d",
			beforeID, secondPage[0].ID)
	}
}

func TestPostgresLedgerAwardedAtUsesLedgerPolicyBeforePolicyEffectiveDate(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	const userID int64 = 9203
	createdAt := fixture.now.UTC()
	effectiveDate := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -1)
	historicalDate := effectiveDate.AddDate(0, 0, -14)

	var policyVersion int64
	if err := fixture.db.QueryRow(ctx, `INSERT INTO points_policy_versions(
		effective_date,enabled,refresh_minute,created_by
	) VALUES($1,FALSE,5,$2) RETURNING version_no`,
		effectiveDate.Format("2006-01-02"), userID).Scan(&policyVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_accounts(user_id) VALUES($1)`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_ledger(
		user_id,kind,delta_points_hundredths,total_after_hundredths,source,
		external_event_id,request_fingerprint,policy_version,business_date,created_at
	) VALUES($1,'usage_points',100,100,'usage_snapshot',$2,$3,$4,$5,$6)`,
		userID, "historical-policy-fallback", strings.Repeat("d", 64), policyVersion,
		historicalDate.Format("2006-01-02"), createdAt); err != nil {
		t.Fatal(err)
	}

	entries, err := fixture.store.Ledger(ctx, userID, 10)
	if err != nil {
		t.Fatalf("list historical ledger: %v", err)
	}
	if len(entries) != 1 || entries[0].BusinessDate == nil {
		t.Fatalf("historical ledger entries = %+v", entries)
	}
	year, month, day := entries[0].BusinessDate.Date()
	expected := time.Date(year, month, day+1, 0, 5, 0, 0, fixture.location)
	if !entries[0].AwardedAt.Equal(expected) {
		t.Fatalf("historical business date %s awarded_at = %s, want %s",
			entries[0].BusinessDate.Format("2006-01-02"), entries[0].AwardedAt, expected)
	}
}

func TestPostgresCheckinGrantCursorStaysStableAcrossInsert(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	const userID int64 = 9202
	createdAt := fixture.now.UTC().Truncate(time.Microsecond)

	var policyVersion int64
	if err := fixture.db.QueryRow(ctx, `INSERT INTO points_policy_versions(
		effective_date,enabled,created_by
	) VALUES($1,FALSE,$2) RETURNING version_no`,
		fixture.store.BusinessDate(fixture.now).Format("2006-01-02"), userID).Scan(&policyVersion); err != nil {
		t.Fatal(err)
	}
	insertGrant := func(id, eventID, fingerprint string) {
		t.Helper()
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_balance_grants(
			id,user_id,amount_microusd,kind,status,external_event_id,
			request_fingerprint,policy_version,created_at,updated_at
		) VALUES($1,$2,10000,'checkin','settled',$3,$4,$5,$6,$6)`,
			id, userID, eventID, fingerprint, policyVersion, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	firstID := "00000000-0000-0000-0000-000000000001"
	secondID := "00000000-0000-0000-0000-000000000002"
	insertGrant(firstID, "grant-cursor-first", strings.Repeat("d", 64))
	insertGrant(secondID, "grant-cursor-second", strings.Repeat("e", 64))

	firstPage, err := fixture.store.ListCheckinBalanceGrantsPage(ctx, userID, 1, nil)
	if err != nil || len(firstPage) != 1 || firstPage[0].ID != secondID {
		t.Fatalf("first grant page = %+v, err=%v", firstPage, err)
	}
	cursor := &pointsstore.BalanceGrantPageCursor{CreatedAt: firstPage[0].CreatedAt, ID: firstPage[0].ID}
	insertGrant("00000000-0000-0000-0000-000000000003", "grant-cursor-inserted", strings.Repeat("f", 64))

	secondPage, err := fixture.store.ListCheckinBalanceGrantsPage(ctx, userID, 1, cursor)
	if err != nil || len(secondPage) != 1 || secondPage[0].ID != firstID {
		t.Fatalf("second grant page = %+v, err=%v", secondPage, err)
	}
}

func TestPostgresDisabledPolicyMarksSnapshotReadyWithoutUsageScan(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	yesterday := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -1)
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_policy_versions(
		effective_date,enabled,created_by
	) VALUES($1,FALSE,9001)`, yesterday.Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	source := &failIfCalledUsageSource{}
	fixture.store.SetUsageSource(source)

	for attempt := 0; attempt < 2; attempt++ {
		result, err := fixture.store.RefreshUsageDay(ctx, yesterday, "startup")
		if err != nil {
			t.Fatalf("mark disabled day ready: %v", err)
		}
		if result.RunID == "" || len(result.SourceFingerprint) != 64 || result.SourceRows != 0 ||
			result.Users != 0 || result.ChangedUsers != 0 {
			t.Fatalf("unexpected disabled-day result: %+v", result)
		}
	}
	if source.calls != 0 {
		t.Fatalf("disabled policy queried usage source %d times", source.calls)
	}
	var succeededRuns int
	if err := fixture.db.QueryRow(ctx, `SELECT COUNT(*) FROM points_snapshot_refresh_runs
		WHERE business_date=$1 AND status='succeeded'`, yesterday.Format("2006-01-02")).Scan(
		&succeededRuns); err != nil {
		t.Fatal(err)
	}
	if succeededRuns != 1 {
		t.Fatalf("disabled-day succeeded runs = %d, want 1", succeededRuns)
	}

	seedCheckinPolicy(t, fixture, 1, 10_000_000, 10_000_000, 100_000, 100_000)
	checkin, err := fixture.store.CheckIn(ctx, 9101, uuid.NewString(), fixture.now)
	if err != nil {
		t.Fatalf("first enabled-day check-in: %v", err)
	}
	if checkin.RewardMicroUSD != 100_000 {
		t.Fatalf("first enabled-day reward = %d, want 100000", checkin.RewardMicroUSD)
	}
}

func TestPostgresSharedDatabasePoolPinsMigrationsToIsolatedSchema(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv(pointsTestDatabaseEnv))
	if databaseURL == "" {
		t.Skip(pointsTestDatabaseEnv + " is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "points_shared_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
	})

	db, err := pointsstore.NewPointsPool(ctx, databaseURL, schema, 3)
	if err != nil {
		t.Fatalf("open shared database points pool: %v", err)
	}
	defer db.Close()
	if err := migrate.Run(ctx, db); err != nil {
		t.Fatalf("migrate isolated points schema: %v", err)
	}
	var currentSchema string
	var isolatedExists, publicExists bool
	if err := db.QueryRow(ctx, `SELECT current_schema()`).Scan(&currentSchema); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL,
		to_regclass('public.points_schema_migrations') IS NOT NULL`,
		schema+".points_schema_migrations").Scan(&isolatedExists, &publicExists); err != nil {
		t.Fatal(err)
	}
	if currentSchema != schema || !isolatedExists || publicExists {
		t.Fatalf("schema isolation failed: current=%q isolated=%t public=%t",
			currentSchema, isolatedExists, publicExists)
	}
}

func TestPostgresCleanupSecurityStateDeletesOnlyExpiredRows(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	now := fixture.now.UTC()
	for _, row := range []struct {
		token   string
		expires time.Time
	}{
		{token: "expired-session", expires: now.Add(-time.Minute)},
		{token: "active-session", expires: now.Add(time.Minute)},
	} {
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_sessions(
			token_hash,user_id,role,theme,language,expires_at
		) VALUES($1,1,'user','light','zh',$2)`, row.token, row.expires); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		nonce   string
		expires time.Time
	}{
		{nonce: "expired-nonce", expires: now.Add(-25 * time.Hour)},
		{nonce: "recent-nonce", expires: now.Add(-23 * time.Hour)},
	} {
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_launch_ticket_nonces(
			jti_hash,subject_user_id,role,expires_at
		) VALUES($1,1,'user',$2)`, row.nonce, row.expires); err != nil {
			t.Fatal(err)
		}
	}

	if err := fixture.store.CleanupSecurityState(ctx, now); err != nil {
		t.Fatalf("cleanup security state: %v", err)
	}
	checks := []struct {
		table string
		want  int
	}{
		{table: "points_sessions", want: 1},
		{table: "points_launch_ticket_nonces", want: 1},
	}
	for _, check := range checks {
		var count int
		if err := fixture.db.QueryRow(ctx, "SELECT COUNT(*) FROM "+check.table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("%s rows = %d, want %d", check.table, count, check.want)
		}
	}
}

func TestPostgresConcurrentCheckinsRespectAllLimits(t *testing.T) {
	tests := []struct {
		name             string
		userIDs          []int64
		requestsPerUser  int
		dailyLimit       int
		platformCap      int64
		userCap          int64
		expectedSuccess  int
		expectedError    error
		maxUserCount     int
		maxUserAward     int64
		expectedPlatform int64
	}{
		{
			name: "daily count", userIDs: []int64{1101}, requestsPerUser: 8,
			dailyLimit: 2, platformCap: 10_000_000, userCap: 10_000_000,
			expectedSuccess: 2, expectedError: domain.ErrCheckinLimit,
			maxUserCount: 2, maxUserAward: 200_000, expectedPlatform: 200_000,
		},
		{
			name: "user amount", userIDs: []int64{1201}, requestsPerUser: 8,
			dailyLimit: 10, platformCap: 10_000_000, userCap: 200_000,
			expectedSuccess: 2, expectedError: domain.ErrCapExhausted,
			maxUserCount: 2, maxUserAward: 200_000, expectedPlatform: 200_000,
		},
		{
			name: "platform amount", userIDs: []int64{
				1301, 1302, 1303, 1304, 1305, 1306, 1307, 1308,
			}, requestsPerUser: 1, dailyLimit: 2, platformCap: 300_000, userCap: 1_000_000,
			expectedSuccess: 3, expectedError: domain.ErrCapExhausted,
			maxUserCount: 1, maxUserAward: 100_000, expectedPlatform: 300_000,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPostgresFixture(t)
			version := seedCheckinPolicy(t, fixture, test.dailyLimit, test.platformCap,
				test.userCap, 100_000, 100_000)
			seedReadySnapshots(t, fixture, version, test.userIDs...)

			outcomes := concurrentCheckins(t, fixture, test.userIDs, test.requestsPerUser)
			successes := 0
			for _, outcome := range outcomes {
				if outcome == nil {
					successes++
					continue
				}
				if !errors.Is(outcome, test.expectedError) {
					t.Errorf("unexpected concurrent check-in error: %v", outcome)
				}
			}
			if successes != test.expectedSuccess {
				t.Fatalf("successful check-ins = %d, want %d", successes, test.expectedSuccess)
			}

			date := fixture.store.BusinessDate(fixture.now).Format("2006-01-02")
			var maxCount int
			var maxAward, platformAward, checkinAward int64
			if err := fixture.db.QueryRow(context.Background(), `SELECT
				COALESCE(MAX(checkin_count),0)::int,
				COALESCE(MAX(awarded_microusd),0)::bigint
				FROM points_checkin_daily WHERE business_date=$1`, date).Scan(
				&maxCount, &maxAward); err != nil {
				t.Fatal(err)
			}
			if err := fixture.db.QueryRow(context.Background(), `SELECT
				COALESCE((SELECT awarded_microusd FROM points_checkin_platform_daily_limits
					WHERE business_date=$1),0)::bigint,
				COALESCE((SELECT SUM(reward_microusd) FROM points_checkins
					WHERE business_date=$1),0)::bigint`, date).Scan(
				&platformAward, &checkinAward); err != nil {
				t.Fatal(err)
			}
			if maxCount > test.maxUserCount || maxAward > test.maxUserAward {
				t.Fatalf("user limits exceeded: count=%d award=%d", maxCount, maxAward)
			}
			if platformAward != test.expectedPlatform || checkinAward != platformAward {
				t.Fatalf("platform award=%d check-in award=%d, want %d",
					platformAward, checkinAward, test.expectedPlatform)
			}
		})
	}
}

func TestPostgresCheckinAvailabilityTracksAuthoritativeEligibility(t *testing.T) {
	fixture := newPostgresFixture(t)
	version := seedCheckinPolicy(t, fixture, 1, 10_000_000, 10_000_000, 100_000, 100_000)
	const userID int64 = 1401
	ctx := context.Background()

	available, err := fixture.store.CheckinAvailable(ctx, userID, fixture.now)
	if err != nil {
		t.Fatalf("check availability before yesterday is settled: %v", err)
	}
	if available {
		t.Fatal("check-in was available before yesterday's snapshot was ready")
	}

	seedReadySnapshots(t, fixture, version, userID)
	available, err = fixture.store.CheckinAvailable(ctx, userID, fixture.now)
	if err != nil {
		t.Fatalf("check availability after yesterday is settled: %v", err)
	}
	if !available {
		t.Fatal("check-in was unavailable after all eligibility rules were satisfied")
	}

	if _, err := fixture.store.CheckIn(ctx, userID, uuid.NewString(), fixture.now); err != nil {
		t.Fatalf("complete eligible check-in: %v", err)
	}
	available, err = fixture.store.CheckinAvailable(ctx, userID, fixture.now)
	if err != nil {
		t.Fatalf("check availability after reaching the daily limit: %v", err)
	}
	if available {
		t.Fatal("check-in remained available after reaching the daily limit")
	}
}

func TestPostgresAccountCountsOnlySettledUnreversedCheckinRewards(t *testing.T) {
	fixture := newPostgresFixture(t)
	version := seedCheckinPolicy(t, fixture, 1, 10_000_000, 10_000_000, 100_000, 100_000)
	ctx := context.Background()
	const userID int64 = 2101
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_accounts(user_id) VALUES($1)`, userID); err != nil {
		t.Fatal(err)
	}
	insertBalanceGrant := func(kind, status string, amount int64, settledAt, reversedAt any) {
		t.Helper()
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_balance_grants(
			id,user_id,amount_microusd,kind,status,external_event_id,request_fingerprint,
			policy_version,settled_at,reversed_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, uuid.New(), userID, amount, kind,
			status, uuid.NewString(), strings.Repeat("a", 64), version, settledAt, reversedAt); err != nil {
			t.Fatalf("insert %s balance grant: %v", status, err)
		}
	}
	now := fixture.now.UTC()
	insertBalanceGrant("checkin", "settled", 100_000, now, nil)
	insertBalanceGrant("checkin", "pending", 200_000, nil, nil)
	insertBalanceGrant("checkin", "reversed", 300_000, now, now.Add(time.Minute))
	insertBalanceGrant("manual_grant", "settled", 400_000, now, nil)

	account, err := fixture.store.Account(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if account.SettledCheckinRewardMicroUSD != 100_000 {
		t.Fatalf("settled check-in rewards = %d, want 100000", account.SettledCheckinRewardMicroUSD)
	}
	summary, err := fixture.store.SummarizeCheckinBalanceGrants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for status, expected := range map[string]int64{"settled": 1, "pending": 1, "reversed": 1} {
		if summary[status] != expected {
			t.Fatalf("check-in grant summary[%q] = %d, want %d", status, summary[status], expected)
		}
	}
}

func TestPostgresSnapshotNotReadyDoesNotConsumeCheckinIdempotency(t *testing.T) {
	fixture := newPostgresFixture(t)
	version := seedCheckinPolicy(t, fixture, 1, 1_000_000, 1_000_000, 100_000, 100_000)
	const userID int64 = 3101
	eventID := "snapshot-not-ready-" + uuid.NewString()

	if _, err := fixture.store.CheckIn(context.Background(), userID, eventID, fixture.now); !errors.Is(err, domain.ErrSnapshotNotReady) {
		t.Fatalf("check-in error = %v, want ErrSnapshotNotReady", err)
	}
	checks := []struct {
		name  string
		query string
		args  []any
	}{
		{name: "idempotency", query: `SELECT COUNT(*) FROM points_idempotency
			WHERE scope='checkin' AND external_event_id=$1`, args: []any{eventID}},
		{name: "daily", query: `SELECT COUNT(*) FROM points_checkin_daily
			WHERE user_id=$1 AND business_date=$2`, args: []any{
			userID, fixture.store.BusinessDate(fixture.now).Format("2006-01-02"),
		}},
		{name: "attempt", query: `SELECT COUNT(*) FROM points_checkin_attempts
			WHERE external_event_id=$1`, args: []any{eventID}},
	}
	for _, check := range checks {
		var count int
		if err := fixture.db.QueryRow(context.Background(), check.query, check.args...).Scan(&count); err != nil {
			t.Fatalf("query %s rollback state: %v", check.name, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after snapshot-not-ready = %d, want 0", check.name, count)
		}
	}

	seedReadySnapshots(t, fixture, version, userID)
	result, err := fixture.store.CheckIn(context.Background(), userID, eventID, fixture.now)
	if err != nil {
		t.Fatalf("retry check-in with the same idempotency key: %v", err)
	}
	if result.RewardMicroUSD != 100_000 {
		t.Fatalf("retry reward = %d, want 100000", result.RewardMicroUSD)
	}
}

func TestPostgresDailyLimitRejectionsConvergeAndPreserveAcceptedReplay(t *testing.T) {
	fixture := newPostgresFixture(t)
	version := seedCheckinPolicy(t, fixture, 1, 10_000_000, 10_000_000, 100_000, 100_000)
	const userID int64 = 3201
	seedReadySnapshots(t, fixture, version, userID)
	ctx := context.Background()
	acceptedEventID := "accepted-" + uuid.NewString()

	accepted, err := fixture.store.CheckIn(ctx, userID, acceptedEventID, fixture.now)
	if err != nil {
		t.Fatalf("initial check-in: %v", err)
	}
	if accepted.TransactionID == "" {
		t.Fatal("initial check-in did not enqueue a balance grant")
	}

	const rejectedRequests = 40
	var rejectedEventID string
	for request := 0; request < rejectedRequests; request++ {
		eventID := fmt.Sprintf("daily-limit-%02d-%s", request, uuid.NewString())
		if request == 0 {
			rejectedEventID = eventID
		}
		if _, err := fixture.store.CheckIn(ctx, userID, eventID, fixture.now); !errors.Is(err, domain.ErrCheckinLimit) {
			t.Fatalf("daily-limit request %d error = %v, want ErrCheckinLimit", request, err)
		}
		if request == 0 {
			yesterday := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -1).Format("2006-01-02")
			if _, err := fixture.db.Exec(ctx, `UPDATE points_daily_snapshots SET status='needs_review'
				WHERE user_id=$1 AND business_date=$2`, userID, yesterday); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := fixture.store.CheckIn(ctx, userID, rejectedEventID, fixture.now); !errors.Is(err, domain.ErrCheckinLimit) {
		t.Fatalf("same-day rejected replay error = %v, want ErrCheckinLimit", err)
	}

	replayed, err := fixture.store.CheckIn(ctx, userID, acceptedEventID, fixture.now)
	if err != nil {
		t.Fatalf("accepted request replay after reaching daily limit: %v", err)
	}
	if replayed.TransactionID != accepted.TransactionID || replayed.RewardMicroUSD != accepted.RewardMicroUSD {
		t.Fatalf("accepted replay changed result: got %+v want %+v", replayed, accepted)
	}

	date := fixture.store.BusinessDate(fixture.now).Format("2006-01-02")
	var idempotencyRows, acceptedAttempts, limitAttempts int
	if err := fixture.db.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM points_idempotency WHERE scope='checkin'),
		(SELECT COUNT(*) FROM points_checkin_attempts WHERE user_id=$1 AND business_date=$2 AND outcome='accepted'),
		(SELECT COUNT(*) FROM points_checkin_attempts WHERE user_id=$1 AND business_date=$2
			AND rejection_reason='daily_limit_reached')`, userID, date).Scan(
		&idempotencyRows, &acceptedAttempts, &limitAttempts); err != nil {
		t.Fatal(err)
	}
	if idempotencyRows != 1 || acceptedAttempts != 1 || limitAttempts != 1 {
		t.Fatalf("rows after %d rejected keys: idempotency=%d accepted=%d limit=%d, want 1/1/1",
			rejectedRequests, idempotencyRows, acceptedAttempts, limitAttempts)
	}
}

func TestPostgresMinimumSpendRejectionsDoNotConsumeClientKeys(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	date := fixture.store.BusinessDate(fixture.now).Format("2006-01-02")
	var policyID, version int64
	if err := fixture.db.QueryRow(ctx, `INSERT INTO points_policy_versions(
		effective_date,enabled,mode,basis,checkin_enabled,checkin_daily_limit,
		minimum_checkin_spend_microusd,checkin_platform_daily_cap_microusd,
		checkin_user_daily_cap_microusd,checkin_single_award_cap_microusd,
		points_per_usd_hundredths,refresh_minute,created_by
	) VALUES($1,TRUE,'all_users','yesterday',TRUE,1,2000000,10000000,10000000,100000,1000,5,9001)
	RETURNING id,version_no`, date).Scan(&policyID, &version); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.Exec(ctx, `INSERT INTO points_policy_tiers(
		policy_id,lower_points_hundredths,reward_mode,fixed_reward_min_microusd,
		fixed_reward_max_microusd
	) VALUES($1,0,'fixed_range',100000,100000)`, policyID); err != nil {
		t.Fatal(err)
	}
	const userID int64 = 3301
	seedReadySnapshots(t, fixture, version, userID)

	for request := 0; request < 40; request++ {
		eventID := fmt.Sprintf("minimum-spend-%02d-%s", request, uuid.NewString())
		if _, err := fixture.store.CheckIn(ctx, userID, eventID, fixture.now); !errors.Is(
			err, domain.ErrCheckinSpendMinimum) {
			t.Fatalf("minimum-spend request %d error = %v", request, err)
		}
	}
	var idempotencyRows, rejectionRows int
	if err := fixture.db.QueryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM points_idempotency WHERE scope='checkin'),
		(SELECT COUNT(*) FROM points_checkin_attempts WHERE user_id=$1 AND business_date=$2
			AND rejection_reason='minimum_spend_not_met')`, userID, date).Scan(
		&idempotencyRows, &rejectionRows); err != nil {
		t.Fatal(err)
	}
	if idempotencyRows != 0 || rejectionRows != 1 {
		t.Fatalf("rows after rejected keys: idempotency=%d minimum_spend=%d, want 0/1",
			idempotencyRows, rejectionRows)
	}
}

func TestPostgresReverseBalanceGrantRequiresKnownCreditOutcome(t *testing.T) {
	fixture := newPostgresFixture(t)
	version := seedCheckinPolicy(t, fixture, 1, 10_000_000, 10_000_000, 100_000, 100_000)
	ctx := context.Background()
	now := fixture.now.UTC()

	tests := []struct {
		name               string
		status             string
		attempts           int
		settledAt          any
		retryBeforeReverse bool
		wantErr            error
		wantStatus         string
	}{
		{name: "never attempted pending can cancel", status: "pending", attempts: 0, wantStatus: "reversed"},
		{name: "failed credit is unknown", status: "failed", attempts: 1, wantErr: domain.ErrInvalidState, wantStatus: "failed"},
		{name: "retried pending credit is unknown", status: "failed", attempts: 1, retryBeforeReverse: true, wantErr: domain.ErrInvalidState, wantStatus: "pending"},
		{name: "settled credit queues debit", status: "settled", attempts: 1, settledAt: now, wantStatus: "reversal_pending"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id := uuid.New()
			if _, err := fixture.db.Exec(ctx, `INSERT INTO points_balance_grants(
				id,user_id,amount_microusd,kind,status,external_event_id,request_fingerprint,
				policy_version,attempts,next_attempt_at,settled_at
			) VALUES($1,$2,100000,'checkin',$3,$4,$5,$6,$7,$8,$9)`, id, int64(4100+index),
				test.status, uuid.NewString(), strings.Repeat("c", 64), version, test.attempts, now, test.settledAt); err != nil {
				t.Fatal(err)
			}
			if test.retryBeforeReverse {
				if err := fixture.store.RetryBalanceGrant(ctx, id.String(), 9001, now.Add(30*time.Second)); err != nil {
					t.Fatalf("retry failed grant: %v", err)
				}
			}

			err := fixture.store.ReverseBalanceGrant(ctx, id.String(), "operator reversal", 9001, now.Add(time.Minute))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ReverseBalanceGrant error = %v, want %v", err, test.wantErr)
			}
			var status string
			var reversalCount int
			if err := fixture.db.QueryRow(ctx, `SELECT status FROM points_balance_grants WHERE id=$1`, id).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if err := fixture.db.QueryRow(ctx, `SELECT COUNT(*) FROM points_balance_grant_reversals
				WHERE balance_grant_id=$1`, id).Scan(&reversalCount); err != nil {
				t.Fatal(err)
			}
			if status != test.wantStatus {
				t.Fatalf("grant status = %q, want %q", status, test.wantStatus)
			}
			wantReversal := 1
			if test.wantErr != nil {
				wantReversal = 0
			}
			if reversalCount != wantReversal {
				t.Fatalf("reversal records = %d, want %d", reversalCount, wantReversal)
			}
		})
	}
}

func TestPostgresDailyPointsAreUserScopedAndChronological(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	baseDate := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -2)
	for day := 0; day < 2; day++ {
		date := baseDate.AddDate(0, 0, day)
		runID := uuid.New()
		if _, err := fixture.db.Exec(ctx, `INSERT INTO points_snapshot_refresh_runs(
			id,business_date,trigger,source_window_start,source_window_end,source_fingerprint,
			source_users,source_rows,changed_users,delta_spend_microusd,delta_points_hundredths,
			status,completed_at
		) VALUES($1,$2,'manual',$3,$4,$5,2,2,2,2000000,2000,'succeeded',$6)`, runID,
			date.Format("2006-01-02"), date, date.AddDate(0, 0, 1), strings.Repeat("d", 64),
			fixture.now.UTC()); err != nil {
			t.Fatalf("insert daily-points refresh run: %v", err)
		}
		for _, userID := range []int64{7301, 7302} {
			points := int64((day + 1) * 100)
			if userID == 7302 {
				points += 10_000
			}
			if _, err := fixture.db.Exec(ctx, `INSERT INTO points_daily_snapshots(
				user_id,business_date,actual_cost_microusd,accounted_spend_microusd,
				points_per_usd_hundredths,target_points_hundredths,awarded_points_hundredths,
				revision,status,source_row_count,source_max_usage_log_id,source_fingerprint,last_refresh_run_id
			) VALUES($1,$2,1000000,1000000,1000,$3,$3,1,'ready',1,$1,$4,$5)`, userID,
				date.Format("2006-01-02"), points, fmt.Sprintf("%064x", userID+int64(day)), runID); err != nil {
				t.Fatalf("insert daily point for user %d: %v", userID, err)
			}
		}
	}

	items, err := fixture.store.DailyPoints(ctx, 7301, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 30 || items[28].AwardedPointsHundredths != 100 ||
		items[29].AwardedPointsHundredths != 200 || !items[0].BusinessDate.Before(items[29].BusinessDate) {
		t.Fatalf("unexpected user-scoped daily points: %+v", items)
	}
	for index := 0; index < 28; index++ {
		if items[index].ActualCostMicroUSD != 0 || items[index].AwardedPointsHundredths != 0 ||
			items[index].Status != "empty" {
			t.Fatalf("daily points gap %d was not zero-filled: %+v", index, items[index])
		}
	}
}

func seedCheckinPolicy(t *testing.T, fixture *postgresFixture, dailyLimit int,
	platformCap, userCap, singleCap, reward int64) int64 {
	t.Helper()
	date := fixture.store.BusinessDate(fixture.now).Format("2006-01-02")
	var policyID, version int64
	err := fixture.db.QueryRow(context.Background(), `INSERT INTO points_policy_versions(
		effective_date,enabled,mode,basis,checkin_enabled,checkin_daily_limit,
		minimum_checkin_spend_microusd,checkin_platform_daily_cap_microusd,
		checkin_user_daily_cap_microusd,checkin_single_award_cap_microusd,
		points_per_usd_hundredths,refresh_minute,created_by
	) VALUES($1,TRUE,'all_users','yesterday',TRUE,$2,0,$3,$4,$5,1000,5,9001)
	RETURNING id,version_no`, date, dailyLimit, platformCap, userCap, singleCap).Scan(
		&policyID, &version)
	if err != nil {
		t.Fatalf("insert check-in policy: %v", err)
	}
	if _, err := fixture.db.Exec(context.Background(), `INSERT INTO points_policy_tiers(
		policy_id,lower_points_hundredths,reward_mode,fixed_reward_min_microusd,
		fixed_reward_max_microusd
	) VALUES($1,0,'fixed_range',$2,$2)`, policyID, reward); err != nil {
		t.Fatalf("insert check-in tier: %v", err)
	}
	return version
}

func seedReadySnapshots(t *testing.T, fixture *postgresFixture, policyVersion int64, userIDs ...int64) {
	t.Helper()
	yesterday := fixture.store.BusinessDate(fixture.now).AddDate(0, 0, -1)
	runID := uuid.New()
	spend := int64(1_000_000)
	if _, err := fixture.db.Exec(context.Background(), `INSERT INTO points_snapshot_refresh_runs(
		id,business_date,trigger,source_window_start,source_window_end,source_fingerprint,
		source_users,source_rows,changed_users,delta_spend_microusd,delta_points_hundredths,
		status,completed_at
	) VALUES($1,$2,'manual',$3,$4,$5,$6::integer,$6::bigint,$6::integer,$7,$8,'succeeded',$9)`, runID,
		yesterday.Format("2006-01-02"), yesterday, yesterday.AddDate(0, 0, 1),
		strings.Repeat("b", 64), len(userIDs), spend*int64(len(userIDs)),
		int64(1_000*len(userIDs)), fixture.now.UTC()); err != nil {
		t.Fatalf("insert successful snapshot run: %v", err)
	}
	for _, userID := range userIDs {
		fingerprint := fmt.Sprintf("%064x", userID)
		if _, err := fixture.db.Exec(context.Background(), `INSERT INTO points_daily_snapshots(
			user_id,business_date,actual_cost_microusd,accounted_spend_microusd,policy_version,
			points_per_usd_hundredths,target_points_hundredths,awarded_points_hundredths,
			revision,status,source_row_count,source_max_usage_log_id,source_fingerprint,last_refresh_run_id
		) VALUES($1,$2,$3,$3,$4,1000,1000,1000,1,'ready',1,$1,$5,$6)`, userID,
			yesterday.Format("2006-01-02"), spend, policyVersion, fingerprint, runID); err != nil {
			t.Fatalf("insert ready snapshot for user %d: %v", userID, err)
		}
	}
}

func concurrentCheckins(t *testing.T, fixture *postgresFixture, userIDs []int64,
	requestsPerUser int) []error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, len(userIDs)*requestsPerUser)
	var wait sync.WaitGroup
	for _, userID := range userIDs {
		for request := 0; request < requestsPerUser; request++ {
			wait.Add(1)
			go func(userID int64) {
				defer wait.Done()
				<-start
				eventID := fmt.Sprintf("concurrent-%d-%s", userID, uuid.NewString())
				_, err := fixture.store.CheckIn(ctx, userID, eventID, fixture.now)
				results <- err
			}(userID)
		}
	}
	close(start)
	wait.Wait()
	close(results)
	outcomes := make([]error, 0, cap(results))
	for err := range results {
		outcomes = append(outcomes, err)
	}
	return outcomes
}

type failIfCalledUsageSource struct {
	calls int
}

func (s *failIfCalledUsageSource) AggregateDay(context.Context, time.Time,
	time.Time) (pointsstore.UsageDay, error) {
	s.calls++
	return pointsstore.UsageDay{}, errors.New("usage source must not be called for a disabled policy")
}
