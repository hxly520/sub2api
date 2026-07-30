package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/jackc/pgx/v5"
)

const (
	checkinRejectionLimit        = "daily_limit_reached"
	checkinRejectionMinimumSpend = "minimum_spend_not_met"
	checkinRejectionNoTier       = "no_matching_tier"
	checkinRejectionCap          = "award_cap_exhausted"
)

type CheckinResult struct {
	Ordinal        int                      `json:"ordinal"`
	RewardMicroUSD int64                    `json:"reward_microusd"`
	TransactionID  string                   `json:"transaction_id,omitempty"`
	DeliveryStatus string                   `json:"delivery_status"`
	Calculation    domain.RewardCalculation `json:"-"`
}

type checkinMetrics struct {
	BasisPointsHundredths  int64
	YesterdaySpendMicroUSD int64
	RewardBaseMicroUSD     int64
}

// CheckinAvailable performs the same read-only eligibility checks used by
// CheckIn. CheckIn remains authoritative because eligibility can change after
// this advisory result is returned.
func (s *Store) CheckinAvailable(ctx context.Context, userID int64, now time.Time) (bool, error) {
	if s == nil || s.DB == nil || s.Location == nil || userID <= 0 {
		return false, errors.New("invalid check-in availability request")
	}
	date := s.BusinessDate(now)
	policy, err := policyForDate(ctx, s.DB, date)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !policy.Enabled || !policy.CheckinEnabled || policy.ValidateForEnable() != nil {
		return false, nil
	}

	count, userAwardedMicroUSD, err := s.CheckinStatus(ctx, userID, now)
	if err != nil {
		return false, err
	}
	if policy.CheckinDailyLimit == nil || count >= *policy.CheckinDailyLimit {
		return false, nil
	}
	metrics, err := checkinMetricsFor(ctx, s.DB, userID, date, policy)
	if err != nil {
		if isCheckinIneligibility(err) {
			return false, nil
		}
		return false, err
	}
	if err := validateCheckinSpend(policy, metrics.YesterdaySpendMicroUSD); err != nil {
		return false, nil
	}
	if _, ok := tierForPoints(policy, metrics.BasisPointsHundredths); !ok {
		return false, nil
	}

	var platformAwardedMicroUSD int64
	if err := s.DB.QueryRow(ctx, `SELECT COALESCE((SELECT awarded_microusd
		FROM points_checkin_platform_daily_limits WHERE business_date=$1),0)::bigint`,
		dateString(date)).Scan(&platformAwardedMicroUSD); err != nil {
		return false, err
	}
	_, err = limitCheckinReward(1, *policy.CheckinSingleAwardCapMicroUSD,
		*policy.CheckinPlatformDailyCapMicroUSD, platformAwardedMicroUSD,
		*policy.CheckinUserDailyCapMicroUSD, userAwardedMicroUSD)
	if err != nil {
		if isCheckinIneligibility(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isCheckinIneligibility(err error) bool {
	return errors.Is(err, domain.ErrDisabled) || errors.Is(err, domain.ErrPolicyIncomplete) ||
		errors.Is(err, domain.ErrCapExhausted) || errors.Is(err, domain.ErrCheckinLimit) ||
		errors.Is(err, domain.ErrCheckinSpendMinimum) || errors.Is(err, domain.ErrSnapshotNotReady) ||
		errors.Is(err, domain.ErrNoMatchingTier) || errors.Is(err, domain.ErrInvalidState)
}

func (s *Store) CheckIn(ctx context.Context, userID int64, eventID string, now time.Time) (CheckinResult, error) {
	if userID <= 0 || eventID == "" {
		return CheckinResult{}, fmt.Errorf("invalid check-in request")
	}
	date := s.BusinessDate(now)
	var (
		result    CheckinResult
		rejection error
	)
	err := s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		result = CheckinResult{}
		rejection = nil
		policy, err := policyForDate(ctx, tx, date)
		if err != nil {
			return err
		}
		if !policy.Enabled || !policy.CheckinEnabled {
			return domain.ErrDisabled
		}
		if err := policy.ValidateForEnable(); err != nil {
			return err
		}

		fingerprint := security.Fingerprint(userID, eventID, dateString(date), policy.VersionNo)
		replayed, err := loadExistingCheckinRequestTx(ctx, tx, eventID, fingerprint, &result, &rejection)
		if err != nil {
			return err
		}
		if replayed {
			return nil
		}

		if _, err := tx.Exec(ctx, `INSERT INTO points_checkin_daily(user_id,business_date) VALUES($1,$2)
			ON CONFLICT(user_id,business_date) DO NOTHING`, userID, dateString(date)); err != nil {
			return err
		}
		var count int
		var userAwardedMicroUSD int64
		if err := tx.QueryRow(ctx, `SELECT checkin_count,awarded_microusd FROM points_checkin_daily
			WHERE user_id=$1 AND business_date=$2 FOR UPDATE`, userID, dateString(date)).Scan(
			&count, &userAwardedMicroUSD); err != nil {
			return err
		}
		if count >= *policy.CheckinDailyLimit {
			rejection = domain.ErrCheckinLimit
			return recordConvergedCheckinRejectionTx(ctx, tx, userID, date, policy, checkinMetrics{},
				domain.RewardCalculation{}, 0, "rejected", checkinRejectionLimit)
		}
		metrics, err := checkinMetricsFor(ctx, tx, userID, date, policy)
		if err != nil {
			return err
		}
		if err := validateCheckinSpend(policy, metrics.YesterdaySpendMicroUSD); err != nil {
			rejection = domain.ErrCheckinSpendMinimum
			return recordConvergedCheckinRejectionTx(ctx, tx, userID, date, policy, metrics,
				domain.RewardCalculation{}, 0, "rejected", checkinRejectionMinimumSpend)
		}
		tier, ok := tierForPoints(policy, metrics.BasisPointsHundredths)
		if !ok {
			rejection = domain.ErrNoMatchingTier
			return recordConvergedCheckinRejectionTx(ctx, tx, userID, date, policy, metrics,
				domain.RewardCalculation{}, 0, "rejected", checkinRejectionNoTier)
		}
		calculation, err := calculateCheckinReward(tier, metrics.RewardBaseMicroUSD)
		if err != nil {
			return err
		}
		result.Calculation = calculation

		if _, err := tx.Exec(ctx, `INSERT INTO points_checkin_platform_daily_limits(business_date)
			VALUES($1) ON CONFLICT(business_date) DO NOTHING`, dateString(date)); err != nil {
			return err
		}
		var platformAwardedMicroUSD int64
		if err := tx.QueryRow(ctx, `SELECT awarded_microusd FROM points_checkin_platform_daily_limits
			WHERE business_date=$1 FOR UPDATE`, dateString(date)).Scan(&platformAwardedMicroUSD); err != nil {
			return err
		}
		candidateMicroUSD, err := limitCheckinReward(calculation.CalculatedRewardMicroUSD,
			*policy.CheckinSingleAwardCapMicroUSD, *policy.CheckinPlatformDailyCapMicroUSD,
			platformAwardedMicroUSD, *policy.CheckinUserDailyCapMicroUSD, userAwardedMicroUSD)
		if errors.Is(err, domain.ErrCapExhausted) {
			rejection = domain.ErrCapExhausted
			return recordConvergedCheckinRejectionTx(ctx, tx, userID, date, policy, metrics,
				calculation, 0, "rejected", checkinRejectionCap)
		}
		if err != nil {
			return err
		}
		claimed, err := claimIdempotency(ctx, tx, "checkin", eventID, fingerprint)
		if err != nil {
			return err
		}
		if !claimed {
			replayed, err := loadExistingCheckinRequestTx(ctx, tx, eventID, fingerprint, &result, &rejection)
			if err != nil {
				return err
			}
			if !replayed {
				return domain.ErrInvalidState
			}
			return nil
		}

		actualMicroUSD := candidateMicroUSD
		result.RewardMicroUSD = actualMicroUSD
		result.Ordinal = count + 1
		result.DeliveryStatus = "not_required"
		var balanceGrantID *uuid.UUID
		if actualMicroUSD > 0 {
			grant, err := enqueueBalanceGrantTx(ctx, tx, EnqueueBalanceGrantRequest{
				UserID: userID, AmountMicroUSD: actualMicroUSD, Kind: "checkin",
				ExternalEventID: eventID, PolicyVersion: policy.VersionNo,
				Reason: "Daily check-in reward", Now: now.UTC(),
			})
			if err != nil {
				return err
			}
			parsedID, err := uuid.Parse(grant.ID)
			if err != nil {
				return fmt.Errorf("parse balance grant id: %w", err)
			}
			balanceGrantID = &parsedID
			result.TransactionID = grant.ID
			result.DeliveryStatus = grant.Status
		}
		if _, err := tx.Exec(ctx, `UPDATE points_checkin_daily SET checkin_count=checkin_count+1,
			awarded_microusd=awarded_microusd+$1,updated_at=NOW()
			WHERE user_id=$2 AND business_date=$3`, actualMicroUSD, userID, dateString(date)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE points_checkin_platform_daily_limits
			SET awarded_microusd=awarded_microusd+$1,updated_at=NOW() WHERE business_date=$2`,
			actualMicroUSD, dateString(date)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO points_checkins(
			user_id,business_date,ordinal,reward_microusd,balance_grant_id,external_event_id,policy_version,
			policy_basis,basis_points_hundredths,yesterday_spend_microusd,reward_base_microusd,reward_mode,
			fixed_reward_min_microusd,fixed_reward_max_microusd,reward_percentage_min_ppm,
			reward_percentage_max_ppm,sampled_percentage_ppm,calculated_reward_microusd
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			userID, dateString(date), result.Ordinal, actualMicroUSD, balanceGrantID,
			eventID, policy.VersionNo, policy.Basis, metrics.BasisPointsHundredths,
			metrics.YesterdaySpendMicroUSD, metrics.RewardBaseMicroUSD, tier.RewardMode,
			tier.FixedRewardMinMicroUSD, tier.FixedRewardMaxMicroUSD, tier.RewardPercentageMinPPM,
			tier.RewardPercentageMaxPPM, calculation.SampledPercentagePPM,
			calculation.CalculatedRewardMicroUSD); err != nil {
			return err
		}
		if err := recordCheckinAttemptTx(ctx, tx, userID, eventID, date, policy, metrics,
			calculation, actualMicroUSD, "accepted", ""); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE points_idempotency SET result_reference=$1
			WHERE scope='checkin' AND external_event_id=$2`, eventID, eventID)
		return err
	})
	if err != nil {
		return CheckinResult{}, fmt.Errorf("check in: %w", err)
	}
	if rejection != nil {
		return CheckinResult{}, fmt.Errorf("check in: %w", rejection)
	}
	return result, nil
}

func checkinMetricsFor(ctx context.Context, q queryer, userID int64, date time.Time,
	policy domain.Policy) (checkinMetrics, error) {
	var hasUnresolvedSnapshot bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM points_daily_snapshots
		WHERE user_id=$1 AND status='needs_review')`, userID).Scan(&hasUnresolvedSnapshot); err != nil {
		return checkinMetrics{}, err
	}
	if hasUnresolvedSnapshot {
		return checkinMetrics{}, domain.ErrInvalidState
	}
	yesterday := date.AddDate(0, 0, -1)
	var snapshotReady bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM points_snapshot_refresh_runs
		WHERE business_date=$1 AND status='succeeded'
	)`, dateString(yesterday)).Scan(&snapshotReady); err != nil {
		return checkinMetrics{}, err
	}
	if !snapshotReady {
		return checkinMetrics{}, domain.ErrSnapshotNotReady
	}
	var metrics checkinMetrics
	var snapshotStatus string
	err := q.QueryRow(ctx, `SELECT COALESCE(actual_cost_microusd,0),COALESCE(awarded_points_hundredths,0),status
		FROM points_daily_snapshots WHERE user_id=$1 AND business_date=$2`, userID, dateString(yesterday)).Scan(
		&metrics.YesterdaySpendMicroUSD, &metrics.BasisPointsHundredths, &snapshotStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return checkinMetrics{}, err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		metrics.YesterdaySpendMicroUSD = 0
		metrics.BasisPointsHundredths = 0
	}
	if snapshotStatus == snapshotStatusReview {
		return checkinMetrics{}, domain.ErrInvalidState
	}
	metrics.RewardBaseMicroUSD = metrics.YesterdaySpendMicroUSD
	if policy.Basis == domain.PolicyBasisTotal {
		err := q.QueryRow(ctx, `SELECT total_points_hundredths,total_spend_microusd
			FROM points_accounts WHERE user_id=$1`, userID).Scan(
			&metrics.BasisPointsHundredths, &metrics.RewardBaseMicroUSD)
		if errors.Is(err, pgx.ErrNoRows) {
			metrics.BasisPointsHundredths = 0
			metrics.RewardBaseMicroUSD = 0
		} else if err != nil {
			return checkinMetrics{}, err
		}
	}
	return metrics, nil
}

