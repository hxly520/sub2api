package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/jackc/pgx/v5"
)

const (
	refreshTriggerStartup   = "startup"
	refreshTriggerScheduled = "scheduled"
	refreshTriggerReconcile = "reconcile"
	refreshTriggerManual    = "manual"
	refreshTriggerBackfill  = "history_backfill"

	snapshotStatusDisabled = "disabled"
	snapshotStatusReady    = "ready"
	snapshotStatusEmpty    = "empty"
	snapshotStatusReview   = "needs_review"
)

type DailyRefreshResult struct {
	RunID                 string    `json:"run_id"`
	BusinessDate          time.Time `json:"business_date"`
	Users                 int       `json:"users"`
	SourceRows            int64     `json:"source_rows"`
	ChangedUsers          int       `json:"changed_users"`
	DeltaSpendMicroUSD    int64     `json:"delta_spend_microusd"`
	DeltaPointsHundredths int64     `json:"delta_points_hundredths"`
	SourceFingerprint     string    `json:"source_fingerprint"`
}

type usageSnapshot struct {
	UserID                  int64
	ActualCostMicroUSD      int64
	AccountedSpendMicroUSD  int64
	PolicyVersion           *int64
	PointsPerUSDHundredths  int64
	TargetPointsHundredths  int64
	AwardedPointsHundredths int64
	Revision                int
	Status                  string
	SourceRowCount          int64
	SourceMaxUsageLogID     int64
	SourceFingerprint       string
}

type snapshotTarget struct {
	UsageAggregate
	PolicyVersion          *int64
	PointsPerUSDHundredths int64
	PointsHundredths       int64
	Status                 string
}

type snapshotApplication struct {
	AccountedSpendMicroUSD  int64
	AwardedPointsHundredths int64
	Status                  string
	DeltaSpendMicroUSD      int64
	DeltaPointsHundredths   int64
}

func (s *Store) ProcessUsageDay(ctx context.Context, businessDate time.Time) (DailyRefreshResult, error) {
	return s.RefreshUsageDay(ctx, businessDate, refreshTriggerManual)
}

func (s *Store) RefreshMinuteForDate(ctx context.Context, date time.Time) (int, error) {
	policy, err := s.PolicyForDate(ctx, s.BusinessDate(date))
	if errors.Is(err, domain.ErrNotFound) {
		return 5, nil
	}
	if err != nil {
		return 0, err
	}
	if policy.RefreshMinute < 0 || policy.RefreshMinute >= 24*60 {
		return 0, domain.ErrPolicyIncomplete
	}
	return policy.RefreshMinute, nil
}

// UsageRefreshEnabledForDate prevents the automatic scheduler from scanning
// Sub2API usage data before the versioned points policy is active.
func (s *Store) UsageRefreshEnabledForDate(ctx context.Context, date time.Time) (bool, error) {
	policy, err := usageAccountingPolicyForDate(ctx, s.DB, s.BusinessDate(date))
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return policy.Enabled, nil
}

func (s *Store) RefreshUsageDay(ctx context.Context, businessDate time.Time, trigger string) (DailyRefreshResult, error) {
	if s == nil || s.DB == nil || s.Location == nil || s.UsageSource == nil {
		return DailyRefreshResult{}, errors.New("usage snapshot refresh is not configured")
	}
	if !validRefreshTrigger(trigger) {
		return DailyRefreshResult{}, fmt.Errorf("invalid usage snapshot refresh trigger")
	}
	date := s.BusinessDate(businessDate)
	windowStart := date.UTC()
	windowEnd := date.AddDate(0, 0, 1).UTC()
	enabled, err := s.UsageRefreshEnabledForDate(ctx, date)
	if err != nil {
		return DailyRefreshResult{}, fmt.Errorf("load usage refresh policy: %w", err)
	}
	if !enabled {
		return s.markUsageDayReadyWithoutSource(ctx, date, trigger)
	}
	runID := uuid.NewString()
	result := DailyRefreshResult{RunID: runID, BusinessDate: date}

	if _, err := s.DB.Exec(ctx, `INSERT INTO points_snapshot_refresh_runs(
		id,business_date,trigger,source_window_start,source_window_end,status
	) VALUES($1,$2,$3,$4,$5,'running')`, runID, dateString(date), trigger, windowStart, windowEnd); err != nil {
		return DailyRefreshResult{}, fmt.Errorf("start usage snapshot refresh audit: %w", err)
	}

	usageDay, err := s.UsageSource.AggregateDay(ctx, windowStart, windowEnd)
	if err != nil {
		s.failRefreshRun(runID, err)
		return DailyRefreshResult{}, err
	}
	if !usageDay.WindowStart.Equal(windowStart) || !usageDay.WindowEnd.Equal(windowEnd) ||
		len(usageDay.Fingerprint) != 64 {
		err := errors.New("usage source returned an invalid natural-day result")
		s.failRefreshRun(runID, err)
		return DailyRefreshResult{}, err
	}
	result.Users = len(usageDay.Aggregates)
	result.SourceRows = usageDay.SourceRows
	result.SourceFingerprint = usageDay.Fingerprint

	err = s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		result.ChangedUsers = 0
		result.DeltaSpendMicroUSD = 0
		result.DeltaPointsHundredths = 0
		return s.applyUsageDayTx(ctx, tx, runID, trigger, date, usageDay, &result)
	})
	if err == nil {
		return result, nil
	}
	s.failRefreshRun(runID, err)
	return DailyRefreshResult{}, fmt.Errorf("apply usage snapshot refresh: %w", err)
}

