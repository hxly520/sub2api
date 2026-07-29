package worker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/store"
)

func TestNextSnapshotRefreshUsesConfiguredShanghaiMinute(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2026, 7, 29, 2, 4, 59, 0, location)
	got, err := NextSnapshotRefresh(before, location, 2*60+5)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 7, 29, 2, 5, 0, 0, location); !got.Equal(want) {
		t.Fatalf("refresh = %s, want %s", got, want)
	}
	after := time.Date(2026, 7, 29, 2, 5, 0, 0, location)
	got, err = NextSnapshotRefresh(after, location, 2*60+5)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 7, 30, 2, 5, 0, 0, location); !got.Equal(want) {
		t.Fatalf("next-day refresh = %s, want %s", got, want)
	}
}

func TestMostRecentSnapshotBusinessDateSupportsRestartCatchup(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	before := time.Date(2026, 7, 29, 0, 4, 59, 0, location)
	got, err := MostRecentSnapshotBusinessDate(before, location, 5)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 7, 27, 0, 0, 0, 0, location); !got.Equal(want) {
		t.Fatalf("pre-cutoff business date = %s, want %s", got, want)
	}
	after := time.Date(2026, 7, 29, 0, 5, 0, 0, location)
	got, err = MostRecentSnapshotBusinessDate(after, location, 5)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 7, 28, 0, 0, 0, 0, location); !got.Equal(want) {
		t.Fatalf("post-cutoff business date = %s, want %s", got, want)
	}
}

func TestSnapshotSchedulerReconcilesConfiguredWindowOldestFirst(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	fake := &schedulerStoreStub{refreshMinute: 5}
	scheduler := &SnapshotScheduler{
		Store: fake, Location: location, ReconcileDays: 3,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	now := time.Date(2026, 7, 29, 0, 5, 0, 0, location)
	if err := scheduler.refreshDueDays(context.Background(), now, "startup"); err != nil {
		t.Fatal(err)
	}
	wantDates := []string{"2026-07-26", "2026-07-27", "2026-07-28"}
	wantTriggers := []string{"reconcile", "reconcile", "startup"}
	if len(fake.calls) != len(wantDates) {
		t.Fatalf("calls = %d, want %d", len(fake.calls), len(wantDates))
	}
	for i, call := range fake.calls {
		if got := call.date.In(location).Format("2006-01-02"); got != wantDates[i] ||
			call.trigger != wantTriggers[i] {
			t.Fatalf("call %d = %s/%s, want %s/%s", i, got, call.trigger,
				wantDates[i], wantTriggers[i])
		}
	}
}

func TestSnapshotScheduleRejectsInvalidMinute(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NextSnapshotRefresh(time.Now(), location, 24*60); err == nil {
		t.Fatal("expected invalid refresh minute error")
	}
	if _, err := MostRecentSnapshotBusinessDate(time.Now(), location, -1); err == nil {
		t.Fatal("expected invalid refresh minute error")
	}
}

func TestSchedulerRechecksVersionedScheduleAtMidnight(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 23, 0, 0, 0, location)
	oldSchedule := time.Date(2026, 7, 30, 2, 0, 0, 0, location)
	wake, scheduled := nextSchedulerWake(now, oldSchedule, location)
	if scheduled || !wake.Equal(time.Date(2026, 7, 30, 0, 0, 0, 0, location)) {
		t.Fatalf("wake=%s scheduled=%v", wake, scheduled)
	}
}

func TestSchedulerRunsPolicyWhoseRefreshMinuteIsMidnight(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	fake := &schedulerStoreStub{refreshMinute: 0}
	scheduler := &SnapshotScheduler{
		Store: fake, Location: location, ReconcileDays: 1,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	midnight := time.Date(2026, 7, 30, 0, 0, 0, 0, location)
	if err := scheduler.handlePolicyBoundary(context.Background(), midnight); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].trigger != "scheduled" ||
		fake.calls[0].date.Format("2006-01-02") != "2026-07-29" {
		t.Fatalf("unexpected midnight refresh calls: %+v", fake.calls)
	}
}

type schedulerStoreCall struct {
	date    time.Time
	trigger string
}

type schedulerStoreStub struct {
	refreshMinute int
	calls         []schedulerStoreCall
}

func (s *schedulerStoreStub) RefreshUsageDay(_ context.Context, date time.Time,
	trigger string) (store.DailyRefreshResult, error) {
	s.calls = append(s.calls, schedulerStoreCall{date: date, trigger: trigger})
	return store.DailyRefreshResult{RunID: "run"}, nil
}

func (s *schedulerStoreStub) RefreshMinuteForDate(context.Context, time.Time) (int, error) {
	return s.refreshMinute, nil
}
