package store

import (
	"testing"
	"time"
)

func TestLedgerAwardedAtUsesBusinessDateWhenCreatedAtMatches(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	createdAt := time.Date(2026, 7, 31, 20, 30, 0, 0, time.UTC)
	refreshMinute := 5
	firstBusinessDate := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	secondBusinessDate := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	first := ledgerAwardedAt("usage_points", &firstBusinessDate, &refreshMinute, createdAt, location)
	second := ledgerAwardedAt("usage_points", &secondBusinessDate, &refreshMinute, createdAt, location)

	firstExpected := time.Date(2026, 6, 22, 0, 5, 0, 0, location)
	secondExpected := time.Date(2026, 7, 31, 0, 5, 0, 0, location)
	if !first.Equal(firstExpected) || !second.Equal(secondExpected) {
		t.Fatalf("awarded times = (%s, %s), want (%s, %s)",
			first, second, firstExpected, secondExpected)
	}
	if first.Equal(second) {
		t.Fatalf("different business dates shared awarded_at %s", first)
	}
}

func TestLedgerAwardedAtFallsBackToCreatedAt(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	createdAt := time.Date(2026, 7, 31, 20, 30, 0, 0, time.UTC)
	businessDate := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	refreshMinute := 5

	tests := []struct {
		name          string
		kind          string
		businessDate  *time.Time
		refreshMinute *int
	}{
		{name: "non usage entry", kind: "manual_adjustment", businessDate: &businessDate, refreshMinute: &refreshMinute},
		{name: "missing business date", kind: "usage_points", refreshMinute: &refreshMinute},
		{name: "missing policy", kind: "usage_points", businessDate: &businessDate},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ledgerAwardedAt(test.kind, test.businessDate, test.refreshMinute, createdAt, location)
			if !got.Equal(createdAt) {
				t.Fatalf("awarded_at = %s, want created_at %s", got, createdAt)
			}
		})
	}
}
