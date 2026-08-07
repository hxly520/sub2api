package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/golang-jwt/jwt/v5"
	"github.com/shopspring/decimal"
)

const (
	APIKeyTypeStandard = "standard"
	APIKeyTypeLink     = "link"

	LinkCardStatePendingActivation = "pending_activation"
	LinkCardStateActive            = "active"
	LinkCardStateFrozen            = "frozen"
	LinkCardStateDepleted          = "depleted"
	LinkCardStateRefunded          = "refunded"
	LinkCardStateRevoked           = "revoked"

	SettingKeyLinkCardsEnabled                 = "link_cards_enabled"
	SettingKeyLinkCardsRolloutUserID           = "link_cards_rollout_user_id"
	SettingKeyLinkCardsDevelopmentMode         = "link_cards_development_mode"
	SettingKeyLinkCardsDevelopmentUserIDs      = "link_cards_development_user_ids"
	SettingKeyLinkCardsDefaultConcurrency      = "link_cards_default_concurrency"
	SettingKeyLinkCardsDefaultRPMLimit         = "link_cards_default_rpm_limit"
	SettingKeyLinkCardsMaxBatchSize            = "link_cards_max_batch_size"
	SettingKeyLinkCardsMinimumDeposit          = "link_cards_minimum_deposit"
	SettingKeyLinkCardsPublicPortalURL         = "link_cards_public_portal_url"
	SettingKeyLinkCardsAPIBaseURL              = "link_cards_api_base_url"
	SettingKeyLinkCardsPublicSessionTTLSeconds = "link_cards_public_session_ttl_seconds"

	linkCardIdempotencyKeyMaxBytes = 255
	linkCardReasonMaxBytes         = 500
)

var (
	ErrLinkCardsDisabled              = infraerrors.NotFound("LINK_CARDS_NOT_AVAILABLE", "link card center is not available")
	ErrLinkCardNotFound               = infraerrors.NotFound("LINK_CARD_NOT_FOUND", "link card not found")
	ErrLinkCardGroupNotAuthorized     = infraerrors.BadRequest("LINK_CARD_GROUP_NOT_AUTHORIZED", "group is not authorized for link cards")
	ErrLinkCardInvalidAmount          = infraerrors.BadRequest("LINK_CARD_INVALID_AMOUNT", "amount must be greater than zero")
	ErrLinkCardInvalidQuantity        = infraerrors.BadRequest("LINK_CARD_INVALID_QUANTITY", "quantity is outside the allowed range")
	ErrLinkCardInsufficientBalance    = infraerrors.Conflict("LINK_CARD_INSUFFICIENT_BALANCE", "insufficient user balance")
	ErrLinkCardIdempotencyRequired    = infraerrors.BadRequest("LINK_CARD_IDEMPOTENCY_REQUIRED", "Idempotency-Key is required")
	ErrLinkCardIdempotencyConflict    = infraerrors.Conflict("LINK_CARD_IDEMPOTENCY_CONFLICT", "Idempotency-Key was reused with a different request")
	ErrLinkCardOperationNotAllowed    = infraerrors.Conflict("LINK_CARD_OPERATION_NOT_ALLOWED", "operation is not allowed for the current card state")
	ErrLinkCardNoRefundableBalance    = infraerrors.Conflict("LINK_CARD_NOT_REFUNDABLE", "link card has no refundable balance")
	ErrLinkCardInFlight               = infraerrors.Conflict("LINK_CARD_IN_FLIGHT", "link card still has active requests")
	ErrLinkCardSessionInvalid         = infraerrors.Unauthorized("LINK_CARD_SESSION_INVALID", "link card session is invalid or expired")
	ErrLinkCardPrepaidExhausted       = infraerrors.TooManyRequests("LINK_CARD_QUOTA_EXHAUSTED", "link card quota is exhausted")
	ErrLinkCardGroupPolicyUnavailable = infraerrors.ServiceUnavailable("LINK_CARD_GROUP_POLICY_UNAVAILABLE", "native group policy is unavailable")
)

type LinkCardSettings struct {
	Enabled                 bool             `json:"enabled"`
	DevelopmentMode         bool             `json:"development_mode"`
	DevelopmentUserIDs      []int64          `json:"development_user_ids,omitempty"`
	PreviewUserID           int64            `json:"preview_user_id"`
	PreviewOnly             bool             `json:"preview_only"`
	PublicPortalURL         string           `json:"public_portal_url"`
	APIBaseURL              string           `json:"api_base_url"`
	DefaultConcurrency      int              `json:"default_concurrency"`
	DefaultRPMLimit         int              `json:"default_rpm_limit"`
	MaxBatchSize            int              `json:"max_batch_size"`
	MinimumDeposit          *decimal.Decimal `json:"minimum_deposit"`
	PublicSessionTTLSeconds int              `json:"public_session_ttl_seconds"`
}

type UpdateLinkCardSettingsRequest struct {
	Enabled                 *bool            `json:"enabled"`
	DevelopmentMode         *bool            `json:"development_mode"`
	DevelopmentUserIDs      *[]int64         `json:"development_user_ids"`
	PublicPortalURL         *string          `json:"public_portal_url"`
	APIBaseURL              *string          `json:"api_base_url"`
	DefaultConcurrency      *int             `json:"default_concurrency"`
	DefaultRPMLimit         *int             `json:"default_rpm_limit"`
	MaxBatchSize            *int             `json:"max_batch_size"`
	MinimumDeposit          *decimal.Decimal `json:"minimum_deposit"`
	ClearMinimumDeposit     bool             `json:"clear_minimum_deposit"`
	PublicSessionTTLSeconds *int             `json:"public_session_ttl_seconds"`
}

type LinkCardAccess struct {
	Enabled         bool   `json:"enabled"`
	Allowed         bool   `json:"allowed"`
	DevelopmentMode bool   `json:"development_mode"`
	Reason          string `json:"reason,omitempty"`
}

