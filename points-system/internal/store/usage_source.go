package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sub2UsageAggregateQuery = `
	SELECT user_id,
		ROUND(SUM(actual_cost) * 1000000)::bigint AS actual_cost_microusd,
		COUNT(*)::bigint AS source_row_count,
		MAX(id)::bigint AS source_max_usage_log_id,
		MD5(STRING_AGG(id::text || ':' || actual_cost::text, ',' ORDER BY id)) AS source_fingerprint
	FROM usage_logs
	WHERE billing_type = 0
		AND actual_cost > 0
		AND created_at >= $1
		AND created_at < $2
	GROUP BY user_id
	HAVING ROUND(SUM(actual_cost) * 1000000)::bigint > 0
	ORDER BY user_id`

const sub2UsageAccessProbeQuery = `
	SELECT id,user_id,billing_type,actual_cost,created_at
	FROM usage_logs
	LIMIT 0`

const sub2SuccessfulUsageBoundsQuery = `
	SELECT MIN(created_at),MAX(created_at)
	FROM usage_logs
	WHERE billing_type = 0
		AND actual_cost > 0`

const sub2UsageHistoryPlanQuery = `
	WITH filtered AS MATERIALIZED (
		SELECT id,user_id,actual_cost,created_at
		FROM usage_logs
		WHERE billing_type = 0
			AND actual_cost > 0
			AND created_at >= $1
			AND created_at < $2
	), daily AS (
		SELECT user_id,(created_at AT TIME ZONE $3)::date AS business_date,
			ROUND(SUM(actual_cost) * 1000000)::bigint AS actual_cost_microusd,
			COUNT(*)::bigint AS source_rows,
			MAX(id)::bigint AS source_max_usage_log_id
		FROM filtered
		GROUP BY user_id,(created_at AT TIME ZONE $3)::date
		HAVING ROUND(SUM(actual_cost) * 1000000)::bigint > 0
	)
	SELECT COUNT(DISTINCT user_id)::bigint,
		COUNT(*)::bigint,
		COUNT(DISTINCT business_date)::bigint,
		COALESCE(SUM(source_rows),0)::text,
		COALESCE(SUM(actual_cost_microusd),0)::text,
		COALESCE(SUM(TRUNC(actual_cost_microusd::numeric * $4::numeric / 1000000)),0)::text,
		COALESCE(MAX(source_max_usage_log_id),0)::bigint
	FROM daily`

// UsageAggregate is a server-derived daily balance-spend fact. Monetary values
// are rounded once, after PostgreSQL sums its DECIMAL(20,10) source values.
type UsageAggregate struct {
	UserID              int64
	ActualCostMicroUSD  int64
	SourceRowCount      int64
	SourceMaxUsageLogID int64
	SourceFingerprint   string
}

type UsageDay struct {
	WindowStart time.Time
	WindowEnd   time.Time
	Aggregates  []UsageAggregate
	Fingerprint string
	SourceRows  int64
}

type SuccessfulUsageBounds struct {
	Found       bool
	EarliestUTC time.Time
	LatestUTC   time.Time
}

type UsageHistorySummary struct {
	SourceUsers         int64 `json:"source_users"`
	SourceUserDays      int64 `json:"source_user_days"`
	SourceBusinessDays  int64 `json:"source_business_days"`
	SourceRows          int64 `json:"source_rows"`
	SpendMicroUSD       int64 `json:"spend_microusd"`
	PointsHundredths    int64 `json:"points_hundredths"`
	SourceMaxUsageLogID int64 `json:"source_max_usage_log_id"`
}

type UsageSource interface {
	AggregateDay(context.Context, time.Time, time.Time) (UsageDay, error)
}

type usageQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// PostgreSQLUsageSource reads Sub2API's usage_logs through a dedicated pool.
// The pool is configured read-only by NewReadOnlyUsagePool; this type exposes
// no write operation.
type PostgreSQLUsageSource struct {
	db usageQueryer
}

func NewPostgreSQLUsageSource(db *pgxpool.Pool) *PostgreSQLUsageSource {
	return &PostgreSQLUsageSource{db: db}
}

func (s *PostgreSQLUsageSource) Validate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("usage source is not configured")
	}
	rows, err := s.db.Query(ctx, sub2UsageAccessProbeQuery)
	if err != nil {
		return fmt.Errorf("validate Sub2API usage_logs read access: %w", err)
	}
	rows.Close()
	return rows.Err()
}

