package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type pointsBridgeRepositoryStub struct {
	input PointsBalanceCreditInput
}

type pointsBalanceCacheStub struct {
	err   error
	calls int
}

func (s *pointsBalanceCacheStub) InvalidateUserBalance(context.Context, int64) error {
	s.calls++
	return s.err
}

func (r *pointsBridgeRepositoryStub) ApplyPointsBalanceCredit(_ context.Context, input PointsBalanceCreditInput) (*PointsBalanceCreditResult, error) {
	r.input = input
	return &PointsBalanceCreditResult{
		TransactionID: input.TransactionID,
		BalanceAfter:  decimal.RequireFromString("12.34"),
	}, nil
}

func testPointsBridgeConfig() *config.Config {
	launchKey := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("l", 32)))
	creditKey := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("c", 32)))
	return &config.Config{PointsSystem: config.PointsSystemConfig{
		Enabled:          true,
		PublicURL:        "https://example.test/points",
		MenuLabel:        "Points",
		LaunchKeyID:      "launch-v1",
		LaunchSecret:     launchKey,
		CreditKeyID:      "credit-v1",
		CreditSecret:     creditKey,
		LaunchTTLSeconds: 60,
		ClockSkewSeconds: 60,
	}}
}

func TestPointsBridgeCreateLaunchURLUsesVersionedTicket(t *testing.T) {
	cfg := testPointsBridgeConfig()
	svc := NewPointsBridgeService(&pointsBridgeRepositoryStub{}, nil, cfg)
	fixedNow := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	launchURL, err := svc.CreateLaunchURL(42, "admin", "dark", "zh-CN")
	if err != nil {
		t.Fatalf("create launch URL: %v", err)
	}
	parsed, err := url.Parse(launchURL)
	if err != nil {
		t.Fatalf("parse launch URL: %v", err)
	}
	if parsed.Path != "/points/launch" {
		t.Fatalf("launch path = %q", parsed.Path)
	}
	parts := strings.Split(parsed.Query().Get("ticket"), ".")
	if len(parts) != 3 || parts[0] != "launch-v1" {
		t.Fatalf("ticket does not contain its key id")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims PointsLaunchClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims.Audience != "points-system" || claims.Issuer != "sub2api" || claims.Subject != 42 || claims.Role != "admin" {
		t.Fatalf("unexpected launch claims: %+v", claims)
	}
	key, _ := cfg.PointsSystem.LaunchKey()
	wantSignature := signPointsPayload(key, parts[0]+"."+parts[1])
	gotSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(wantSignature, gotSignature) {
		t.Fatal("launch signature mismatch")
	}
}

func TestPointsBridgeAllowsAdminSetupWhileUserEntryIsDisabled(t *testing.T) {
	cfg := testPointsBridgeConfig()
	cfg.PointsSystem.Enabled = false
	svc := NewPointsBridgeService(&pointsBridgeRepositoryStub{}, nil, cfg)

	status := svc.Status()
	if !status.Configured || status.Active || status.Enabled {
		t.Fatalf("unexpected disabled bridge status: %+v", status)
	}
	if !status.LaunchSecretConfigured || !status.CreditSecretConfigured {
		t.Fatalf("configured secrets were not reflected in status: %+v", status)
	}
	if _, err := svc.CreateLaunchURL(42, "admin", "light", "zh-CN"); err != nil {
		t.Fatalf("admin setup launch should remain available: %v", err)
	}
	if _, err := svc.CreateLaunchURL(42, "user", "light", "zh-CN"); err != ErrPointsSystemUnavailable {
		t.Fatalf("disabled user launch error = %v", err)
	}

	cfg.PointsSystem.PublicURL = "https://user:password@example.test/points?token=secret"
	status = svc.Status()
	if status.Configured || status.PublicURL != "" {
		t.Fatalf("unsafe public URL must not be exposed in status: %+v", status)
	}
}

func TestPointsBridgeVerifyAndApplyCreditBindsKeyAndBody(t *testing.T) {
	cfg := testPointsBridgeConfig()
	repo := &pointsBridgeRepositoryStub{}
	svc := NewPointsBridgeService(repo, nil, cfg)
	fixedNow := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }
	transactionID := uuid.MustParse("b754d48e-2bb3-4d61-9428-e8de88c18670")
	body := []byte(`{"transaction_id":"b754d48e-2bb3-4d61-9428-e8de88c18670","user_id":42,"amount":"0.05","kind":"checkin","source_reference":"checkin:42:2026-07-29:1","reason":"daily check-in"}`)
	timestamp := "1785326400"
	keyID := "credit-v1"
	nonce := transactionID.String()
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{"v1", keyID, http.MethodPost, pointsCreditPath, timestamp, nonce, hex.EncodeToString(bodyHash[:])}, "\n")
	key, _ := cfg.PointsSystem.CreditKey()
	signature := hex.EncodeToString(signPointsPayload(key, canonical))

	result, err := svc.VerifyAndApplyCredit(context.Background(), http.MethodPost, pointsCreditPath,
		timestamp, keyID, nonce, signature, body, "request-1")
	if err != nil {
		t.Fatalf("verify and apply credit: %v", err)
	}
	if result.TransactionID != transactionID || !result.BalanceAfter.Equal(decimal.RequireFromString("12.34")) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repo.input.UserID != 42 || !repo.input.Amount.Equal(decimal.RequireFromString("0.05")) || repo.input.Kind != "checkin" {
		t.Fatalf("unexpected repository input: %+v", repo.input)
	}

	_, err = svc.VerifyAndApplyCredit(context.Background(), http.MethodPost, pointsCreditPath,
		timestamp, "credit-v2", nonce, signature, body, "request-2")
	if err != ErrPointsSignatureInvalid {
		t.Fatalf("wrong key id error = %v", err)
	}

	cache := &pointsBalanceCacheStub{err: errors.New("redis unavailable")}
	svc = NewPointsBridgeService(repo, cache, cfg)
	svc.now = func() time.Time { return fixedNow }
	_, err = svc.VerifyAndApplyCredit(context.Background(), http.MethodPost, pointsCreditPath,
		timestamp, keyID, nonce, signature, body, "request-3")
	if err != ErrPointsCacheSyncPending || cache.calls != 1 {
		t.Fatalf("cache synchronization error = %v, calls = %d", err, cache.calls)
	}
}

func TestNormalizePointsLanguageDefaultsUnsafeValues(t *testing.T) {
	if got := normalizePointsLanguage("zh-CN"); got != "zh-CN" {
		t.Fatalf("valid language changed to %q", got)
	}
	for _, value := range []string{"", "zh_CN", "language-value-that-is-too-long", "中文"} {
		if got := normalizePointsLanguage(value); got != "en" {
			t.Fatalf("normalizePointsLanguage(%q) = %q", value, got)
		}
	}
}
