package config

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseKeyringUsesBase64AndVersionedKeyID(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	keys, err := parseKeyring("launch-v1:" + encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(keys["launch-v1"]) != string(secret) {
		t.Fatal("decoded keyring secret mismatch")
	}
	for _, raw := range []string{
		"launch.v1:" + encoded,
		"launch v1:" + encoded,
		"launch-v1:not-base64",
	} {
		if _, err := parseKeyring(raw); err == nil {
			t.Fatalf("invalid keyring entry %q was accepted", raw)
		}
	}
}

func TestDatabaseSchemaRejectsPublicAndUnsafeNames(t *testing.T) {
	for _, value := range []string{"points", "points_v2", "tenant42"} {
		if !validDatabaseSchema(value) {
			t.Fatalf("valid schema %q was rejected", value)
		}
	}
	for _, value := range []string{"", "public", "pg_catalog", "pg_temp_1", "Points", "points-data", "42points"} {
		if validDatabaseSchema(value) {
			t.Fatalf("unsafe schema %q was accepted", value)
		}
	}
}

func TestValidHTTPOriginRequiresOneExactOrigin(t *testing.T) {
	for _, value := range []string{
		"https://sub2api.example.test",
		"https://sub2api.example.test:8443",
		"http://localhost:5173",
	} {
		parsed, err := url.Parse(value)
		if err != nil || !validHTTPOrigin(value, parsed) {
			t.Fatalf("valid origin %q was rejected", value)
		}
	}
	for _, value := range []string{
		"", "sub2api.example.test", "https://*.example.test", "https://sub2api.example.test/",
		"https://sub2api.example.test/path", "https://sub2api.example.test?mode=embedded",
		"https://sub2api.example.test#fragment", "https://user@sub2api.example.test",
		"https://sub2api.example.test:", "https://sub2api.example.test:0",
		"https://sub2api.example.test:65536", "javascript:alert(1)",
	} {
		parsed, _ := url.Parse(value)
		if validHTTPOrigin(value, parsed) {
			t.Fatalf("unsafe origin %q was accepted", value)
		}
	}
}

func TestConfigRequiresExactEmbedParentOrigin(t *testing.T) {
	base := Config{
		DatabaseURL: "postgres://points", DatabaseSchema: "points", DatabaseMaxConns: 1,
		UsageDatabaseURL: "postgres://usage", UsageReconcileDays: 1,
		PublicOrigin: "https://points.example.test", EmbedParentOrigin: "https://sub2api.example.test",
		UserAccessMode: "all",
		Timezone:       time.UTC, SessionTTL: time.Hour, CookieSecure: true,
		LaunchKeys:    map[string][]byte{"v1": []byte(strings.Repeat("l", 32))},
		SessionSecret: []byte(strings.Repeat("s", 32)), Sub2URL: "http://sub2api:8080",
		Sub2Key:     HMACKey{ID: "v1", Secret: []byte(strings.Repeat("c", 32))},
		HTTPTimeout: time.Second, WorkerInterval: time.Second,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config failed validation: %v", err)
	}
	for _, value := range []string{"", "https://*.example.test", "https://sub2api.example.test/path"} {
		candidate := base
		candidate.EmbedParentOrigin = value
		if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), "POINTS_EMBED_PARENT_ORIGIN") {
			t.Fatalf("embed parent %q validation error = %v", value, err)
		}
	}
}

func TestUserAccessRolloutGate(t *testing.T) {
	preview, err := parsePositiveIDSet("1, 42,1")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{UserAccessMode: "preview", UserPreviewIDs: preview}
	if !cfg.UserAccessAllowed(1) || !cfg.UserAccessAllowed(42) {
		t.Fatal("preview users were rejected")
	}
	if cfg.UserAccessAllowed(2) || cfg.UserAccessAllowed(0) {
		t.Fatal("non-preview user was allowed")
	}
	cfg.UserAccessMode = "all"
	cfg.UserPreviewIDs = nil
	if !cfg.UserAccessAllowed(2) {
		t.Fatal("all-users mode rejected a positive user")
	}
	for _, raw := range []string{"-1", "0", "abc", "1,2.5"} {
		if _, err := parsePositiveIDSet(raw); err == nil {
			t.Fatalf("invalid preview list %q was accepted", raw)
		}
	}
}

func TestConfigRequiresExplicitPreviewUsers(t *testing.T) {
	base := Config{
		DatabaseURL: "postgres://points", DatabaseSchema: "points", DatabaseMaxConns: 1,
		UsageDatabaseURL: "postgres://usage", UsageReconcileDays: 1,
		PublicOrigin: "https://points.example.test", EmbedParentOrigin: "https://sub2api.example.test",
		UserAccessMode: "preview", UserPreviewIDs: map[int64]struct{}{1: {}},
		Timezone: time.UTC, SessionTTL: time.Hour, CookieSecure: true,
		LaunchKeys:    map[string][]byte{"v1": []byte(strings.Repeat("l", 32))},
		SessionSecret: []byte(strings.Repeat("s", 32)), Sub2URL: "http://sub2api:8080",
		Sub2Key:     HMACKey{ID: "v1", Secret: []byte(strings.Repeat("c", 32))},
		HTTPTimeout: time.Second, WorkerInterval: time.Second,
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid preview config failed validation: %v", err)
	}
	missing := base
	missing.UserPreviewIDs = nil
	if err := missing.Validate(); err == nil || !strings.Contains(err.Error(), "POINTS_USER_PREVIEW_IDS") {
		t.Fatalf("missing preview list validation error = %v", err)
	}
	ambiguous := base
	ambiguous.UserAccessMode = "all"
	if err := ambiguous.Validate(); err == nil || !strings.Contains(err.Error(), "must be empty") {
		t.Fatalf("ambiguous all-users config validation error = %v", err)
	}
}
