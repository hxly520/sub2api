package sub2client

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const balanceCreditPath = "/api/internal/points/credits"

var (
	ErrInvalidAmount = errors.New("balance adjustment must use whole cents")
	ErrRejected      = errors.New("Sub2API rejected balance adjustment")
)

type Client struct {
	baseURL string
	keyID   string
	secret  []byte
	http    *http.Client
	now     func() time.Time
}

type Adjustment struct {
	TransactionID   string
	UserID          int64
	AmountMicroUSD  int64
	Kind            string
	SourceReference string
	Reason          string
}

type Result struct {
	TransactionID string `json:"transaction_id"`
	BalanceAfter  string `json:"balance_after"`
	Idempotent    bool   `json:"idempotent"`
}

type HTTPError struct {
	StatusCode int
	Code       int
	Message    string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ErrRejected.Error()
	}
	return fmt.Sprintf("%s: http=%d code=%d message=%s", ErrRejected, e.StatusCode, e.Code, e.Message)
}

func New(baseURL, keyID string, secret []byte, httpClient *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	keyID = strings.TrimSpace(keyID)
	if baseURL == "" || keyID == "" || strings.ContainsAny(keyID, "\r\n") || len(secret) < 32 {
		return nil, errors.New("incomplete Sub2API client configuration")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{baseURL: baseURL, keyID: keyID, secret: append([]byte(nil), secret...), http: httpClient, now: time.Now}, nil
}

func (c *Client) AdjustBalance(ctx context.Context, adjustment Adjustment) (Result, error) {
	amount, err := formatMicroUSD(adjustment.AmountMicroUSD)
	if err != nil {
		return Result{}, err
	}
	if adjustment.TransactionID == "" || adjustment.UserID <= 0 || adjustment.SourceReference == "" {
		return Result{}, errors.New("invalid balance adjustment")
	}
	if adjustment.Kind != "checkin" && adjustment.Kind != "manual_grant" && adjustment.Kind != "reversal" {
		return Result{}, errors.New("invalid balance adjustment kind")
	}
	payload := struct {
		TransactionID   string `json:"transaction_id"`
		UserID          int64  `json:"user_id"`
		Amount          string `json:"amount"`
		Kind            string `json:"kind"`
		SourceReference string `json:"source_reference"`
		Reason          string `json:"reason"`
	}{
		TransactionID: adjustment.TransactionID, UserID: adjustment.UserID, Amount: amount,
		Kind: adjustment.Kind, SourceReference: adjustment.SourceReference, Reason: adjustment.Reason,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+balanceCreditPath, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	timestamp := fmt.Sprint(c.now().UTC().Unix())
	nonce := adjustment.TransactionID
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Points-Key-ID", c.keyID)
	req.Header.Set("X-Points-Timestamp", timestamp)
	req.Header.Set("X-Points-Nonce", nonce)
	req.Header.Set("X-Points-Signature", sign(c.secret, canonical(c.keyID, timestamp, nonce, body)))

	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, err
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return Result{}, fmt.Errorf("decode Sub2API response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || envelope.Code != 0 {
		return Result{}, &HTTPError{StatusCode: resp.StatusCode, Code: envelope.Code, Message: envelope.Message}
	}
	var result Result
	if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return Result{}, fmt.Errorf("decode Sub2API result: %w", err)
	}
	if result.TransactionID != adjustment.TransactionID {
		return Result{}, errors.New("Sub2API returned a mismatched transaction id")
	}
	return result, nil
}

func canonical(keyID, timestamp, nonce string, body []byte) string {
	hash := sha256.Sum256(body)
	return strings.Join([]string{
		"v1", keyID, http.MethodPost, balanceCreditPath, timestamp, nonce, hex.EncodeToString(hash[:]),
	}, "\n")
}

func sign(secret []byte, canonical string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func formatMicroUSD(amount int64) (string, error) {
	if amount == 0 || amount%10_000 != 0 {
		return "", ErrInvalidAmount
	}
	negative := amount < 0
	if negative {
		amount = -amount
	}
	cents := amount / 10_000
	value := fmt.Sprintf("%d.%02d", cents/100, cents%100)
	if negative {
		value = "-" + value
	}
	return value, nil
}
