package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/jackc/pgx/v5"
)

const historyBackfillLock = "points-usage-history-backfill"

var (
	ErrHistoryBackfillBusy        = errors.New("another usage history backfill step is running")
	ErrHistoryBackfillNeedsResume = errors.New("usage history backfill is failed and must be resumed")
	ErrHistoryBackfillPlanDrift   = errors.New("usage history no longer matches the confirmed dry-run plan")
)

type usageHistorySource interface {
	UsageSource
	SuccessfulUsageBounds(context.Context) (SuccessfulUsageBounds, error)
	SummarizeHistory(context.Context, time.Time, time.Time, string, int64) (UsageHistorySummary, error)
}

func initialHistoryBackfillPending(ctx context.Context, q queryer) (bool, error) {
	var pending bool
	err := q.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM points_admin_audit audit
		WHERE audit.action='policy.initial_activate'
		  AND audit.target_type='policy'
		  AND NOT EXISTS (
			SELECT 1 FROM points_usage_history_backfill_jobs job
			WHERE job.policy_version = audit.target_id::bigint
			  AND job.status='succeeded'
		  )
	)`).Scan(&pending)
	return pending, err
}

// policyCivilDate preserves the DATE value returned by PostgreSQL. pgx may
// represent a DATE as UTC midnight; converting that instant into a west-of-UTC
// location would incorrectly move the policy one calendar day backward.
func policyCivilDate(value time.Time, location *time.Location) time.Time {
	if location == nil {
		location = time.UTC
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

type HistoryBackfillPlan struct {
	FromDate                time.Time `json:"from_date"`
	ThroughDate             time.Time `json:"through_date"`
	CalendarDays            int       `json:"calendar_days"`
	PolicyVersion           int64     `json:"policy_version"`
	PointsPerUSDHundredths  int64     `json:"points_per_usd_hundredths"`
	SourceUsers             int64     `json:"source_users"`
	SourceUserDays          int64     `json:"source_user_days"`
	SourceBusinessDays      int64     `json:"source_business_days"`
	SourceRows              int64     `json:"source_rows"`
	SpendMicroUSD           int64     `json:"spend_microusd"`
	PointsHundredths        int64     `json:"points_hundredths"`
	SourceMaxUsageLogID     int64     `json:"source_max_usage_log_id"`
	ConfirmationFingerprint string    `json:"confirmation_fingerprint"`
}

type HistoryBackfillJob struct {
	ID                         string     `json:"id"`
	PolicyVersion              int64      `json:"policy_version"`
	PointsPerUSDHundredths     int64      `json:"points_per_usd_hundredths"`
	FromDate                   time.Time  `json:"from_date"`
	ThroughDate                time.Time  `json:"through_date"`
	NextDate                   time.Time  `json:"next_date"`
	PlanFingerprint            string     `json:"plan_fingerprint"`
	PlannedSourceUsers         int64      `json:"planned_source_users"`
	PlannedSourceUserDays      int64      `json:"planned_source_user_days"`
	PlannedSourceBusinessDays  int64      `json:"planned_source_business_days"`
	PlannedSourceRows          int64      `json:"planned_source_rows"`
	PlannedSpendMicroUSD       int64      `json:"planned_spend_microusd"`
	PlannedPointsHundredths    int64      `json:"planned_points_hundredths"`
	PlannedSourceMaxUsageLogID int64      `json:"planned_source_max_usage_log_id"`
	CompletedDays              int        `json:"completed_days"`
	AppliedSourceUserDays      int64      `json:"applied_source_user_days"`
	AppliedSourceBusinessDays  int64      `json:"applied_source_business_days"`
	AppliedSourceRows          int64      `json:"applied_source_rows"`
	AppliedSourceMaxUsageLogID int64      `json:"applied_source_max_usage_log_id"`
	ChangedUsers               int64      `json:"changed_users"`
	DeltaSpendMicroUSD         int64      `json:"delta_spend_microusd"`
	DeltaPointsHundredths      int64      `json:"delta_points_hundredths"`
	Status                     string     `json:"status"`
	ErrorMessage               string     `json:"error_message,omitempty"`
	CreatedBy                  int64      `json:"created_by"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
	CompletedAt                *time.Time `json:"completed_at,omitempty"`
}

