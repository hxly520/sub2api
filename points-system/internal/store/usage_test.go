package store

import (
	"testing"

	"github.com/hxly520/sub2api/points-system/internal/domain"
)

func TestPointsForSpendPreservesTwoDecimalRatioAndFloorsHundredths(t *testing.T) {
	tests := []struct {
		spendMicroUSD, ratio, want int64
	}{
		{spendMicroUSD: 1_000_000, ratio: 1_025, want: 1_025},
		{spendMicroUSD: 50_000, ratio: 1_025, want: 51},
		{spendMicroUSD: 999, ratio: 1_000, want: 0},
	}
	for _, test := range tests {
		got, err := pointsForSpend(test.spendMicroUSD, test.ratio)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("pointsForSpend(%d,%d) = %d, want %d", test.spendMicroUSD,
				test.ratio, got, test.want)
		}
	}
}

func TestMakeSnapshotTargetUsesPolicyEffectiveForConsumptionDay(t *testing.T) {
	target, err := makeSnapshotTarget(UsageAggregate{
		UserID: 7, ActualCostMicroUSD: 2_000_000, SourceRowCount: 2,
		SourceMaxUsageLogID: 9, SourceFingerprint: sixtyFourHex,
	}, domain.Policy{Enabled: true, VersionNo: 12, PointsPerUSDHundredths: 1_025})
	if err != nil {
		t.Fatal(err)
	}
	if target.PolicyVersion == nil || *target.PolicyVersion != 12 ||
		target.PointsPerUSDHundredths != 1_025 || target.PointsHundredths != 2_050 ||
		target.Status != snapshotStatusReady {
		t.Fatalf("unexpected target: %+v", target)
	}
}

func TestReconcileSnapshotLateUsageAppliesOnlyIncrement(t *testing.T) {
	previous := usageSnapshot{
		UserID: 1, ActualCostMicroUSD: 1_000_000, AccountedSpendMicroUSD: 1_000_000,
		AwardedPointsHundredths: 1_000, Status: snapshotStatusReady,
	}
	target := snapshotTarget{UsageAggregate: UsageAggregate{UserID: 1, ActualCostMicroUSD: 1_250_000},
		PointsHundredths: 1_250, Status: snapshotStatusReady}
	got := reconcileSnapshot(previous, target)
	if got.DeltaSpendMicroUSD != 250_000 || got.DeltaPointsHundredths != 250 ||
		got.AccountedSpendMicroUSD != 1_250_000 || got.AwardedPointsHundredths != 1_250 {
		t.Fatalf("unexpected late-usage reconciliation: %+v", got)
	}
}

func TestReconcileSnapshotSourceCorrectionAppliesSignedDeltas(t *testing.T) {
	previous := usageSnapshot{
		UserID: 1, ActualCostMicroUSD: 1_000_000, AccountedSpendMicroUSD: 1_000_000,
		AwardedPointsHundredths: 1_000, Status: snapshotStatusReady,
	}
	target := snapshotTarget{UsageAggregate: UsageAggregate{UserID: 1, ActualCostMicroUSD: 800_000},
		PointsHundredths: 800, Status: snapshotStatusReady}
	got := reconcileSnapshot(previous, target)
	if got.Status != snapshotStatusReady || got.DeltaSpendMicroUSD != -200_000 ||
		got.DeltaPointsHundredths != -200 || got.AccountedSpendMicroUSD != 800_000 ||
		got.AwardedPointsHundredths != 800 {
		t.Fatalf("source correction was not applied bidirectionally: %+v", got)
	}
}

func TestGuardAccountTotalsMarksInconsistentCorrectionForReview(t *testing.T) {
	previous := usageSnapshot{UserID: 1, AccountedSpendMicroUSD: 1_000_000,
		AwardedPointsHundredths: 1_000}
	application := snapshotApplication{AccountedSpendMicroUSD: 800_000,
		AwardedPointsHundredths: 800, Status: snapshotStatusReady,
		DeltaSpendMicroUSD: -200_000, DeltaPointsHundredths: -200}
	got, err := guardAccountTotals(previous, application, 100, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != snapshotStatusReview || got.DeltaSpendMicroUSD != 0 ||
		got.DeltaPointsHundredths != 0 || got.AccountedSpendMicroUSD != 1_000_000 ||
		got.AwardedPointsHundredths != 1_000 {
		t.Fatalf("negative account guard failed: %+v", got)
	}
}

func TestNeedsReviewSnapshotIsIdempotentUntilSourceChanges(t *testing.T) {
	previous := usageSnapshot{
		UserID: 1, ActualCostMicroUSD: 800_000, AccountedSpendMicroUSD: 1_000_000,
		PointsPerUSDHundredths: 1_000, TargetPointsHundredths: 800,
		AwardedPointsHundredths: 1_000, Status: snapshotStatusReview,
		SourceRowCount: 3, SourceMaxUsageLogID: 10, SourceFingerprint: sixtyFourHex,
	}
	version := int64(1)
	previous.PolicyVersion = &version
	target := snapshotTarget{UsageAggregate: UsageAggregate{
		UserID: 1, ActualCostMicroUSD: 800_000, SourceRowCount: 3,
		SourceMaxUsageLogID: 10, SourceFingerprint: sixtyFourHex,
	}, PolicyVersion: &version, PointsPerUSDHundredths: 1_000,
		PointsHundredths: 800, Status: snapshotStatusReady}
	if snapshotMatches(previous, target) {
		t.Fatal("needs-review snapshot must revalidate account totals")
	}
	application := reconcileSnapshot(previous, target)
	application, err := guardAccountTotals(previous, application, 100, 100_000)
	if err != nil {
		t.Fatal(err)
	}
	if !pendingReviewUnchanged(previous, target, application) {
		t.Fatal("unchanged unresolved review should not create another revision")
	}
	application = reconcileSnapshot(previous, target)
	application, err = guardAccountTotals(previous, application, 2_000, 2_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if pendingReviewUnchanged(previous, target, application) || application.Status != snapshotStatusReady {
		t.Fatal("repaired account totals should allow the correction to resume")
	}
}

const sixtyFourHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
