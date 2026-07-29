package sub2client

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdjustBalanceMatchesSub2APIContract(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		keyID := r.Header.Get("X-Points-Key-ID")
		timestamp := r.Header.Get("X-Points-Timestamp")
		nonce := r.Header.Get("X-Points-Nonce")
		hash := sha256.Sum256(body)
		canonical := "v1\n" + keyID + "\nPOST\n/api/internal/points/credits\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(hash[:])
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write([]byte(canonical))
		if got := r.Header.Get("X-Points-Signature"); got != hex.EncodeToString(mac.Sum(nil)) {
			t.Fatalf("signature mismatch: %s", got)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["amount"] != "12.34" || payload["transaction_id"] != nonce {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"transaction_id":"00000000-0000-4000-8000-000000000001","balance_after":"18.50","idempotent":false}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "credit-v1", secret, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	client.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	result, err := client.AdjustBalance(context.Background(), Adjustment{
		TransactionID: "00000000-0000-4000-8000-000000000001", UserID: 7,
		AmountMicroUSD: 12_340_000, Kind: "manual_grant", SourceReference: "balance-grant:1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BalanceAfter != "18.50" {
		t.Fatalf("unexpected balance: %s", result.BalanceAfter)
	}
}

func TestFormatMicroUSD(t *testing.T) {
	tests := map[int64]string{10_000: "0.01", 1_000_000: "1.00", -12_340_000: "-12.34"}
	for input, expected := range tests {
		actual, err := formatMicroUSD(input)
		if err != nil || actual != expected {
			t.Fatalf("formatMicroUSD(%d) = %q, %v", input, actual, err)
		}
	}
	if _, err := formatMicroUSD(1); err == nil {
		t.Fatal("expected sub-cent amount to fail")
	}
}
