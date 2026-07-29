package config

import (
	"encoding/base64"
	"strings"
	"testing"
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