func recordCheckinAttemptTx(ctx context.Context, tx pgx.Tx, userID int64, eventID string,
	date time.Time, policy domain.Policy, metrics checkinMetrics, calculation domain.RewardCalculation,
	actualMicroUSD int64, outcome, reason string) error {
	return insertCheckinAttemptTx(ctx, tx, userID, eventID, date, policy, metrics, calculation,
		actualMicroUSD, outcome, reason, false)
}

func recordConvergedCheckinRejectionTx(ctx context.Context, tx pgx.Tx, userID int64,
	date time.Time, policy domain.Policy, metrics checkinMetrics, calculation domain.RewardCalculation,
	actualMicroUSD int64, outcome, reason string) error {
	eventID := convergedCheckinRejectionEventID(userID, date, reason)
	return insertCheckinAttemptTx(ctx, tx, userID, eventID, date, policy, metrics, calculation,
		actualMicroUSD, outcome, reason, true)
}

func insertCheckinAttemptTx(ctx context.Context, tx pgx.Tx, userID int64, eventID string,
	date time.Time, policy domain.Policy, metrics checkinMetrics, calculation domain.RewardCalculation,
	actualMicroUSD int64, outcome, reason string, converge bool) error {
	var rewardMode any
	if calculation.Mode != "" {
		rewardMode = calculation.Mode
	}
	query := `INSERT INTO points_checkin_attempts(
		user_id,business_date,external_event_id,outcome,rejection_reason,policy_version,policy_basis,
		basis_points_hundredths,yesterday_spend_microusd,reward_base_microusd,reward_mode,
		fixed_reward_min_microusd,fixed_reward_max_microusd,reward_percentage_min_ppm,
		reward_percentage_max_ppm,sampled_percentage_ppm,calculated_reward_microusd,
		actual_reward_microusd
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`
	if converge {
		query += ` ON CONFLICT(external_event_id) DO NOTHING`
	}
	_, err := tx.Exec(ctx, query,
		userID, dateString(date), eventID, outcome, nullableString(reason), policy.VersionNo,
		policy.Basis, metrics.BasisPointsHundredths, metrics.YesterdaySpendMicroUSD, metrics.RewardBaseMicroUSD,
		rewardMode, fixedMinimum(policy, calculation.Mode, metrics.BasisPointsHundredths),
		fixedMaximum(policy, calculation.Mode, metrics.BasisPointsHundredths),
		percentageMinimum(policy, calculation.Mode, metrics.BasisPointsHundredths),
		percentageMaximum(policy, calculation.Mode, metrics.BasisPointsHundredths),
		calculation.SampledPercentagePPM, nullableReward(calculation.Mode, calculation.CalculatedRewardMicroUSD),
		nullableReward(calculation.Mode, actualMicroUSD))
	return err
}

