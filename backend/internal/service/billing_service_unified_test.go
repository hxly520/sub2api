//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CalculateCostUnified
// ---------------------------------------------------------------------------

func TestCalculateCostUnified_NilResolver_FallsBackToOldPath(t *testing.T) {
	svc := newTestBillingService()

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500}
	input := CostInput{
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 1.0,
		Resolver:       nil, // no resolver
	}
	cost, err := svc.CalculateCostUnified(input)
	require.NoError(t, err)

	// Should match the old-path result exactly
	expected, err := svc.calculateCostInternal("claude-sonnet-4", tokens, 1.0, "", nil)
	require.NoError(t, err)
	require.InDelta(t, expected.TotalCost, cost.TotalCost, 1e-10)
	require.InDelta(t, expected.ActualCost, cost.ActualCost, 1e-10)
	// BillingMode is NOT set by old path through CalculateCostUnified (resolver == nil)
	require.Empty(t, cost.BillingMode)
}

func TestCalculateCostUnified_TokenMode(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500}
	input := CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 1.5,
		Resolver:       resolver,
	}
	cost, err := bs.CalculateCostUnified(input)
	require.NoError(t, err)
	require.NotNil(t, cost)

	// Verify token billing: Input: 1000*3e-6=0.003, Output: 500*15e-6=0.0075
	expectedTotal := 1000*3e-6 + 500*15e-6
	require.InDelta(t, expectedTotal, cost.TotalCost, 1e-10)
	require.InDelta(t, expectedTotal*1.5, cost.ActualCost, 1e-10)
	require.Equal(t, string(BillingModeToken), cost.BillingMode)
}

func TestCalculateCostUnified_TokenModeAppliesRateMultiplierToImageTokens(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 600, ImageOutputTokens: 100}
	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 3.0,
		Resolver:       resolver,
	})
	require.NoError(t, err)

	textInput := 1000 * 3e-6
	textOutput := 500 * 15e-6
	imageOutput := 100 * 15e-6
	require.InDelta(t, textInput+textOutput+imageOutput, cost.TotalCost, 1e-10)
	require.InDelta(t, (textInput+textOutput+imageOutput)*3.0, cost.ActualCost, 1e-10)
	require.InDelta(t, imageOutput, cost.ImageOutputCost, 1e-10)
}

func TestCalculateCostUnified_GPT56ChannelFlatAppliesLongContextTier(t *testing.T) {
	tests := []struct {
		model                                string
		input, output, cacheWrite, cacheRead float64
	}{
		{model: "gpt-5.6-sol", input: 5e-6, output: 30e-6, cacheWrite: 6.25e-6, cacheRead: 0.5e-6},
		{model: "gpt-5.6-terra", input: 2.5e-6, output: 15e-6, cacheWrite: 3.125e-6, cacheRead: 0.25e-6},
		{model: "gpt-5.6-luna", input: 1e-6, output: 6e-6, cacheWrite: 1.25e-6, cacheRead: 0.1e-6},
	}
	tokens := UsageTokens{
		InputTokens:         100000,
		CacheCreationTokens: 100000,
		CacheReadTokens:     73000,
		OutputTokens:        10,
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			resolver := newResolverWithChannelPlatform(t, "openai", []ChannelModelPricing{{
				Platform:        "openai",
				Models:          []string{tt.model},
				BillingMode:     BillingModeToken,
				InputPrice:      testPtrFloat64(tt.input),
				OutputPrice:     testPtrFloat64(tt.output),
				CacheWritePrice: testPtrFloat64(tt.cacheWrite),
				CacheReadPrice:  testPtrFloat64(tt.cacheRead),
			}})
			groupID := int64(100)

			cost, err := resolver.billingService.CalculateCostUnified(CostInput{
				Ctx:            context.Background(),
				Model:          tt.model,
				GroupID:        &groupID,
				Tokens:         tokens,
				RateMultiplier: 1,
				Resolver:       resolver,
			})
			require.NoError(t, err)

			require.InDelta(t, float64(tokens.InputTokens)*tt.input*2, cost.InputCost, 1e-12)
			require.InDelta(t, float64(tokens.CacheCreationTokens)*tt.cacheWrite*2, cost.CacheCreationCost, 1e-12)
			require.InDelta(t, float64(tokens.CacheReadTokens)*tt.cacheRead*2, cost.CacheReadCost, 1e-12)
			require.InDelta(t, float64(tokens.OutputTokens)*tt.output*1.5, cost.OutputCost, 1e-12)
		})
	}
}

