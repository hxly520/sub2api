package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

type linkCardQuotaUpdaterStub struct{}

func (linkCardQuotaUpdaterStub) UpdateQuotaUsed(context.Context, int64, float64) error { return nil }
func (linkCardQuotaUpdaterStub) UpdateRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

type linkCardBillingStateLoaderStub struct {
	state *LinkCardBillingState
	err   error
}

func (s *linkCardBillingStateLoaderStub) GetLinkCardBillingState(context.Context, int64) (*LinkCardBillingState, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.state, nil
}

func (*linkCardBillingStateLoaderStub) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	return &APIKeyRateLimitData{}, nil
}

func TestBuildUsageBillingCommandLinkCardNeverChargesCreatorBalance(t *testing.T) {
	cmd := buildUsageBillingCommand("link-request", nil, &postUsageBillingParams{
		Cost:   &CostBreakdown{TotalCost: 10, ActualCost: 0.8},
		User:   &User{ID: 1},
		APIKey: &APIKey{ID: 9, UserID: 1, KeyType: APIKeyTypeLink, Quota: 100},
		Account: &Account{
			ID: 17,
		},
		APIKeyService: linkCardQuotaUpdaterStub{},
	})

	require.NotNil(t, cmd)
	require.InDelta(t, 0.8, cmd.PrepaidLinkCost, 0.0000000001)
	require.Zero(t, cmd.BalanceCost)
	require.Zero(t, cmd.SubscriptionCost)
	require.Zero(t, cmd.APIKeyQuotaCost, "link reserve settlement owns quota_used updates")
}

func TestBuildUsageBillingCommandKeepsStandardKeyBillingUnchanged(t *testing.T) {
	cmd := buildUsageBillingCommand("standard-request", nil, &postUsageBillingParams{
		Cost:   &CostBreakdown{TotalCost: 10, ActualCost: 0.8},
		User:   &User{ID: 1},
		APIKey: &APIKey{ID: 9, UserID: 1, KeyType: APIKeyTypeStandard, Quota: 100},
		Account: &Account{
			ID: 17,
		},
		APIKeyService: linkCardQuotaUpdaterStub{},
	})

	require.NotNil(t, cmd)
	require.Zero(t, cmd.PrepaidLinkCost)
	require.InDelta(t, 0.8, cmd.BalanceCost, 0.0000000001)
	require.InDelta(t, 0.8, cmd.APIKeyQuotaCost, 0.0000000001)
}

func TestStandardKeyIgnoresAllLinkOnlyRuntimeFields(t *testing.T) {
	user := &User{ID: 1, Concurrency: 20}
	standard := &APIKey{
		ID:                 9,
		KeyType:            APIKeyTypeStandard,
		LinkState:          LinkCardStateActive,
		LinkRateMultiplier: 0.08,
		LinkConcurrency:    5,
		LinkRPMLimit:       1,
		User:               user,
	}

	require.False(t, standard.IsLinkKey())
	require.Equal(t, user.Concurrency, standard.EffectiveConcurrency())

	legacyStandard := *standard
	legacyStandard.KeyType = ""
	require.False(t, legacyStandard.IsLinkKey())
	require.Equal(t, user.Concurrency, legacyStandard.EffectiveConcurrency())
}

