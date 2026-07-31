package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/config"
	"github.com/hxly520/sub2api/points-system/internal/domain"
	"github.com/hxly520/sub2api/points-system/internal/security"
	"github.com/hxly520/sub2api/points-system/internal/store"
)

func TestClientIPOnlyTrustsConfiguredProxy(t *testing.T) {
	_, trusted, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{trustedProxy: trusted}

	trustedRequest := httptest.NewRequest(http.MethodGet, "https://points.example.test/launch", nil)
	trustedRequest.RemoteAddr = "10.0.0.5:1234"
	trustedRequest.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := server.clientIP(trustedRequest); got != "203.0.113.9" {
		t.Fatalf("trusted proxy client IP = %q", got)
	}

	untrustedRequest := httptest.NewRequest(http.MethodGet, "https://points.example.test/launch", nil)
	untrustedRequest.RemoteAddr = "198.51.100.7:1234"
	untrustedRequest.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := server.clientIP(untrustedRequest); got != "198.51.100.7" {
		t.Fatalf("untrusted forwarded header was accepted: %q", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	server := &Server{Config: config.Config{EmbedParentOrigin: "https://sub2api.example.test"}}
	handler := server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://points.example.test/app/", nil))
	for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "Permissions-Policy"} {
		if recorder.Header().Get(header) == "" {
			t.Fatalf("security header %s is missing", header)
		}
	}
	csp := recorder.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors https://sub2api.example.test;") ||
		strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("unexpected frame-ancestors policy: %q", csp)
	}
	if !strings.Contains(csp, "img-src 'self' data: https://sub2api.example.test;") {
		t.Fatalf("uploaded logo origin is missing from img-src: %q", csp)
	}
	if value := recorder.Header().Get("X-Frame-Options"); value != "" {
		t.Fatalf("X-Frame-Options conflicts with exact CSP embedding policy: %q", value)
	}
}

func TestSecurityHeadersFailClosedWithoutEmbedParent(t *testing.T) {
	server := &Server{}
	handler := server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://points.example.test/app/", nil))
	if csp := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none';") {
		t.Fatalf("missing fail-closed frame-ancestors: %q", csp)
	}
}

func TestWorkspaceDestinationPreservesOnlyEmbeddedPresentationMode(t *testing.T) {
	for _, test := range []struct {
		role, mode, want string
	}{
		{role: "user", mode: embeddedUIMode, want: "/app/?ui_mode=embedded"},
		{role: "admin", mode: embeddedUIMode, want: "/admin/?ui_mode=embedded"},
		{role: "user", mode: "standalone", want: "/app/"},
		{role: "admin", mode: "unexpected", want: "/admin/"},
	} {
		if got := workspaceDestination(test.role, test.mode); got != test.want {
			t.Fatalf("workspaceDestination(%q, %q) = %q, want %q", test.role, test.mode, got, test.want)
		}
	}
}

func TestRequestedUIModeAcceptsOnlyExactEmbeddedValue(t *testing.T) {
	for _, test := range []struct {
		query, want string
	}{
		{query: "ui_mode=embedded", want: embeddedUIMode},
		{query: "ui_mode=standalone", want: ""},
		{query: "ui_mode=EMBEDDED", want: ""},
		{query: "ui_mode=embedded%20", want: ""},
		{query: "other=embedded", want: ""},
	} {
		request := httptest.NewRequest(http.MethodGet, "https://points.example.test/launch?"+test.query, nil)
		if got := requestedUIMode(request); got != test.want {
			t.Fatalf("requestedUIMode(%q) = %q, want %q", test.query, got, test.want)
		}
	}
}

func TestDirectRootDoesNotOpenWorkspace(t *testing.T) {
	server := &Server{mux: http.NewServeMux()}
	server.routes()
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://points.example.test/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("direct root status = %d, want 404", recorder.Code)
	}
}