type LinkCardGroup struct {
	ID                 int64     `json:"id"`
	GroupID            int64     `json:"group_id"`
	Name               string    `json:"name"`
	Platform           string    `json:"platform"`
	Description        string    `json:"description,omitempty"`
	RateMultiplier     float64   `json:"rate_multiplier"`
	DefaultConcurrency int       `json:"default_concurrency"`
	Models             []string  `json:"models,omitempty"`
	Capabilities       []string  `json:"capabilities,omitempty"`
	Enabled            bool      `json:"enabled"`
	Authorized         bool      `json:"authorized"`
	SortOrder          int       `json:"sort_order"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type LinkCard struct {
	ID                    int64           `json:"id"`
	APIKeyID              int64           `json:"api_key_id"`
	CreatorUserID         int64           `json:"creator_user_id"`
	CreatorEmail          string          `json:"creator_email,omitempty"`
	Key                   string          `json:"key,omitempty"`
	KeyPrefix             string          `json:"key_prefix,omitempty"`
	MaskedKey             string          `json:"masked_key,omitempty"`
	GroupID               int64           `json:"group_id"`
	GroupName             string          `json:"group_name"`
	Platform              string          `json:"platform,omitempty"`
	IssueRateMultiplier   float64         `json:"issue_rate_multiplier"`
	Status                string          `json:"status"`
	OriginalDepositAmount decimal.Decimal `json:"original_deposit_amount"`
	TotalDepositAmount    decimal.Decimal `json:"total_deposit_amount"`
	RefundableAmount      decimal.Decimal `json:"refundable_amount"`
	IssuedQuota           decimal.Decimal `json:"issued_quota"`
	UsedQuota             decimal.Decimal `json:"used_quota"`
	RemainingQuota        decimal.Decimal `json:"remaining_quota"`
	InFlightQuota         decimal.Decimal `json:"in_flight_quota"`
	RequestCount          int64           `json:"request_count"`
	Concurrency           int             `json:"concurrency"`
	RPMLimit              int             `json:"rpm_limit"`
	ActivatedAt           *time.Time      `json:"activated_at"`
	RevokedAt             *time.Time      `json:"revoked_at"`
	FrozenReason          string          `json:"frozen_reason,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	internalActualUsed    decimal.Decimal
	internalTotalRefunded decimal.Decimal
	internalReserved      decimal.Decimal
}

func (c *LinkCard) NormalizeDerivedFields() {
	if c == nil {
		return
	}
	c.ID = c.APIKeyID
	if c.Key != "" {
		c.KeyPrefix = linkCardKeyPrefix(c.Key)
		c.MaskedKey = maskLinkCardKey(c.Key)
	}
	remainingActual := c.TotalDepositAmount.Sub(c.UsedActualAmount()).Sub(c.TotalRefundedAmount()).Sub(c.ReservedActualAmount())
	if remainingActual.IsNegative() {
		remainingActual = decimal.Zero
	}
	c.RefundableAmount = remainingActual
	rate := decimal.NewFromFloat(c.IssueRateMultiplier)
	if !rate.IsPositive() {
		rate = decimal.NewFromInt(1)
	}
	c.IssuedQuota = c.TotalDepositAmount.Div(rate)
	c.UsedQuota = c.UsedActualAmount().Div(rate)
	c.RemainingQuota = remainingActual.Div(rate)
	c.InFlightQuota = c.ReservedActualAmount().Div(rate)
}

// InternalActualUsed and InternalTotalRefunded are populated by repositories
// but intentionally omitted from API JSON in favor of the 1x quota values.
func (c *LinkCard) UsedActualAmount() decimal.Decimal     { return c.internalActualUsed }
func (c *LinkCard) TotalRefundedAmount() decimal.Decimal  { return c.internalTotalRefunded }
func (c *LinkCard) ReservedActualAmount() decimal.Decimal { return c.internalReserved }

// unexported fields keep transport responses unambiguous.
func (c *LinkCard) SetFinancialState(actualUsed, totalRefunded decimal.Decimal) {
	c.internalActualUsed = actualUsed
	c.internalTotalRefunded = totalRefunded
}

func (c *LinkCard) SetReservedAmount(reserved decimal.Decimal) {
	if reserved.IsNegative() {
		reserved = decimal.Zero
	}
	c.internalReserved = reserved
}

type LinkCardListFilters struct {
	Search        string
	Status        string
	GroupID       *int64
	CreatorUserID *int64
	CreatorEmail  string
}

type LinkCardUsageFilters struct {
	CardID        *int64
	RequestID     string
	Model         string
	GroupID       *int64
	RequestType   string
	Stream        *bool
	StartAt       *time.Time
	EndAt         *time.Time
	CreatorUserID *int64
	CreatorEmail  string
	Key           string
}

