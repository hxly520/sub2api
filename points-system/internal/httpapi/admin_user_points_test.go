package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQueryOffsetUsesBoundedDefault(t *testing.T) {
	for raw, expected := range map[string]int{
		"": 0, "-1": 0, "bad": 0, "1000001": 0, "0": 0, "75": 75, "1000000": 1_000_000,
	} {
		request := httptest.NewRequest(http.MethodGet,
			"https://points.example.test/api/v1/admin/users/points?offset="+raw, nil)
		if actual := queryOffset(request); actual != expected {
			t.Fatalf("queryOffset(%q) = %d, want %d", raw, actual, expected)
		}
	}
}

func TestAdminUserPointsResponseUsesWhitelistedFields(t *testing.T) {
	date := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	body, err := json.Marshal(publicAdminUserPoints{
		UserID:                    7,
		TotalPointsHundredths:     12_345,
		YesterdayPointsHundredths: 125,
		TotalSpendMicroUSD:        12_345_000,
		YesterdaySpendMicroUSD:    125_000,
		SnapshotBusinessDate:      &date,
		SnapshotStatus:            "ready",
	})
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	for _, required := range []string{
		`"user_id"`, `"total_points_hundredths"`, `"yesterday_points_hundredths"`,
		`"total_spend_microusd"`, `"yesterday_spend_microusd"`,
		`"snapshot_business_date"`, `"snapshot_status"`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("administrator user points response is missing %s: %s", required, content)
		}
	}
	for _, forbidden := range []string{
		"email", "balance", "policy_version", "source_fingerprint", "source_max_usage_log_id",
		"external_event_id", "request_fingerprint", "metadata",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("administrator user points response exposed %q: %s", forbidden, content)
		}
	}
}

func TestUserCannotCallAdminUserPointsEndpoint(t *testing.T) {
	server := testRoleServer("user", true)
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/api/v1/admin/users/points"))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("user GET admin user points status = %d, want 403", recorder.Code)
	}
}

func TestUserCannotCallAdminBalanceGrantSummaryEndpoint(t *testing.T) {
	server := testRoleServer("user", true)
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/api/v1/admin/balance-grants/summary"))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("user GET admin balance grant summary status = %d, want 403", recorder.Code)
	}
}

func TestAdminWebRemovesManualGrantAndManualSnapshotRefresh(t *testing.T) {
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
		"用户积分明细", "admin-users-body", "/api/v1/admin/users/points",
		"签到余额发放记录", "每日自动结算", "这里不是积分功能开关",
		"/api/v1/admin/balance-grants/summary", "reversal_permanently_failed",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("administrator web is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"手工赠送", "grant-form", "/api/v1/admin/grants", "snapshot-form",
		"/api/v1/admin/snapshots/refresh", "执行刷新",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("administrator web still exposes removed action %q", forbidden)
		}
	}
}

func TestRemovedAdminMutationRoutesAreNotRegistered(t *testing.T) {
	server := testRoleServer("admin", true)
	for _, path := range []string{"/api/v1/admin/grants", "/api/v1/admin/snapshots/refresh"} {
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, requestWithSession(http.MethodPost, path))
		if recorder.Code < http.StatusBadRequest {
			t.Fatalf("removed POST %s unexpectedly returned status %d", path, recorder.Code)
		}
	}
}