func TestPointsPagesUseUploadedSub2APILogoAndLocalFallback(t *testing.T) {
	server := testRoleServer("user", true)
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/app/"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("user page status = %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `src="https://sub2api.example.test/api/v1/settings/logo"`) ||
		!strings.Contains(body, `name="sub2api-parent-origin" content="https://sub2api.example.test"`) ||
		strings.Contains(body, brandLogoPlaceholder) || strings.Contains(body, embedParentOriginPlaceholder) ||
		strings.Contains(body, `aria-hidden="true">积`) {
		t.Fatalf("user page did not receive uploaded logo URL: %s", body)
	}

	adminServer := testRoleServer("admin", true)
	recorder = httptest.NewRecorder()
	adminServer.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/admin/"))
	body = recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `src="https://sub2api.example.test/api/v1/settings/logo"`) ||
		strings.Contains(body, `aria-hidden="true">管`) {
		t.Fatalf("admin page did not receive uploaded logo URL: status=%d body=%s", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	adminServer.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/assets/logo.svg"))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/svg+xml" ||
		!strings.Contains(recorder.Body.String(), "Sub2API") {
		t.Fatalf("local logo fallback failed: status=%d type=%q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
}

func TestPolicyRequestDefaultsAndLocksConsumerBasis(t *testing.T) {
	policy := (policyRequest{
		Enabled:        true,
		Mode:           domain.PolicyModeConsumerOnly,
		Basis:          domain.PolicyBasisTotal,
		CheckinEnabled: true,
	}).toPolicy(time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), 7)
	if policy.Basis != domain.PolicyBasisYesterday {
		t.Fatalf("consumer-only basis = %q, want yesterday", policy.Basis)
	}
	if policy.CheckinDailyLimit == nil || *policy.CheckinDailyLimit != 1 {
		t.Fatalf("default daily limit = %v, want 1", policy.CheckinDailyLimit)
	}
	if policy.PointsPerUSDHundredths != 1_000 {
		t.Fatalf("default ratio = %d, want 1000", policy.PointsPerUSDHundredths)
	}
	if policy.RefreshMinute != 5 {
		t.Fatalf("default refresh minute = %d, want 5", policy.RefreshMinute)
	}
}

func TestPolicyRequestPreservesExplicitMidnightAndHundredthRatio(t *testing.T) {
	midnight := 0
	ratio := int64(1_025)
	policy := (policyRequest{
		Enabled:                true,
		Mode:                   domain.PolicyModeAllUsers,
		Basis:                  domain.PolicyBasisTotal,
		PointsPerUSDHundredths: &ratio,
		RefreshMinute:          &midnight,
	}).toPolicy(time.Now(), 9)
	if policy.PointsPerUSDHundredths != 1_025 || policy.RefreshMinute != 0 {
		t.Fatalf("explicit policy values changed: %+v", policy)
	}
}

func TestPolicyRequestDoesNotDefaultAnExplicitZeroRatio(t *testing.T) {
	zero := int64(0)
	policy := (policyRequest{
		Enabled:                true,
		Mode:                   domain.PolicyModeAllUsers,
		Basis:                  domain.PolicyBasisYesterday,
		PointsPerUSDHundredths: &zero,
	}).toPolicy(time.Now(), 9)
	if err := policy.ValidateForEnable(); err == nil {
		t.Fatal("explicit zero points/U ratio was treated as the default")
	}
}

func TestPolicyRequestMapsCheckinEligibilityAndSafetyLimits(t *testing.T) {
	dailyLimit, refreshMinute := 2, 305
	minimumSpend := int64(2_500_000)
	platformCap, userCap, singleCap := int64(100_000_000), int64(2_000_000), int64(1_000_000)
	ratio := int64(1_025)
	policy := (policyRequest{
		Enabled:                         true,
		Mode:                            domain.PolicyModeAllUsers,
		Basis:                           domain.PolicyBasisTotal,
		CheckinEnabled:                  true,
		CheckinDailyLimit:               &dailyLimit,
		MinimumCheckinSpendMicroUSD:     minimumSpend,
		CheckinPlatformDailyCapMicroUSD: &platformCap,
		CheckinUserDailyCapMicroUSD:     &userCap,
		CheckinSingleAwardCapMicroUSD:   &singleCap,
		PointsPerUSDHundredths:          &ratio,
		RefreshMinute:                   &refreshMinute,
	}).toPolicy(time.Now(), 9)
	if policy.MinimumCheckinSpendMicroUSD != minimumSpend ||
		policy.CheckinPlatformDailyCapMicroUSD != &platformCap ||
		policy.CheckinUserDailyCapMicroUSD != &userCap ||
		policy.CheckinSingleAwardCapMicroUSD != &singleCap ||
		policy.RefreshMinute != refreshMinute {
		t.Fatalf("policy request fields were not preserved: %+v", policy)
	}
}

func TestRoleSeparatedWebAssetsAreChineseAndLeastPrivilege(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	userJS, err := webFS.ReadFile("web/assets/user.js")
	if err != nil {
		t.Fatal(err)
	}
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	adminJS, err := webFS.ReadFile("web/assets/admin.js")
	if err != nil {
		t.Fatal(err)
	}
	commonJS, err := webFS.ReadFile("web/assets/common.js")
	if err != nil {
		t.Fatal(err)
	}
	userContent := string(userHTML) + string(userJS)
	adminContent := string(adminHTML) + string(adminJS)
	for _, required := range []string{"lang=\"zh-CN\"", "我的总积分", "昨日积分", "累计签到赠送", "points-chart", "个人积分记录", "签到奖励记录"} {
		if !strings.Contains(userContent, required) {
			t.Fatalf("user web is missing %q", required)
		}
	}
	for _, forbidden := range []string{"/api/v1/admin/", "grant-form", "policy-form", "admin.js", "管理员"} {
		if strings.Contains(userContent, forbidden) {
			t.Fatalf("user web exposes administrator content %q", forbidden)
		}
	}
	for _, required := range []string{
		"lang=\"zh-CN\"",
		"policy-minimum-spend",
		"policy-consumer-only",
		"policy-refresh-time",
		"points_per_usd_hundredths",
		"reward_percentage_min_ppm",
		"checkin_platform_daily_cap_microusd",
	} {
		if !strings.Contains(adminContent, required) {
			t.Fatalf("admin web is missing %q", required)
		}
	}
	for _, forbidden := range []string{"policy-min-redeem", "policy-max-redeem", "microusd_per_point"} {
		if strings.Contains(adminContent, forbidden) {
			t.Fatalf("admin web still contains removed field %q", forbidden)
		}
	}
	commonContent := string(commonJS)
	for _, required := range []string{"sub2api:points-ready", "window.parent.postMessage", "admin-shell"} {
		if !strings.Contains(commonContent, required) {
			t.Fatalf("embedded ready handshake is missing %q", required)
		}
	}
}

func TestEmbeddedWebPresentationKeepsRolePagesInsideSub2API(t *testing.T) {
	userHTML, err := webFS.ReadFile("web/user.html")
	if err != nil {
		t.Fatal(err)
	}
	adminHTML, err := webFS.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	commonJS, err := webFS.ReadFile("web/assets/common.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := webFS.ReadFile("web/assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"user": string(userHTML), "admin": string(adminHTML)} {
		if !strings.Contains(content, `data-ui-mode="standalone"`) {
			t.Fatalf("%s page is missing the default presentation marker", name)
		}
	}
	for _, required := range []string{`get("ui_mode") === "embedded"`, "document.body.dataset.uiMode"} {
		if !strings.Contains(string(commonJS), required) {
			t.Fatalf("common script is missing embedded-mode behavior %q", required)
		}
	}
	for _, required := range []string{
		`body[data-ui-mode="embedded"] .user-topbar`,
		`body[data-ui-mode="embedded"] .admin-brand`,
		`body[data-ui-mode="embedded"] .admin-sidebar-footer`,
		`body[data-ui-mode="embedded"] .admin-nav`,
	} {
		if !strings.Contains(string(css), required) {
			t.Fatalf("stylesheet is missing embedded layout rule %q", required)
		}
	}
}

func testRoleServer(role string, enabled bool) *Server {
	policy := domain.Policy{Enabled: enabled, Mode: domain.PolicyModeAllUsers,
		Basis: domain.PolicyBasisYesterday, PointsPerUSDHundredths: 1_000, RefreshMinute: 5}
	server := &Server{
		Config: configForWebTest(), mux: http.NewServeMux(), limits: security.NewRateLimiter(),
		sessionLookup: func(context.Context, string, time.Time) (store.Session, error) {
			return store.Session{UserID: 7, Role: role, Theme: "light", Language: "zh-CN",
				ExpiresAt: time.Now().Add(time.Hour)}, nil
		},
		policyLookup:   func(context.Context, time.Time) (domain.Policy, error) { return policy, nil },
		usernameLookup: func(context.Context, int64) (string, error) { return "测试用户", nil },
	}
	server.routes()
	return server
}

func configForWebTest() config.Config {
	return config.Config{Timezone: time.UTC, PublicOrigin: "https://points.example.test",
		EmbedParentOrigin: "https://sub2api.example.test",
		SessionSecret:     []byte("01234567890123456789012345678901")}
}

func requestWithSession(method, path string) *http.Request {
	request := httptest.NewRequest(method, "https://points.example.test"+path, nil)
	request.AddCookie(&http.Cookie{Name: "points_session", Value: "test-session-token"})
	return request
}

func TestUserWebCannotSeeOrCallAdministratorSurface(t *testing.T) {
	server := testRoleServer("user", true)
	for _, test := range []struct {
		path string
		want int
	}{
		{path: "/app/", want: http.StatusOK},
		{path: "/assets/user.js", want: http.StatusOK},
		{path: "/admin/", want: http.StatusForbidden},
		{path: "/assets/admin.js", want: http.StatusForbidden},
		{path: "/api/v1/admin/me", want: http.StatusForbidden},
		{path: "/api/v1/admin/policies", want: http.StatusForbidden},
	} {
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, test.path))
		if recorder.Code != test.want {
			t.Fatalf("GET %s status = %d, want %d", test.path, recorder.Code, test.want)
		}
	}
}

