package store

import (
	"testing"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
)

func TestHistoryBackfillPolicyMustBeEnabledAt0005WithCheckinOff(t *testing.T) {
	valid := domain.Policy{
		Enabled: true, CheckinEnabled: false, PointsPerUSDHundredths: 1000,
		RefreshMinute: 5,
	}
	if err := validateHistoryBackfillPolicy(valid); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	invalid := []domain.Policy{
		{Enabled: false, PointsPerUSDHundredths: 1000, RefreshMinute: 5},
		{Enabled: true, CheckinEnabled: true, PointsPerUSDHundredths: 1000, RefreshMinute: 5},
		{Enabled: true, PointsPerUSDHundredths: 1000, RefreshMinute: 6},
		{Enabled: true, PointsPerUSDHundredths: 0, RefreshMinute: 5},
	}
	for index, policy := range invalid {
		if err := validateHistoryBackfillPolicy(policy); err == nil {
			t.Fatalf("invalid policy %d was accepted", index)
		}
	}
}

func TestHistoryBackfillPlanFingerprintBindsSourceAndPolicy(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	plan := HistoryBackfillPlan{
		FromDate:     time.Date(2026, 6, 21, 0, 0, 0, 0, location),
		ThroughDate:  time.Date(2026, 7, 29, 0, 0, 0, 0, location),
		CalendarDays: 39, PolicyVersion: 2, PointsPerUSDHundredths: 1000,
		SourceUsers: 29, SourceUserDays: 300, SourceBusinessDays: 39,
		SourceRows: 211240, SpendMicroUSD: 2_541_818_675,
		PointsHundredths: 2_541_800, SourceMaxUsageLogID: 900000,
	}
	if err := validateHistoryBackfillPlan(plan); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	first := historyBackfillPlanFingerprint(plan)
	if len(first) != 64 || first != historyBackfillPlanFingerprint(plan) {
		t.Fatalf("plan fingerprint is not stable: %q", first)
	}
	plan.SourceRows++
	if first == historyBackfillPlanFingerprint(plan) {
		t.Fatal("source change did not alter confirmation fingerprint")
	}
	plan.SourceRows--
	plan.PolicyVersion++
	if first == historyBackfillPlanFingerprint(plan) {
		t.Fatal("policy change did not alter confirmation fingerprint")
	}
}

func TestInclusiveCalendarDaysUsesNaturalDates(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 3, 7, 0, 0, 0, 0, location)
	through := time.Date(2026, 3, 9, 0, 0, 0, 0, location)
	if got := inclusiveCalendarDays(from, through); got != 3 {
		t.Fatalf("inclusive natural days = %d, want 3", got)
	}
}

func TestPolicyCivilDateDoesNotShiftWestOfUTC(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	databaseDate := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	got := policyCivilDate(databaseDate, location)
	want := time.Date(2026, 7, 31, 0, 0, 0, 0, location)
	if !got.Equal(want) || got.Format("2006-01-02") != "2026-07-31" {
		t.Fatalf("policy civil date = %s, want %s", got, want)
	}
}

func TestHistorySummaryMustExactlyMatchConfirmedJob(t *testing.T) {
	summary := UsageHistorySummary{
		SourceUsers: 29, SourceUserDays: 300, SourceBusinessDays: 39,
		SourceRows: 211240, SpendMicroUSD: 2_541_818_675,
		PointsHundredths: 2_541_800, SourceMaxUsageLogID: 900000,
	}
	job := HistoryBackfillJob{
		PlannedSourceUsers:         summary.SourceUsers,
		PlannedSourceUserDays:      summary.SourceUserDays,
		PlannedSourceBusinessDays:  summary.SourceBusinessDays,
		PlannedSourceRows:          summary.SourceRows,
		PlannedSpendMicroUSD:       summary.SpendMicroUSD,
		PlannedPointsHundredths:    summary.PointsHundredths,
		PlannedSourceMaxUsageLogID: summary.SourceMaxUsageLogID,
	}
	if !historySummaryMatchesJob(summary, job) {
		t.Fatal("identical summary and job did not match")
	}
	summary.SourceRows++
	if historySummaryMatchesJob(summary, job) {
		t.Fatal("source drift was accepted")
	}
}
