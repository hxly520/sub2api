//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	pointsstore "github.com/hxly520/sub2api/points-system/internal/store"
)

func TestPostgresHistoryBackfillIsResumableIdempotentAndPinsPolicy(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	today := fixture.store.BusinessDate(fixture.now)
	from := today.AddDate(0, 0, -2)
	through := today.AddDate(0, 0, -1)
	source := &historyUsageSourceStub{
		location: fixture.location,
		bounds: pointsstore.SuccessfulUsageBounds{
			Found: true, EarliestUTC: from.Add(2 * time.Hour).UTC(),
			LatestUTC: through.Add(3 * time.Hour).UTC(),
		},
		days: map[string]pointsstore.UsageDay{},
		summary: pointsstore.UsageHistorySummary{
			SourceUsers: 2, SourceUserDays: 3, SourceBusinessDays: 2, SourceRows: 6,
			SpendMicroUSD: 3_750_000, PointsHundredths: 3_750,
			SourceMaxUsageLogID: 6,
		},
	}
	source.days[from.UTC().Format(time.RFC3339)] = historyUsageDay(from,
		pointsstore.UsageAggregate{UserID: 1, ActualCostMicroUSD: 1_250_000,
			SourceRowCount: 2, SourceMaxUsageLogID: 2, SourceFingerprint: strings.Repeat("a", 64)})
	source.days[through.UTC().Format(time.RFC3339)] = historyUsageDay(through,
		pointsstore.UsageAggregate{UserID: 1, ActualCostMicroUSD: 500_000,
			SourceRowCount: 1, SourceMaxUsageLogID: 3, SourceFingerprint: strings.Repeat("b", 64)},
		pointsstore.UsageAggregate{UserID: 2, ActualCostMicroUSD: 2_000_000,
			SourceRowCount: 3, SourceMaxUsageLogID: 6, SourceFingerprint: strings.Repeat("c", 64)})
	fixture.store.SetUsageSource(source)

	policy, err := fixture.store.CreateInitialActivationPolicy(ctx, 1, 1000, fixture.now)
	if err != nil {
		t.Fatalf("create initial activation policy: %v", err)
	}
	if !policy.Enabled || policy.CheckinEnabled || policy.RefreshMinute != 5 ||
		policy.PointsPerUSDHundredths != 1000 || !fixture.store.BusinessDate(policy.EffectiveDate).Equal(today) {
		t.Fatalf("unexpected initial policy: %+v", policy)
	}
	repeated, err := fixture.store.CreateInitialActivationPolicy(ctx, 1, 1000, fixture.now)
	if err != nil || repeated.VersionNo != policy.VersionNo {
		t.Fatalf("initial activation is not idempotent: policy=%+v err=%v", repeated, err)
	}
	ready, err := fixture.store.UserAccessReadyForPolicy(ctx, policy.VersionNo)
	if err != nil || ready {
		t.Fatalf("initial policy was exposed before backfill: ready=%v err=%v", ready, err)
	}

	plan, err := fixture.store.PlanHistoryBackfill(ctx, time.Time{}, through,
		policy.VersionNo, fixture.now)
	if err != nil {
		t.Fatalf("plan history backfill: %v", err)
	}
	if !plan.FromDate.Equal(from) || plan.CalendarDays != 2 || len(plan.ConfirmationFingerprint) != 64 {
		t.Fatalf("unexpected history plan: %+v", plan)
	}
	job, err := fixture.store.CreateHistoryBackfillJob(ctx, plan, 1, fixture.now)
	if err != nil {
		t.Fatalf("create history backfill job: %v", err)
	}
	if job.Status != "running" || !job.NextDate.Equal(from) {
		t.Fatalf("unexpected new history job: %+v", job)
	}
	_, err = fixture.store.CreatePolicyAtomic(ctx, domain.Policy{
		EffectiveDate: today.AddDate(0, 0, 1), Enabled: true,
		Mode: domain.PolicyModeAllUsers, Basis: domain.PolicyBasisYesterday,
		PointsPerUSDHundredths: 1000, RefreshMinute: 5, CreatedBy: 1,
	}, fixture.now)
	if err == nil {
		t.Fatal("enabled future policy bypassed the pending initial history gate")
	}

	job, first, done, err := fixture.store.ProcessHistoryBackfillDay(ctx, job.ID)
	if err != nil || done || first.BusinessDate.Format("2006-01-02") != from.Format("2006-01-02") ||
		job.CompletedDays != 1 {
		t.Fatalf("first history day: job=%+v result=%+v done=%v err=%v", job, first, done, err)
	}
	source.summary.SourceRows++
	failed, _, done, err := fixture.store.ProcessHistoryBackfillDay(ctx, job.ID)
	if !errors.Is(err, pointsstore.ErrHistoryBackfillPlanDrift) || done {
		t.Fatalf("drifted history plan was not rejected: job=%+v done=%v err=%v", failed, done, err)
	}
	failed, err = fixture.store.GetHistoryBackfillJob(ctx, job.ID)
	if err != nil || failed.Status != "failed" || failed.CompletedDays != 1 {
		t.Fatalf("drift failure was not recoverable: job=%+v err=%v", failed, err)
	}
	source.summary.SourceRows--
	job, err = fixture.store.ResumeHistoryBackfillJob(ctx, job.ID, 1)
	if err != nil || job.Status != "running" {
		t.Fatalf("resume history backfill: job=%+v err=%v", job, err)
	}
	job, second, done, err := fixture.store.ProcessHistoryBackfillDay(ctx, job.ID)
	if err != nil || !done || second.BusinessDate.Format("2006-01-02") != through.Format("2006-01-02") ||
		job.Status != "succeeded" || job.CompletedDays != 2 {
		t.Fatalf("second history day: job=%+v result=%+v done=%v err=%v", job, second, done, err)
	}
	final, empty, done, err := fixture.store.ProcessHistoryBackfillDay(ctx, job.ID)
	if err != nil || !done || empty.RunID != "" || final.CompletedDays != 2 {
		t.Fatalf("idempotent completed run: job=%+v result=%+v done=%v err=%v", final, empty, done, err)
	}
	reused, err := fixture.store.CreateHistoryBackfillJob(ctx, plan, 1, fixture.now)
	if err != nil || reused.ID != job.ID || reused.Status != "succeeded" {
		t.Fatalf("repeated apply did not reuse the successful baseline: job=%+v err=%v", reused, err)
	}
	source.summary = pointsstore.UsageHistorySummary{
		SourceUsers: 2, SourceUserDays: 2, SourceBusinessDays: 1, SourceRows: 4,
		SpendMicroUSD: 2_500_000, PointsHundredths: 2_500, SourceMaxUsageLogID: 6,
	}
	differentPlan, err := fixture.store.PlanHistoryBackfill(ctx, through, through,
		policy.VersionNo, fixture.now)
	if err != nil {
		t.Fatalf("plan alternate history baseline: %v", err)
	}
	if _, err = fixture.store.CreateHistoryBackfillJob(ctx, differentPlan, 1, fixture.now); err == nil {
		t.Fatal("a second, different history baseline was accepted")
	}
	ready, err = fixture.store.UserAccessReadyForPolicy(ctx, policy.VersionNo)
	if err != nil || !ready {
		t.Fatalf("initial policy stayed private after backfill: ready=%v err=%v", ready, err)
	}

	refresh, err := fixture.store.RefreshUsageDay(ctx, from, "manual")
	if err != nil || refresh.ChangedUsers != 0 || refresh.DeltaPointsHundredths != 0 {
		t.Fatalf("pinned-policy reconcile changed history: result=%+v err=%v", refresh, err)
	}
	for userID, expected := range map[int64][2]int64{
		1: {1_750, 1_750_000},
		2: {2_000, 2_000_000},
	} {
		var points, spend int64
		if err := fixture.db.QueryRow(ctx, `SELECT total_points_hundredths,total_spend_microusd
			FROM points_accounts WHERE user_id=$1`, userID).Scan(&points, &spend); err != nil {
			t.Fatal(err)
		}
		if points != expected[0] || spend != expected[1] {
			t.Fatalf("user %d totals=(%d,%d), want=%v", userID, points, spend, expected)
		}
	}
	var mappedDays, ledgerRows int
	if err := fixture.db.QueryRow(ctx, `SELECT COUNT(*) FROM points_usage_history_backfill_days
		WHERE job_id=$1`, job.ID).Scan(&mappedDays); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.QueryRow(ctx, `SELECT COUNT(*) FROM points_ledger`).Scan(&ledgerRows); err != nil {
		t.Fatal(err)
	}
	if mappedDays != 2 || ledgerRows != 3 {
		t.Fatalf("mapped days=%d ledger rows=%d, want 2 and 3", mappedDays, ledgerRows)
	}
}

