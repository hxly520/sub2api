package httpapi

import (
	"strings"
	"testing"
)

func TestUserDashboardKeepsCoreMetricsAndDelaysEmbeddedReady(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	commonJS, err := webFS.ReadFile("web/assets/common.js")
	if err != nil {
		t.Fatal(err)
	}
	userContent := string(userHTML) + string(userJS)
	for _, required := range []string{
		`aria-busy="true"`, `id="total-points"`, `id="yesterday-points"`,
		`id="total-checkin-rewards"`, `id="today-rewards"`, `id="average-points"`,
		`Math.round(totalPoints / chartState.days)`, `.finally(ui.notifyReady)`,
	} {
		if !strings.Contains(userContent, required) {
			t.Fatalf("user dashboard is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`id="yesterday-spend"`, `id="period-spend"`, `totalPoints / rows.length`, "/api/v1/admin/",
	} {
		if strings.Contains(userContent, forbidden) {
			t.Fatalf("user dashboard exposes out-of-scope content %q", forbidden)
		}
	}
	commonContent := string(commonJS)
	for _, required := range []string{"function notifyReady()", "readySent", "window.parent.postMessage", "notifyReady,", `"needs_review"].includes(value)`,
		"sub2api:points-theme", "event.source !== window.parent", "event.origin !== parentOrigin"} {
		if !strings.Contains(commonContent, required) {
			t.Fatalf("shared embedded UI is missing delayed ready behavior %q", required)
		}
	}
}

func TestAdminDashboardUsesCompleteTabsAndLocksInactiveCheckinSettings(t *testing.T) {
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS, err := webFS.ReadFile("web/assets/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(adminHTML) + string(adminJS)
	for _, required := range []string{
		`role="tablist"`, `role="tabpanel"`, `aria-controls="view-users"`,
		`aria-labelledby="tab-users"`, "moveAdminTab", `"ArrowLeft"`, `"ArrowRight"`,
		`id="checkin-settings"`, `id="checkin-tiers"`, `classList.toggle("is-locked"`,
		`control.disabled = !checkinEnabled`, `finally {`, "ui.notifyReady()",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("administrator dashboard is missing %q", required)
		}
	}
	for _, forbidden := range []string{"手工赠送", "grant-form", "snapshot-form", "/api/v1/admin/snapshots/refresh"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("administrator dashboard restored removed action %q", forbidden)
		}
	}
}
