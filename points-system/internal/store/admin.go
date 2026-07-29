package store

import (
	"context"
	"time"
)

type Snapshot struct {
	UserID                  int64     `json:"user_id"`
	BusinessDate            time.Time `json:"business_date"`
	ActualCostMicroUSD      int64     `json:"actual_cost_microusd"`
	AccountedSpendMicroUSD  int64     `json:"accounted_spend_microusd"`
	PolicyVersion           *int64    `json:"policy_version,omitempty"`
	PointsPerUSDHundredths  int64     `json:"points_per_usd_hundredths"`
	TargetPointsHundredths  int64     `json:"target_points_hundredths"`
	AwardedPointsHundredths int64     `json:"awarded_points_hundredths"`
	Revision                int       `json:"revision"`
	Status                  string    `json:"status"`
	SourceRowCount          int64     `json:"source_row_count"`
	SourceMaxUsageLogID     int64     `json:"source_max_usage_log_id"`
	SourceFingerprint       string    `json:"source_fingerprint"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (s *Store) Snapshot(ctx context.Context, userID int64, businessDate time.Time) (Snapshot, error) {
	var snapshot Snapshot
	err := s.DB.QueryRow(ctx, `SELECT user_id,business_date,actual_cost_microusd,
		accounted_spend_microusd,policy_version,points_per_usd_hundredths,
		target_points_hundredths,awarded_points_hundredths,revision,status,
		source_row_count,source_max_usage_log_id,COALESCE(source_fingerprint,''),updated_at
		FROM points_daily_snapshots WHERE user_id=$1 AND business_date=$2`, userID,
		dateString(s.BusinessDate(businessDate))).Scan(&snapshot.UserID, &snapshot.BusinessDate,
		&snapshot.ActualCostMicroUSD, &snapshot.AccountedSpendMicroUSD, &snapshot.PolicyVersion,
		&snapshot.PointsPerUSDHundredths, &snapshot.TargetPointsHundredths,
		&snapshot.AwardedPointsHundredths, &snapshot.Revision, &snapshot.Status,
		&snapshot.SourceRowCount, &snapshot.SourceMaxUsageLogID, &snapshot.SourceFingerprint,
		&snapshot.UpdatedAt)
	return snapshot, translateNotFound(err)
}

func (s *Store) Audit(ctx context.Context, actorID int64, action, targetType, targetID, requestID string, detail []byte) error {
	if len(detail) == 0 {
		detail = []byte(`{}`)
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO points_admin_audit(actor_user_id,action,target_type,target_id,
		request_id,detail) VALUES($1,$2,$3,$4,$5,$6::jsonb)`, actorID, action, targetType,
		targetID, nullableString(requestID), detail)
	return err
}

func (s *Store) Ping(ctx context.Context) error {
	return s.DB.Ping(ctx)
}