func TestAdministratorCannotUseUserFinancialSurface(t *testing.T) {
	server := testRoleServer("admin", true)
	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/app/"},
		{method: http.MethodGet, path: "/assets/user.js"},
		{method: http.MethodGet, path: "/api/v1/me"},
		{method: http.MethodGet, path: "/api/v1/ledger"},
		{method: http.MethodGet, path: "/api/v1/daily-points"},
		{method: http.MethodGet, path: "/api/v1/balance-grants"},
		{method: http.MethodPost, path: "/api/v1/checkins"},
	} {
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, requestWithSession(test.method, test.path))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want 403", test.method, test.path, recorder.Code)
		}
	}

	for _, path := range []string{"/admin/", "/assets/app.css", "/assets/common.js", "/assets/admin.js"} {
		recorder := httptest.NewRecorder()
		server.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, path))
		if recorder.Code != http.StatusOK {
			t.Fatalf("admin GET %s status = %d, want 200", path, recorder.Code)
		}
	}
}

func TestInvalidatedSourceUserSessionCannotReachAnyRoleSurface(t *testing.T) {
	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/me"},
		{method: http.MethodGet, path: "/api/v1/admin/me"},
		{method: http.MethodGet, path: "/api/v1/admin/policies"},
		{method: http.MethodPost, path: "/api/v1/admin/policies"},
		{method: http.MethodPost, path: "/api/v1/admin/balance-grants/00000000-0000-0000-0000-000000000000/retry"},
		{method: http.MethodPost, path: "/api/v1/admin/balance-grants/00000000-0000-0000-0000-000000000000/reverse"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			server := &Server{
				Config: configForWebTest(), mux: http.NewServeMux(), limits: security.NewRateLimiter(),
				sessionLookup: func(context.Context, string, time.Time) (store.Session, error) {
					// Store.Session maps a missing or soft-deleted source user to not found.
					return store.Session{}, domain.ErrNotFound
				},
			}
			server.routes()
			recorder := httptest.NewRecorder()
			server.mux.ServeHTTP(recorder, requestWithSession(test.method, test.path))
			if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "unauthorized") {
				t.Fatalf("invalidated session response: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestDisabledPolicyRejectsUserButKeepsAdminWorkspace(t *testing.T) {
	userServer := testRoleServer("user", false)
	for _, path := range []string{"/app/", "/assets/user.js", "/api/v1/me", "/api/v1/daily-points"} {
		recorder := httptest.NewRecorder()
		userServer.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, path))
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "points_disabled") {
			t.Fatalf("disabled user GET %s: status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}

	adminServer := testRoleServer("admin", false)
	recorder := httptest.NewRecorder()
	adminServer.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/admin/"))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "积分管理后台") {
		t.Fatalf("disabled admin workspace: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	adminServer.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/app/"))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin app status=%d, want 403", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	adminServer.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/app/?ui_mode=embedded"))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("embedded admin app status=%d, want 403", recorder.Code)
	}
}