func (s *Store) PlanHistoryBackfill(ctx context.Context, from, through time.Time,
	policyVersion int64, now time.Time) (HistoryBackfillPlan, error) {
	if s == nil || s.DB == nil || s.Location == nil {
		return HistoryBackfillPlan{}, errors.New("history backfill is not configured")
	}
	source, ok := s.UsageSource.(usageHistorySource)
	if !ok || source == nil {
		return HistoryBackfillPlan{}, errors.New("usage source does not support history planning")
	}
	policy, err := policyForVersion(ctx, s.DB, policyVersion)
	if err != nil {
		return HistoryBackfillPlan{}, fmt.Errorf("load history backfill policy: %w", err)
	}
	if err := validateHistoryBackfillPolicy(policy); err != nil {
		return HistoryBackfillPlan{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	today := s.BusinessDate(now)
	if through.IsZero() {
		through = today.AddDate(0, 0, -1)
	} else {
		through = s.BusinessDate(through)
	}
	if from.IsZero() {
		bounds, boundsErr := source.SuccessfulUsageBounds(ctx)
		if boundsErr != nil {
			return HistoryBackfillPlan{}, boundsErr
		}
		if !bounds.Found {
			// A new installation still needs a completed baseline before the
			// initial policy can be exposed. Use one completed empty day so the
			// same audited apply path can close that gate without inventing spend.
			from = through
		} else {
			from = s.BusinessDate(bounds.EarliestUTC)
		}
	} else {
		from = s.BusinessDate(from)
	}
	if through.Before(from) {
		return HistoryBackfillPlan{}, errors.New("history backfill through date precedes from date")
	}
	if !through.Before(today) {
		return HistoryBackfillPlan{}, errors.New("history backfill may only include completed natural days")
	}
	policyEffectiveDate := policyCivilDate(policy.EffectiveDate, s.Location)
	if !through.Before(policyEffectiveDate) {
		return HistoryBackfillPlan{}, errors.New("history backfill must end before its policy effective date")
	}

	summary, err := source.SummarizeHistory(ctx, from.UTC(), through.AddDate(0, 0, 1).UTC(),
		s.Location.String(), policy.PointsPerUSDHundredths)
	if err != nil {
		return HistoryBackfillPlan{}, err
	}
	plan := HistoryBackfillPlan{
		FromDate: from, ThroughDate: through, CalendarDays: inclusiveCalendarDays(from, through),
		PolicyVersion: policy.VersionNo, PointsPerUSDHundredths: policy.PointsPerUSDHundredths,
		SourceUsers: summary.SourceUsers, SourceUserDays: summary.SourceUserDays,
		SourceBusinessDays: summary.SourceBusinessDays, SourceRows: summary.SourceRows,
		SpendMicroUSD: summary.SpendMicroUSD, PointsHundredths: summary.PointsHundredths,
		SourceMaxUsageLogID: summary.SourceMaxUsageLogID,
	}
	if err := validateHistoryBackfillPlan(plan); err != nil {
		return HistoryBackfillPlan{}, err
	}
	plan.ConfirmationFingerprint = historyBackfillPlanFingerprint(plan)
	return plan, nil
}

func (s *Store) CreateHistoryBackfillJob(ctx context.Context, plan HistoryBackfillPlan,
	actorUserID int64, now time.Time) (HistoryBackfillJob, error) {
	if actorUserID <= 0 {
		return HistoryBackfillJob{}, errors.New("history backfill actor user ID must be positive")
	}
	if err := validateHistoryBackfillPlan(plan); err != nil {
		return HistoryBackfillJob{}, err
	}
	if plan.ConfirmationFingerprint != historyBackfillPlanFingerprint(plan) {
		return HistoryBackfillJob{}, errors.New("history backfill plan fingerprint is invalid")
	}
	policy, err := policyForVersion(ctx, s.DB, plan.PolicyVersion)
	if err != nil {
		return HistoryBackfillJob{}, fmt.Errorf("load history backfill policy: %w", err)
	}
	if err := validateHistoryBackfillPolicy(policy); err != nil {
		return HistoryBackfillJob{}, err
	}
	if policy.PointsPerUSDHundredths != plan.PointsPerUSDHundredths {
		return HistoryBackfillJob{}, errors.New("history backfill plan no longer matches its policy")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if !plan.ThroughDate.Before(s.BusinessDate(now)) ||
		!plan.ThroughDate.Before(policyCivilDate(policy.EffectiveDate, s.Location)) {
		return HistoryBackfillJob{}, errors.New("history backfill plan contains an ineligible date")
	}

	jobID := uuid.NewString()
	var job HistoryBackfillJob
	err = s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		if _, lockErr := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			historyBackfillLock); lockErr != nil {
			return lockErr
		}
		// A successful baseline is immutable and a failed/running baseline must
		// be resumed. Reusing the exact existing job keeps a repeated CLI apply
		// idempotent instead of creating a poison job at the daily-date PK.
		var existingID, existingStatus, existingFingerprint string
		var existingPolicyVersion int64
		var existingFrom, existingThrough string
		existingErr := tx.QueryRow(ctx, `SELECT id::text,status,policy_version,
			from_date::text,through_date::text,plan_fingerprint
			FROM points_usage_history_backfill_jobs
			ORDER BY created_at ASC LIMIT 1`).Scan(&existingID, &existingStatus,
			&existingPolicyVersion, &existingFrom, &existingThrough, &existingFingerprint)
		if existingErr == nil {
			exactPlan := existingPolicyVersion == plan.PolicyVersion &&
				existingFrom == dateString(plan.FromDate) &&
				existingThrough == dateString(plan.ThroughDate) &&
				existingFingerprint == plan.ConfirmationFingerprint
			switch existingStatus {
			case "succeeded":
				if !exactPlan {
					return errors.New("usage history baseline already completed with a different plan")
				}
				var loadErr error
				job, loadErr = s.loadHistoryBackfillJob(ctx, tx, existingID, false)
				return loadErr
			case "failed":
				return ErrHistoryBackfillNeedsResume
			case "running":
				return ErrHistoryBackfillBusy
			default:
				return fmt.Errorf("history backfill job has invalid status %q", existingStatus)
			}
		}
		if !errors.Is(existingErr, pgx.ErrNoRows) {
			return existingErr
		}
		_, insertErr := tx.Exec(ctx, `INSERT INTO points_usage_history_backfill_jobs(
			id,policy_version,points_per_usd_hundredths,from_date,through_date,next_date,
			plan_fingerprint,planned_source_users,planned_source_user_days,
			planned_source_business_days,planned_source_rows,planned_spend_microusd,
			planned_points_hundredths,planned_source_max_usage_log_id,
			status,created_by
		) VALUES($1,$2,$3,$4,$5,$4,$6,$7,$8,$9,$10,$11,$12,$13,'running',$14)`, jobID,
			plan.PolicyVersion, plan.PointsPerUSDHundredths, dateString(plan.FromDate),
			dateString(plan.ThroughDate), plan.ConfirmationFingerprint, plan.SourceUsers,
			plan.SourceUserDays, plan.SourceBusinessDays, plan.SourceRows, plan.SpendMicroUSD,
			plan.PointsHundredths, plan.SourceMaxUsageLogID, actorUserID)
		if insertErr != nil {
			return insertErr
		}
		if _, insertErr = tx.Exec(ctx, `INSERT INTO points_admin_audit(
			actor_user_id,action,target_type,target_id,detail
		) VALUES($1,'usage_history_backfill.create','usage_history_backfill',$2,
			jsonb_build_object('from_date',$3::text,'through_date',$4::text,'policy_version',$5::bigint,
			'points_per_usd_hundredths',$6::bigint,'plan_fingerprint',$7::text))`, actorUserID, jobID,
			dateString(plan.FromDate), dateString(plan.ThroughDate), plan.PolicyVersion,
			plan.PointsPerUSDHundredths, plan.ConfirmationFingerprint); insertErr != nil {
			return insertErr
		}
		var loadErr error
		job, loadErr = s.loadHistoryBackfillJob(ctx, tx, jobID, false)
		return loadErr
	})
	if err != nil {
		return HistoryBackfillJob{}, fmt.Errorf("create history backfill job: %w", err)
	}
	return job, nil
}

