package httpapi

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hxly520/sub2api/points-system/internal/config"
)

func TestPageCursorRoundTripsAndBindsUserAndScope(t *testing.T) {
	server := &Server{Config: config.Config{
		SessionSecret: []byte("01234567890123456789012345678901"),
	}}

	ledgerCursor, err := server.encodeLedgerPageCursor(7, 42)
	if err != nil {
		t.Fatal(err)
	}
	ledgerID, err := server.decodeLedgerPageCursor(ledgerCursor, 7)
	if err != nil || ledgerID == nil || *ledgerID != 42 {
		t.Fatalf("decoded ledger cursor = %v, %v", ledgerID, err)
	}
	if _, err := server.decodeLedgerPageCursor(ledgerCursor, 8); err == nil {
		t.Fatal("ledger cursor was reusable by another user")
	}
	if _, err := server.decodeGrantPageCursor(ledgerCursor, 7); err == nil {
		t.Fatal("ledger cursor was reusable for the grants feed")
	}

	createdAt := time.Date(2026, 7, 31, 9, 18, 0, 123_000, time.FixedZone("CST", 8*60*60))
	grantID := uuid.New().String()
	grantCursor, err := server.encodeGrantPageCursor(7, createdAt, grantID)
	if err != nil {
		t.Fatal(err)
	}
	decodedGrant, err := server.decodeGrantPageCursor(grantCursor, 7)
	if err != nil || decodedGrant == nil || decodedGrant.ID != grantID ||
		!decodedGrant.CreatedAt.Equal(createdAt) {
		t.Fatalf("decoded grant cursor = %#v, %v", decodedGrant, err)
	}
	if _, err := server.decodeAdminGrantPageCursor(grantCursor, 7); err == nil {
		t.Fatal("user grant cursor was reusable for the administrator feed")
	}
	adminCursor, err := server.encodeAdminGrantPageCursor(7, createdAt, grantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.decodeAdminGrantPageCursor(adminCursor, 7); err != nil {
		t.Fatalf("decode administrator grant cursor: %v", err)
	}
	if _, err := server.decodeGrantPageCursor(adminCursor, 7); err == nil {
		t.Fatal("administrator grant cursor was reusable for the user feed")
	}
}

func TestPageCursorRejectsTamperingAndMalformedValues(t *testing.T) {
	server := &Server{Config: config.Config{
		SessionSecret: []byte("01234567890123456789012345678901"),
	}}
	cursor, err := server.encodeLedgerPageCursor(7, 42)
	if err != nil {
		t.Fatal(err)
	}

	last := cursor[len(cursor)-1]
	replacement := byte('A')
	if last == replacement {
		replacement = 'B'
	}
	tampered := cursor[:len(cursor)-1] + string(replacement)
	for name, value := range map[string]string{
		"tampered":   tampered,
		"whitespace": " " + cursor,
		"oversized":  strings.Repeat("a", pageCursorMaxLength+1),
		"malformed":  "pc1.not-base64.not-base64",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := server.decodeLedgerPageCursor(value, 7); err == nil {
				t.Fatalf("accepted invalid cursor %q", value)
			}
		})
	}

	if decoded, err := server.decodeLedgerPageCursor("", 7); err != nil || decoded != nil {
		t.Fatalf("empty cursor = %v, %v", decoded, err)
	}
}