type LinkCardUsageLog struct {
	ID                    int64           `json:"id"`
	LinkCardID            int64           `json:"link_card_id"`
	APIKeyID              int64           `json:"api_key_id"`
	CreatorUserID         int64           `json:"creator_user_id,omitempty"`
	CreatorEmail          string          `json:"creator_email,omitempty"`
	KeyPrefix             string          `json:"key_prefix,omitempty"`
	MaskedKey             string          `json:"masked_key,omitempty"`
	RequestID             string          `json:"request_id"`
	Model                 string          `json:"model"`
	InboundEndpoint       *string         `json:"inbound_endpoint"`
	GroupID               *int64          `json:"group_id"`
	GroupName             string          `json:"group_name,omitempty"`
	RequestType           string          `json:"request_type"`
	BillingMode           *string         `json:"billing_mode"`
	Stream                bool            `json:"stream"`
	InputTokens           int             `json:"input_tokens"`
	OutputTokens          int             `json:"output_tokens"`
	CacheCreationTokens   int             `json:"cache_creation_tokens"`
	CacheCreation5mTokens int             `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int             `json:"cache_creation_1h_tokens"`
	CacheReadTokens       int             `json:"cache_read_tokens"`
	ImageInputTokens      int             `json:"image_input_tokens"`
	ImageOutputTokens     int             `json:"image_output_tokens"`
	TotalTokens           int             `json:"total_tokens"`
	InputCost             decimal.Decimal `json:"input_cost"`
	OutputCost            decimal.Decimal `json:"output_cost"`
	CacheCreationCost     decimal.Decimal `json:"cache_creation_cost"`
	CacheReadCost         decimal.Decimal `json:"cache_read_cost"`
	ImageInputCost        decimal.Decimal `json:"image_input_cost"`
	ImageOutputCost       decimal.Decimal `json:"image_output_cost"`
	TotalCost             decimal.Decimal `json:"total_cost"`
	ActualCost            decimal.Decimal `json:"actual_cost"`
	RateMultiplier        float64         `json:"rate_multiplier"`
	ServiceTier           *string         `json:"service_tier"`
	DurationMS            *int            `json:"duration_ms"`
	FirstTokenMS          *int            `json:"first_token_ms"`
	CreatedAt             time.Time       `json:"created_at"`
	issueRateMultiplier   float64
}

func (l *LinkCardUsageLog) SetIssueRateMultiplier(rate float64) {
	if l != nil && rate > 0 {
		l.issueRateMultiplier = rate
	}
}

func (l LinkCardUsageLog) IssueRateMultiplier() float64 { return l.issueRateMultiplier }

type LinkCardSummary struct {
	TotalCards    int64           `json:"total_cards"`
	ActiveCards   int64           `json:"active_cards"`
	TotalReserved decimal.Decimal `json:"total_reserved"`
	TotalConsumed decimal.Decimal `json:"total_consumed"`
}

type CreateLinkCardsRequest struct {
	GroupID        int64
	Quantity       int
	Amount         decimal.Decimal
	IdempotencyKey string
}

type CreateLinkCardsCommand struct {
	CreatorUserID      int64
	Group              LinkCardGroup
	Quantity           int
	AmountPerCard      decimal.Decimal
	TotalDebit         decimal.Decimal
	Keys               []string
	Concurrency        int
	RPMLimit           int
	IdempotencyKeyHash string
	RequestFingerprint string
}

type CreateLinkCardsResult struct {
	Cards                []LinkCard      `json:"cards"`
	Quantity             int             `json:"quantity"`
	AmountPerCard        decimal.Decimal `json:"amount_per_card"`
	TotalDebited         decimal.Decimal `json:"total_debited"`
	RemainingUserBalance decimal.Decimal `json:"remaining_user_balance"`
	Replayed             bool            `json:"-"`
}

type LinkCardMutationCommand struct {
	APIKeyID           int64
	ActorUserID        int64
	CreatorUserID      int64
	Admin              bool
	Scope              string
	Amount             decimal.Decimal
	Reason             string
	Concurrency        *int
	RPMLimit           *int
	IdempotencyKeyHash string
	RequestFingerprint string
}

type LinkCardMutationResult struct {
	Card                 LinkCard        `json:"card"`
	Action               string          `json:"action,omitempty"`
	DebitedAmount        decimal.Decimal `json:"debited_amount,omitempty"`
	RefundedAmount       decimal.Decimal `json:"refunded_amount,omitempty"`
	RemainingUserBalance decimal.Decimal `json:"remaining_user_balance,omitempty"`
	UserBalance          decimal.Decimal `json:"user_balance,omitempty"`
	LedgerID             int64           `json:"ledger_id,omitempty"`
	Replayed             bool            `json:"-"`
}

type LinkCardRepository interface {
	ListAuthorizedGroups(ctx context.Context, includeDisabled bool, defaultConcurrency int) ([]LinkCardGroup, error)
	UpsertAuthorizedGroup(ctx context.Context, groupID int64, enabled bool, sortOrder int, actorUserID int64, defaultConcurrency int) (*LinkCardGroup, error)
	RemoveAuthorizedGroup(ctx context.Context, groupID int64) error
	GetAuthorizedGroup(ctx context.Context, groupID int64, defaultConcurrency int) (*LinkCardGroup, error)
	CreateCards(ctx context.Context, cmd CreateLinkCardsCommand) (*CreateLinkCardsResult, error)
	ListCards(ctx context.Context, creatorUserID *int64, params pagination.PaginationParams, filters LinkCardListFilters) ([]LinkCard, *pagination.PaginationResult, error)
	Summary(ctx context.Context) (*LinkCardSummary, error)
	GetCard(ctx context.Context, apiKeyID int64, creatorUserID *int64) (*LinkCard, error)
	FreezeForRefund(ctx context.Context, apiKeyID int64) (*LinkCard, error)
	Recharge(ctx context.Context, cmd LinkCardMutationCommand) (*LinkCardMutationResult, error)
	Refund(ctx context.Context, cmd LinkCardMutationCommand) (*LinkCardMutationResult, error)
	SetState(ctx context.Context, cmd LinkCardMutationCommand, state string) (*LinkCardMutationResult, error)
	SetLimits(ctx context.Context, cmd LinkCardMutationCommand) (*LinkCardMutationResult, error)
	ActivateByKey(ctx context.Context, key string) (*LinkCard, error)
	ListUsage(ctx context.Context, creatorUserID *int64, params pagination.PaginationParams, filters LinkCardUsageFilters) ([]LinkCardUsageLog, *pagination.PaginationResult, error)
}

type LinkCardBalanceCache interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}

type LinkCardConcurrency interface {
	GetAPIKeyConcurrencyStrict(ctx context.Context, apiKeyID int64) (int, error)
}

type LinkCardService struct {
	repo         LinkCardRepository
	settings     SettingRepository
	apiKeys      *APIKeyService
	balanceCache LinkCardBalanceCache
	concurrency  LinkCardConcurrency
	cfg          *config.Config
}

func NewLinkCardService(repo LinkCardRepository, settings SettingRepository, apiKeys *APIKeyService, balanceCache *BillingCacheService, concurrency *ConcurrencyService, cfg *config.Config) *LinkCardService {
	svc := &LinkCardService{repo: repo, settings: settings, apiKeys: apiKeys, concurrency: concurrency, cfg: cfg}
	if balanceCache != nil {
		svc.balanceCache = balanceCache
	}
	return svc
}

func defaultLinkCardSettings() LinkCardSettings {
	return LinkCardSettings{
		DevelopmentMode: true, DevelopmentUserIDs: []int64{1}, PreviewUserID: 1, PreviewOnly: true,
		PublicPortalURL: "https://key.52token.org", APIBaseURL: "https://api.52token.org/v1",
		DefaultConcurrency: 5, DefaultRPMLimit: 0, MaxBatchSize: 100, PublicSessionTTLSeconds: 3600,
	}
}

func (s *LinkCardService) GetSettings(ctx context.Context) (LinkCardSettings, error) {
	out := defaultLinkCardSettings()
	if s == nil || s.settings == nil {
		return out, nil
	}
	keys := []string{SettingKeyLinkCardsEnabled, SettingKeyLinkCardsRolloutUserID, SettingKeyLinkCardsDevelopmentMode,
		SettingKeyLinkCardsDevelopmentUserIDs, SettingKeyLinkCardsDefaultConcurrency, SettingKeyLinkCardsDefaultRPMLimit,
		SettingKeyLinkCardsMaxBatchSize, SettingKeyLinkCardsMinimumDeposit, SettingKeyLinkCardsPublicPortalURL,
		SettingKeyLinkCardsAPIBaseURL, SettingKeyLinkCardsPublicSessionTTLSeconds}
	values, err := s.settings.GetMultiple(ctx, keys)
	if err != nil {
		return out, fmt.Errorf("load link card settings: %w", err)
	}
	out.Enabled = values[SettingKeyLinkCardsEnabled] == "true"
	out.DevelopmentMode = values[SettingKeyLinkCardsDevelopmentMode] != "false"
	if ids := parseLinkCardUserIDs(values[SettingKeyLinkCardsDevelopmentUserIDs]); len(ids) > 0 {
		out.DevelopmentUserIDs = ids
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(values[SettingKeyLinkCardsRolloutUserID]), 10, 64); err == nil && id > 0 {
		out.PreviewUserID = id
		if !containsLinkCardUserID(out.DevelopmentUserIDs, id) {
			out.DevelopmentUserIDs = append(out.DevelopmentUserIDs, id)
		}
	}
	if v := positiveIntOr(values[SettingKeyLinkCardsDefaultConcurrency], out.DefaultConcurrency); v > 0 {
		out.DefaultConcurrency = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(values[SettingKeyLinkCardsDefaultRPMLimit])); err == nil && v >= 0 {
		out.DefaultRPMLimit = v
	}
	if v := positiveIntOr(values[SettingKeyLinkCardsMaxBatchSize], out.MaxBatchSize); v > 0 && v <= 1000 {
		out.MaxBatchSize = v
	}
	if v := strings.TrimSpace(values[SettingKeyLinkCardsMinimumDeposit]); v != "" {
		if d, err := decimal.NewFromString(v); err == nil && d.IsPositive() {
			out.MinimumDeposit = &d
		}
	}
	if v := strings.TrimSpace(values[SettingKeyLinkCardsPublicPortalURL]); v != "" {
		if normalized, ok := normalizeLinkCardURL(v, false); ok {
			out.PublicPortalURL = normalized
		}
	}
	if v := strings.TrimSpace(values[SettingKeyLinkCardsAPIBaseURL]); v != "" {
		if normalized, ok := normalizeLinkCardURL(v, true); ok {
			out.APIBaseURL = normalized
		}
	}
	if v := positiveIntOr(values[SettingKeyLinkCardsPublicSessionTTLSeconds], out.PublicSessionTTLSeconds); v >= 300 && v <= 86400 {
		out.PublicSessionTTLSeconds = v
	}
	out.PreviewOnly = !out.Enabled
	return out, nil
}

func (s *LinkCardService) UpdateSettings(ctx context.Context, req UpdateLinkCardSettingsRequest) (LinkCardSettings, error) {
	if s == nil || s.settings == nil {
		return LinkCardSettings{}, fmt.Errorf("link card settings repository is nil")
	}
	updates := map[string]string{}
	if req.Enabled != nil {
		updates[SettingKeyLinkCardsEnabled] = strconv.FormatBool(*req.Enabled)
	}
	if req.DevelopmentMode != nil {
		updates[SettingKeyLinkCardsDevelopmentMode] = strconv.FormatBool(*req.DevelopmentMode)
	}
	if req.DevelopmentUserIDs != nil {
		ids := normalizeLinkCardUserIDs(*req.DevelopmentUserIDs)
		b, _ := json.Marshal(ids)
		updates[SettingKeyLinkCardsDevelopmentUserIDs] = string(b)
		if len(ids) > 0 {
			updates[SettingKeyLinkCardsRolloutUserID] = strconv.FormatInt(ids[0], 10)
		}
	}
	if req.PublicPortalURL != nil {
		value, ok := normalizeLinkCardURL(*req.PublicPortalURL, false)
		if !ok {
			return LinkCardSettings{}, infraerrors.BadRequest("LINK_CARD_INVALID_PORTAL_URL", "public_portal_url must be an absolute http(s) URL without credentials, query, or fragment")
		}
		updates[SettingKeyLinkCardsPublicPortalURL] = value
	}
	if req.APIBaseURL != nil {
		value, ok := normalizeLinkCardURL(*req.APIBaseURL, true)
		if !ok {
			return LinkCardSettings{}, infraerrors.BadRequest("LINK_CARD_INVALID_API_BASE_URL", "api_base_url must be an absolute http(s) URL without credentials, query, or fragment")
		}
		updates[SettingKeyLinkCardsAPIBaseURL] = value
	}
	if req.DefaultConcurrency != nil {
		if *req.DefaultConcurrency <= 0 || *req.DefaultConcurrency > 1000 {
			return LinkCardSettings{}, infraerrors.BadRequest("LINK_CARD_INVALID_CONCURRENCY", "default_concurrency must be between 1 and 1000")
		}
		updates[SettingKeyLinkCardsDefaultConcurrency] = strconv.Itoa(*req.DefaultConcurrency)
	}
	if req.DefaultRPMLimit != nil {
		if *req.DefaultRPMLimit < 0 {
			return LinkCardSettings{}, infraerrors.BadRequest("LINK_CARD_INVALID_RPM", "default_rpm_limit cannot be negative")
		}
		updates[SettingKeyLinkCardsDefaultRPMLimit] = strconv.Itoa(*req.DefaultRPMLimit)
	}
	if req.MaxBatchSize != nil {
		if *req.MaxBatchSize <= 0 || *req.MaxBatchSize > 1000 {
			return LinkCardSettings{}, infraerrors.BadRequest("LINK_CARD_INVALID_BATCH_SIZE", "max_batch_size must be between 1 and 1000")
		}
		updates[SettingKeyLinkCardsMaxBatchSize] = strconv.Itoa(*req.MaxBatchSize)
	}
	if req.ClearMinimumDeposit {
		updates[SettingKeyLinkCardsMinimumDeposit] = ""
	} else if req.MinimumDeposit != nil {
		if !req.MinimumDeposit.IsPositive() {
			return LinkCardSettings{}, ErrLinkCardInvalidAmount
		}
		updates[SettingKeyLinkCardsMinimumDeposit] = req.MinimumDeposit.StringFixed(8)
	}
	if req.PublicSessionTTLSeconds != nil {
		if *req.PublicSessionTTLSeconds < 300 || *req.PublicSessionTTLSeconds > 86400 {
			return LinkCardSettings{}, infraerrors.BadRequest("LINK_CARD_INVALID_SESSION_TTL", "public_session_ttl_seconds must be between 300 and 86400")
		}
		updates[SettingKeyLinkCardsPublicSessionTTLSeconds] = strconv.Itoa(*req.PublicSessionTTLSeconds)
	}
	if len(updates) > 0 {
		if err := s.settings.SetMultiple(ctx, updates); err != nil {
			return LinkCardSettings{}, fmt.Errorf("update link card settings: %w", err)
		}
	}
	return s.GetSettings(ctx)
}

func (s *LinkCardService) Access(ctx context.Context, userID int64) (LinkCardAccess, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return LinkCardAccess{}, err
	}
	allowed := userID > 0 && (settings.Enabled || (settings.DevelopmentMode && containsLinkCardUserID(settings.DevelopmentUserIDs, userID)))
	out := LinkCardAccess{Enabled: settings.Enabled, Allowed: allowed, DevelopmentMode: settings.DevelopmentMode}
	if !allowed {
		out.Reason = "not_available"
	}
	return out, nil
}

func (s *LinkCardService) requireAccess(ctx context.Context, userID int64) (LinkCardSettings, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return settings, err
	}
	if userID <= 0 || (!settings.Enabled && (!settings.DevelopmentMode || !containsLinkCardUserID(settings.DevelopmentUserIDs, userID))) {
		return settings, ErrLinkCardsDisabled
	}
	return settings, nil
}

func (s *LinkCardService) ListGroups(ctx context.Context, userID int64, admin bool) ([]LinkCardGroup, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !admin {
		if _, err = s.requireAccess(ctx, userID); err != nil {
			return nil, err
		}
	}
	groups, err := s.repo.ListAuthorizedGroups(ctx, admin, settings.DefaultConcurrency)
	if err != nil || admin {
		return groups, err
	}
	if s.apiKeys == nil {
		return nil, ErrLinkCardGroupPolicyUnavailable
	}

	// Link-card authorization is an additional administrator-controlled
	// allow-list.  The native API-key service remains authoritative for the
	// user's public/exclusive group access and subscription checks, so a group
	// is shown here only when it exists in both sets.
	available, err := s.apiKeys.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get native available groups: %w", err)
	}
	nativeByID := make(map[int64]Group, len(available))
	for _, group := range available {
		nativeByID[group.ID] = group
	}
	rates, err := s.apiKeys.GetUserGroupRates(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get native user group rates: %w", err)
	}
	filtered := make([]LinkCardGroup, 0, len(groups))
	for _, group := range groups {
		native, ok := nativeByID[group.GroupID]
		if !ok {
			continue
		}
		// Keep the native group pricing as the source of truth, then apply a
		// user-specific override when one is configured. Invalid native pricing
		// fails closed instead of falling back to a stale authorization snapshot.
		if native.RateMultiplier <= 0 {
			continue
		}
		group.RateMultiplier = native.RateMultiplier
		if rate, ok := rates[group.GroupID]; ok {
			if rate <= 0 {
				continue
			}
			group.RateMultiplier = rate
		}
		filtered = append(filtered, group)
	}
	return filtered, nil
}

// resolveNativeLinkCardGroup verifies that a group is available through the
// regular Sub2API API-key rules and returns the effective rate for the user.
// It is intentionally kept in the service layer so UI/API callers receive the
// same result, while CreateCards repeats the checks in its financial
// transaction to prevent request tampering or a time-of-check race.
func (s *LinkCardService) resolveNativeLinkCardGroup(ctx context.Context, userID, groupID int64, group *LinkCardGroup) error {
	if s == nil || s.apiKeys == nil {
		return ErrLinkCardGroupPolicyUnavailable
	}
	if group == nil {
		return ErrLinkCardGroupNotAuthorized
	}
	available, err := s.apiKeys.GetAvailableGroups(ctx, userID)
	if err != nil {
		return fmt.Errorf("get native available groups: %w", err)
	}
	var native *Group
	for i := range available {
		if available[i].ID == groupID {
			native = &available[i]
			break
		}
	}
	if native == nil {
		return ErrLinkCardGroupNotAuthorized
	}
	if native.RateMultiplier <= 0 {
		return ErrLinkCardGroupNotAuthorized
	}
	group.RateMultiplier = native.RateMultiplier
	rates, err := s.apiKeys.GetUserGroupRates(ctx, userID)
	if err != nil {
		return fmt.Errorf("get native user group rates: %w", err)
	}
	if rate, ok := rates[groupID]; ok {
		if rate <= 0 {
			return ErrLinkCardGroupNotAuthorized
		}
		group.RateMultiplier = rate
	}
	return nil
}

func (s *LinkCardService) AuthorizeGroup(ctx context.Context, actorUserID, groupID int64, enabled bool, sortOrder int) (*LinkCardGroup, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.repo.UpsertAuthorizedGroup(ctx, groupID, enabled, sortOrder, actorUserID, settings.DefaultConcurrency)
	if err != nil {
		return nil, err
	}
	if !enabled && s.apiKeys != nil {
		s.apiKeys.InvalidateAuthCacheByGroupID(ctx, groupID)
	}
	return item, nil
}

func (s *LinkCardService) RemoveAuthorizedGroup(ctx context.Context, groupID int64) error {
	if err := s.repo.RemoveAuthorizedGroup(ctx, groupID); err != nil {
		return err
	}
	// The repository freezes existing link keys in the same transaction. Clear
	// every instance's auth snapshot after commit so a revoked authorization is
	// not hidden behind the normal API-key cache TTL.
	if s.apiKeys != nil {
		s.apiKeys.InvalidateAuthCacheByGroupID(ctx, groupID)
	}
	return nil
}

func (s *LinkCardService) Create(ctx context.Context, creatorUserID int64, req CreateLinkCardsRequest) (*CreateLinkCardsResult, error) {
	settings, err := s.requireAccess(ctx, creatorUserID)
	if err != nil {
		return nil, err
	}
	if req.Quantity <= 0 || req.Quantity > settings.MaxBatchSize {
		return nil, ErrLinkCardInvalidQuantity
	}
	amount := req.Amount.Round(8)
	if !amount.IsPositive() || (settings.MinimumDeposit != nil && amount.LessThan(*settings.MinimumDeposit)) {
		return nil, ErrLinkCardInvalidAmount
	}
	keyHash, err := validateAndHashLinkCardIdempotency(req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	group, err := s.repo.GetAuthorizedGroup(ctx, req.GroupID, settings.DefaultConcurrency)
	if err != nil {
		return nil, err
	}
	if group == nil || !group.Enabled || group.RateMultiplier <= 0 {
		return nil, ErrLinkCardGroupNotAuthorized
	}
	if err := s.resolveNativeLinkCardGroup(ctx, creatorUserID, req.GroupID, group); err != nil {
		return nil, err
	}
	keys := make([]string, req.Quantity)
	for i := range keys {
		keys[i], err = generateLinkCardKey()
		if err != nil {
			return nil, err
		}
	}
	total := amount.Mul(decimal.NewFromInt(int64(req.Quantity))).Round(8)
	fingerprint := hashLinkCardFingerprint("create", creatorUserID, req.GroupID, req.Quantity, amount.StringFixed(8))
	result, err := s.repo.CreateCards(ctx, CreateLinkCardsCommand{CreatorUserID: creatorUserID, Group: *group,
		Quantity: req.Quantity, AmountPerCard: amount, TotalDebit: total, Keys: keys,
		Concurrency: settings.DefaultConcurrency, RPMLimit: settings.DefaultRPMLimit,
		IdempotencyKeyHash: keyHash, RequestFingerprint: fingerprint})
	if err != nil {
		return nil, err
	}
	s.invalidateBalance(ctx, creatorUserID)
	return result, nil
}

func (s *LinkCardService) ListCards(ctx context.Context, userID int64, admin bool, params pagination.PaginationParams, filters LinkCardListFilters) ([]LinkCard, *pagination.PaginationResult, error) {
	var owner *int64
	if !admin {
		if _, err := s.requireAccess(ctx, userID); err != nil {
			return nil, nil, err
		}
		owner = &userID
	}
	return s.repo.ListCards(ctx, owner, params, filters)
}

func (s *LinkCardService) Summary(ctx context.Context) (*LinkCardSummary, error) {
	return s.repo.Summary(ctx)
}

func (s *LinkCardService) Recharge(ctx context.Context, actorUserID, cardID int64, amount decimal.Decimal, idem string, admin bool) (*LinkCardMutationResult, error) {
	if !admin {
		if _, err := s.requireAccess(ctx, actorUserID); err != nil {
			return nil, err
		}
	}
	amount = amount.Round(8)
	if !amount.IsPositive() {
		return nil, ErrLinkCardInvalidAmount
	}
	keyHash, err := validateAndHashLinkCardIdempotency(idem)
	if err != nil {
		return nil, err
	}
	fingerprint := hashLinkCardFingerprint("recharge", actorUserID, cardID, amount.StringFixed(8), admin)
	result, err := s.repo.Recharge(ctx, LinkCardMutationCommand{APIKeyID: cardID, ActorUserID: actorUserID, Admin: admin,
		Scope: "recharge", Amount: amount, IdempotencyKeyHash: keyHash, RequestFingerprint: fingerprint})
	if err != nil {
		return nil, err
	}
	s.invalidateCardCaches(ctx, &result.Card)
	return result, nil
}

func (s *LinkCardService) Refund(ctx context.Context, actorUserID, cardID int64, reason, idem string, admin bool) (*LinkCardMutationResult, error) {
	if !admin {
		if _, err := s.requireAccess(ctx, actorUserID); err != nil {
			return nil, err
		}
	}
	keyHash, err := validateAndHashLinkCardIdempotency(idem)
	if err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > linkCardReasonMaxBytes {
		return nil, infraerrors.BadRequest("LINK_CARD_REASON_TOO_LONG", "reason is too long")
	}
	fingerprint := hashLinkCardFingerprint("refund", actorUserID, cardID, reason, admin)
	if admin {
		card, err := s.repo.GetCard(ctx, cardID, nil)
		if err != nil {
			return nil, err
		}
		switch card.Status {
		case LinkCardStateActive, LinkCardStateFrozen, LinkCardStateDepleted:
			card, err = s.repo.FreezeForRefund(ctx, cardID)
			if err != nil {
				return nil, err
			}
			s.invalidateCardCaches(ctx, card)
			if err := s.waitForCardIdle(ctx, cardID); err != nil {
				return nil, err
			}
		}
	}
	result, err := s.repo.Refund(ctx, LinkCardMutationCommand{APIKeyID: cardID, ActorUserID: actorUserID, Admin: admin,
		Scope: "refund", Reason: reason, IdempotencyKeyHash: keyHash, RequestFingerprint: fingerprint})
	if err != nil {
		return nil, err
	}
	s.invalidateCardCaches(ctx, &result.Card)
	return result, nil
}

func (s *LinkCardService) waitForCardIdle(ctx context.Context, cardID int64) error {
	if s == nil || s.concurrency == nil {
		return infraerrors.ServiceUnavailable("LINK_CARD_CONCURRENCY_UNAVAILABLE", "link card concurrency state is unavailable")
	}
	drainCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		count, err := s.concurrency.GetAPIKeyConcurrencyStrict(drainCtx, cardID)
		if err != nil {
			return infraerrors.ServiceUnavailable("LINK_CARD_CONCURRENCY_UNAVAILABLE", "link card concurrency state is unavailable").WithCause(err)
		}
		if count == 0 {
			card, cardErr := s.repo.GetCard(drainCtx, cardID, nil)
			if cardErr != nil {
				return infraerrors.ServiceUnavailable("LINK_CARD_HOLD_STATE_UNAVAILABLE", "link card hold state is unavailable").WithCause(cardErr)
			}
			if card == nil || !card.ReservedActualAmount().IsPositive() {
				return nil
			}
		}
		select {
		case <-drainCtx.Done():
			return ErrLinkCardInFlight
		case <-ticker.C:
		}
	}
}

func (s *LinkCardService) AdminAction(ctx context.Context, actorUserID, cardID int64, action string, amount decimal.Decimal, concurrency, rpmLimit *int, reason, idem string) (*LinkCardMutationResult, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "recharge":
		return s.Recharge(ctx, actorUserID, cardID, amount, idem, true)
	case "refund":
		return s.Refund(ctx, actorUserID, cardID, reason, idem, true)
	}
	keyHash, err := validateAndHashLinkCardIdempotency(idem)
	if err != nil {
		return nil, err
	}
	cmd := LinkCardMutationCommand{APIKeyID: cardID, ActorUserID: actorUserID, Admin: true, Scope: action,
		Reason: strings.TrimSpace(reason), Concurrency: concurrency, RPMLimit: rpmLimit,
		IdempotencyKeyHash: keyHash, RequestFingerprint: hashLinkCardFingerprint(action, actorUserID, cardID, concurrency, rpmLimit, reason)}
	var result *LinkCardMutationResult
	switch action {
	case "freeze":
		result, err = s.repo.SetState(ctx, cmd, LinkCardStateFrozen)
	case "unfreeze":
		result, err = s.repo.SetState(ctx, cmd, LinkCardStateActive)
	case "revoke", "delete":
		result, err = s.repo.SetState(ctx, cmd, LinkCardStateRevoked)
	case "set_limits":
		if concurrency != nil && (*concurrency <= 0 || *concurrency > 1000) {
			return nil, infraerrors.BadRequest("LINK_CARD_INVALID_CONCURRENCY", "concurrency must be between 1 and 1000")
		}
		if rpmLimit != nil && *rpmLimit < 0 {
			return nil, infraerrors.BadRequest("LINK_CARD_INVALID_RPM", "rpm_limit cannot be negative")
		}
		result, err = s.repo.SetLimits(ctx, cmd)
	default:
		return nil, infraerrors.BadRequest("LINK_CARD_INVALID_ACTION", "unsupported link card action")
	}
	if err != nil {
		return nil, err
	}
	s.invalidateCardCaches(ctx, &result.Card)
	return result, nil
}

func (s *LinkCardService) ListUsage(ctx context.Context, userID int64, admin bool, params pagination.PaginationParams, filters LinkCardUsageFilters) ([]LinkCardUsageLog, *pagination.PaginationResult, error) {
	var owner *int64
	if !admin {
		if _, err := s.requireAccess(ctx, userID); err != nil {
			return nil, nil, err
		}
		owner = &userID
	}
	items, page, err := s.repo.ListUsage(ctx, owner, params, filters)
	if err != nil {
		return nil, nil, err
	}
	if !admin {
		normalizeLinkCardExternalUsage(items)
	}
	return items, page, nil
}

func normalizeLinkCardExternalUsage(items []LinkCardUsageLog) {
	for i := range items {
		// Public/creator views use the card's fixed issuance 1x units. Native
		// usage rows retain the current Sub2API rate for administrator auditing.
		issueRate := decimal.NewFromFloat(items[i].IssueRateMultiplier())
		if issueRate.IsPositive() {
			if items[i].TotalCost.IsPositive() {
				items[i].RateMultiplier = items[i].ActualCost.Div(items[i].TotalCost).Div(issueRate).InexactFloat64()
			} else {
				items[i].RateMultiplier /= items[i].IssueRateMultiplier()
			}
			items[i].ActualCost = items[i].ActualCost.Div(issueRate)
			continue
		}
		items[i].ActualCost = items[i].TotalCost
		items[i].RateMultiplier = 1
	}
}

type PublicLinkCard struct {
	MaskedKey      string          `json:"masked_key"`
	Status         string          `json:"status"`
	GroupName      string          `json:"group_name"`
	IssuedQuota    decimal.Decimal `json:"issued_quota"`
	UsedQuota      decimal.Decimal `json:"used_quota"`
	RemainingQuota decimal.Decimal `json:"remaining_quota"`
	RequestCount   int64           `json:"request_count"`
	ActivatedAt    *time.Time      `json:"activated_at"`
	CreatedAt      time.Time       `json:"created_at"`
}

type PublicLinkCardProfile struct {
	Card       PublicLinkCard `json:"card"`
	APIBaseURL string         `json:"api_base_url"`
}

// PublicLinkCardUsageLog is the public portal projection of a usage row.
// Card, creator and database identifiers stay server-side; the session token
// already scopes the query to one card.
type PublicLinkCardUsageLog struct {
	RequestID             string          `json:"request_id"`
	Model                 string          `json:"model"`
	InboundEndpoint       *string         `json:"inbound_endpoint"`
	RequestType           string          `json:"request_type"`
	BillingMode           *string         `json:"billing_mode"`
	Stream                bool            `json:"stream"`
	InputTokens           int             `json:"input_tokens"`
	OutputTokens          int             `json:"output_tokens"`
	CacheCreationTokens   int             `json:"cache_creation_tokens"`
	CacheCreation5mTokens int             `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int             `json:"cache_creation_1h_tokens"`
	CacheReadTokens       int             `json:"cache_read_tokens"`
	ImageInputTokens      int             `json:"image_input_tokens"`
	ImageOutputTokens     int             `json:"image_output_tokens"`
	TotalTokens           int             `json:"total_tokens"`
	InputCost             decimal.Decimal `json:"input_cost"`
	OutputCost            decimal.Decimal `json:"output_cost"`
	CacheCreationCost     decimal.Decimal `json:"cache_creation_cost"`
	CacheReadCost         decimal.Decimal `json:"cache_read_cost"`
	ImageInputCost        decimal.Decimal `json:"image_input_cost"`
	ImageOutputCost       decimal.Decimal `json:"image_output_cost"`
	TotalCost             decimal.Decimal `json:"total_cost"`
	ActualCost            decimal.Decimal `json:"actual_cost"`
	RateMultiplier        float64         `json:"rate_multiplier"`
	ServiceTier           *string         `json:"service_tier"`
	DurationMS            *int            `json:"duration_ms"`
	FirstTokenMS          *int            `json:"first_token_ms"`
	CreatedAt             time.Time       `json:"created_at"`
}

