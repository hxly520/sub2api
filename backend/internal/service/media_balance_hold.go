package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const mediaBalanceHoldRequestPrefix = "media_balance_hold:"

const mediaBalanceAmountScale = 1e8

var (
	ErrMediaBalanceHoldUnavailable = infraerrors.New(http.StatusServiceUnavailable, "MEDIA_BALANCE_HOLD_UNAVAILABLE", "media balance hold service is unavailable")
	ErrMediaInsufficientBalance    = infraerrors.New(http.StatusPaymentRequired, "MEDIA_INSUFFICIENT_BALANCE", "insufficient balance for media generation")
	ErrMediaBalanceHoldConflict    = infraerrors.New(http.StatusConflict, "MEDIA_BALANCE_HOLD_CONFLICT", "media balance hold is already settled or has a different request")
	ErrMediaBalanceHoldNotFound    = infraerrors.New(http.StatusConflict, "MEDIA_BALANCE_HOLD_NOT_FOUND", "media balance hold was not found")
	ErrMediaBalanceCostExceedsHold = infraerrors.New(http.StatusConflict, "MEDIA_BALANCE_COST_EXCEEDS_HOLD", "media generation cost exceeds the reserved balance")
)

// MediaBalanceHoldCommand describes a single media-generation balance hold.
// The request ID is stable across async polling and retries, while the
// fingerprint prevents accidental reuse for a different payload or price.
type MediaBalanceHoldCommand struct {
	RequestID          string
	APIKeyID           int64
	UserID             int64
	RequestFingerprint string
	RequestPayloadHash string
	HoldAmount         float64
}

func (c *MediaBalanceHoldCommand) Normalize() {
	if c == nil {
		return
	}
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.RequestFingerprint = strings.TrimSpace(c.RequestFingerprint)
	c.RequestPayloadHash = strings.TrimSpace(c.RequestPayloadHash)
	if c.HoldAmount < 0 || math.IsNaN(c.HoldAmount) || math.IsInf(c.HoldAmount, 0) {
		c.HoldAmount = 0
	} else {
		c.HoldAmount = normalizeMediaBalanceAmount(c.HoldAmount)
	}
	if c.RequestFingerprint == "" {
		raw := fmt.Sprintf("%d|%d|%s|%0.10f|%s", c.UserID, c.APIKeyID, c.RequestID, c.HoldAmount, c.RequestPayloadHash)
		sum := sha256.Sum256([]byte(raw))
		c.RequestFingerprint = hex.EncodeToString(sum[:])
	}
}

type MediaBalanceHoldResult struct {
	Applied       bool
	NewBalance    *float64
	FrozenBalance *float64
}

// NewMediaBalanceHoldRequestID creates a unique hold ID for synchronous media
// requests. Async tasks use MediaBalanceHoldRequestID(taskID) so later polling
// can settle the same hold.
func NewMediaBalanceHoldRequestID() string {
	return mediaBalanceHoldRequestPrefix + uuid.NewString()
}

func MediaBalanceHoldRequestID(taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	return mediaBalanceHoldRequestPrefix + taskID
}

func (s *OpenAIGatewayService) MediaGenerationBalanceHoldRequired(apiKey *APIKey, subscription *UserSubscription) bool {
	if s == nil || s.cfg == nil || s.cfg.RunMode == config.RunModeSimple || apiKey == nil || apiKey.User == nil {
		return false
	}
	if subscription != nil && apiKey.Group != nil && apiKey.Group.IsSubscriptionType() {
		return false
	}
	return true
}

func (s *OpenAIGatewayService) ReserveMediaBalance(ctx context.Context, cmd *MediaBalanceHoldCommand) error {
	repo, ok := s.usageBillingRepo.(MediaBalanceHoldRepository)
	if !ok || repo == nil {
		return ErrMediaBalanceHoldUnavailable
	}
	result, err := repo.ReserveMediaBalance(ctx, cmd)
	if err != nil {
		return err
	}
	if result != nil && cmd != nil && s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateUserBalance(ctx, cmd.UserID)
	}
	return nil
}

func (s *OpenAIGatewayService) ReleaseMediaBalance(ctx context.Context, cmd *MediaBalanceHoldCommand) error {
	repo, ok := s.usageBillingRepo.(MediaBalanceHoldRepository)
	if !ok || repo == nil {
		return ErrMediaBalanceHoldUnavailable
	}
	result, err := repo.ReleaseMediaBalance(ctx, cmd)
	if err == nil && result != nil && cmd != nil && s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateUserBalance(ctx, cmd.UserID)
	}
	return err
}

// MarkMediaBalanceForCapture records that the upstream has produced or
// accepted a billable media result. If normal usage recording remains
// unavailable until the hold expires, the repository can still settle the
// exact frozen amount without replaying the upstream request.
func (s *OpenAIGatewayService) MarkMediaBalanceForCapture(ctx context.Context, cmd *MediaBalanceHoldCommand, actualCost float64) error {
	repo, ok := s.usageBillingRepo.(MediaBalanceHoldRepository)
	if !ok || repo == nil {
		return ErrMediaBalanceHoldUnavailable
	}
	result, err := repo.MarkMediaBalanceForCapture(ctx, cmd, normalizeMediaBalanceAmount(actualCost))
	if err == nil && result != nil && cmd != nil && s.billingCacheService != nil {
		_ = s.billingCacheService.InvalidateUserBalance(ctx, cmd.UserID)
	}
	return err
}

func (s *OpenAIGatewayService) SettleMediaBalanceHoldCommand(cmd *UsageBillingCommand, actualCost float64) {
	if cmd == nil {
		return
	}
	cmd.MediaBalanceHoldActualCost = actualCost
	cmd.BalanceCost = 0
}

func (p *MediaGenerationPricingSnapshot) EstimatedCost(count, durationSeconds int) float64 {
	if p == nil || p.UnitPrice <= 0 || p.RateMultiplier < 0 {
		return 0
	}
	if count <= 0 {
		count = 1
	}
	units := count
	if p.Mode == BillingModeVideo {
		units *= NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
	}
	return normalizeMediaBalanceAmount(p.UnitPrice * float64(units) * p.RateMultiplier)
}

func normalizeMediaBalanceAmount(value float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Ceil(value*mediaBalanceAmountScale) / mediaBalanceAmountScale
}