func (s *PostgreSQLUsageSource) SuccessfulUsageBounds(ctx context.Context) (SuccessfulUsageBounds, error) {
	if s == nil || s.db == nil {
		return SuccessfulUsageBounds{}, fmt.Errorf("usage source is not configured")
	}
	var earliest, latest *time.Time
	if err := s.db.QueryRow(ctx, sub2SuccessfulUsageBoundsQuery).Scan(&earliest, &latest); err != nil {
		return SuccessfulUsageBounds{}, fmt.Errorf("query Sub2API usage history bounds: %w", err)
	}
	if earliest == nil || latest == nil {
		return SuccessfulUsageBounds{}, nil
	}
	return SuccessfulUsageBounds{
		Found: true, EarliestUTC: earliest.UTC(), LatestUTC: latest.UTC(),
	}, nil
}

func (s *PostgreSQLUsageSource) SummarizeHistory(ctx context.Context, start, end time.Time,
	timezone string, pointsPerUSDHundredths int64) (UsageHistorySummary, error) {
	if s == nil || s.db == nil || start.IsZero() || !end.After(start) || timezone == "" ||
		pointsPerUSDHundredths <= 0 {
		return UsageHistorySummary{}, fmt.Errorf("invalid usage history summary request")
	}
	var summary UsageHistorySummary
	var sourceRows, spend, points string
	err := s.db.QueryRow(ctx, sub2UsageHistoryPlanQuery, start.UTC(), end.UTC(), timezone,
		pointsPerUSDHundredths).Scan(&summary.SourceUsers, &summary.SourceUserDays,
		&summary.SourceBusinessDays, &sourceRows, &spend, &points,
		&summary.SourceMaxUsageLogID)
	if err != nil {
		return UsageHistorySummary{}, fmt.Errorf("summarize Sub2API usage history: %w", err)
	}
	for label, item := range map[string]struct {
		raw    string
		target *int64
	}{
		"source rows": {sourceRows, &summary.SourceRows},
		"spend":       {spend, &summary.SpendMicroUSD},
		"points":      {points, &summary.PointsHundredths},
	} {
		value, parseErr := strconv.ParseInt(item.raw, 10, 64)
		if parseErr != nil || value < 0 {
			return UsageHistorySummary{}, fmt.Errorf("Sub2API usage history %s exceeds supported range", label)
		}
		*item.target = value
	}
	return summary, nil
}

func NewReadOnlyUsagePool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	poolConfig, err := readOnlyUsagePoolConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open POINTS_USAGE_DATABASE_URL: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping POINTS_USAGE_DATABASE_URL: %w", err)
	}
	return pool, nil
}

func readOnlyUsagePoolConfig(databaseURL string) (*pgxpool.Config, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse POINTS_USAGE_DATABASE_URL: %w", err)
	}
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	poolConfig.ConnConfig.RuntimeParams["application_name"] = "sub2api-points-usage-reader"
	poolConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	poolConfig.MinConns = 0
	poolConfig.MaxConns = 4
	return poolConfig, nil
}

func (s *PostgreSQLUsageSource) AggregateDay(ctx context.Context, start, end time.Time) (UsageDay, error) {
	if s == nil || s.db == nil || start.IsZero() || !end.After(start) {
		return UsageDay{}, fmt.Errorf("usage source is not configured")
	}
	start = start.UTC()
	end = end.UTC()
	rows, err := s.db.Query(ctx, sub2UsageAggregateQuery, start, end)
	if err != nil {
		return UsageDay{}, fmt.Errorf("query Sub2API usage_logs: %w", err)
	}
	defer rows.Close()

	day := UsageDay{WindowStart: start, WindowEnd: end}
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d\x00%d\x00", start.UnixMicro(), end.UnixMicro())
	for rows.Next() {
		var item UsageAggregate
		if err := rows.Scan(&item.UserID, &item.ActualCostMicroUSD, &item.SourceRowCount,
			&item.SourceMaxUsageLogID, &item.SourceFingerprint); err != nil {
			return UsageDay{}, fmt.Errorf("scan Sub2API usage aggregate: %w", err)
		}
		if item.UserID <= 0 || item.ActualCostMicroUSD <= 0 || item.SourceRowCount <= 0 ||
			item.SourceMaxUsageLogID <= 0 || len(item.SourceFingerprint) != 32 {
			return UsageDay{}, fmt.Errorf("Sub2API usage aggregate contains invalid values")
		}
		fingerprint := sha256.Sum256([]byte(item.SourceFingerprint))
		item.SourceFingerprint = hex.EncodeToString(fingerprint[:])
		day.Aggregates = append(day.Aggregates, item)
		day.SourceRows += item.SourceRowCount
		_, _ = fmt.Fprintf(hash, "%d\x00%d\x00%d\x00%d\x00%s\x00", item.UserID,
			item.ActualCostMicroUSD, item.SourceRowCount, item.SourceMaxUsageLogID,
			item.SourceFingerprint)
	}
	if err := rows.Err(); err != nil {
		return UsageDay{}, fmt.Errorf("iterate Sub2API usage aggregates: %w", err)
	}
	day.Fingerprint = hex.EncodeToString(hash.Sum(nil))
	return day, nil
}