func publicLinkCardUsage(item LinkCardUsageLog) PublicLinkCardUsageLog {
	return PublicLinkCardUsageLog{
		RequestID: item.RequestID, Model: item.Model, InboundEndpoint: item.InboundEndpoint,
		RequestType: item.RequestType, BillingMode: item.BillingMode, Stream: item.Stream,
		InputTokens: item.InputTokens, OutputTokens: item.OutputTokens,
		CacheCreationTokens: item.CacheCreationTokens, CacheCreation5mTokens: item.CacheCreation5mTokens,
		CacheCreation1hTokens: item.CacheCreation1hTokens, CacheReadTokens: item.CacheReadTokens,
		ImageInputTokens: item.ImageInputTokens, ImageOutputTokens: item.ImageOutputTokens,
		TotalTokens: item.TotalTokens, InputCost: item.InputCost, OutputCost: item.OutputCost,
		CacheCreationCost: item.CacheCreationCost, CacheReadCost: item.CacheReadCost,
		ImageInputCost: item.ImageInputCost, ImageOutputCost: item.ImageOutputCost,
		TotalCost: item.TotalCost, ActualCost: item.ActualCost, RateMultiplier: item.RateMultiplier,
		ServiceTier: item.ServiceTier, DurationMS: item.DurationMS, FirstTokenMS: item.FirstTokenMS,
		CreatedAt: item.CreatedAt,
	}
}

