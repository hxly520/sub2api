package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type HMACKey struct {
	ID     string
	Secret []byte
}

type Config struct {
	ListenAddr         string
	DatabaseURL        string
	UsageDatabaseURL   string
	UsageReconcileDays int
	PublicOrigin       string
	TrustedProxyCIDR   string
	Timezone           *time.Location
	SessionTTL         time.Duration
	CookieSecure       bool
	LaunchIssuer       string
	LaunchAudience     string
	LaunchKeys         map[string][]byte
	SessionSecret      []byte
	Sub2URL            string
	Sub2Key            HMACKey
	HTTPTimeout        time.Duration
	WorkerInterval     time.Duration
}

func Load() (Config, error) {
	tzName := env("POINTS_TIMEZONE", "Asia/Shanghai")
	location, err := time.LoadLocation(tzName)
	if err != nil {
		return Config{}, fmt.Errorf("load POINTS_TIMEZONE: %w", err)
	}
	launchKeys, err := parseKeyring(os.Getenv("POINTS_LAUNCH_HMAC_KEYS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse POINTS_LAUNCH_HMAC_KEYS: %w", err)
	}
	sessionSecret, err := decodeSecret(os.Getenv("POINTS_SESSION_SECRET"))
	if err != nil {
		return Config{}, fmt.Errorf("parse POINTS_SESSION_SECRET: %w", err)
	}
	sub2Secret, err := decodeSecret(os.Getenv("POINTS_SUB2_CREDIT_SECRET"))
	if err != nil {
		return Config{}, fmt.Errorf("parse POINTS_SUB2_CREDIT_SECRET: %w", err)
	}
	cfg := Config{
		ListenAddr:         env("POINTS_LISTEN_ADDR", ":8090"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("POINTS_DATABASE_URL")),
		UsageDatabaseURL:   strings.TrimSpace(os.Getenv("POINTS_USAGE_DATABASE_URL")),
		UsageReconcileDays: intEnv("POINTS_USAGE_RECONCILE_DAYS", 7),
		PublicOrigin:       strings.TrimRight(strings.TrimSpace(os.Getenv("POINTS_PUBLIC_ORIGIN")), "/"),
		TrustedProxyCIDR:   strings.TrimSpace(os.Getenv("POINTS_TRUSTED_PROXY_CIDR")),
		Timezone:           location,
		SessionTTL:         durationEnv("POINTS_SESSION_TTL", 8*time.Hour),
		CookieSecure:       boolEnv("POINTS_COOKIE_SECURE", true),
		LaunchIssuer:       env("POINTS_LAUNCH_ISSUER", "sub2api"),
		LaunchAudience:     env("POINTS_LAUNCH_AUDIENCE", "points-system"),
		LaunchKeys:         launchKeys,
		SessionSecret:      sessionSecret,
		Sub2URL:            strings.TrimRight(strings.TrimSpace(os.Getenv("POINTS_SUB2_BALANCE_URL")), "/"),
		Sub2Key: HMACKey{
			ID:     env("POINTS_SUB2_CREDIT_KEY_ID", "v1"),
			Secret: sub2Secret,
		},
		HTTPTimeout:    durationEnv("POINTS_HTTP_TIMEOUT", 10*time.Second),
		WorkerInterval: durationEnv("POINTS_WORKER_INTERVAL", 5*time.Second),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("POINTS_DATABASE_URL is required")
	}
	if c.UsageDatabaseURL == "" {
		return errors.New("POINTS_USAGE_DATABASE_URL is required")
	}
	if c.UsageReconcileDays < 1 || c.UsageReconcileDays > 31 {
		return errors.New("POINTS_USAGE_RECONCILE_DAYS must be between 1 and 31")
	}
	if len(c.SessionSecret) < 32 {
		return errors.New("POINTS_SESSION_SECRET must decode to at least 32 bytes")
	}
	if c.PublicOrigin == "" {
		return errors.New("POINTS_PUBLIC_ORIGIN is required")
	}
	u, err := url.Parse(c.PublicOrigin)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" {
		return errors.New("POINTS_PUBLIC_ORIGIN must be an HTTP(S) origin without a path")
	}
	if c.CookieSecure && u.Scheme != "https" {
		return errors.New("secure cookies require an HTTPS POINTS_PUBLIC_ORIGIN")
	}
	if c.SessionTTL < 5*time.Minute || c.SessionTTL > 24*time.Hour {
		return errors.New("POINTS_SESSION_TTL must be between 5m and 24h")
	}
	if c.HTTPTimeout <= 0 || c.HTTPTimeout > time.Minute {
		return errors.New("POINTS_HTTP_TIMEOUT must be between 1ns and 1m")
	}
	if len(c.LaunchKeys) == 0 {
		return errors.New("POINTS_LAUNCH_HMAC_KEYS requires at least one 32-byte secret")
	}
	if !c.Sub2Configured() {
		return errors.New("POINTS_SUB2_BALANCE_URL and POINTS_SUB2_CREDIT_SECRET are required")
	}
	if c.WorkerInterval <= 0 || c.WorkerInterval > time.Minute {
		return errors.New("POINTS_WORKER_INTERVAL must be between 1ns and 1m")
	}
	return nil
}

func (c Config) Sub2Configured() bool {
	return c.Sub2URL != "" && validKeyID(c.Sub2Key.ID) && len(c.Sub2Key.Secret) >= 32
}

func parseKeyring(raw string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 || !validKeyID(parts[0]) {
			return nil, errors.New("keyring entries must use key_id:base64_secret")
		}
		secret, err := decodeSecret(parts[1])
		if err != nil {
			return nil, err
		}
		if len(secret) < 32 {
			return nil, fmt.Errorf("key %q is shorter than 32 bytes", parts[0])
		}
		result[strings.TrimSpace(parts[0])] = secret
	}
	return result, nil
}

func validKeyID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func decodeSecret(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	secret, err := base64.RawURLEncoding.DecodeString(raw)
	if err == nil {
		return secret, nil
	}
	secret, err = base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("secret must be base64 or base64url")
	}
	return secret, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
