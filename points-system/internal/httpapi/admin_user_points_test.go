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
		LoginEmail:                "alice@example.com",
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
		`"login_email":"alice@example.com"`, `"total_points_hundredths"`, `"yesterday_points_hundredths"`,
		`"total_spend_microusd"`, `"yesterday_spend_microusd"`,
		`"snapshot_business_date"`, `"snapshot_status"`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("administrator user points response is missing %s: %s", required, content)
		}
	}
	for _, forbidden := range []string{
		"user_id", "username", "balance", "policy_version", "source_fingerprint", "source_max_usage_log_id",
		"external_event_id", "request_fingerprint", "metadata",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("administrator user points response exposed %q: %s", forbidden, content)
		}
	}
}

func TestAdminBalanceGrantResponseUsesLoginEmailWithoutInternalUserID(t *testing.T) {
	body, err := json.Marshal(publicAdminBalanceGrant{
		ID: "00000000-0000-4000-8000-000000000001", LoginEmail: "alice@example.com",
		AmountMicroUSD: 100_000, Kind: "checkin", Status: "settled", Attempts: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	for _, required := range []string{`"id"`, `"login_email":"alice@example.com"`, `"amount_microusd"`, `"status"`, `"attempts"`} {
		if !strings.Contains(content, required) {
			t.Fatalf("administrator balance grant response is missing %s: %s", required, content)
		}
	}
	for _, forbidden := range []string{"user_id", "external_event_id", "policy_version", "request_fingerprint"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("administrator balance grant response exposed %q: %s", forbidden, content)
		}
	}
}

func TestBrowserIdentitySurfacesUseLoginEmailOnly(t *testing.T) {
	paths := []string{
		"web/user.html", "web/admin.html", "web/assets/user.js", "web/assets/admin.js",
	}
	var content strings.Builder
	for _, path := range paths {
		body, err := webFS.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content.Write(body)
	}
	combined := content.String()
	for _, required := range []string{"login_email", "login-email", "admin-login-email", "登录邮箱"} {
		if !strings.Contains(combined, required) {
			t.Fatalf("browser identity surfaces are missing %q", required)
		}
	}
	for _, forbidden := range []string{"username", "用户名", "user_id", "user-id", "admin-user-id", "用户 ID", "用户ID"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("browser identity surfaces still expose %q", forbidden)
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