func publicLinkCard(card *LinkCard) *PublicLinkCard {
	if card == nil {
		return nil
	}
	return &PublicLinkCard{MaskedKey: card.MaskedKey, Status: card.Status, GroupName: card.GroupName,
		IssuedQuota: card.IssuedQuota, UsedQuota: card.UsedQuota, RemainingQuota: card.RemainingQuota,
		RequestCount: card.RequestCount, ActivatedAt: card.ActivatedAt, CreatedAt: card.CreatedAt}
}

type LinkCardPortalSession struct {
	jwt.RegisteredClaims
	CardID int64 `json:"card_id"`
}

type LinkCardActivationResult struct {
	SessionToken string          `json:"session_token"`
	ExpiresAt    time.Time       `json:"expires_at"`
	Card         *PublicLinkCard `json:"card,omitempty"`
}

func (s *LinkCardService) Activate(ctx context.Context, key string) (*LinkCardActivationResult, error) {
	key = strings.TrimSpace(key)
	if len(key) < 16 || len(key) > MaxAPIKeyCredentialBytes {
		return nil, ErrLinkCardNotFound
	}
	if s == nil || s.apiKeys == nil {
		return nil, ErrLinkCardNotFound
	}
	apiKey, err := s.apiKeys.GetByKey(ctx, key)
	if err != nil || apiKey == nil || !apiKey.IsLinkKey() {
		return nil, ErrLinkCardNotFound
	}
	if _, err := s.requireAccess(ctx, apiKey.UserID); err != nil {
		return nil, err
	}
	// Validate all session prerequisites before mutating the card. A failed
	// signing configuration must never leave a pending card permanently active.
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if s.cfg == nil || strings.TrimSpace(s.cfg.JWT.Secret) == "" || settings.PublicSessionTTLSeconds < 300 {
		return nil, infraerrors.ServiceUnavailable("LINK_CARD_PORTAL_UNAVAILABLE", "link card portal is not configured")
	}
	card, err := s.repo.ActivateByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	s.invalidateCardCaches(ctx, card)
	now := time.Now()
	expires := now.Add(time.Duration(settings.PublicSessionTTLSeconds) * time.Second)
	claims := LinkCardPortalSession{RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(card.APIKeyID, 10),
		IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), Audience: jwt.ClaimStrings{"link-card-portal"}}, CardID: card.APIKeyID}
	token, err := s.signPortalClaims(claims)
	if err != nil {
		return nil, err
	}
	card.NormalizeDerivedFields()
	return &LinkCardActivationResult{SessionToken: token, ExpiresAt: expires, Card: publicLinkCard(card)}, nil
}