func (s *Store) GetHistoryBackfillJob(ctx context.Context, jobID string) (HistoryBackfillJob, error) {
	if _, err := uuid.Parse(jobID); err != nil {
		return HistoryBackfillJob{}, domain.ErrNotFound
	}
	return s.loadHistoryBackfillJob(ctx, s.DB, jobID, false)
}

// UserAccessReadyForPolicy keeps the one-time initial activation private until
// its confirmed history job has completed. Ordinary policy versions do not
// require a history job and retain their existing access semantics.
func (s *Store) UserAccessReadyForPolicy(ctx context.Context, policyVersion int64) (bool, error) {
	if s == nil || s.DB == nil || policyVersion <= 0 {
		return false, errors.New("invalid points policy readiness request")
	}
	// The initial baseline is a global one-time gate. Checking only the
	// currently effective policy would let a future policy bypass the gate at
	// midnight while the original history job is still running.
	pending, err := initialHistoryBackfillPending(ctx, s.DB)
	if err != nil {
		return false, err
	}
	return !pending, nil
}

func (s *Store) ResumeHistoryBackfillJob(ctx context.Context, jobID string,
	actorUserID int64) (HistoryBackfillJob, error) {
	if actorUserID <= 0 {
		return HistoryBackfillJob{}, errors.New("history backfill actor user ID must be positive")
	}
	if _, err := uuid.Parse(jobID); err != nil {
		return HistoryBackfillJob{}, domain.ErrNotFound
	}
	var job HistoryBackfillJob
	err := s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		if _, lockErr := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			historyBackfillLock); lockErr != nil {
			return lockErr
		}
		current, loadErr := s.loadHistoryBackfillJob(ctx, tx, jobID, true)
		if loadErr != nil {
			return loadErr
		}
		if current.Status == "failed" {
			if _, updateErr := tx.Exec(ctx, `UPDATE points_usage_history_backfill_jobs
				SET status='running',error_message=NULL,updated_at=NOW() WHERE id=$1`, jobID); updateErr != nil {
				return updateErr
			}
			if _, auditErr := tx.Exec(ctx, `INSERT INTO points_admin_audit(
				actor_user_id,action,target_type,target_id,detail
			) VALUES($1,'usage_history_backfill.resume','usage_history_backfill',$2,
				jsonb_build_object('next_date',$3::text))`, actorUserID, jobID,
				dateString(current.NextDate)); auditErr != nil {
				return auditErr
			}
		}
		job, loadErr = s.loadHistoryBackfillJob(ctx, tx, jobID, false)
		return loadErr
	})
	if err != nil {
		return HistoryBackfillJob{}, fmt.Errorf("resume history backfill job: %w", err)
	}
	return job, nil
}

