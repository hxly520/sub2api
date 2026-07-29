package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLaunchTicketMatchesSub2APIThreePartFormat(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	secret := []byte("01234567890123456789012345678901")
	claims := LaunchClaims{
		Issuer: "sub2api", Audience: "points-system", Subject: 42, Role: "user",
		Theme: "dark", Language: "zh-CN", Nonce: "a-long-random-launch-nonce",
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(),
	}
	ticket, err := SignLaunchTicket(claims, "launch-v1", secret)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := ValidateLaunchTicket(ticket, "sub2api", "points-system", map[string][]byte{"launch-v1": secret}, now)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Subject != 42 || actual.Nonce != claims.Nonce ||
		actual.Theme != claims.Theme || actual.Language != claims.Language {
		t.Fatalf("unexpected claims: %#v", actual)
	}
}

func TestLaunchTicketRejectsReplayWindowAndTampering(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	secret := []byte("01234567890123456789012345678901")
	claims := LaunchClaims{Issuer: "sub2api", Audience: "points-system", Subject: 1, Role: "admin",
		Theme: "light", Language: "en", Nonce: "nonce", IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix()}
	ticket, _ := SignLaunchTicket(claims, "launch-v1", secret)
	keys := map[string][]byte{"launch-v1": secret}
	if _, err := ValidateLaunchTicket(ticket+"x", "sub2api", "points-system", keys, now); err == nil {
		t.Fatal("tampered ticket was accepted")
	}
	if _, err := ValidateLaunchTicket(ticket, "sub2api", "points-system", keys, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired ticket was accepted")
	}
	if _, err := ValidateLaunchTicket(ticket, "sub2api", "points-system", map[string][]byte{"other": secret}, now); err == nil {
		t.Fatal("ticket signed with an unknown key id was accepted")
	}
}

func TestCSRFTokenIsBoundToSession(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	token := CSRFToken("session-one", secret)
	if !VerifyCSRF("session-one", token, secret) {
		t.Fatal("valid CSRF token was rejected")
	}
	if VerifyCSRF("session-two", token, secret) || VerifyCSRF("session-one", token+"x", secret) {
		t.Fatal("CSRF token was not bound to the session")
	}
}

func TestRequestSignatureBindsKeyIDAndBody(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	secret := []byte("01234567890123456789012345678901")
	body := []byte(`{"events":[]}`)
	req := httptest.NewRequest(http.MethodPost, "https://points.example.test/api/internal/v1/usage-events", nil)
	if err := SignRequest(req, body, "internal-v1", secret, now, "0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyRequest(req, body, map[string][]byte{"internal-v1": secret}, now, time.Minute)
	if err != nil || verified.KeyID != "internal-v1" {
		t.Fatalf("verified=%+v err=%v", verified, err)
	}

	req.Header.Set(HeaderKeyID, "internal-v2")
	if _, err := VerifyRequest(req, body, map[string][]byte{
		"internal-v1": secret,
		"internal-v2": secret,
	}, now, time.Minute); err == nil {
		t.Fatal("signature remained valid after changing the key id")
	}
}