func TestLinkCardCurrentRateKeepsNativeGroupModifiers(t *testing.T) {
	groupID := int64(8)
	group := &Group{
		ID:                   groupID,
		SubscriptionType:     SubscriptionTypeSubscription,
		PeakRateEnabled:      true,
		PeakStart:            "09:00",
		PeakEnd:              "18:00",
		PeakRateMultiplier:   1.5,
		ImageRateIndependent: true,
		ImageRateMultiplier:  0.11,
		VideoRateIndependent: true,
		VideoRateMultiplier:  0.12,
	}
	card := &APIKey{KeyType: APIKeyTypeLink, LinkRateMultiplier: 0.10, GroupID: &groupID, Group: group}
	now := time.Date(2026, time.August, 7, 10, 0, 0, 0, timezone.Location())

	currentRate := 0.15
	textRate, imageRate := computePeakAwareMultipliers(card, currentRate, now)
	require.InDelta(t, 0.225, textRate, 1e-12)
	require.InDelta(t, 0.11, imageRate, 1e-12)
	require.InDelta(t, 0.12, resolveVideoRateMultiplier(card, currentRate), 1e-12)

	rateRepo := &openAIUserGroupRateRepoStub{rate: &currentRate}
	openAI := &OpenAIGatewayService{
		cfg:                   &config.Config{},
		userGroupRateResolver: newUserGroupRateResolver(rateRepo, nil, time.Minute, nil, "link-card-rate-test"),
	}
	require.InDelta(t, 0.11, openAI.resolveOpenAIMediaRateMultiplier(context.Background(), card, 1, false), 1e-12)
	require.InDelta(t, 0.12, openAI.resolveOpenAIMediaRateMultiplier(context.Background(), card, 1, true), 1e-12)

	group.ImageRateIndependent = false
	group.VideoRateIndependent = false
	require.InDelta(t, currentRate, openAI.resolveOpenAIMediaRateMultiplier(context.Background(), card, 1, false), 1e-12)
	require.InDelta(t, currentRate, openAI.resolveOpenAIMediaRateMultiplier(context.Background(), card, 1, true), 1e-12)
}

func TestLinkCardQuotaKeepsIssuanceValueAndAppliesCurrentRateRatio(t *testing.T) {
	card := &LinkCard{
		IssueRateMultiplier: 0.10,
		TotalDepositAmount:  decimal.NewFromInt(10),
	}
	card.SetFinancialState(decimal.RequireFromString("0.15"), decimal.Zero)
	card.NormalizeDerivedFields()

	require.True(t, card.IssuedQuota.Equal(decimal.NewFromInt(100)))
	require.True(t, card.UsedQuota.Equal(decimal.RequireFromString("1.5")))
	require.True(t, card.RemainingQuota.Equal(decimal.RequireFromString("98.5")))
}

func TestLinkCardQuotaProjectsDebtWithoutNegativeAvailableQuota(t *testing.T) {
	card := &LinkCard{
		IssueRateMultiplier: 0.10,
		TotalDepositAmount:  decimal.NewFromInt(10),
		Status:              LinkCardStateDepleted,
	}
	card.SetFinancialState(decimal.RequireFromString("10.5"), decimal.Zero)
	card.NormalizeDerivedFields()

	require.True(t, card.IssuedQuota.Equal(decimal.NewFromInt(100)))
	require.True(t, card.UsedQuota.Equal(decimal.NewFromInt(105)))
	require.True(t, card.RemainingQuota.IsZero())
}

func TestLinkCardExternalUsageUsesCurrentToIssuanceRateRatio(t *testing.T) {
	items := []LinkCardUsageLog{{
		TotalCost:      decimal.NewFromInt(1),
		ActualCost:     decimal.RequireFromString("0.15"),
		RateMultiplier: 0.15,
	}}
	items[0].SetIssueRateMultiplier(0.10)

	normalizeLinkCardExternalUsage(items)

	require.True(t, items[0].TotalCost.Equal(decimal.NewFromInt(1)))
	require.True(t, items[0].ActualCost.Equal(decimal.RequireFromString("1.5")))
	require.InDelta(t, 1.5, items[0].RateMultiplier, 1e-12)
	public := publicLinkCardUsage(items[0])
	require.True(t, public.ActualCost.Equal(decimal.RequireFromString("1.5")))
	require.InDelta(t, 1.5, public.RateMultiplier, 1e-12)
}

func TestLinkCardExternalUsageUsesRoundedUpQuotaRate(t *testing.T) {
	items := []LinkCardUsageLog{{
		TotalCost:      decimal.NewFromInt(1),
		ActualCost:     decimal.RequireFromString("0.084"),
		RateMultiplier: 0.084,
	}}
	items[0].SetIssueRateMultiplier(0.07)

	normalizeLinkCardExternalUsage(items)

	require.True(t, items[0].ActualCost.Equal(decimal.RequireFromString("1.2")))
	require.InDelta(t, 1.2, items[0].RateMultiplier, 1e-12)
}

