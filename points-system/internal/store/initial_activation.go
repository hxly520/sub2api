package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/jackc/pgx/v5"
)

// CreateInitialActivationPolicy appends the first enabled policy on the
// current natural day. It exists for the initial rollout only; later policy
// changes continue to use CreatePolicyAtomic and its next-day boundary.
func (s *Store) CreateInitialActivationPolicy(ctx context.Context, actorUserID,
	pointsPerUSDHundredths int64, now time.Time) (domain.Policy, error) {
	if s == nil || s.DB == nil || s.Location == nil || actorUserID <= 0 ||
		pointsPerUSDHundredths <= 0 {
		return domain.Policy{}, errors.New("invalid initial points activation")
	}
	if now.IsZero() {
		now = time.Now()
	}
	effectiveDate := s.BusinessDate(now)
	var created domain.Policy
	err := s.withSerializableTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			historyBackfillLock); err != nil {
			return err
		}
		var enabledPolicies int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM points_policy_versions
			WHERE enabled=TRUE`).Scan(&enabledPolicies); err != nil {
			return err
		}
		if enabledPolicies > 0 {
			current, err := policyForDate(ctx, tx, effectiveDate)
			if err == nil && current.Enabled && !current.CheckinEnabled &&
				current.RefreshMinute == 5 &&
				current.PointsPerUSDHundredths == pointsPerUSDHundredths &&
				dateString(current.EffectiveDate) == dateString(effectiveDate) {
				created = current
				return nil
			}
			return errors.New("an enabled points policy already exists; use normal versioning")
		}

		var version int64
		if err := tx.QueryRow(ctx, `INSERT INTO points_policy_versions(
			effective_date,enabled,mode,basis,checkin_enabled,points_per_usd_hundredths,
			refresh_minute,created_by
		) VALUES($1,TRUE,'all_users','yesterday',FALSE,$2,5,$3)
		RETURNING version_no`, dateString(effectiveDate), pointsPerUSDHundredths,
			actorUserID).Scan(&version); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO points_admin_audit(
			actor_user_id,action,target_type,target_id,detail
		) VALUES($1,'policy.initial_activate','policy',$2,
			jsonb_build_object('effective_date',$3::text,'enabled',TRUE,'checkin_enabled',FALSE,
			'points_per_usd_hundredths',$4::bigint,'refresh_minute',5))`, actorUserID,
			fmt.Sprint(version), dateString(effectiveDate), pointsPerUSDHundredths); err != nil {
			return err
		}
		var err error
		created, err = policyForVersion(ctx, tx, version)
		return err
	})
	if err != nil {
		return domain.Policy{}, fmt.Errorf("create initial points activation policy: %w", err)
	}
	return created, nil
}