func (s *LinkCardService) PortalCard(ctx context.Context, sessionToken string) (*PublicLinkCardProfile, error) {
	cardID, err := s.verifyPortalToken(sessionToken)
	if err != nil {
		return nil, err
	}
	card, err := s.repo.GetCard(ctx, cardID, nil)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireAccess(ctx, card.CreatorUserID); err != nil {
		return nil, err
	}
	if card.Status == LinkCardStateRevoked || card.Status == LinkCardStateRefunded {
		return nil, ErrLinkCardSessionInvalid
	}
	card.NormalizeDerivedFields()
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return &PublicLinkCardProfile{Card: *publicLinkCard(card), APIBaseURL: settings.APIBaseURL}, nil
}

func (s *LinkCardService) PortalUsage(ctx context.Context, sessionToken string, params pagination.PaginationParams, filters LinkCardUsageFilters) ([]PublicLinkCardUsageLog, *pagination.PaginationResult, error) {
	cardID, err := s.verifyPortalToken(sessionToken)
	if err != nil {
		return nil, nil, err
	}
	card, err := s.repo.GetCard(ctx, cardID, nil)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.requireAccess(ctx, card.CreatorUserID); err != nil {
		return nil, nil, err
	}
	if card.Status == LinkCardStateRevoked || card.Status == LinkCardStateRefunded {
		return nil, nil, ErrLinkCardSessionInvalid
	}

	// A portal session is scoped to exactly one card. Only date bounds are
	// accepted from the public request; every identity/search filter is rebuilt
	// server-side to prevent cross-card probing.
	portalFilters := LinkCardUsageFilters{
		CardID:  &cardID,
		StartAt: filters.StartAt,
		EndAt:   filters.EndAt,
	}
	items, page, err := s.repo.ListUsage(ctx, nil, params, portalFilters)
	if err != nil {
		return nil, nil, err
	}
	normalizeLinkCardExternalUsage(items)
	publicItems := make([]PublicLinkCardUsageLog, len(items))
	for i := range items {
		publicItems[i] = publicLinkCardUsage(items[i])
	}
	return publicItems, page, nil
}