func TestLinkCardRecordUsageResolvesCurrentUserGroupRate(t *testing.T) {
	groupID := int64(8)
	issuedRate := 0.07
	currentRate := 0.08
	chargeRate := 0.084
	usage := OpenAIUsage{InputTokens: 1000, OutputTokens: 200}
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	rateRepo := &openAIUserGroupRateRepoStub{rate: &currentRate}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
		usageRepo,
		billingRepo,
		&openAIRecordUsageUserRepoStub{},
		&openAIRecordUsageSubRepoStub{},
		rateRepo,
	)

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID: "link-current-rate",
			Usage:     usage,
			Model:     "gpt-5.1",
			Duration:  time.Second,
		},
		APIKey: &APIKey{
			ID:                 91,
			UserID:             1,
			KeyType:            APIKeyTypeLink,
			LinkState:          LinkCardStateActive,
			LinkRateMultiplier: issuedRate,
			Quota:              10,
			GroupID:            &groupID,
			Group:              &Group{ID: groupID, RateMultiplier: issuedRate},
		},
		User:    &User{ID: 1},
		Account: &Account{ID: 17},
	})

	require.NoError(t, err)
	require.Equal(t, 1, rateRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, chargeRate, usageRepo.lastLog.RateMultiplier, 1e-12)
	expected := expectedOpenAICost(t, svc, "gpt-5.1", usage, chargeRate)
	require.NotNil(t, billingRepo.lastCmd)
	require.InDelta(t, expected.ActualCost, billingRepo.lastCmd.PrepaidLinkCost, 1e-12)
	require.Zero(t, billingRepo.lastCmd.BalanceCost)
}

func TestLinkCardOpenAIUsagePathsUseRoundedQuotaRate(t *testing.T) {
	const (
		issueRate   = 0.07
		currentRate = 0.08
		chargeRate  = 0.084 // ceil((0.08 / 0.07) * 10) / 10 * 0.07
	)
	groupID := int64(8)
	searchPrice := 1.0
	newKey := func() *APIKey {
		return &APIKey{
			ID:                 91,
			UserID:             1,
			KeyType:            APIKeyTypeLink,
			LinkState:          LinkCardStateActive,
			LinkRateMultiplier: issueRate,
			GroupID:            &groupID,
			Group: &Group{ID: groupID, Platform: PlatformOpenAI, RateMultiplier: issueRate,
				WebSearchPricePerCall: &searchPrice},
		}
	}

	cases := []struct {
		name       string
		result     *OpenAIForwardResult
		snapshot   *MediaGenerationPricingSnapshot
		nativeCost float64
	}{
		{
			name: "streaming text",
			result: &OpenAIForwardResult{RequestID: "link-stream", Model: "gpt-5.1", Stream: true,
				Usage: OpenAIUsage{InputTokens: 1000, OutputTokens: 200}},
		},
		{
			name:       "responses web search",
			result:     &OpenAIForwardResult{RequestID: "link-search", Model: "gpt-5.1", WebSearchCalls: 1},
			nativeCost: searchPrice * currentRate,
		},
		{
			name:       "image snapshot",
			result:     &OpenAIForwardResult{RequestID: "link-image", Model: "gpt-image-1", ImageCount: 1, MediaType: "image"},
			snapshot:   &MediaGenerationPricingSnapshot{Mode: BillingModeImage, UnitPrice: 1, RateMultiplier: chargeRate},
			nativeCost: 1 * currentRate,
		},
		{
			name:       "video snapshot",
			result:     &OpenAIForwardResult{RequestID: "link-video", Model: "sora-2", VideoCount: 1, MediaType: "video"},
			snapshot:   &MediaGenerationPricingSnapshot{Mode: BillingModePerRequest, UnitPrice: 1, RateMultiplier: chargeRate},
			nativeCost: 1 * currentRate,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
			rate := currentRate
			rateRepo := &openAIUserGroupRateRepoStub{rate: &rate}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(
				usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, rateRepo)
			result := *tc.result
			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result:               &result,
				APIKey:               newKey(),
				User:                 &User{ID: 1},
				Account:              &Account{ID: 17},
				MediaPricingSnapshot: tc.snapshot,
			})
			require.NoError(t, err)
			require.NotNil(t, billingRepo.lastCmd)
			nativeCost := tc.nativeCost
			if nativeCost == 0 {
				nativeCost = usageRepo.lastLog.TotalCost * currentRate
			}
			require.GreaterOrEqual(t, billingRepo.lastCmd.PrepaidLinkCost+1e-12, nativeCost,
				"link settlement must not be below the current native cost")
			if tc.name == "streaming text" {
				require.InDelta(t, chargeRate, usageRepo.lastLog.RateMultiplier, 1e-12)
			}
		})
	}
}

