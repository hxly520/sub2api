package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreatePolicy(ctx context.Context, policy domain.Policy, now time.Time) (domain.Policy, error) {
	return s.CreatePolicyAtomic(ctx, policy, now)
}

func (s *Store) CreatePolicyAtomic(ctx context.Context, policy domain.Policy, now time.Time) (domain.Policy, error) {
	if policy.Mode == "" {
		policy.Mode = domain.PolicyModeAllUsers
	}
	if policy.Basis == "" {
		policy.Basis = domain.PolicyBasisYesterday
	}
	if policy.EffectiveDate.Before(s.BusinessDate(now).AddDate(0, 0, 1)) {
		return domain.Policy{}, fmt.Errorf("effective_date must be tomorrow or later")
	}
	if err := validatePolicy(policy); err != nil {
		return domain.Policy{}, err
	}

	err := s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			historyBackfillLock); err != nil {
			return err
		}
		if policy.Enabled {
			pending, err := initialHistoryBackfillPending(ctx, tx)
			if err != nil {
				return err
			}
			if pending {
				return errors.New("an initial usage history backfill is pending; resume it before enabling a new policy")
			}
		}
		policy.ID = 0
		policy.VersionNo = 0
		policy.CreatedAt = time.Time{}
		if err := tx.QueryRow(ctx, `
			INSERT INTO points_policy_versions (
				effective_date, enabled, mode, basis, checkin_enabled,
				checkin_daily_limit, minimum_checkin_spend_microusd,
				checkin_platform_daily_cap_microusd, checkin_user_daily_cap_microusd,
				checkin_single_award_cap_microusd, points_per_usd_hundredths,
				refresh_minute, created_by
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING id, version_no, created_at
		`, dateString(policy.EffectiveDate), policy.Enabled, policy.Mode, policy.Basis,
			policy.CheckinEnabled, policy.CheckinDailyLimit, policy.MinimumCheckinSpendMicroUSD,
			policy.CheckinPlatformDailyCapMicroUSD, policy.CheckinUserDailyCapMicroUSD,
			policy.CheckinSingleAwardCapMicroUSD, policy.PointsPerUSDHundredths,
			policy.RefreshMinute, policy.CreatedBy,
		).Scan(&policy.ID, &policy.VersionNo, &policy.CreatedAt); err != nil {
			return err
		}
		for _, tier := range policy.Tiers {
			if _, err := tx.Exec(ctx, `
				INSERT INTO points_policy_tiers(
					policy_id, lower_points_hundredths, upper_points_hundredths, reward_mode,
					fixed_reward_min_microusd, fixed_reward_max_microusd,
					reward_percentage_min_ppm, reward_percentage_max_ppm
				) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
			`, policy.ID, tier.LowerPointsHundredths, tier.UpperPointsHundredths, tier.RewardMode,
				tier.FixedRewardMinMicroUSD, tier.FixedRewardMaxMicroUSD,
				tier.RewardPercentageMinPPM, tier.RewardPercentageMaxPPM); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO points_admin_audit(actor_user_id,action,target_type,target_id,detail)
			VALUES($1,'policy.create','policy',$2,jsonb_build_object(
				'effective_date',$3,'enabled',$4,'basis',$5,'mode',$6
			))`, policy.CreatedBy, policy.VersionNo, dateString(policy.EffectiveDate),
			policy.Enabled, policy.Basis, policy.Mode)
		return err
	})
	if err != nil {
		return domain.Policy{}, fmt.Errorf("create policy: %w", err)
	}
	return policy, nil
}

func validatePolicy(policy domain.Policy) error {
	return policy.ValidateForEnable()
}

func (s *Store) PolicyForDate(ctx context.Context, date time.Time) (domain.Policy, error) {
	return policyForDate(ctx, s.DB, date)
}

func (s *Store) PolicyByVersion(ctx context.Context, version int64) (domain.Policy, error) {
	return policyForVersion(ctx, s.DB, version)
}

func policyForVersion(ctx context.Context, q queryer, version int64) (domain.Policy, error) {
	if version <= 0 {
		return domain.Policy{}, domain.ErrNotFound
	}
	var policy domain.Policy
	err := q.QueryRow(ctx, `
		SELECT id,version_no,effective_date,enabled,mode,basis,checkin_enabled,
			checkin_daily_limit,minimum_checkin_spend_microusd,
			checkin_platform_daily_cap_microusd,checkin_user_daily_cap_microusd,
			checkin_single_award_cap_microusd,points_per_usd_hundredths,
			refresh_minute,created_by,created_at
		FROM points_policy_versions
		WHERE version_no=$1
	`, version).Scan(
		&policy.ID, &policy.VersionNo, &policy.EffectiveDate, &policy.Enabled, &policy.Mode,
		&policy.Basis, &policy.CheckinEnabled, &policy.CheckinDailyLimit,
		&policy.MinimumCheckinSpendMicroUSD, &policy.CheckinPlatformDailyCapMicroUSD,
		&policy.CheckinUserDailyCapMicroUSD, &policy.CheckinSingleAwardCapMicroUSD,
		&policy.PointsPerUSDHundredths, &policy.RefreshMinute, &policy.CreatedBy, &policy.CreatedAt,
	)
	if err != nil {
		return domain.Policy{}, translateNotFound(err)
	}
	if err := loadPolicyTiers(ctx, q, &policy); err != nil {
		return domain.Policy{}, err
	}
	return policy, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func policyForDate(ctx context.Context, q queryer, date time.Time) (domain.Policy, error) {
	var policy domain.Policy
	err := q.QueryRow(ctx, `
		SELECT id,version_no,effective_date,enabled,mode,basis,checkin_enabled,
			checkin_daily_limit,minimum_checkin_spend_microusd,
			checkin_platform_daily_cap_microusd,checkin_user_daily_cap_microusd,
			checkin_single_award_cap_microusd,points_per_usd_hundredths,
			refresh_minute,created_by,created_at
		FROM points_policy_versions
		WHERE effective_date <= $1
		ORDER BY effective_date DESC, version_no DESC LIMIT 1
	`, dateString(date)).Scan(
		&policy.ID, &policy.VersionNo, &policy.EffectiveDate, &policy.Enabled, &policy.Mode,
		&policy.Basis, &policy.CheckinEnabled, &policy.CheckinDailyLimit,
		&policy.MinimumCheckinSpendMicroUSD, &policy.CheckinPlatformDailyCapMicroUSD,
		&policy.CheckinUserDailyCapMicroUSD, &policy.CheckinSingleAwardCapMicroUSD,
		&policy.PointsPerUSDHundredths, &policy.RefreshMinute, &policy.CreatedBy, &policy.CreatedAt,
	)
	if err != nil {
		return domain.Policy{}, translateNotFound(err)
	}
	if err := loadPolicyTiers(ctx, q, &policy); err != nil {
		return domain.Policy{}, err
	}
	return policy, nil
}

func loadPolicyTiers(ctx context.Context, q queryer, policy *domain.Policy) error {
	rows, err := q.Query(ctx, `SELECT lower_points_hundredths,upper_points_hundredths,reward_mode,
		fixed_reward_min_microusd,fixed_reward_max_microusd,
		reward_percentage_min_ppm,reward_percentage_max_ppm
		FROM points_policy_tiers WHERE policy_id=$1 ORDER BY lower_points_hundredths`, policy.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tier domain.Tier
		if err := rows.Scan(&tier.LowerPointsHundredths, &tier.UpperPointsHundredths, &tier.RewardMode,
			&tier.FixedRewardMinMicroUSD, &tier.FixedRewardMaxMicroUSD,
			&tier.RewardPercentageMinPPM, &tier.RewardPercentageMaxPPM); err != nil {
			return err
		}
		policy.Tiers = append(policy.Tiers, tier)
	}
	return rows.Err()
}

func (s *Store) ListPolicies(ctx context.Context, limit int) ([]domain.Policy, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.DB.Query(ctx, `SELECT id,version_no,effective_date,enabled,mode,basis,checkin_enabled,
		checkin_daily_limit,minimum_checkin_spend_microusd,
		checkin_platform_daily_cap_microusd,checkin_user_daily_cap_microusd,
		checkin_single_award_cap_microusd,points_per_usd_hundredths,
		refresh_minute,created_by,created_at
		FROM points_policy_versions ORDER BY version_no DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	var result []domain.Policy
	for rows.Next() {
		var policy domain.Policy
		if err := rows.Scan(&policy.ID, &policy.VersionNo, &policy.EffectiveDate, &policy.Enabled,
			&policy.Mode, &policy.Basis, &policy.CheckinEnabled, &policy.CheckinDailyLimit,
			&policy.MinimumCheckinSpendMicroUSD, &policy.CheckinPlatformDailyCapMicroUSD,
			&policy.CheckinUserDailyCapMicroUSD, &policy.CheckinSingleAwardCapMicroUSD,
			&policy.PointsPerUSDHundredths, &policy.RefreshMinute,
			&policy.CreatedBy, &policy.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, policy)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range result {
		if err := loadPolicyTiers(ctx, s.DB, &result[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func IsRetryableTransactionFailure(err error) bool {
	var pgErr interface{ SQLState() string }
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.SQLState() == "40001" || pgErr.SQLState() == "40P01"
}