func (s *LinkCardService) signPortalClaims(claims LinkCardPortalSession) (string, error) {
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.JWT.Secret) == "" {
		return "", fmt.Errorf("link card portal signing key is unavailable")
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWT.Secret))
}

func (s *LinkCardService) verifyPortalToken(raw string) (int64, error) {
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.JWT.Secret) == "" {
		return 0, ErrLinkCardSessionInvalid
	}
	claims := &LinkCardPortalSession{}
	token, err := jwt.ParseWithClaims(strings.TrimSpace(raw), claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrLinkCardSessionInvalid
		}
		return []byte(s.cfg.JWT.Secret), nil
	}, jwt.WithAudience("link-card-portal"), jwt.WithExpirationRequired())
	if err != nil || token == nil || !token.Valid || claims.CardID <= 0 {
		return 0, ErrLinkCardSessionInvalid
	}
	return claims.CardID, nil
}

func (s *LinkCardService) invalidateBalance(ctx context.Context, userID int64) {
	if s != nil && s.balanceCache != nil && userID > 0 {
		_ = s.balanceCache.InvalidateUserBalance(ctx, userID)
	}
}
func (s *LinkCardService) invalidateCardCaches(ctx context.Context, card *LinkCard) {
	if card == nil {
		return
	}
	s.invalidateBalance(ctx, card.CreatorUserID)
	if s.apiKeys != nil && card.Key != "" {
		s.apiKeys.InvalidateAuthCacheByKey(ctx, card.Key)
	}
}