func (s *Store) markUsageDayReadyWithoutSource(ctx context.Context, date time.Time,
	trigger string) (DailyRefreshResult, error) {
	windowStart := date.UTC()
	windowEnd := date.AddDate(0, 0, 1).UTC()
	result := DailyRefreshResult{
		BusinessDate: date,
		SourceFingerprint: security.Fingerprint("points-policy-disabled", dateString(date),
			windowStart.UnixMicro(), windowEnd.UnixMicro()),
	}
	err := s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		result.RunID = ""
		result.Users = 0
		result.SourceRows = 0
		result.ChangedUsers = 0
		result.DeltaSpendMicroUSD = 0
		result.DeltaPointsHundredths = 0
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			"points-snapshot-refresh:"+dateString(date)); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT id,source_fingerprint,source_users,source_rows,
			changed_users,delta_spend_microusd,delta_points_hundredths
			FROM points_snapshot_refresh_runs
			WHERE business_date=$1 AND status='succeeded'
			ORDER BY completed_at DESC,created_at DESC LIMIT 1`, dateString(date)).Scan(
			&result.RunID, &result.SourceFingerprint, &result.Users, &result.SourceRows,
			&result.ChangedUsers, &result.DeltaSpendMicroUSD, &result.DeltaPointsHundredths)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		result.RunID = uuid.NewString()
		_, err = tx.Exec(ctx, `INSERT INTO points_snapshot_refresh_runs(
			id,business_date,trigger,source_window_start,source_window_end,source_fingerprint,
			source_users,source_rows,changed_users,delta_spend_microusd,delta_points_hundredths,
			status,completed_at
		) VALUES($1,$2,$3,$4,$5,$6,0,0,0,0,0,'succeeded',NOW())`, result.RunID,
			dateString(date), trigger, windowStart, windowEnd, result.SourceFingerprint)
		return err
	})
	if err != nil {
		return DailyRefreshResult{}, fmt.Errorf("mark disabled usage day ready: %w", err)
	}
	return result, nil
}

func (s *Store) applyUsageDayTx(ctx context.Context, tx pgx.Tx, runID, trigger string, date time.Time,
	usageDay UsageDay, result *DailyRefreshResult) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"points-snapshot-refresh:"+dateString(date)); err != nil {
		return err
	}
	policy, err := usageAccountingPolicyForDate(ctx, tx, date)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if errors.Is(err, domain.ErrNotFound) {
		policy = domain.Policy{}
	}
	return s.applyUsageDayLockedTx(ctx, tx, runID, trigger, date, usageDay, policy, result)
}

func (s *Store) applyUsageDayLockedTx(ctx context.Context, tx pgx.Tx, runID, trigger string,
	date time.Time, usageDay UsageDay, policy domain.Policy, result *DailyRefreshResult) error {
	existing, err := loadUsageSnapshotsTx(ctx, tx, date)
	if err != nil {
		return err
	}
	targets := make(map[int64]UsageAggregate, len(existing)+len(usageDay.Aggregates))
	for userID := range existing {
		targets[userID] = UsageAggregate{UserID: userID}
	}
	seenSourceUsers := make(map[int64]struct{}, len(usageDay.Aggregates))
	for _, item := range usageDay.Aggregates {
		if _, duplicate := seenSourceUsers[item.UserID]; duplicate {
			return fmt.Errorf("duplicate Sub2API aggregate for user %d", item.UserID)
		}
		seenSourceUsers[item.UserID] = struct{}{}
		targets[item.UserID] = item
	}
	userIDs := make([]int64, 0, len(targets))
	for userID := range targets {
		userIDs = append(userIDs, userID)
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })

	for _, userID := range userIDs {
		target, err := makeSnapshotTarget(targets[userID], policy)
		if err != nil {
			return err
		}
		changed, deltaSpend, deltaPoints, err := applyUsageSnapshotTx(ctx, tx, runID, trigger,
			date, existing[userID], target)
		if err != nil {
			return err
		}
		if changed {
			result.ChangedUsers++
			result.DeltaSpendMicroUSD, err = checkedAddSigned(result.DeltaSpendMicroUSD, deltaSpend)
			if err != nil {
				return err
			}
			result.DeltaPointsHundredths, err = checkedAddSigned(result.DeltaPointsHundredths, deltaPoints)
			if err != nil {
				return err
			}
		}
	}
	_, err = tx.Exec(ctx, `UPDATE points_snapshot_refresh_runs SET
		source_fingerprint=$1,source_users=$2,source_rows=$3,changed_users=$4,
		delta_spend_microusd=$5,delta_points_hundredths=$6,status='succeeded',completed_at=NOW()
		WHERE id=$7 AND status='running'`, usageDay.Fingerprint, len(usageDay.Aggregates), usageDay.SourceRows,
		result.ChangedUsers, result.DeltaSpendMicroUSD, result.DeltaPointsHundredths, runID)
	return err
}

func loadUsageSnapshotsTx(ctx context.Context, tx pgx.Tx, date time.Time) (map[int64]usageSnapshot, error) {
	rows, err := tx.Query(ctx, `SELECT user_id,actual_cost_microusd,accounted_spend_microusd,policy_version,
		points_per_usd_hundredths,target_points_hundredths,awarded_points_hundredths,
		revision,status,source_row_count,source_max_usage_log_id,COALESCE(source_fingerprint,'')
		FROM points_daily_snapshots WHERE business_date=$1 ORDER BY user_id FOR UPDATE`, dateString(date))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]usageSnapshot)
	for rows.Next() {
		var item usageSnapshot
		if err := rows.Scan(&item.UserID, &item.ActualCostMicroUSD, &item.AccountedSpendMicroUSD,
			&item.PolicyVersion,
			&item.PointsPerUSDHundredths, &item.TargetPointsHundredths,
			&item.AwardedPointsHundredths, &item.Revision, &item.Status,
			&item.SourceRowCount, &item.SourceMaxUsageLogID, &item.SourceFingerprint); err != nil {
			return nil, err
		}
		result[item.UserID] = item
	}
	return result, rows.Err()
}

func makeSnapshotTarget(aggregate UsageAggregate, policy domain.Policy) (snapshotTarget, error) {
	target := snapshotTarget{UsageAggregate: aggregate, Status: snapshotStatusEmpty}
	if aggregate.UserID <= 0 || aggregate.ActualCostMicroUSD < 0 || aggregate.SourceRowCount < 0 ||
		aggregate.SourceMaxUsageLogID < 0 ||
		(aggregate.SourceFingerprint != "" && len(aggregate.SourceFingerprint) != 64) {
		return snapshotTarget{}, fmt.Errorf("invalid usage aggregate")
	}
	if aggregate.SourceFingerprint == "" {
		target.SourceFingerprint = security.Fingerprint(aggregate.UserID, aggregate.ActualCostMicroUSD,
			aggregate.SourceRowCount, aggregate.SourceMaxUsageLogID)
	}
	if aggregate.ActualCostMicroUSD == 0 {
		return target, nil
	}
	if !policy.Enabled || policy.VersionNo <= 0 {
		target.Status = snapshotStatusDisabled
		if policy.VersionNo > 0 {
			version := policy.VersionNo
			target.PolicyVersion = &version
		}
		return target, nil
	}
	if policy.PointsPerUSDHundredths <= 0 {
		return snapshotTarget{}, domain.ErrPolicyIncomplete
	}
	points, err := pointsForSpend(aggregate.ActualCostMicroUSD, policy.PointsPerUSDHundredths)
	if err != nil {
		return snapshotTarget{}, err
	}
	version := policy.VersionNo
	target.PolicyVersion = &version
	target.PointsPerUSDHundredths = policy.PointsPerUSDHundredths
	target.PointsHundredths = points
	target.Status = snapshotStatusReady
	return target, nil
}

func applyUsageSnapshotTx(ctx context.Context, tx pgx.Tx, runID, trigger string, date time.Time,
	previous usageSnapshot, target snapshotTarget) (bool, int64, int64, error) {
	if previous.UserID != 0 && snapshotMatches(previous, target) {
		return false, 0, 0, nil
	}
	revision := 1
	if previous.UserID != 0 {
		revision = previous.Revision + 1
	}
	application := reconcileSnapshot(previous, target)
	if _, err := tx.Exec(ctx, `INSERT INTO points_accounts(user_id) VALUES($1)
		ON CONFLICT(user_id) DO NOTHING`, target.UserID); err != nil {
		return false, 0, 0, err
	}
	var currentTotalPoints, currentTotalSpend int64
	if err := tx.QueryRow(ctx, `SELECT total_points_hundredths,total_spend_microusd
		FROM points_accounts WHERE user_id=$1 FOR UPDATE`, target.UserID).Scan(
		&currentTotalPoints, &currentTotalSpend); err != nil {
		return false, 0, 0, err
	}
	application, err := guardAccountTotals(previous, application, currentTotalPoints, currentTotalSpend)
	if err != nil {
		return false, 0, 0, err
	}
	if pendingReviewUnchanged(previous, target, application) {
		return false, 0, 0, nil
	}
	deltaSpend := application.DeltaSpendMicroUSD
	deltaPoints := application.DeltaPointsHundredths
	accountedSpend := application.AccountedSpendMicroUSD
	awardedPoints := application.AwardedPointsHundredths
	status := application.Status

	if previous.UserID == 0 {
		_, err := tx.Exec(ctx, `INSERT INTO points_daily_snapshots(
			user_id,business_date,actual_cost_microusd,accounted_spend_microusd,policy_version,points_per_usd_hundredths,
			target_points_hundredths,awarded_points_hundredths,revision,status,source_row_count,
			source_max_usage_log_id,source_fingerprint,last_refresh_run_id
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, target.UserID,
			dateString(date), target.ActualCostMicroUSD, accountedSpend, target.PolicyVersion,
			target.PointsPerUSDHundredths, target.PointsHundredths, awardedPoints, revision, status,
			target.SourceRowCount, target.SourceMaxUsageLogID, target.SourceFingerprint, runID)
		if err != nil {
			return false, 0, 0, err
		}
	} else {
		_, err := tx.Exec(ctx, `UPDATE points_daily_snapshots SET actual_cost_microusd=$1,
			accounted_spend_microusd=$2,policy_version=$3,points_per_usd_hundredths=$4,
			target_points_hundredths=$5,awarded_points_hundredths=$6,revision=$7,status=$8,
			source_row_count=$9,source_max_usage_log_id=$10,source_fingerprint=$11,
			last_refresh_run_id=$12,updated_at=NOW() WHERE user_id=$13 AND business_date=$14`,
			target.ActualCostMicroUSD, accountedSpend, target.PolicyVersion,
			target.PointsPerUSDHundredths, target.PointsHundredths, awardedPoints,
			revision, status, target.SourceRowCount, target.SourceMaxUsageLogID,
			target.SourceFingerprint, runID, target.UserID, dateString(date))
		if err != nil {
			return false, 0, 0, err
		}
	}

	var totalPoints, totalSpend int64
	if err := tx.QueryRow(ctx, `UPDATE points_accounts SET
		total_points_hundredths=total_points_hundredths+$1,
		total_spend_microusd=total_spend_microusd+$2,updated_at=NOW()
		WHERE user_id=$3 RETURNING total_points_hundredths,total_spend_microusd`,
		deltaPoints, deltaSpend, target.UserID).Scan(&totalPoints, &totalSpend); err != nil {
		return false, 0, 0, err
	}
	if totalPoints < 0 || totalSpend < 0 {
		return false, 0, 0, errors.New("usage snapshot correction would make account totals negative")
	}

	reason := snapshotRevisionReason(previous, target)
	if _, err := tx.Exec(ctx, `INSERT INTO points_daily_snapshot_revisions(
		user_id,business_date,revision,actual_cost_microusd,reason,refresh_run_id,
		previous_actual_cost_microusd,delta_actual_cost_microusd,
		previous_accounted_spend_microusd,accounted_spend_microusd,policy_version,
		points_per_usd_hundredths,target_points_hundredths,previous_awarded_points_hundredths,
		awarded_points_hundredths,delta_points_hundredths,status,source_row_count,
		source_max_usage_log_id,source_fingerprint
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		target.UserID, dateString(date), revision, target.ActualCostMicroUSD, reason, runID,
		nullablePreviousSpend(previous), target.ActualCostMicroUSD-previous.ActualCostMicroUSD,
		nullablePreviousAccountedSpend(previous), accountedSpend, target.PolicyVersion,
		target.PointsPerUSDHundredths, target.PointsHundredths,
		nullablePreviousPoints(previous), awardedPoints, deltaPoints, status,
		target.SourceRowCount, target.SourceMaxUsageLogID, target.SourceFingerprint); err != nil {
		return false, 0, 0, err
	}

	if deltaPoints != 0 {
		metadata, _ := json.Marshal(map[string]any{
			"refresh_run_id": runID, "trigger": trigger, "revision": revision,
			"actual_cost_microusd":       target.ActualCostMicroUSD,
			"delta_actual_cost_microusd": target.ActualCostMicroUSD - previous.ActualCostMicroUSD,
			"points_per_usd_hundredths":  target.PointsPerUSDHundredths,
			"source_row_count":           target.SourceRowCount,
			"source_max_usage_log_id":    target.SourceMaxUsageLogID,
			"source_fingerprint":         target.SourceFingerprint,
		})
		eventID := fmt.Sprintf("%d:%s:r%d", target.UserID, dateString(date), revision)
		fingerprint := security.Fingerprint(target.UserID, dateString(date), revision, deltaPoints,
			totalPoints, runID, target.SourceFingerprint)
		if _, err := tx.Exec(ctx, `INSERT INTO points_ledger(
			user_id,kind,delta_points_hundredths,total_after_hundredths,source,
			external_event_id,request_fingerprint,policy_version,business_date,reference_id,metadata
		) VALUES($1,'usage_points',$2,$3,'usage_snapshot',$4,$5,$6,$7,$8,$9)`,
			target.UserID, deltaPoints, totalPoints, eventID, fingerprint, target.PolicyVersion,
			dateString(date), runID, metadata); err != nil {
			return false, 0, 0, err
		}
	}
	return true, deltaSpend, deltaPoints, nil
}

func snapshotMatches(previous usageSnapshot, target snapshotTarget) bool {
	return previous.ActualCostMicroUSD == target.ActualCostMicroUSD &&
		optionalInt64Equal(previous.PolicyVersion, target.PolicyVersion) &&
		previous.PointsPerUSDHundredths == target.PointsPerUSDHundredths &&
		previous.TargetPointsHundredths == target.PointsHundredths &&
		previous.AwardedPointsHundredths == target.PointsHundredths &&
		previous.AccountedSpendMicroUSD == countedTargetSpend(target) &&
		previous.Status == target.Status &&
		previous.SourceRowCount == target.SourceRowCount &&
		previous.SourceMaxUsageLogID == target.SourceMaxUsageLogID &&
		previous.SourceFingerprint == target.SourceFingerprint
}

func pendingReviewUnchanged(previous usageSnapshot, target snapshotTarget,
	application snapshotApplication) bool {
	return previous.Status == snapshotStatusReview && application.Status == snapshotStatusReview &&
		previous.ActualCostMicroUSD == target.ActualCostMicroUSD &&
		optionalInt64Equal(previous.PolicyVersion, target.PolicyVersion) &&
		previous.PointsPerUSDHundredths == target.PointsPerUSDHundredths &&
		previous.TargetPointsHundredths == target.PointsHundredths &&
		previous.AwardedPointsHundredths == application.AwardedPointsHundredths &&
		previous.AccountedSpendMicroUSD == application.AccountedSpendMicroUSD &&
		previous.SourceRowCount == target.SourceRowCount &&
		previous.SourceMaxUsageLogID == target.SourceMaxUsageLogID &&
		previous.SourceFingerprint == target.SourceFingerprint
}

func countedTargetSpend(target snapshotTarget) int64 {
	if target.Status == snapshotStatusReady {
		return target.ActualCostMicroUSD
	}
	return 0
}

func reconcileSnapshot(previous usageSnapshot, target snapshotTarget) snapshotApplication {
	targetSpend := countedTargetSpend(target)
	application := snapshotApplication{
		AccountedSpendMicroUSD:  targetSpend,
		AwardedPointsHundredths: target.PointsHundredths,
		Status:                  target.Status,
		DeltaSpendMicroUSD:      targetSpend - previous.AccountedSpendMicroUSD,
		DeltaPointsHundredths:   target.PointsHundredths - previous.AwardedPointsHundredths,
	}
	return application
}

func guardAccountTotals(previous usageSnapshot, application snapshotApplication,
	currentTotalPoints, currentTotalSpend int64) (snapshotApplication, error) {
	nextPoints, err := checkedAddSigned(currentTotalPoints, application.DeltaPointsHundredths)
	if err != nil {
		return snapshotApplication{}, err
	}
	nextSpend, err := checkedAddSigned(currentTotalSpend, application.DeltaSpendMicroUSD)
	if err != nil {
		return snapshotApplication{}, err
	}
	if nextPoints >= 0 && nextSpend >= 0 {
		return application, nil
	}
	application.AccountedSpendMicroUSD = previous.AccountedSpendMicroUSD
	application.AwardedPointsHundredths = previous.AwardedPointsHundredths
	application.Status = snapshotStatusReview
	application.DeltaSpendMicroUSD = 0
	application.DeltaPointsHundredths = 0
	return application, nil
}

func pointsForSpend(actualCostMicroUSD, pointsPerUSDHundredths int64) (int64, error) {
	if actualCostMicroUSD < 0 || pointsPerUSDHundredths <= 0 {
		return 0, fmt.Errorf("invalid points conversion")
	}
	product := new(big.Int).Mul(big.NewInt(actualCostMicroUSD), big.NewInt(pointsPerUSDHundredths))
	product.Quo(product, big.NewInt(1_000_000))
	if !product.IsInt64() {
		return 0, fmt.Errorf("points conversion overflow")
	}
	return product.Int64(), nil
}

func snapshotRevisionReason(previous usageSnapshot, target snapshotTarget) string {
	if previous.UserID == 0 {
		return "initial"
	}
	if previous.Status == snapshotStatusReview {
		return "review_resolved"
	}
	if previous.ActualCostMicroUSD < target.ActualCostMicroUSD {
		return "late_usage"
	}
	if previous.ActualCostMicroUSD > target.ActualCostMicroUSD {
		return "source_correction"
	}
	if !optionalInt64Equal(previous.PolicyVersion, target.PolicyVersion) ||
		previous.PointsPerUSDHundredths != target.PointsPerUSDHundredths ||
		previous.Status != target.Status {
		return "policy_recalculation"
	}
	return "source_recomposition"
}

func nullablePreviousSpend(previous usageSnapshot) any {
	if previous.UserID == 0 {
		return nil
	}
	return previous.ActualCostMicroUSD
}

func nullablePreviousPoints(previous usageSnapshot) any {
	if previous.UserID == 0 {
		return nil
	}
	return previous.AwardedPointsHundredths
}

func nullablePreviousAccountedSpend(previous usageSnapshot) any {
	if previous.UserID == 0 {
		return nil
	}
	return previous.AccountedSpendMicroUSD
}

func optionalInt64Equal(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func checkedAddSigned(left, right int64) (int64, error) {
	result := left + right
	if (right > 0 && result < left) || (right < 0 && result > left) {
		return 0, fmt.Errorf("daily refresh aggregate overflow")
	}
	return result, nil
}

func validRefreshTrigger(trigger string) bool {
	switch trigger {
	case refreshTriggerStartup, refreshTriggerScheduled, refreshTriggerReconcile, refreshTriggerManual,
		refreshTriggerBackfill:
		return true
	default:
		return false
	}
}

func (s *Store) failRefreshRun(runID string, refreshErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	message := "unknown refresh failure"
	if refreshErr != nil {
		message = strings.TrimSpace(refreshErr.Error())
	}
	if message == "" {
		message = "unknown refresh failure"
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	_, _ = s.DB.Exec(ctx, `UPDATE points_snapshot_refresh_runs SET status='failed',
		error_message=$1,completed_at=NOW() WHERE id=$2 AND status='running'`, message, runID)
}