func TestCalculateCostUnified_GPT56ChannelIntervalsDoNotStackLongContextTier(t *testing.T) {
	resolver := newResolverWithChannelPlatform(t, "openai", []ChannelModelPricing{{
		Platform:    "openai",
		Models:      []string{"gpt-5.6-sol"},
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{
				MinTokens:       0,
				MaxTokens:       testPtrInt(272000),
				InputPrice:      testPtrFloat64(5e-6),
				OutputPrice:     testPtrFloat64(30e-6),
				CacheWritePrice: testPtrFloat64(6.25e-6),
				CacheReadPrice:  testPtrFloat64(0.5e-6),
			},
			{
				MinTokens:       272000,
				InputPrice:      testPtrFloat64(10e-6),
				OutputPrice:     testPtrFloat64(45e-6),
				CacheWritePrice: testPtrFloat64(12.5e-6),
				CacheReadPrice:  testPtrFloat64(1e-6),
			},
		},
	}})
	groupID := int64(100)
	tokens := UsageTokens{
		InputTokens:         100000,
		CacheCreationTokens: 100000,
		CacheReadTokens:     73000,
		OutputTokens:        10,
	}

	cost, err := resolver.billingService.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "gpt-5.6-sol",
		GroupID:        &groupID,
		Tokens:         tokens,
		RateMultiplier: 1,
		Resolver:       resolver,
	})
	require.NoError(t, err)

	// The selected channel interval already contains the >272K prices.
	// Model-specific long-context multipliers must not be applied a second time.
	require.InDelta(t, float64(tokens.InputTokens)*10e-6, cost.InputCost, 1e-12)
	require.InDelta(t, float64(tokens.CacheCreationTokens)*12.5e-6, cost.CacheCreationCost, 1e-12)
	require.InDelta(t, float64(tokens.CacheReadTokens)*1e-6, cost.CacheReadCost, 1e-12)
	require.InDelta(t, float64(tokens.OutputTokens)*45e-6, cost.OutputCost, 1e-12)
}

func TestCalculateCostUnified_ChannelIntervalsRespectGroupOrAccountToggle(t *testing.T) {
	resolver := newResolverWithChannelPlatform(t, "openai", []ChannelModelPricing{{
		Platform:    "openai",
		Models:      []string{"gpt-5.6-sol"},
		BillingMode: BillingModeToken,
		Intervals: []PricingInterval{
			{MinTokens: 0, MaxTokens: testPtrInt(272000), InputPrice: testPtrFloat64(5e-6), OutputPrice: testPtrFloat64(30e-6)},
			{MinTokens: 272000, InputPrice: testPtrFloat64(10e-6), OutputPrice: testPtrFloat64(45e-6)},
		},
	}})
	groupID := int64(100)
	tokens := UsageTokens{InputTokens: 300000, OutputTokens: 1000}

	groupOff := false
	groupOn := true
	accountOn := true
	accountOff := false

	// A group opt-in enables the higher interval even when the account switch
	// is explicitly off.
	groupEnabled, err := resolver.billingService.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-5.6-sol", GroupID: &groupID,
		Group: &Group{LongContextPricingEnabled: groupOn}, Tokens: tokens,
		RateMultiplier: 1, Resolver: resolver, LongContextBillingEnabled: &accountOff,
	})
	require.NoError(t, err)
	require.InDelta(t, 300000*10e-6, groupEnabled.InputCost, 1e-12)
	require.InDelta(t, 1000*45e-6, groupEnabled.OutputCost, 1e-12)

	// An account opt-in enables the higher interval even when the group switch
	// is off.
	accountEnabled, err := resolver.billingService.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-5.6-sol", GroupID: &groupID,
		Group: &Group{LongContextPricingEnabled: groupOff}, Tokens: tokens,
		RateMultiplier: 1, Resolver: resolver, LongContextBillingEnabled: &accountOn,
	})
	require.NoError(t, err)
	require.InDelta(t, 300000*10e-6, accountEnabled.InputCost, 1e-12)
	require.InDelta(t, 1000*45e-6, accountEnabled.OutputCost, 1e-12)

	// With both switches off, the interval list remains intact but the lowest
	// tier is selected.
	groupDisabled, err := resolver.billingService.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-5.6-sol", GroupID: &groupID,
		Group: &Group{LongContextPricingEnabled: groupOff}, Tokens: tokens,
		RateMultiplier: 1, Resolver: resolver, LongContextBillingEnabled: &accountOff,
	})
	require.NoError(t, err)
	require.InDelta(t, 300000*5e-6, groupDisabled.InputCost, 1e-12)
	require.InDelta(t, 1000*30e-6, groupDisabled.OutputCost, 1e-12)
}

func TestCalculateCostUnified_PerRequestMode(t *testing.T) {
	// Set up a ChannelService with a per-request pricing channel
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 1, model: "claude-sonnet-4"}: {
				BillingMode:     BillingModePerRequest,
				PerRequestPrice: testPtrFloat64(0.05),
			},
		},
		channelByGroupID: map[int64]*Channel{
			1: {ID: 1, Status: StatusActive},
		},
		groupPlatform:           map[int64]string{1: ""},
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel:     map[channelModelKey]string{},
		wildcardMappingByGP:     map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID:                    map[int64]*Channel{},
	})

	bs := newTestBillingService()
	resolver := NewModelPricingResolver(cs, bs)
	groupID := int64(1)

	input := CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		GroupID:        &groupID,
		Tokens:         UsageTokens{InputTokens: 100, OutputTokens: 50},
		RequestCount:   3,
		RateMultiplier: 2.0,
		Resolver:       resolver,
	}
	cost, err := bs.CalculateCostUnified(input)
	require.NoError(t, err)
	require.NotNil(t, cost)

	// 3 requests * $0.05 = $0.15
	require.InDelta(t, 0.15, cost.TotalCost, 1e-10)
	// ActualCost = 0.15 * 2.0 = 0.30
	require.InDelta(t, 0.30, cost.ActualCost, 1e-10)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
}

