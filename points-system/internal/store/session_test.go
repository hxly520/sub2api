package store

import (
	"testing"
	"time"
)

func TestAdminSessionTTLIsCapped(t *testing.T) {
	if got := sessionTTLForRole("admin", 8*time.Hour); got != 30*time.Minute {
		t.Fatalf("admin session TTL = %s", got)
	}
	if got := sessionTTLForRole("admin", 15*time.Minute); got != 15*time.Minute {
		t.Fatalf("short admin session TTL changed to %s", got)
	}
	if got := sessionTTLForRole("user", 8*time.Hour); got != 8*time.Hour {
		t.Fatalf("user session TTL changed to %s", got)
	}
}