func TestLinkCardGatewayTextAndStreamUseRoundedQuotaRate(t *testing.T) {
	const (
		issueRate   = 0.07
		currentRate = 0.08
		chargeRate  = 0.084
	)
	for _, stream := range []bool{false, true} {
		name := "sync"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Default.RateMultiplier = 1
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
			rate := currentRate
			rateRepo := &openAIUserGroupRateRepoStub{rate: &rate}
			svc := &GatewayService{
				usageLogRepo:          usageRepo,
				usageBillingRepo:      billingRepo,
				cfg:                   cfg,
				billingService:        NewBillingService(cfg, nil),
				billingCacheService:   &BillingCacheService{},
				deferredService:       &DeferredService{},
				userGroupRateResolver: newUserGroupRateResolver(rateRepo, nil, time.Minute, nil, "link-card-gateway-rate-test"),
			}
			groupID := int64(8)
			err := svc.RecordUsage(context.Background(), &RecordUsageInput{
				Result: &ForwardResult{
					RequestID: "link-gateway-" + name,
					Model:     "claude-sonnet-4",
					Stream:    stream,
					Usage:     ClaudeUsage{InputTokens: 1000, OutputTokens: 200},
				},
				APIKey: &APIKey{ID: 91, UserID: 1, KeyType: APIKeyTypeLink, LinkState: LinkCardStateActive,
					LinkRateMultiplier: issueRate, GroupID: &groupID,
					Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: issueRate}},
				User:    &User{ID: 1},
				Account: &Account{ID: 17},
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.NotNil(t, billingRepo.lastCmd)
			require.Equal(t, stream, usageRepo.lastLog.Stream)
			require.InDelta(t, chargeRate, usageRepo.lastLog.RateMultiplier, 1e-12)
			require.InDelta(t, usageRepo.lastLog.TotalCost*chargeRate, billingRepo.lastCmd.PrepaidLinkCost, 1e-12)
			require.GreaterOrEqual(t, billingRepo.lastCmd.PrepaidLinkCost+1e-12,
				usageRepo.lastLog.TotalCost*currentRate)
			require.Zero(t, billingRepo.lastCmd.BalanceCost)
		})
	}
}

func TestStandardGatewayRateIsUnchangedByLinkConversion(t *testing.T) {
	const currentRate = 0.08
	cfg := &config.Config{}
	cfg.Default.RateMultiplier = 1
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	rate := currentRate
	rateRepo := &openAIUserGroupRateRepoStub{rate: &rate}
	svc := &GatewayService{
		usageLogRepo:          usageRepo,
		usageBillingRepo:      billingRepo,
		cfg:                   cfg,
		billingService:        NewBillingService(cfg, nil),
		billingCacheService:   &BillingCacheService{},
		deferredService:       &DeferredService{},
		userGroupRateResolver: newUserGroupRateResolver(rateRepo, nil, time.Minute, nil, "standard-gateway-rate-test"),
	}
	groupID := int64(8)
	err := svc.RecordUsage(context.Background(), &RecordUsageInput{
		Result: &ForwardResult{RequestID: "standard-gateway-rate", Model: "claude-sonnet-4",
			Usage: ClaudeUsage{InputTokens: 1000, OutputTokens: 200}},
		APIKey: &APIKey{ID: 92, UserID: 1, KeyType: APIKeyTypeStandard, GroupID: &groupID,
			Group: &Group{ID: groupID, Platform: PlatformAnthropic, RateMultiplier: 0.07}},
		User:    &User{ID: 1},
		Account: &Account{ID: 17},
	})
	require.NoError(t, err)
	require.InDelta(t, currentRate, usageRepo.lastLog.RateMultiplier, 1e-12)
	require.Zero(t, billingRepo.lastCmd.PrepaidLinkCost)
	require.InDelta(t, usageRepo.lastLog.TotalCost*currentRate, billingRepo.lastCmd.BalanceCost, 1e-12)
}