func TestPostgresHistoryBackfillCompletesAnEmptyInitialBaseline(t *testing.T) {
	fixture := newPostgresFixture(t)
	ctx := context.Background()
	today := fixture.store.BusinessDate(fixture.now)
	yesterday := today.AddDate(0, 0, -1)
	source := &historyUsageSourceStub{
		location: fixture.location,
		days: map[string]pointsstore.UsageDay{
			yesterday.UTC().Format(time.RFC3339): historyUsageDay(yesterday),
		},
	}
	fixture.store.SetUsageSource(source)

	policy, err := fixture.store.CreateInitialActivationPolicy(ctx, 1, 1000, fixture.now)
	if err != nil {
		t.Fatalf("create initial activation policy: %v", err)
	}
	plan, err := fixture.store.PlanHistoryBackfill(ctx, time.Time{}, yesterday,
		policy.VersionNo, fixture.now)
	if err != nil {
		t.Fatalf("plan empty history baseline: %v", err)
	}
	if !plan.FromDate.Equal(yesterday) || plan.CalendarDays != 1 || plan.SourceRows != 0 ||
		plan.SpendMicroUSD != 0 || plan.PointsHundredths != 0 {
		t.Fatalf("unexpected empty history plan: %+v", plan)
	}
	job, err := fixture.store.CreateHistoryBackfillJob(ctx, plan, 1, fixture.now)
	if err != nil {
		t.Fatalf("create empty history job: %v", err)
	}
	job, _, done, err := fixture.store.ProcessHistoryBackfillDay(ctx, job.ID)
	if err != nil || !done || job.Status != "succeeded" {
		t.Fatalf("complete empty history job: job=%+v done=%v err=%v", job, done, err)
	}
	ready, err := fixture.store.UserAccessReadyForPolicy(ctx, policy.VersionNo)
	if err != nil || !ready {
		t.Fatalf("empty baseline did not open the initial gate: ready=%v err=%v", ready, err)
	}
}

