package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestPointsSystemConfigUserAccessAllowsOnlyConfiguredPreviewUsers(t *testing.T) {
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	cfg := PointsSystemConfig{
		Enabled:          false,
		PublicURL:        "https://example.test/points",
		PreviewUserIDs:   []int64{1, 42},
		LaunchKeyID:      "launch-v1",
		LaunchSecret:     secret,
		CreditKeyID:      "credit-v1",
		CreditSecret:     secret,
		LaunchTTLSeconds: 60,
		ClockSkewSeconds: 60,
	}
	if !cfg.UserAccessAllowed(1) || !cfg.UserAccessAllowed(42) {
		t.Fatal("configured preview users should be allowed while the global switch is disabled")
	}
	if cfg.UserAccessAllowed(2) || cfg.UserAccessAllowed(0) {
		t.Fatal("users outside the preview list must remain blocked")
	}

	cfg.Enabled = true
	if !cfg.UserAccessAllowed(2) {
		t.Fatal("enabling the global switch should allow all positive users")
	}

	cfg.LaunchSecret = "invalid"
	if cfg.UserAccessAllowed(1) {
		t.Fatal("preview access must still require a configured bridge")
	}
}

func TestValidatePointsPreviewUserIDsRejectsInvalidEntries(t *testing.T) {
	for _, userIDs := range [][]int64{{0}, {-1}, {1, -2}} {
		if err := validatePointsPreviewUserIDs(userIDs); err == nil {
			t.Fatalf("expected invalid preview IDs %v to be rejected", userIDs)
		}
	}
	tooMany := make([]int64, maxPointsPreviewUserIDs+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	if err := validatePointsPreviewUserIDs(tooMany); err == nil {
		t.Fatal("expected oversized preview user list to be rejected")
	}
}

func TestLoadPointsPreviewUserIDsFromEnvironment(t *testing.T) {
	resetViperWithJWTSecret(t)
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	t.Setenv("POINTS_SYSTEM_ENABLED", "false")
	t.Setenv("POINTS_SYSTEM_PUBLIC_URL", "https://points.example.test")
	t.Setenv("POINTS_SYSTEM_PREVIEW_USER_IDS", "1,42")
	t.Setenv("POINTS_SYSTEM_LAUNCH_KEY_ID", "launch-v1")
	t.Setenv("POINTS_SYSTEM_LAUNCH_SECRET", secret)
	t.Setenv("POINTS_SYSTEM_CREDIT_KEY_ID", "credit-v1")
	t.Setenv("POINTS_SYSTEM_CREDIT_SECRET", secret)
	t.Setenv("POINTS_SYSTEM_LAUNCH_TTL_SECONDS", "60")
	t.Setenv("POINTS_SYSTEM_CLOCK_SKEW_SECONDS", "60")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, []int64{1, 42}, cfg.PointsSystem.PreviewUserIDs)
	require.True(t, cfg.PointsSystem.UserAccessAllowed(1))
	require.False(t, cfg.PointsSystem.UserAccessAllowed(2))
}

func TestLoadPointsPreviewUsersRequiresCompleteBridgeConfiguration(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("POINTS_SYSTEM_ENABLED", "false")
	t.Setenv("POINTS_SYSTEM_PREVIEW_USER_IDS", "1")

	_, err := Load()
	require.ErrorContains(t, err, "points_system.public_url invalid")
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