func TestCalculateCostUnified_ImageMode(t *testing.T) {
	cs := newTestChannelServiceWithCache(t, &channelCache{
		pricingByGroupModel: map[channelModelKey]*ChannelModelPricing{
			{groupID: 2, model: "gemini-image"}: {
				BillingMode:     BillingModeImage,
				PerRequestPrice: testPtrFloat64(0.10),
			},
		},
		channelByGroupID: map[int64]*Channel{
			2: {ID: 2, Status: StatusActive},
		},
		groupPlatform:           map[int64]string{2: ""},
		wildcardByGroupPlatform: map[channelGroupPlatformKey][]*wildcardPricingEntry{},
		mappingByGroupModel:     map[channelModelKey]string{},
		wildcardMappingByGP:     map[channelGroupPlatformKey][]*wildcardMappingEntry{},
		byID:                    map[int64]*Channel{},
	})

	bs := &BillingService{
		cfg:            &config.Config{},
		fallbackPrices: map[string]*ModelPricing{},
	}
	resolver := NewModelPricingResolver(cs, bs)
	groupID := int64(2)

	input := CostInput{
		Ctx:            context.Background(),
		Model:          "gemini-image",
		GroupID:        &groupID,
		Tokens:         UsageTokens{},
		RequestCount:   2,
		RateMultiplier: 1.0,
		Resolver:       resolver,
	}
	cost, err := bs.CalculateCostUnified(input)
	require.NoError(t, err)
	require.NotNil(t, cost)

	// 2 * $0.10 = $0.20
	require.InDelta(t, 0.20, cost.TotalCost, 1e-10)
	require.InDelta(t, 0.20, cost.ActualCost, 1e-10)
	require.Equal(t, string(BillingModeImage), cost.BillingMode)
}

// TestCalculateCostUnified_RateMultiplierZeroProducesZero 锁定新行为：
// 保存时强制 > 0；若 0 仍泄漏到计费层，按 0 计费（而非历史上的 1.0）。
func TestCalculateCostUnified_RateMultiplierZeroProducesZero(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500}

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: 0,
		Resolver:       resolver,
	})
	require.NoError(t, err)
	require.Greater(t, cost.TotalCost, 0.0)
	require.InDelta(t, 0.0, cost.ActualCost, 1e-10)
}

// TestCalculateCostUnified_NegativeRateMultiplierClampedToZero 锁定新行为：
// 负数倍率按 0 计费，避免历史的 <=0 → 1.0 把配置异常静默按标准价扣费。
func TestCalculateCostUnified_NegativeRateMultiplierClampedToZero(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	tokens := UsageTokens{InputTokens: 1000}

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         tokens,
		RateMultiplier: -5.0,
		Resolver:       resolver,
	})
	require.NoError(t, err)
	require.Greater(t, cost.TotalCost, 0.0)
	require.InDelta(t, 0.0, cost.ActualCost, 1e-10)
}

func TestCalculateCostUnified_BillingModeFieldFilled(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         UsageTokens{InputTokens: 100},
		RateMultiplier: 1.0,
		Resolver:       resolver,
	})
	require.NoError(t, err)
	require.Equal(t, "token", cost.BillingMode)
}

func TestCalculateCostUnified_UsesPreResolvedPricing(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)

	// Pre-resolve with per_request mode to verify it's used instead of re-resolving
	preResolved := &ResolvedPricing{
		Mode:                   BillingModePerRequest,
		DefaultPerRequestPrice: 0.07,
	}

	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx:            context.Background(),
		Model:          "claude-sonnet-4",
		Tokens:         UsageTokens{InputTokens: 100},
		RequestCount:   2,
		RateMultiplier: 1.0,
		Resolver:       resolver,
		Resolved:       preResolved,
	})
	require.NoError(t, err)
	require.NotNil(t, cost)

	// 2 * $0.07 = $0.14
	require.InDelta(t, 0.14, cost.TotalCost, 1e-10)
	require.Equal(t, string(BillingModePerRequest), cost.BillingMode)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestChannelServiceWithCache creates a ChannelService with a pre-populated
// cache snapshot, bypassing the repository layer entirely.
func newTestChannelServiceWithCache(t *testing.T, cache *channelCache) *ChannelService {
	t.Helper()
	cs := &ChannelService{}
	cache.loadedAt = time.Now()
	cs.cache.Store(cache)
	return cs
}