func TestDisabledPolicyRejectsUserLaunchBeforeSessionCreation(t *testing.T) {
	now := time.Now().UTC()
	secret := []byte("01234567890123456789012345678901")
	ticket, err := security.SignLaunchTicket(security.LaunchClaims{
		Issuer: "sub2api", Audience: "points-system", Subject: 8, Role: "user", Theme: "light",
		Language: "zh-CN", Nonce: "disabled-user-launch", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(),
	}, "launch-v1", secret)
	if err != nil {
		t.Fatal(err)
	}
	server := testRoleServer("user", false)
	server.Config.LaunchIssuer = "sub2api"
	server.Config.LaunchAudience = "points-system"
	server.Config.LaunchKeys = map[string][]byte{"launch-v1": secret}
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet,
		"https://points.example.test/launch?ticket="+ticket, nil))
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "points_disabled") {
		t.Fatalf("disabled launch: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPolicyAccessRequiresEnabledValidPolicy(t *testing.T) {
	valid := domain.Policy{Enabled: true, Mode: domain.PolicyModeAllUsers,
		Basis: domain.PolicyBasisYesterday, PointsPerUSDHundredths: 1_000, RefreshMinute: 5}
	if !policyAllowsUserAccess(valid) {
		t.Fatal("enabled valid policy did not allow user access")
	}
	valid.Enabled = false
	if policyAllowsUserAccess(valid) {
		t.Fatal("disabled policy allowed user access")
	}
	valid.Enabled = true
	valid.PointsPerUSDHundredths = 0
	if policyAllowsUserAccess(valid) {
		t.Fatal("invalid policy allowed user access")
	}
}

func TestDailyPointResponseContainsNoIdentityOrPolicyInternals(t *testing.T) {
	body, err := json.Marshal(store.DailyPoint{BusinessDate: time.Now(), ActualCostMicroUSD: 1_000_000,
		AwardedPointsHundredths: 1_000, Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	for _, forbidden := range []string{"user_id", "policy_version", "source_fingerprint", "source_max_usage_log_id"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("daily point response exposed %q: %s", forbidden, content)
		}
	}
}

func TestPublicActivityResponsesHideInternalIdentifiers(t *testing.T) {
	ledger, err := json.Marshal(publicLedgerEntry{Kind: "usage_points", DeltaPointsHundredths: 100,
		TotalAfterHundredths: 200, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := json.Marshal(publicBalanceGrant{AmountMicroUSD: 10_000, Kind: "checkin",
		Status: "settled", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	checkin, err := json.Marshal(publicCheckinResult{Ordinal: 1, RewardMicroUSD: 10_000,
		DeliveryStatus: "settled"})
	if err != nil {
		t.Fatal(err)
	}
	content := string(ledger) + string(grant) + string(checkin)
	for _, forbidden := range []string{"user_id", "external_event_id", "policy_version", "reference_id",
		"metadata", "last_error", "reason", "transaction_id", "request_fingerprint"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("public activity response exposed %q: %s", forbidden, content)
		}
	}
}

func TestPublicAccountAndAdminProfileHideNumericUserID(t *testing.T) {
	account, err := json.Marshal(publicAccountFrom(domain.Account{
		UserID: 7, TotalPointsHundredths: 123, TotalSpendMicroUSD: 456_000,
		SettledCheckinRewardMicroUSD: 10_000, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(account), "user_id") {
		t.Fatalf("public account exposed numeric identity: %s", account)
	}

	server := testRoleServer("admin", true)
	recorder := httptest.NewRecorder()
	server.mux.ServeHTTP(recorder, requestWithSession(http.MethodGet, "/api/v1/admin/me"))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"username":"测试用户"`) ||
		strings.Contains(recorder.Body.String(), "user_id") {
		t.Fatalf("administrator profile identity response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicUsernameNeverFallsBackToNumericIdentity(t *testing.T) {
	if got := publicUsername("  alice  "); got != "alice" {
		t.Fatalf("trimmed username = %q, want alice", got)
	}
	if got := publicUsername("  "); got != "未设置用户名" {
		t.Fatalf("empty username fallback = %q", got)
	}
}

func TestQueryDaysUsesBoundedDefault(t *testing.T) {
	for raw, expected := range map[string]int{"": 30, "0": 30, "-1": 30, "91": 30, "bad": 30, "7": 7, "90": 90} {
		request := httptest.NewRequest(http.MethodGet, "https://points.example.test/api/v1/daily-points?days="+raw, nil)
		if actual := queryDays(request); actual != expected {
			t.Fatalf("queryDays(%q) = %d, want %d", raw, actual, expected)
		}
	}
}

func TestSnapshotNotReadyIsRetryableServiceUnavailable(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeDomainError(recorder, domain.ErrSnapshotNotReady)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "snapshot_not_ready") {
		t.Fatalf("unexpected response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicSnapshotHidesSourceAndPolicyInternals(t *testing.T) {
	value := publicSnapshot(store.Snapshot{SourceFingerprint: strings.Repeat("a", 64),
		SourceMaxUsageLogID: 99, PolicyVersion: func() *int64 { value := int64(7); return &value }()})
	for _, key := range []string{"source_fingerprint", "source_max_usage_log_id", "policy_version"} {
		if _, exposed := value[key]; exposed {
			t.Fatalf("public snapshot exposed %q", key)
		}
	}
}
