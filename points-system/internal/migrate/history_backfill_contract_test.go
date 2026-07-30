package migrate

import (
	"strings"
	"testing"
)

func TestHistoryBackfillMigrationDefinesAuditedImmutableDailyBaseline(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/003_usage_history_backfill.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(body)
	for _, required := range []string{
		"points_usage_history_backfill_jobs",
		"points_usage_history_backfill_days",
		"history_backfill",
		"plan_fingerprint",
		"next_date",
		"policy_version",
		"points_per_usd_hundredths",
		"planned_source_business_days",
		"applied_source_max_usage_log_id",
		"points_usage_history_backfill_days_immutable",
		"points_usage_history_backfill_jobs_guard",
		"status <> 'succeeded'",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("history backfill migration is missing %q", required)
		}
	}
	if strings.Contains(schema, "UPDATE points_policy_versions") {
		t.Fatal("history backfill migration must not mutate immutable policy versions")
	}
}