func TestLinkCardBatchUsageLogPreservesRawTotalForQuotaProjection(t *testing.T) {
	apiKeyID, accountID := int64(91), int64(17)
	holdID := LinkCardBatchImageHoldID("batch-rate-audit")
	const chargeRate = 0.084
	job := &BatchImageJob{
		BatchID:                 "batch-rate-audit",
		UserID:                  1,
		APIKeyID:                &apiKeyID,
		AccountID:               &accountID,
		Model:                   "image-model",
		SuccessCount:            1,
		GroupRateMultiplier:     chargeRate,
		BatchDiscountMultiplier: 1,
		AccountRateMultiplier:   1,
		HoldID:                  &holdID,
	}
	usageRepo := &openAIRecordUsageLogRepoStub{}
	settlement := &BatchImageSettlementService{UsageLogRepo: usageRepo}
	settlement.recordUsageLog(context.Background(), job, chargeRate, "batch-rate-audit-request", time.Now())

	require.NotNil(t, usageRepo.lastLog)
	require.InDelta(t, 1, usageRepo.lastLog.TotalCost, 1e-12)
	require.InDelta(t, chargeRate, usageRepo.lastLog.ActualCost, 1e-12)
	require.InDelta(t, chargeRate, usageRepo.lastLog.RateMultiplier, 1e-12)
	item := LinkCardUsageLog{
		TotalCost:      decimal.NewFromFloat(usageRepo.lastLog.TotalCost),
		ActualCost:     decimal.NewFromFloat(usageRepo.lastLog.ActualCost),
		RateMultiplier: usageRepo.lastLog.RateMultiplier,
	}
	item.SetIssueRateMultiplier(0.07)
	items := []LinkCardUsageLog{item}
	normalizeLinkCardExternalUsage(items)
	require.True(t, items[0].ActualCost.Equal(decimal.RequireFromString("1.2")))
	require.InDelta(t, 1.2, items[0].RateMultiplier, 1e-12)
}

func TestStandardUsageBillingFingerprintRemainsRollingUpgradeCompatible(t *testing.T) {
	subscriptionID := int64(17)
	cmd := &UsageBillingCommand{
		UserID: 1, AccountID: 2, APIKeyID: 3, AccountType: "oauth", Model: "MODEL",
		ServiceTier: "default", ReasoningEffort: "medium", BillingType: 4,
		InputTokens: 5, OutputTokens: 6, CacheCreationTokens: 7, CacheReadTokens: 8,
		ImageCount: 9, MediaType: "image", SubscriptionID: &subscriptionID,
		BalanceCost: 0.1, SubscriptionCost: 0.2, APIKeyQuotaCost: 0.3,
		APIKeyRateLimitCost: 0.4, AccountQuotaCost: 0.5,
		RequestPayloadHash: "payload-hash",
	}

	require.Equal(t, legacyUsageBillingFingerprint(cmd), buildUsageBillingFingerprint(cmd))

	link := *cmd
	link.PrepaidLinkCost = 0.08
	require.NotEqual(t, legacyUsageBillingFingerprint(&link), buildUsageBillingFingerprint(&link))
	require.NotEqual(t, buildUsageBillingFingerprint(cmd), buildUsageBillingFingerprint(&link))
}

