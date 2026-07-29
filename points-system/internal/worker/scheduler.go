package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/store"
)

const defaultSnapshotReconcileDays = 7

type UsageRefresher interface {
	RefreshUsageDay(context.Context, time.Time, string) (store.DailyRefreshResult, error)
	RefreshMinuteForDate(context.Context, time.Time) (int, error)
	UsageRefreshEnabledForDate(context.Context, time.Time) (bool, error)
}

type SnapshotScheduler struct {
	Store         UsageRefresher
	Location      *time.Location
	Logger        *slog.Logger
	Now           func() time.Time
	ReconcileDays int
}

func (s *SnapshotScheduler) Run(ctx context.Context) error {
	if s.Store == nil || s.Location == nil {
		return errors.New("snapshot scheduler is not configured")
	}
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.ReconcileDays <= 0 {
		s.ReconcileDays = defaultSnapshotReconcileDays
	}
	if err := s.refreshDueDays(ctx, s.Now(), "startup"); err != nil {
		s.Logger.Error("startup points usage reconciliation failed", "error", err)
	}
	for {
		now := s.Now()
		refreshMinute, err := s.Store.RefreshMinuteForDate(ctx, now.In(s.Location))
		if err != nil {
			s.Logger.Error("load points snapshot refresh schedule failed", "error", err)
			if !waitFor(ctx, time.Minute) {
				return nil
			}
			continue
		}
		next, err := NextSnapshotRefresh(now, s.Location, refreshMinute)
		if err != nil {
			return err
		}
		wake, scheduled := nextSchedulerWake(now, next, s.Location)
		timer := time.NewTimer(wake.Sub(now))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
			if !scheduled {
				if err := s.handlePolicyBoundary(ctx, wake); err != nil {
					s.Logger.Error("midnight points snapshot schedule failed", "error", err)
				}
				continue
			}
			if err := s.refreshDueDays(ctx, next, "scheduled"); err != nil {
				s.Logger.Error("scheduled points usage reconciliation failed", "error", err)
			}
		}
	}
}

func (s *SnapshotScheduler) handlePolicyBoundary(ctx context.Context, midnight time.Time) error {
	refreshMinute, err := s.Store.RefreshMinuteForDate(ctx, midnight.In(s.Location))
	if err != nil {
		return err
	}
	if refreshMinute != 0 {
		return nil
	}
	return s.refreshDueDays(ctx, midnight, "scheduled")
}

func nextSchedulerWake(now, next time.Time, location *time.Location) (time.Time, bool) {
	local := now.In(location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
	if midnight.Before(next) {
		return midnight, false
	}
	return next, true
}

func (s *SnapshotScheduler) refreshDueDays(ctx context.Context, now time.Time, primaryTrigger string) error {
	refreshMinute, err := s.Store.RefreshMinuteForDate(ctx, now.In(s.Location))
	if err != nil {
		return err
	}
	mostRecent, err := MostRecentSnapshotBusinessDate(now, s.Location, refreshMinute)
	if err != nil {
		return err
	}
	for offset := s.ReconcileDays - 1; offset >= 0; offset-- {
		businessDate := mostRecent.AddDate(0, 0, -offset)
		enabled, enabledErr := s.Store.UsageRefreshEnabledForDate(ctx, businessDate)
		if enabledErr != nil {
			s.Logger.Error("load points usage refresh policy failed",
				"business_date", businessDate.Format("2006-01-02"), "error", enabledErr)
			continue
		}
		if !enabled {
			s.Logger.Info("points usage snapshot refresh skipped for disabled policy",
				"business_date", businessDate.Format("2006-01-02"))
			continue
		}
		trigger := "reconcile"
		if offset == 0 {
			trigger = primaryTrigger
		}
		result, refreshErr := s.Store.RefreshUsageDay(ctx, businessDate, trigger)
		if refreshErr != nil {
			s.Logger.Error("daily points usage snapshot refresh failed",
				"business_date", businessDate.Format("2006-01-02"), "trigger", trigger,
				"error", refreshErr)
			continue
		}
		s.Logger.Info("daily points usage snapshots refreshed",
			"business_date", businessDate.Format("2006-01-02"), "trigger", trigger,
			"run_id", result.RunID, "source_users", result.Users,
			"changed_users", result.ChangedUsers, "source_rows", result.SourceRows)
	}
	return nil
}

func NextSnapshotRefresh(now time.Time, location *time.Location, refreshMinute int) (time.Time, error) {
	if location == nil || refreshMinute < 0 || refreshMinute >= 24*60 {
		return time.Time{}, errors.New("invalid points snapshot refresh schedule")
	}
	local := now.In(location)
	next := time.Date(local.Year(), local.Month(), local.Day(), refreshMinute/60,
		refreshMinute%60, 0, 0, location)
	if !next.After(local) {
		next = next.AddDate(0, 0, 1)
	}
	return next, nil
}

func MostRecentSnapshotBusinessDate(now time.Time, location *time.Location, refreshMinute int) (time.Time, error) {
	if location == nil || refreshMinute < 0 || refreshMinute >= 24*60 {
		return time.Time{}, errors.New("invalid points snapshot refresh schedule")
	}
	local := now.In(location)
	cutoff := time.Date(local.Year(), local.Month(), local.Day(), refreshMinute/60,
		refreshMinute%60, 0, 0, location)
	daysAgo := -1
	if local.Before(cutoff) {
		daysAgo = -2
	}
	date := local.AddDate(0, 0, daysAgo)
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location), nil
}

func waitFor(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
