package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestPointsSystemConfigActiveRequiresVersionedDecodedKeys(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	cfg := PointsSystemConfig{
		Enabled:          true,
		PublicURL:        "https://example.test/points",
		LaunchKeyID:      "launch-v1",
		LaunchSecret:     secret,
		CreditKeyID:      "credit-v1",
		CreditSecret:     secret,
		LaunchTTLSeconds: 60,
		ClockSkewSeconds: 60,
	}
	if !cfg.Active() {
		t.Fatal("complete points system configuration should be active")
	}

	cfg.CreditSecret = strings.Repeat("x", 32)
	if cfg.Active() {
		t.Fatal("raw, non-base64 credit secret must not be accepted")
	}

	cfg.CreditSecret = secret
	cfg.LaunchKeyID = "invalid key id"
	if cfg.Active() {
		t.Fatal("invalid key id must not be accepted")
	}

	cfg.LaunchKeyID = "launch.v1"
	if cfg.Active() {
		t.Fatal("ticket delimiter must not be accepted in a key id")
	}
}

func TestPointsSystemConfigConfiguredDoesNotEnableUserAccess(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	cfg := PointsSystemConfig{
		Enabled:          false,
		PublicURL:        "https://example.test/points",
		LaunchKeyID:      "launch-v1",
		LaunchSecret:     secret,
		CreditKeyID:      "credit-v1",
		CreditSecret:     secret,
		LaunchTTLSeconds: 60,
		ClockSkewSeconds: 60,
	}
	if !cfg.Configured() {
		t.Fatal("complete disabled integration should remain available for admin setup")
	}
	if cfg.Active() {
		t.Fatal("disabled integration must not expose the user entry")
	}

	cfg.PublicURL = "https://user@example.test/points"
	if cfg.Configured() {
		t.Fatal("URL credentials must not be accepted")
	}
}

func TestPointsSystemSecretAcceptsPaddedAndRawBase64(t *testing.T) {
	raw := []byte(strings.Repeat("s", 32))
	for _, encoded := range []string{
		base64.RawURLEncoding.EncodeToString(raw),
		base64.StdEncoding.EncodeToString(raw),
	} {
		decoded, err := decodePointsSystemSecret(encoded)
		if err != nil {
			t.Fatalf("decode secret: %v", err)
		}
		if string(decoded) != string(raw) {
			t.Fatalf("decoded secret mismatch")
		}
	}
}
