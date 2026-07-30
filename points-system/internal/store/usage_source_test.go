package store

import (
	"strings"
	"testing"
	"time"
)

func TestReadOnlyUsagePoolConfigEnforcesReadOnlyAndConnectionLimit(t *testing.T) {
	cfg, err := readOnlyUsagePoolConfig("postgres://reader:secret@localhost/sub2api?sslmode=disable&pool_min_conns=10&pool_max_conns=20")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ConnConfig.RuntimeParams["default_transaction_read_only"]; got != "on" {
		t.Fatalf("default_transaction_read_only = %q, want on", got)
	}
	if got := cfg.ConnConfig.RuntimeParams["application_name"]; got != "sub2api-points-usage-reader" {
		t.Fatalf("application_name = %q", got)
	}
	if cfg.MaxConns != 4 {
		t.Fatalf("MaxConns = %d, want 4", cfg.MaxConns)
	}
	if cfg.MinConns != 0 {
		t.Fatalf("MinConns = %d, want 0", cfg.MinConns)
	}
}

func TestUsageAggregateQueryOnlyCountsSuccessfulBalanceSpend(t *testing.T) {
	for _, required := range []string{
		"FROM usage_logs", "billing_type = 0", "actual_cost > 0",
		"created_at >= $1", "created_at < $2",
		"HAVING ROUND(SUM(actual_cost) * 1000000)::bigint > 0",
	} {
		if !strings.Contains(sub2UsageAggregateQuery, required) {
			t.Fatalf("usage query is missing %q", required)
		}
	}
	if strings.Contains(sub2UsageAggregateQuery, "subscription_id") {
		t.Fatal("usage query must not include subscription consumption")
	}
}

func TestUsageAccessProbeRequiresOnlyGrantedColumns(t *testing.T) {
	for _, column := range []string{"id", "user_id", "billing_type", "actual_cost", "created_at"} {
		if !strings.Contains(sub2UsageAccessProbeQuery, column) {
			t.Fatalf("usage access probe is missing %q", column)
		}
	}
	if !strings.Contains(sub2UsageAccessProbeQuery, "LIMIT 0") {
		t.Fatal("usage access probe must not read business rows")
	}
}

func TestUsageHistoryQueriesOnlyCountSuccessfulBalanceSpend(t *testing.T) {
	for _, query := range []string{sub2SuccessfulUsageBoundsQuery, sub2UsageHistoryPlanQuery} {
		for _, required := range []string{"FROM usage_logs", "billing_type = 0", "actual_cost > 0"} {
			if !strings.Contains(query, required) {
				t.Fatalf("history query is missing %q", required)
			}
		}
	}
	for _, required := range []string{
		"created_at >= $1", "created_at < $2", "AT TIME ZONE $3", "TRUNC(",
		"ROUND(SUM(actual_cost) * 1000000)",
	} {
		if !strings.Contains(sub2UsageHistoryPlanQuery, required) {
			t.Fatalf("history plan query is missing %q", required)
		}
	}
	if strings.Contains(sub2UsageHistoryPlanQuery, "subscription_id") {
		t.Fatal("history plan must not include subscription consumption")
	}
}

func TestBusinessDayWindowUsesShanghaiNaturalDayAcrossUTC(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 7, 29, 0, 0, 0, 0, location)
	if got := date.UTC(); !got.Equal(time.Date(2026, 7, 28, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("window start = %s", got)
	}
	if got := date.AddDate(0, 0, 1).UTC(); !got.Equal(time.Date(2026, 7, 29, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("window end = %s", got)
	}
}