func legacyUsageBillingFingerprint(c *UsageBillingCommand) string {
	raw := fmt.Sprintf(
		"%d|%d|%d|%s|%s|%s|%s|%d|%d|%d|%d|%d|%d|%s|%d|%0.10f|%0.10f|%0.10f|%0.10f|%0.10f",
		c.UserID, c.AccountID, c.APIKeyID,
		strings.TrimSpace(c.AccountType), strings.TrimSpace(c.Model),
		strings.TrimSpace(c.ServiceTier), strings.TrimSpace(c.ReasoningEffort),
		c.BillingType, c.InputTokens, c.OutputTokens, c.CacheCreationTokens,
		c.CacheReadTokens, c.ImageCount, strings.TrimSpace(c.MediaType),
		valueOrZero(c.SubscriptionID), c.BalanceCost, c.SubscriptionCost,
		c.APIKeyQuotaCost, c.APIKeyRateLimitCost, c.AccountQuotaCost,
	)
	if holdRequestID := strings.TrimSpace(c.MediaBalanceHoldRequestID); holdRequestID != "" {
		raw += "|hold=" + holdRequestID
		raw += fmt.Sprintf("|hold_actual=%0.10f", c.MediaBalanceHoldActualCost)
	}
	if payloadHash := strings.TrimSpace(c.RequestPayloadHash); payloadHash != "" {
		raw += "|" + payloadHash
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func TestApplyUsageBillingLinkCardFailsClosedWithoutAtomicRepository(t *testing.T) {
	applied, err := applyUsageBilling(context.Background(), "link-request", nil, &postUsageBillingParams{
		Cost:    &CostBreakdown{TotalCost: 10, ActualCost: 0.8},
		User:    &User{ID: 1},
		APIKey:  &APIKey{ID: 9, UserID: 1, KeyType: APIKeyTypeLink, Quota: 100},
		Account: &Account{ID: 17},
	}, &billingDeps{}, nil)

	require.Error(t, err)
	require.False(t, applied)
}

func TestBillingEligibilityUsesLinkReserveInsteadOfCreatorBalance(t *testing.T) {
	loader := &linkCardBillingStateLoaderStub{state: &LinkCardBillingState{
		Status:    StatusAPIKeyActive,
		LinkState: LinkCardStateActive,
		Quota:     100,
		QuotaUsed: 25,
	}}
	svc := &BillingCacheService{
		cfg:                   &config.Config{},
		apiKeyRateLimitLoader: loader,
	}
	active := &APIKey{
		ID:        9,
		KeyType:   APIKeyTypeLink,
		LinkState: LinkCardStateActive,
		Quota:     100,
		QuotaUsed: 25,
	}

	err := svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 1, Balance: 0},
		active,
		nil,
		nil,
		"",
	)
	require.NoError(t, err)

	depleted := *active
	depleted.LinkState = LinkCardStateDepleted
	depleted.QuotaUsed = depleted.Quota
	loader.state = &LinkCardBillingState{
		Status:    StatusAPIKeyQuotaExhausted,
		LinkState: LinkCardStateDepleted,
		Quota:     depleted.Quota,
		QuotaUsed: depleted.QuotaUsed,
	}
	require.ErrorIs(t, svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 1, Balance: 0},
		&depleted,
		nil,
		nil,
		"",
	), ErrLinkCardPrepaidExhausted)

	// The quota check independently rejects debt even if a stale status writer
	// left the state marked active.  This is the final guard before forwarding.
	loader.state = &LinkCardBillingState{
		Status:    StatusAPIKeyActive,
		LinkState: LinkCardStateActive,
		Quota:     100,
		QuotaUsed: 100.25,
	}
	require.ErrorIs(t, svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 1, Balance: 0},
		active,
		nil,
		nil,
		"",
	), ErrLinkCardPrepaidExhausted)
}

func TestLinkCardEligibilityFailsClosedWithoutAuthoritativeStateLoader(t *testing.T) {
	svc := &BillingCacheService{cfg: &config.Config{}}
	err := svc.CheckBillingEligibility(
		context.Background(),
		&User{ID: 1, Balance: 100},
		&APIKey{ID: 9, KeyType: APIKeyTypeLink, LinkState: LinkCardStateActive, Quota: 100, QuotaUsed: 0},
		nil,
		nil,
		"",
	)
	require.ErrorIs(t, err, ErrBillingServiceUnavailable)
}

func TestLinkCardMediaUsesCardHoldWithoutFreezingCreatorBalance(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	user := &User{ID: 1, Balance: 0}

	require.True(t, svc.MediaGenerationBalanceHoldRequired(&APIKey{
		ID:      9,
		KeyType: APIKeyTypeLink,
		User:    user,
	}, nil), "link media must reserve the card quota, not bypass admission")

	require.True(t, svc.MediaGenerationBalanceHoldRequired(&APIKey{
		ID:      10,
		KeyType: APIKeyTypeStandard,
		User:    user,
	}, nil), "standard balance-funded media behavior must remain unchanged")
}