func convergedCheckinRejectionEventID(userID int64, date time.Time, reason string) string {
	return "\x1fpoints-checkin-rejection:" + security.Fingerprint(userID, dateString(date), reason)
}

func fixedMinimum(policy domain.Policy, mode string, points int64) any {
	if mode != domain.RewardModeFixedRange {
		return nil
	}
	tier, ok := tierForPoints(policy, points)
	if !ok {
		return nil
	}
	return tier.FixedRewardMinMicroUSD
}

func fixedMaximum(policy domain.Policy, mode string, points int64) any {
	if mode != domain.RewardModeFixedRange {
		return nil
	}
	tier, ok := tierForPoints(policy, points)
	if !ok {
		return nil
	}
	return tier.FixedRewardMaxMicroUSD
}

func percentageMinimum(policy domain.Policy, mode string, points int64) any {
	if mode != domain.RewardModePercentageRange {
		return nil
	}
	tier, ok := tierForPoints(policy, points)
	if !ok {
		return nil
	}
	return tier.RewardPercentageMinPPM
}

func percentageMaximum(policy domain.Policy, mode string, points int64) any {
	if mode != domain.RewardModePercentageRange {
		return nil
	}
	tier, ok := tierForPoints(policy, points)
	if !ok {
		return nil
	}
	return tier.RewardPercentageMaxPPM
}