func (s *Store) ProcessHistoryBackfillDay(ctx context.Context,
	jobID string) (HistoryBackfillJob, DailyRefreshResult, bool, error) {
	if _, err := uuid.Parse(jobID); err != nil {
		return HistoryBackfillJob{}, DailyRefreshResult{}, false, domain.ErrNotFound
	}
	lockConn, err := s.DB.Acquire(ctx)
	if err != nil {
		return HistoryBackfillJob{}, DailyRefreshResult{}, false, err
	}
	defer lockConn.Release()
	var locked bool
	if err := lockConn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`,
		historyBackfillLock).Scan(&locked); err != nil {
		return HistoryBackfillJob{}, DailyRefreshResult{}, false, err
	}
	if !locked {
		return HistoryBackfillJob{}, DailyRefreshResult{}, false, ErrHistoryBackfillBusy
	}
	defer func() {
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,0))`,
			historyBackfillLock)
	}()

	job, err := s.GetHistoryBackfillJob(ctx, jobID)
	if err != nil {
		return HistoryBackfillJob{}, DailyRefreshResult{}, false, err
	}
	if job.Status == "succeeded" {
		return job, DailyRefreshResult{}, true, nil
	}
	if job.Status == "failed" {
		return job, DailyRefreshResult{}, false, ErrHistoryBackfillNeedsResume
	}
	date := job.NextDate
	windowStart := date.UTC()
	windowEnd := date.AddDate(0, 0, 1).UTC()
	if _, err := s.DB.Exec(ctx, `UPDATE points_snapshot_refresh_runs SET status='failed',
		error_message='abandoned history backfill attempt',completed_at=NOW()
		WHERE business_date=$1 AND trigger=$2 AND status='running'`, dateString(date),
		refreshTriggerBackfill); err != nil {
		s.markHistoryBackfillFailed(job, err)
		return HistoryBackfillJob{}, DailyRefreshResult{}, false,
			fmt.Errorf("close abandoned history backfill attempt: %w", err)
	}

	runID := uuid.NewString()
	if _, err := s.DB.Exec(ctx, `INSERT INTO points_snapshot_refresh_runs(
		id,business_date,trigger,source_window_start,source_window_end,status
	) VALUES($1,$2,$3,$4,$5,'running')`, runID, dateString(date), refreshTriggerBackfill,
		windowStart, windowEnd); err != nil {
		s.markHistoryBackfillFailed(job, err)
		return HistoryBackfillJob{}, DailyRefreshResult{}, false,
			fmt.Errorf("start history backfill day: %w", err)
	}

	usageDay, err := s.UsageSource.AggregateDay(ctx, windowStart, windowEnd)
	if err != nil {
		s.failRefreshRun(runID, err)
		s.markHistoryBackfillFailed(job, err)
		return HistoryBackfillJob{}, DailyRefreshResult{}, false, err
	}
	if !usageDay.WindowStart.Equal(windowStart) || !usageDay.WindowEnd.Equal(windowEnd) ||
		len(usageDay.Fingerprint) != 64 {
		err = errors.New("usage source returned an invalid natural-day result")
		s.failRefreshRun(runID, err)
		s.markHistoryBackfillFailed(job, err)
		return HistoryBackfillJob{}, DailyRefreshResult{}, false, err
	}
	if date.Equal(job.ThroughDate) {
		source, ok := s.UsageSource.(usageHistorySource)
		if !ok || source == nil {
			err = errors.New("usage source does not support final history verification")
		} else {
			var summary UsageHistorySummary
			summary, err = source.SummarizeHistory(ctx, job.FromDate.UTC(),
				job.ThroughDate.AddDate(0, 0, 1).UTC(), s.Location.String(),
				job.PointsPerUSDHundredths)
			if err == nil && !historySummaryMatchesJob(summary, job) {
				err = ErrHistoryBackfillPlanDrift
			}
		}
		if err != nil {
			s.failRefreshRun(runID, err)
			s.markHistoryBackfillFailed(job, err)
			return HistoryBackfillJob{}, DailyRefreshResult{}, false, err
		}
	}

	result := DailyRefreshResult{
		RunID: runID, BusinessDate: date, Users: len(usageDay.Aggregates),
		SourceRows: usageDay.SourceRows, SourceFingerprint: usageDay.Fingerprint,
	}
	done := false
	err = s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		result.ChangedUsers = 0
		result.DeltaSpendMicroUSD = 0
		result.DeltaPointsHundredths = 0
		if _, lockErr := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			"points-snapshot-refresh:"+dateString(date)); lockErr != nil {
			return lockErr
		}
		current, loadErr := s.loadHistoryBackfillJob(ctx, tx, jobID, true)
		if loadErr != nil {
			return loadErr
		}
		if current.Status != "running" || !current.NextDate.Equal(date) {
			return domain.ErrInvalidState
		}
		policy, policyErr := policyForVersion(ctx, tx, current.PolicyVersion)
		if policyErr != nil {
			return policyErr
		}
		if policyErr = validateHistoryBackfillPolicy(policy); policyErr != nil {
			return policyErr
		}
		if policy.PointsPerUSDHundredths != current.PointsPerUSDHundredths {
			return errors.New("history backfill policy ratio changed")
		}
		if err := s.applyUsageDayLockedTx(ctx, tx, runID, refreshTriggerBackfill, date,
			usageDay, policy, &result); err != nil {
			return err
		}
		if _, insertErr := tx.Exec(ctx, `INSERT INTO points_usage_history_backfill_days(
			business_date,job_id,refresh_run_id,policy_version,points_per_usd_hundredths,
			source_users,source_rows,changed_users,delta_spend_microusd,
			delta_points_hundredths,source_fingerprint
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, dateString(date), jobID, runID,
			current.PolicyVersion, current.PointsPerUSDHundredths, result.Users,
			result.SourceRows, result.ChangedUsers, result.DeltaSpendMicroUSD,
			result.DeltaPointsHundredths, result.SourceFingerprint); insertErr != nil {
			return insertErr
		}
		done = date.Equal(current.ThroughDate)
		dayBusinessDays := int64(0)
		if result.Users > 0 {
			dayBusinessDays = 1
		}
		daySourceMaxUsageLogID := int64(0)
		for _, aggregate := range usageDay.Aggregates {
			if aggregate.SourceMaxUsageLogID > daySourceMaxUsageLogID {
				daySourceMaxUsageLogID = aggregate.SourceMaxUsageLogID
			}
		}
		if done {
			appliedUserDays, addErr := checkedAddSigned(current.AppliedSourceUserDays,
				int64(result.Users))
			if addErr != nil {
				return addErr
			}
			appliedBusinessDays, addErr := checkedAddSigned(current.AppliedSourceBusinessDays,
				dayBusinessDays)
			if addErr != nil {
				return addErr
			}
			appliedRows, addErr := checkedAddSigned(current.AppliedSourceRows, result.SourceRows)
			if addErr != nil {
				return addErr
			}
			deltaSpend, addErr := checkedAddSigned(current.DeltaSpendMicroUSD,
				result.DeltaSpendMicroUSD)
			if addErr != nil {
				return addErr
			}
			deltaPoints, addErr := checkedAddSigned(current.DeltaPointsHundredths,
				result.DeltaPointsHundredths)
			if addErr != nil {
				return addErr
			}
			appliedMaxUsageLogID := max(current.AppliedSourceMaxUsageLogID,
				daySourceMaxUsageLogID)
			var appliedSourceUsers int64
			if countErr := tx.QueryRow(ctx, `SELECT COUNT(DISTINCT snapshot.user_id)::bigint
				FROM points_daily_snapshots snapshot
				JOIN points_usage_history_backfill_days backfill
					ON backfill.business_date=snapshot.business_date
				WHERE backfill.job_id=$1 AND snapshot.actual_cost_microusd > 0`, jobID).Scan(
				&appliedSourceUsers); countErr != nil {
				return countErr
			}
			if current.CompletedDays+1 != inclusiveCalendarDays(current.FromDate, current.ThroughDate) ||
				appliedSourceUsers != current.PlannedSourceUsers ||
				appliedUserDays != current.PlannedSourceUserDays ||
				appliedBusinessDays != current.PlannedSourceBusinessDays ||
				appliedRows != current.PlannedSourceRows ||
				deltaSpend != current.PlannedSpendMicroUSD ||
				deltaPoints != current.PlannedPointsHundredths ||
				appliedMaxUsageLogID != current.PlannedSourceMaxUsageLogID {
				return ErrHistoryBackfillPlanDrift
			}
		}
		status := "running"
		if done {
			status = "succeeded"
		}
		if _, updateErr := tx.Exec(ctx, `UPDATE points_usage_history_backfill_jobs SET
			next_date=$1,completed_days=completed_days+1,
			applied_source_user_days=applied_source_user_days+$2,
			applied_source_business_days=applied_source_business_days+$3,
			applied_source_rows=applied_source_rows+$4,
			applied_source_max_usage_log_id=GREATEST(applied_source_max_usage_log_id,$5),
			changed_users=changed_users+$6,delta_spend_microusd=delta_spend_microusd+$7,
			delta_points_hundredths=delta_points_hundredths+$8,status=$9,
			updated_at=NOW(),completed_at=CASE WHEN $9='succeeded' THEN NOW() ELSE NULL END
			WHERE id=$10`, dateString(date.AddDate(0, 0, 1)), result.Users, dayBusinessDays,
			result.SourceRows, daySourceMaxUsageLogID, result.ChangedUsers,
			result.DeltaSpendMicroUSD, result.DeltaPointsHundredths, status, jobID); updateErr != nil {
			return updateErr
		}
		if done {
			if _, auditErr := tx.Exec(ctx, `INSERT INTO points_admin_audit(
				actor_user_id,action,target_type,target_id,detail
			) VALUES($1,'usage_history_backfill.complete','usage_history_backfill',$2,
				jsonb_build_object('through_date',$3::text,'refresh_run_id',$4::text))`, current.CreatedBy,
				jobID, dateString(date), runID); auditErr != nil {
				return auditErr
			}
		}
		job, loadErr = s.loadHistoryBackfillJob(ctx, tx, jobID, false)
		return loadErr
	})
	if err != nil {
		s.failRefreshRun(runID, err)
		s.markHistoryBackfillFailed(job, err)
		return HistoryBackfillJob{}, DailyRefreshResult{}, false,
			fmt.Errorf("apply history backfill day %s: %w", dateString(date), err)
	}
	return job, result, done, nil
}

func usageAccountingPolicyForDate(ctx context.Context, q queryer,
	date time.Time) (domain.Policy, error) {
	var policy domain.Policy
	err := q.QueryRow(ctx, `SELECT policy_version,points_per_usd_hundredths
		FROM points_usage_history_backfill_days WHERE business_date=$1`, dateString(date)).Scan(
		&policy.VersionNo, &policy.PointsPerUSDHundredths)
	if err == nil {
		policy.Enabled = true
		return policy, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Policy{}, err
	}
	return policyForDate(ctx, q, date)
}

func validateHistoryBackfillPolicy(policy domain.Policy) error {
	if !policy.Enabled || policy.CheckinEnabled || policy.PointsPerUSDHundredths <= 0 ||
		policy.RefreshMinute != 5 {
		return errors.New("history backfill requires an enabled 00:05 policy with check-in disabled")
	}
	return nil
}

func validateHistoryBackfillPlan(plan HistoryBackfillPlan) error {
	if plan.FromDate.IsZero() || plan.ThroughDate.Before(plan.FromDate) ||
		plan.CalendarDays != inclusiveCalendarDays(plan.FromDate, plan.ThroughDate) ||
		plan.PolicyVersion <= 0 || plan.PointsPerUSDHundredths <= 0 ||
		plan.SourceUsers < 0 || plan.SourceUserDays < 0 || plan.SourceBusinessDays < 0 ||
		plan.SourceRows < 0 || plan.SpendMicroUSD < 0 || plan.PointsHundredths < 0 ||
		plan.SourceMaxUsageLogID < 0 || plan.SourceUsers > plan.SourceUserDays ||
		plan.SourceBusinessDays > int64(plan.CalendarDays) || plan.SourceUserDays > plan.SourceRows ||
		(plan.SourceRows == 0) != (plan.SourceMaxUsageLogID == 0) {
		return errors.New("invalid history backfill plan")
	}
	return nil
}

func historyBackfillPlanFingerprint(plan HistoryBackfillPlan) string {
	return security.Fingerprint("points-usage-history-backfill-plan-v1", dateString(plan.FromDate),
		dateString(plan.ThroughDate), plan.CalendarDays, plan.PolicyVersion,
		plan.PointsPerUSDHundredths, plan.SourceUsers, plan.SourceUserDays,
		plan.SourceBusinessDays, plan.SourceRows, plan.SpendMicroUSD, plan.PointsHundredths,
		plan.SourceMaxUsageLogID)
}

func inclusiveCalendarDays(from, through time.Time) int {
	days := 0
	for date := from; !date.After(through); date = date.AddDate(0, 0, 1) {
		days++
	}
	return days
}

const historyBackfillJobColumns = `id::text,policy_version,points_per_usd_hundredths,
	from_date::text,through_date::text,next_date::text,plan_fingerprint,
	planned_source_users,planned_source_user_days,planned_source_business_days,
	planned_source_rows,planned_spend_microusd,
	planned_points_hundredths,planned_source_max_usage_log_id,completed_days,
	applied_source_user_days,applied_source_business_days,applied_source_rows,
	applied_source_max_usage_log_id,changed_users,delta_spend_microusd,delta_points_hundredths,
	status,COALESCE(error_message,''),created_by,created_at,updated_at,
	completed_at`

func (s *Store) loadHistoryBackfillJob(ctx context.Context, q queryer, jobID string,
	forUpdate bool) (HistoryBackfillJob, error) {
	query := `SELECT ` + historyBackfillJobColumns +
		` FROM points_usage_history_backfill_jobs WHERE id=$1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var job HistoryBackfillJob
	var fromDate, throughDate, nextDate string
	err := q.QueryRow(ctx, query, jobID).Scan(&job.ID, &job.PolicyVersion,
		&job.PointsPerUSDHundredths, &fromDate, &throughDate, &nextDate, &job.PlanFingerprint,
		&job.PlannedSourceUsers, &job.PlannedSourceUserDays, &job.PlannedSourceBusinessDays,
		&job.PlannedSourceRows, &job.PlannedSpendMicroUSD, &job.PlannedPointsHundredths,
		&job.PlannedSourceMaxUsageLogID, &job.CompletedDays, &job.AppliedSourceUserDays,
		&job.AppliedSourceBusinessDays, &job.AppliedSourceRows,
		&job.AppliedSourceMaxUsageLogID, &job.ChangedUsers, &job.DeltaSpendMicroUSD,
		&job.DeltaPointsHundredths, &job.Status, &job.ErrorMessage, &job.CreatedBy,
		&job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	if err != nil {
		return HistoryBackfillJob{}, translateNotFound(err)
	}
	job.FromDate, err = time.ParseInLocation("2006-01-02", fromDate, s.Location)
	if err == nil {
		job.ThroughDate, err = time.ParseInLocation("2006-01-02", throughDate, s.Location)
	}
	if err == nil {
		job.NextDate, err = time.ParseInLocation("2006-01-02", nextDate, s.Location)
	}
	if err != nil {
		return HistoryBackfillJob{}, fmt.Errorf("parse history backfill date: %w", err)
	}
	return job, nil
}

func (s *Store) markHistoryBackfillFailed(job HistoryBackfillJob, backfillErr error) {
	if job.ID == "" || backfillErr == nil {
		return
	}
	message := strings.TrimSpace(backfillErr.Error())
	if message == "" {
		message = "unknown history backfill failure"
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE points_usage_history_backfill_jobs SET
			status='failed',error_message=$1,updated_at=NOW()
			WHERE id=$2 AND status='running'`, message, job.ID)
		if err != nil || tag.RowsAffected() != 1 {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO points_admin_audit(
			actor_user_id,action,target_type,target_id,detail
		) SELECT created_by,'usage_history_backfill.fail','usage_history_backfill',id,
			jsonb_build_object('next_date',next_date,'error',$1::text)
			FROM points_usage_history_backfill_jobs WHERE id=$2 AND status='failed'`, message, job.ID)
		return err
	})
}

func historySummaryMatchesJob(summary UsageHistorySummary, job HistoryBackfillJob) bool {
	return summary.SourceUsers == job.PlannedSourceUsers &&
		summary.SourceUserDays == job.PlannedSourceUserDays &&
		summary.SourceBusinessDays == job.PlannedSourceBusinessDays &&
		summary.SourceRows == job.PlannedSourceRows &&
		summary.SpendMicroUSD == job.PlannedSpendMicroUSD &&
		summary.PointsHundredths == job.PlannedPointsHundredths &&
		summary.SourceMaxUsageLogID == job.PlannedSourceMaxUsageLogID
}
