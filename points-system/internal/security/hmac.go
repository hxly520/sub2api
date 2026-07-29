package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderKeyID     = "X-Points-Key-Id"
	HeaderTimestamp = "X-Points-Timestamp"
	HeaderNonce     = "X-Points-Nonce"
	HeaderSignature = "X-Points-Signature"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrExpiredSignature = errors.New("signature timestamp outside tolerance")
	ErrInvalidTicket    = errors.New("invalid launch ticket")
)

type LaunchClaims struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	Subject   int64  `json:"sub"`
	Role      string `json:"role"`
	Theme     string `json:"theme,omitempty"`
	Language  string `json:"lang,omitempty"`
	Nonce     string `json:"nonce"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func ValidateLaunchTicket(raw, issuer, audience string, keys map[string][]byte, now time.Time) (LaunchClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || len(keys) == 0 || parts[0] == "" {
		return LaunchClaims{}, ErrInvalidTicket
	}
	secret := keys[parts[0]]
	if len(secret) < 32 {
		return LaunchClaims{}, ErrInvalidTicket
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return LaunchClaims{}, ErrInvalidTicket
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(mac.Sum(nil), provided) {
		return LaunchClaims{}, ErrInvalidTicket
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return LaunchClaims{}, ErrInvalidTicket
	}
	var claims LaunchClaims
	if json.Unmarshal(payload, &claims) != nil {
		return LaunchClaims{}, ErrInvalidTicket
	}
	nowUnix := now.Unix()
	if claims.Issuer != issuer || claims.Audience != audience || claims.Subject <= 0 || claims.Nonce == "" ||
		(claims.Role != "user" && claims.Role != "admin") || claims.IssuedAt > nowUnix+30 ||
		(claims.Theme != "light" && claims.Theme != "dark") || !validLanguage(claims.Language) ||
		claims.ExpiresAt <= nowUnix || claims.ExpiresAt <= claims.IssuedAt || claims.ExpiresAt-claims.IssuedAt > 300 {
		return LaunchClaims{}, ErrInvalidTicket
	}
	return claims, nil
}

func validLanguage(value string) bool {
	if len(value) == 0 || len(value) > 16 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return true
}

func SignLaunchTicket(claims LaunchClaims, keyID string, secret []byte) (string, error) {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" || strings.Contains(keyID, ".") || len(secret) < 32 {
		return "", ErrInvalidTicket
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(keyID + "." + payload))
	return keyID + "." + payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func BodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func CanonicalRequest(keyID, method, path, timestamp, nonce, bodyHash string) string {
	return strings.Join([]string{
		"v1", keyID, strings.ToUpper(strings.TrimSpace(method)), path, timestamp, nonce, bodyHash,
	}, "\n")
}

func SignRequest(req *http.Request, body []byte, keyID string, secret []byte, now time.Time, nonce string) error {
	if req == nil || keyID == "" || len(secret) < 32 || nonce == "" {
		return errors.New("incomplete request signing configuration")
	}
	timestamp := strconv.FormatInt(now.Unix(), 10)
	canonical := CanonicalRequest(keyID, req.Method, req.URL.EscapedPath(), timestamp, nonce, BodyHash(body))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	req.Header.Set(HeaderKeyID, keyID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	return nil
}

type VerifiedRequest struct {
	KeyID     string
	Nonce     string
	Timestamp time.Time
}

func VerifyRequest(req *http.Request, body []byte, keys map[string][]byte, now time.Time, tolerance time.Duration) (VerifiedRequest, error) {
	if req == nil {
		return VerifiedRequest{}, ErrInvalidSignature
	}
	keyID := strings.TrimSpace(req.Header.Get(HeaderKeyID))
	timestampRaw := strings.TrimSpace(req.Header.Get(HeaderTimestamp))
	nonce := strings.TrimSpace(req.Header.Get(HeaderNonce))
	signatureRaw := strings.TrimSpace(req.Header.Get(HeaderSignature))
	secret := keys[keyID]
	if len(secret) < 32 || timestampRaw == "" || len(nonce) < 16 || len(nonce) > 128 || signatureRaw == "" {
		return VerifiedRequest{}, ErrInvalidSignature
	}
	seconds, err := strconv.ParseInt(timestampRaw, 10, 64)
	if err != nil {
		return VerifiedRequest{}, ErrInvalidSignature
	}
	timestamp := time.Unix(seconds, 0)
	if now.Sub(timestamp) > tolerance || timestamp.Sub(now) > tolerance {
		return VerifiedRequest{}, ErrExpiredSignature
	}
	provided, err := base64.RawURLEncoding.DecodeString(signatureRaw)
	if err != nil {
		return VerifiedRequest{}, ErrInvalidSignature
	}
	canonical := CanonicalRequest(keyID, req.Method, req.URL.EscapedPath(), timestampRaw, nonce, BodyHash(body))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	if !hmac.Equal(mac.Sum(nil), provided) {
		return VerifiedRequest{}, ErrInvalidSignature
	}
	return VerifiedRequest{KeyID: keyID, Nonce: nonce, Timestamp: timestamp}, nil
}

func Fingerprint(parts ...any) string {
	hash := sha256.New()
	for i, part := range parts {
		if i > 0 {
			_, _ = hash.Write([]byte{0})
		}
		_, _ = fmt.Fprint(hash, part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func CSRFToken(sessionToken string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("csrf\x00" + sessionToken))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func VerifyCSRF(sessionToken, provided string, secret []byte) bool {
	expected := CSRFToken(sessionToken, secret)
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(provided)))
}