func nullableReward(mode string, reward int64) any {
	if mode == "" {
		return nil
	}
	return reward
}

func loadExistingCheckinRequestTx(ctx context.Context, tx pgx.Tx, eventID, fingerprint string,
	result *CheckinResult, rejection *error) (bool, error) {
	var existingFingerprint string
	err := tx.QueryRow(ctx, `SELECT request_fingerprint FROM points_idempotency
		WHERE scope='checkin' AND external_event_id=$1`, eventID).Scan(&existingFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingFingerprint != fingerprint {
		return false, domain.ErrIdempotencyConflict
	}

	var outcome, reason string
	if err := tx.QueryRow(ctx, `SELECT outcome,COALESCE(rejection_reason,'')
		FROM points_checkin_attempts WHERE external_event_id=$1`, eventID).Scan(&outcome, &reason); err != nil {
		return false, translateNotFound(err)
	}
	if outcome == "rejected" {
		*rejection = checkinErrorForReason(reason)
		return true, nil
	}
	if err := loadCheckinResultTx(ctx, tx, eventID, result); err != nil {
		return false, err
	}
	return true, nil
}

func loadCheckinResultTx(ctx context.Context, tx pgx.Tx, eventID string, result *CheckinResult) error {
	var sampledPercentage *int64
	var transactionID *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT c.ordinal,c.reward_microusd,c.balance_grant_id,
		COALESCE(g.status,'not_required'),c.reward_mode,c.reward_base_microusd,
		c.sampled_percentage_ppm,c.calculated_reward_microusd
		FROM points_checkins c LEFT JOIN points_balance_grants g ON g.id=c.balance_grant_id
		WHERE c.external_event_id=$1`, eventID).Scan(
		&result.Ordinal, &result.RewardMicroUSD, &transactionID, &result.DeliveryStatus,
		&result.Calculation.Mode, &result.Calculation.BaseMicroUSD, &sampledPercentage,
		&result.Calculation.CalculatedRewardMicroUSD); err != nil {
		return err
	}
	if transactionID != nil {
		result.TransactionID = transactionID.String()
	}
	result.Calculation.SampledPercentagePPM = sampledPercentage
	if sampledPercentage != nil {
		result.Calculation.SampledValue = *sampledPercentage
	} else {
		result.Calculation.SampledValue = result.Calculation.CalculatedRewardMicroUSD
	}
	return nil
}

func checkinErrorForReason(reason string) error {
	switch reason {
	case checkinRejectionLimit:
		return domain.ErrCheckinLimit
	case checkinRejectionMinimumSpend:
		return domain.ErrCheckinSpendMinimum
	case checkinRejectionNoTier:
		return domain.ErrNoMatchingTier
	case checkinRejectionCap:
		return domain.ErrCapExhausted
	default:
		return domain.ErrInvalidState
	}
}

func (s *Store) CheckinStatus(ctx context.Context, userID int64, now time.Time) (count int, amountMicroUSD int64, err error) {
	err = s.DB.QueryRow(ctx, `SELECT d.checkin_count,COALESCE((
		SELECT SUM(c.reward_microusd) FROM points_checkins c
		WHERE c.user_id=d.user_id AND c.business_date=d.business_date
	),0)::bigint FROM points_checkin_daily d WHERE d.user_id=$1 AND d.business_date=$2`,
		userID, dateString(s.BusinessDate(now))).Scan(&count, &amountMicroUSD)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	return count, amountMicroUSD, err
}
