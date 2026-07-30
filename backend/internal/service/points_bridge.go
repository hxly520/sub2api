package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

const pointsCreditPath = "/api/internal/points/credits"

var (
	ErrPointsSystemUnavailable = infraerrors.New(http.StatusServiceUnavailable, "POINTS_SYSTEM_UNAVAILABLE", "points system is not configured")
	ErrPointsSignatureInvalid  = infraerrors.Unauthorized("POINTS_SIGNATURE_INVALID", "invalid points service signature")
	ErrPointsCreditConflict    = infraerrors.Conflict("POINTS_CREDIT_CONFLICT", "points transaction id was reused with different data")
	ErrPointsCreditUserMissing = infraerrors.NotFound("POINTS_CREDIT_USER_NOT_FOUND", "points credit user was not found")
	ErrPointsCacheSyncPending  = infraerrors.New(http.StatusServiceUnavailable, "POINTS_CACHE_SYNC_PENDING", "balance cache synchronization is pending")
)

type PointsLaunchClaims struct {
	Audience  string `json:"aud"`
	Issuer    string `json:"iss"`
	Subject   int64  `json:"sub"`
	Role      string `json:"role"`
	Theme     string `json:"theme,omitempty"`
	Language  string `json:"lang,omitempty"`
	Nonce     string `json:"nonce"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type PointsBalanceCreditRequest struct {
	TransactionID   string `json:"transaction_id"`
	UserID          int64  `json:"user_id"`
	Amount          string `json:"amount"`
	Kind            string `json:"kind"`
	SourceReference string `json:"source_reference"`
	Reason          string `json:"reason"`
}

type PointsBalanceCreditInput struct {
	TransactionID   uuid.UUID
	UserID          int64
	Amount          decimal.Decimal
	Kind            string
	SourceReference string
	Reason          string
	Nonce           string
	PayloadHash     string
	RequestID       string
}

type PointsBalanceCreditResult struct {
	TransactionID uuid.UUID
	BalanceAfter  decimal.Decimal
	Idempotent    bool
}

type PointsBridgeRepository interface {
	ApplyPointsBalanceCredit(ctx context.Context, input PointsBalanceCreditInput) (*PointsBalanceCreditResult, error)
}

type pointsBalanceCache interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}

type PointsBridgeService struct {
	repo         PointsBridgeRepository
	billingCache pointsBalanceCache
	cfg          *config.Config
	now          func() time.Time
}

type PointsBridgeStatus struct {
	Enabled                bool
	Configured             bool
	Active                 bool
	PublicURL              string
	MenuLabel              string
	LaunchKeyID            string
	LaunchSecretConfigured bool
	CreditKeyID            string
	CreditSecretConfigured bool
	LaunchTTLSeconds       int
	ClockSkewSeconds       int
}

func NewPointsBridgeService(repo PointsBridgeRepository, billingCache pointsBalanceCache, cfg *config.Config) *PointsBridgeService {
	return &PointsBridgeService{repo: repo, billingCache: billingCache, cfg: cfg, now: time.Now}
}

func (s *PointsBridgeService) Enabled() bool {
	return s != nil && s.cfg != nil && s.cfg.PointsSystem.Active()
}

func (s *PointsBridgeService) Status() PointsBridgeStatus {
	status := PointsBridgeStatus{MenuLabel: "积分中心"}
	if s == nil || s.cfg == nil {
		return status
	}
	points := s.cfg.PointsSystem
	launchKey, launchErr := points.LaunchKey()
	creditKey, creditErr := points.CreditKey()
	status.Enabled = points.Enabled
	status.Configured = points.Configured()
	status.Active = points.Active()
	publicURL := strings.TrimSpace(points.PublicURL)
	if err := config.ValidateAbsoluteHTTPURL(publicURL); err == nil {
		parsedURL, parseErr := url.Parse(publicURL)
		if parseErr == nil && parsedURL != nil && parsedURL.User == nil && parsedURL.RawQuery == "" {
			status.PublicURL = publicURL
		}
	}
	status.MenuLabel = strings.TrimSpace(points.MenuLabel)
	if status.MenuLabel == "" {
		status.MenuLabel = "积分中心"
	}
	status.LaunchKeyID = strings.TrimSpace(points.LaunchKeyID)
	status.LaunchSecretConfigured = launchErr == nil && len(launchKey) >= 32
	status.CreditKeyID = strings.TrimSpace(points.CreditKeyID)
	status.CreditSecretConfigured = creditErr == nil && len(creditKey) >= 32
	status.LaunchTTLSeconds = points.LaunchTTLSeconds
	status.ClockSkewSeconds = points.ClockSkewSeconds
	return status
}

func (s *PointsBridgeService) CreateLaunchURL(userID int64, role, theme, language string) (string, error) {
	if s == nil || s.cfg == nil || userID <= 0 {
		return "", ErrPointsSystemUnavailable
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "user" && role != "admin" {
		return "", infraerrors.BadRequest("POINTS_ROLE_INVALID", "invalid points launch role")
	}
	if (role == "admin" && !s.cfg.PointsSystem.Configured()) ||
		(role == "user" && !s.cfg.PointsSystem.Active()) {
		return "", ErrPointsSystemUnavailable
	}
	theme = strings.ToLower(strings.TrimSpace(theme))
	if theme != "dark" {
		theme = "light"
	}
	language = normalizePointsLanguage(language)
	nonce, err := randomPointsToken(24)
	if err != nil {
		return "", fmt.Errorf("generate points launch nonce: %w", err)
	}
	now := s.now().UTC()
	claims := PointsLaunchClaims{
		Audience:  "points-system",
		Issuer:    "sub2api",
		Subject:   userID,
		Role:      role,
		Theme:     theme,
		Language:  language,
		Nonce:     nonce,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Duration(s.cfg.PointsSystem.LaunchTTLSeconds) * time.Second).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal points launch claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	launchKey, err := s.cfg.PointsSystem.LaunchKey()
	if err != nil {
		return "", ErrPointsSystemUnavailable
	}
	keyID := strings.TrimSpace(s.cfg.PointsSystem.LaunchKeyID)
	signature := signPointsPayload(launchKey, keyID+"."+encodedPayload)
	ticket := keyID + "." + encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature)

	launchURL, err := url.Parse(strings.TrimSpace(s.cfg.PointsSystem.PublicURL))
	if err != nil {
		return "", ErrPointsSystemUnavailable
	}
	launchURL.Path = strings.TrimRight(launchURL.Path, "/") + "/launch"
	query := launchURL.Query()
	query.Set("ticket", ticket)
	launchURL.RawQuery = query.Encode()
	return launchURL.String(), nil
}

func normalizePointsLanguage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 16 {
		return "en"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return "en"
	}
	return value
}

func (s *PointsBridgeService) VerifyAndApplyCredit(
	ctx context.Context,
	method, path, timestamp, keyID, nonce, signature string,
	body []byte,
	requestID string,
) (*PointsBalanceCreditResult, error) {
	if !s.Enabled() || s.repo == nil {
		return nil, ErrPointsSystemUnavailable
	}
	if !strings.EqualFold(strings.TrimSpace(method), http.MethodPost) || path != pointsCreditPath {
		return nil, ErrPointsSignatureInvalid
	}
	if err := s.verifyCreditSignature(method, path, timestamp, keyID, nonce, signature, body); err != nil {
		return nil, err
	}

	var request PointsBalanceCreditRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return nil, infraerrors.BadRequest("POINTS_CREDIT_INVALID", "invalid points credit request")
	}
	input, err := normalizePointsCreditRequest(request, nonce, body, requestID)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.ApplyPointsBalanceCredit(ctx, input)
	if err != nil {
		return nil, err
	}
	if s.billingCache != nil {
		if err := s.billingCache.InvalidateUserBalance(ctx, input.UserID); err != nil {
			logger.L().Warn("points.credit.balance_cache_invalidation_failed",
				zap.Int64("user_id", input.UserID),
				zap.String("transaction_id", input.TransactionID.String()),
				zap.Error(err),
			)
			// The balance transaction is already committed. Returning a retryable
			// error makes the outbox repeat the same idempotent transaction until
			// Redis invalidation succeeds, instead of serving a stale balance.
			return nil, ErrPointsCacheSyncPending
		}
	}
	return result, nil
}

func (s *PointsBridgeService) verifyCreditSignature(method, path, timestamp, keyID, nonce, signature string, body []byte) error {
	timestamp = strings.TrimSpace(timestamp)
	keyID = strings.TrimSpace(keyID)
	nonce = strings.TrimSpace(nonce)
	signature = strings.TrimSpace(signature)
	if timestamp == "" || keyID != strings.TrimSpace(s.cfg.PointsSystem.CreditKeyID) ||
		nonce == "" || signature == "" || len(nonce) > 128 {
		return ErrPointsSignatureInvalid
	}
	unixSeconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrPointsSignatureInvalid
	}
	requestTime := time.Unix(unixSeconds, 0)
	clockSkew := time.Duration(s.cfg.PointsSystem.ClockSkewSeconds) * time.Second
	if delta := s.now().UTC().Sub(requestTime.UTC()); delta > clockSkew || delta < -clockSkew {
		return ErrPointsSignatureInvalid
	}
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{
		"v1",
		keyID,
		strings.ToUpper(strings.TrimSpace(method)),
		path,
		timestamp,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	}, "\n")
	creditKey, err := s.cfg.PointsSystem.CreditKey()
	if err != nil {
		return ErrPointsSignatureInvalid
	}
	expected := signPointsPayload(creditKey, canonical)
	provided, err := hex.DecodeString(signature)
	if err != nil || !hmac.Equal(expected, provided) {
		return ErrPointsSignatureInvalid
	}
	return nil
}

func normalizePointsCreditRequest(request PointsBalanceCreditRequest, nonce string, body []byte, requestID string) (PointsBalanceCreditInput, error) {
	transactionID, err := uuid.Parse(strings.TrimSpace(request.TransactionID))
	if err != nil || transactionID == uuid.Nil || request.UserID <= 0 {
		return PointsBalanceCreditInput{}, infraerrors.BadRequest("POINTS_CREDIT_INVALID", "invalid points credit transaction")
	}
	if nonce != transactionID.String() {
		return PointsBalanceCreditInput{}, ErrPointsSignatureInvalid
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(request.Amount))
	if err != nil || amount.IsZero() || amount.Exponent() < -2 {
		return PointsBalanceCreditInput{}, infraerrors.BadRequest("POINTS_CREDIT_AMOUNT_INVALID", "points credit amount must use at most two decimal places")
	}
	kind := strings.ToLower(strings.TrimSpace(request.Kind))
	switch kind {
	case "checkin", "manual_grant":
		if amount.IsNegative() {
			return PointsBalanceCreditInput{}, infraerrors.BadRequest("POINTS_CREDIT_AMOUNT_INVALID", "credit amount must be positive")
		}
	case "reversal":
		if amount.IsPositive() {
			return PointsBalanceCreditInput{}, infraerrors.BadRequest("POINTS_CREDIT_AMOUNT_INVALID", "reversal amount must be negative")
		}
	default:
		return PointsBalanceCreditInput{}, infraerrors.BadRequest("POINTS_CREDIT_KIND_INVALID", "invalid points credit kind")
	}
	sourceReference := strings.TrimSpace(request.SourceReference)
	reason := strings.TrimSpace(request.Reason)
	if sourceReference == "" || len(sourceReference) > 128 || len(reason) > 500 {
		return PointsBalanceCreditInput{}, infraerrors.BadRequest("POINTS_CREDIT_INVALID", "invalid points credit metadata")
	}
	payloadHash := sha256.Sum256(body)
	return PointsBalanceCreditInput{
		TransactionID:   transactionID,
		UserID:          request.UserID,
		Amount:          amount,
		Kind:            kind,
		SourceReference: sourceReference,
		Reason:          reason,
		Nonce:           nonce,
		PayloadHash:     hex.EncodeToString(payloadHash[:]),
		RequestID:       strings.TrimSpace(requestID),
	}, nil
}

func signPointsPayload(secret []byte, payload string) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func randomPointsToken(size int) (string, error) {
	if size <= 0 {
		return "", errors.New("invalid random token size")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
