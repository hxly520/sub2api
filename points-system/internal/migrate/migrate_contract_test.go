package migrate

import (
	"strings"
	"testing"
)

func TestInitialSchemaUsesReadOnlyHundredthPointsModel(t *testing.T) {
	body, err := migrationFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(body)
	for _, required := range []string{
		"points_per_usd_hundredths",
		"lower_points_hundredths",
		"total_points_hundredths",
		"minimum_checkin_spend_microusd",
		"refresh_minute",
		"points_snapshot_refresh_runs",
		"accounted_spend_microusd",
		"target_points_hundredths",
		"awarded_points_hundredths",
		"source_fingerprint",
		"points_snapshot_refresh_runs_guard",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("schema is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"points_redemptions", "points_redemption_attempts", "microusd_per_point",
		"points_usage_events",
	} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("schema still contains removed redemption model %q", forbidden)
		}
	}
}
