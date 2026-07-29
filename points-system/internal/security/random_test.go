package security

import "testing"

func TestRandomSteppedInt64AlwaysUsesConfiguredStep(t *testing.T) {
	for i := 0; i < 200; i++ {
		value, err := RandomSteppedInt64(10_000, 50_000, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		if value < 10_000 || value > 50_000 || value%10_000 != 0 {
			t.Fatalf("invalid stepped sample: %d", value)
		}
	}
}
