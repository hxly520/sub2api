package httpapi

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hxly520/sub2api/points-system/internal/domain"
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
	server := &Server{}
	handler := server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://points.example.test/app/", nil))
	for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "X-Frame-Options", "Permissions-Policy"} {
		if recorder.Header().Get(header) == "" {
			t.Fatalf("security header %s is missing", header)
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

func TestAdminWebUsesCanonicalPolicyFieldsWithoutRedemptionControls(t *testing.T) {
	html, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	javascript, err := webFS.ReadFile("web/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(html) + string(javascript)
	for _, required := range []string{
		"policy-minimum-spend",
		"policy-consumer-only",
		"policy-refresh-time",
		"total-checkin-rewards",
		"settled_checkin_reward_microusd",
		"points_per_usd_hundredths",
		"reward_percentage_min_ppm",
		"checkin_platform_daily_cap_microusd",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("admin web is missing %q", required)
		}
	}
	for _, forbidden := range []string{"policy-min-redeem", "policy-max-redeem", "microusd_per_point"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("admin web still contains removed field %q", forbidden)
		}
	}
}

func TestParseDollarAmountUsesExactCents(t *testing.T) {
	tests := map[string]int64{"0.01": 10_000, "1": 1_000_000, "12.34": 12_340_000, "12.3": 12_300_000}
	for input, expected := range tests {
		actual, err := parseDollarAmount(input)
		if err != nil || actual != expected {
			t.Fatalf("parseDollarAmount(%q) = %d, %v", input, actual, err)
		}
	}
	for _, input := range []string{"", "0", "-1", "1.001", "1e2", ".50"} {
		if _, err := parseDollarAmount(input); err == nil {
			t.Fatalf("invalid amount %q was accepted", input)
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