func validateAndHashLinkCardIdempotency(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrLinkCardIdempotencyRequired
	}
	if len(value) > linkCardIdempotencyKeyMaxBytes {
		return "", infraerrors.BadRequest("LINK_CARD_IDEMPOTENCY_TOO_LONG", "Idempotency-Key is too long")
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), nil
}
func hashLinkCardFingerprint(values ...any) string {
	payload, err := json.Marshal(values)
	if err != nil {
		payload = []byte(fmt.Sprintf("%#v", values))
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
func generateLinkCardKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate link card key: %w", err)
	}
	return "sk-card-" + hex.EncodeToString(b), nil
}
func linkCardKeyPrefix(key string) string {
	if len(key) <= 14 {
		return key
	}
	return key[:14]
}
func maskLinkCardKey(key string) string {
	if len(key) <= 12 {
		return key
	}
	return key[:8] + "..." + key[len(key)-4:]
}
func containsLinkCardUserID(ids []int64, id int64) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
func normalizeLinkCardUserIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
func parseLinkCardUserIDs(raw string) []int64 {
	var ids []int64
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return nil
	}
	return normalizeLinkCardUserIDs(ids)
}

// normalizeLinkCardURL accepts only display/routing URLs that cannot carry
// credentials or hidden query/fragment state. The API base may include a
// version path (for example /v1), while both URL forms are kept canonical for
// the frontend and activation response.
func normalizeLinkCardURL(raw string, apiBase bool) (string, bool) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" || len(value) > 2048 {
		return "", false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", false
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if apiBase {
		path := strings.TrimRight(parsed.Path, "/")
		if path != "/v1" && !strings.HasSuffix(path, "/v1") {
			return "", false
		}
	}
	if !apiBase && parsed.Path != "/" {
		// Portal paths are allowed for reverse-proxy deployments, but never
		// preserve a trailing slash so links remain stable.
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}
	return strings.TrimRight(parsed.String(), "/"), true
}
func positiveIntOr(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