type historyUsageSourceStub struct {
	location *time.Location
	bounds   pointsstore.SuccessfulUsageBounds
	summary  pointsstore.UsageHistorySummary
	days     map[string]pointsstore.UsageDay
}

func (s *historyUsageSourceStub) SuccessfulUsageBounds(context.Context) (
	pointsstore.SuccessfulUsageBounds, error) {
	return s.bounds, nil
}

func (s *historyUsageSourceStub) SummarizeHistory(context.Context, time.Time, time.Time,
	string, int64) (pointsstore.UsageHistorySummary, error) {
	return s.summary, nil
}

func (s *historyUsageSourceStub) AggregateDay(_ context.Context, start, end time.Time) (
	pointsstore.UsageDay, error) {
	day, ok := s.days[start.UTC().Format(time.RFC3339)]
	if !ok {
		return pointsstore.UsageDay{}, fmt.Errorf("unexpected history day %s", start)
	}
	if !day.WindowEnd.Equal(end) {
		return pointsstore.UsageDay{}, fmt.Errorf("unexpected history window end %s", end)
	}
	return day, nil
}

func historyUsageDay(date time.Time, aggregates ...pointsstore.UsageAggregate) pointsstore.UsageDay {
	var rows int64
	for _, aggregate := range aggregates {
		rows += aggregate.SourceRowCount
	}
	return pointsstore.UsageDay{
		WindowStart: date.UTC(), WindowEnd: date.AddDate(0, 0, 1).UTC(), Aggregates: aggregates,
		Fingerprint: strings.Repeat("d", 64), SourceRows: rows,
	}
}
