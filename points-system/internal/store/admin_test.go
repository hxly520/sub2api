package store

import (
	"testing"
	"time"
)

func TestCompleteDailyPointsWindowUsesNaturalDaysAndFillsMissingDates(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	windowStart := time.Date(2026, 3, 7, 0, 0, 0, 0, location)
	items := []DailyPoint{
		{BusinessDate: windowStart.AddDate(0, 0, -1), AwardedPointsHundredths: 9_999, Status: snapshotStatusReady},
		{BusinessDate: windowStart, ActualCostMicroUSD: 1_000_000, AwardedPointsHundredths: 100, Status: snapshotStatusReady},
		{BusinessDate: windowStart.AddDate(0, 0, 2), ActualCostMicroUSD: 3_000_000, AwardedPointsHundredths: 300, Status: snapshotStatusReady},
		{BusinessDate: windowStart.AddDate(0, 0, 4), AwardedPointsHundredths: 8_888, Status: snapshotStatusReady},
	}

	got := completeDailyPointsWindow(items, windowStart, 4)
	if len(got) != 4 {
		t.Fatalf("daily points length = %d, want 4: %+v", len(got), got)
	}
	wantPoints := []int64{100, 0, 300, 0}
	wantStatuses := []string{snapshotStatusReady, snapshotStatusEmpty, snapshotStatusReady, snapshotStatusEmpty}
	for index := range got {
		wantDate := windowStart.AddDate(0, 0, index)
		if !got[index].BusinessDate.Equal(wantDate) || got[index].AwardedPointsHundredths != wantPoints[index] ||
			got[index].Status != wantStatuses[index] {
			t.Fatalf("daily point %d = %+v, want date=%s points=%d status=%s", index, got[index],
				wantDate.Format("2006-01-02"), wantPoints[index], wantStatuses[index])
		}
	}
	if interval := got[2].BusinessDate.Sub(got[1].BusinessDate); interval != 23*time.Hour {
		t.Fatalf("DST natural-day interval = %s, want 23h", interval)
	}
}
