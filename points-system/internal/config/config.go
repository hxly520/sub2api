package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
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
	DatabaseSchema     string
	DatabaseMaxConns   int
	UsageDatabaseURL   string
	UsageReconcileDays int
	PublicOrigin       string
	EmbedParentOrigin  string
	EmbedParentOrigins []string
	UserAccessMode     string
	UserPreviewIDs     map[int64]struct{}
	CheckinAccessMode  string
	CheckinPreviewIDs  map[int64]struct{}
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
		DatabaseSchema:     env("POINTS_DATABASE_SCHEMA", "points"),
		DatabaseMaxConns:   intEnv("POINTS_DATABASE_MAX_CONNS", 8),
		UsageDatabaseURL:   strings.TrimSpace(os.Getenv("POINTS_USAGE_DATABASE_URL")),
		UsageReconcileDays: intEnv("POINTS_USAGE_RECONCILE_DAYS", 7),
		PublicOrigin:       strings.TrimRight(strings.TrimSpace(os.Getenv("POINTS_PUBLIC_ORIGIN")), "/"),
		EmbedParentOrigin:  strings.TrimSpace(os.Getenv("POINTS_EMBED_PARENT_ORIGIN")),
		EmbedParentOrigins: splitCommaList(os.Getenv("POINTS_EMBED_PARENT_ORIGINS")),
		UserAccessMode:     strings.ToLower(strings.TrimSpace(os.Getenv("POINTS_USER_ACCESS_MODE"))),
		CheckinAccessMode:  strings.ToLower(strings.TrimSpace(env("POINTS_CHECKIN_ACCESS_MODE", "all"))),
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
	cfg.UserPreviewIDs, err = parsePositiveIDSet(os.Getenv("POINTS_USER_PREVIEW_IDS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse POINTS_USER_PREVIEW_IDS: %w", err)
	}
	cfg.CheckinPreviewIDs, err = parsePositiveIDSet(os.Getenv("POINTS_CHECKIN_PREVIEW_IDS"))
	if err != nil {
		return Config{}, fmt.Errorf("parse POINTS_CHECKIN_PREVIEW_IDS: %w", err)
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
	if !validDatabaseSchema(c.DatabaseSchema) {
		return errors.New("POINTS_DATABASE_SCHEMA must be a non-system PostgreSQL identifier")
	}
	if c.DatabaseMaxConns < 1 || c.DatabaseMaxConns > 32 {
		return errors.New("POINTS_DATABASE_MAX_CONNS must be between 1 and 32")
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
	if err != nil || !validHTTPOrigin(c.PublicOrigin, u) {
		return errors.New("POINTS_PUBLIC_ORIGIN must be an HTTP(S) origin without a path")
	}
	if c.CookieSecure && u.Scheme != "https" {
		return errors.New("secure cookies require an HTTPS POINTS_PUBLIC_ORIGIN")
	}
	if _, err := c.embedParentOrigins(); err != nil {
		return err
	}
	switch c.UserAccessMode {
	case "all":
		if len(c.UserPreviewIDs) != 0 {
			return errors.New("POINTS_USER_PREVIEW_IDS must be empty when POINTS_USER_ACCESS_MODE=all")
		}
	case "preview":
		if len(c.UserPreviewIDs) == 0 {
			return errors.New("POINTS_USER_PREVIEW_IDS requires at least one user when POINTS_USER_ACCESS_MODE=preview")
		}
	default:
		return errors.New("POINTS_USER_ACCESS_MODE must be all or preview")
	}
	checkinMode := c.CheckinAccessMode
	if checkinMode == "" {
		checkinMode = "all"
	}
	switch checkinMode {
	case "all":
		if len(c.CheckinPreviewIDs) != 0 {
			return errors.New("POINTS_CHECKIN_PREVIEW_IDS must be empty when POINTS_CHECKIN_ACCESS_MODE=all")
		}
	case "preview":
		if len(c.CheckinPreviewIDs) == 0 {
			return errors.New("POINTS_CHECKIN_PREVIEW_IDS requires at least one user when POINTS_CHECKIN_ACCESS_MODE=preview")
		}
	default:
		return errors.New("POINTS_CHECKIN_ACCESS_MODE must be all or preview")
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

// AllowedEmbedParentOrigins returns the validated, de-duplicated exact
// browser origins allowed to embed the points workspace. Invalid configuration
// fails closed; callers only receive origins after Config.Validate succeeds.
func (c Config) AllowedEmbedParentOrigins() []string {
	origins, err := c.embedParentOrigins()
	if err != nil {
		return nil
	}
	return origins
}

// PrimaryEmbedParentOrigin is the exact Sub2API origin used for uploaded logo
// retrieval. The legacy singular setting remains primary when configured;
// otherwise the first list item is used.
func (c Config) PrimaryEmbedParentOrigin() string {
	origins := c.AllowedEmbedParentOrigins()
	if len(origins) == 0 {
		return ""
	}
	return origins[0]
}

func (c Config) embedParentOrigins() ([]string, error) {
	candidates := make([]string, 0, 1+len(c.EmbedParentOrigins))
	if c.EmbedParentOrigin != "" {
		candidates = append(candidates, c.EmbedParentOrigin)
	}
	candidates = append(candidates, c.EmbedParentOrigins...)
	if len(candidates) == 0 {
		return nil, errors.New("POINTS_EMBED_PARENT_ORIGIN or POINTS_EMBED_PARENT_ORIGINS is required")
	}
	seen := make(map[string]struct{}, len(candidates))
	origins := make([]string, 0, len(candidates))
	for _, origin := range candidates {
		parsed, err := url.Parse(origin)
		if err != nil || !validHTTPOrigin(origin, parsed) {
			return nil, errors.New("POINTS_EMBED_PARENT_ORIGIN and POINTS_EMBED_PARENT_ORIGINS must contain only exact HTTP(S) origins without paths or wildcards")
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
		if len(origins) > maxEmbedParentOrigins {
			return nil, fmt.Errorf("POINTS_EMBED_PARENT_ORIGINS allows at most %d origins", maxEmbedParentOrigins)
		}
	}
	return origins, nil
}

const maxEmbedParentOrigins = 16

func validHTTPOrigin(raw string, parsed *url.URL) bool {
	if parsed == nil || strings.TrimSpace(raw) != raw || parsed.Scheme == "" || parsed.Host == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Hostname() == "" || !validOriginHostname(parsed.Hostname()) || strings.Contains(parsed.Host, "*") || strings.HasSuffix(parsed.Host, ":") {
		return false
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return false
		}
	}
	return true
}

func validOriginHostname(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') || char == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func validDatabaseSchema(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 63 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || char == '_' ||
			(index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	switch value {
	case "public", "pg_catalog", "information_schema":
		return false
	default:
		return !strings.HasPrefix(value, "pg_")
	}
}

func (c Config) Sub2Configured() bool {
	return c.Sub2URL != "" && validKeyID(c.Sub2Key.ID) && len(c.Sub2Key.Secret) >= 32
}

// UserAccessAllowed is the points service's independent rollout gate. It is
// checked both when a user ticket is exchanged and on every user-session
// request, so narrowing a rollout also revokes sessions issued previously.
func (c Config) UserAccessAllowed(userID int64) bool {
	if userID <= 0 {
		return false
	}
	switch c.UserAccessMode {
	case "all":
		return true
	case "preview":
		_, allowed := c.UserPreviewIDs[userID]
		return allowed
	default:
		return false
	}
}

// CheckinAccessAllowed is an independent rollout gate for balance-affecting
// check-in requests. It does not hide the points center from other users.
func (c Config) CheckinAccessAllowed(userID int64) bool {
	if userID <= 0 {
		return false
	}
	switch c.CheckinAccessMode {
	case "", "all":
		return true
	case "preview":
		_, allowed := c.CheckinPreviewIDs[userID]
		return allowed
	default:
		return false
	}
}

const maxUserPreviewIDs = 10000

func parsePositiveIDSet(raw string) (map[int64]struct{}, error) {
	result := make(map[int64]struct{})
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		userID, err := strconv.ParseInt(item, 10, 64)
		if err != nil || userID <= 0 {
			return nil, errors.New("values must be comma-separated positive integers")
		}
		result[userID] = struct{}{}
		if len(result) > maxUserPreviewIDs {
			return nil, fmt.Errorf("at most %d users are allowed", maxUserPreviewIDs)
		}
	}
	return result, nil
}

func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	for index := range items {
		items[index] = strings.TrimSpace(items[index])
	}
	return items
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
