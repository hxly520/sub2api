package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotGroupPricingRoundtrip(t *testing.T) {
	groupID := int64(50)
	inputPrice := 1e-6
	outputPrice := 2e-6
	apiKey := &APIKey{
		ID: 82, UserID: 40, GroupID: &groupID, Key: "sk-pricing-roundtrip", Status: StatusActive,
		KeyType: APIKeyTypeLink, LinkState: LinkCardStateActive, LinkRateMultiplier: 0.08,
		User: &User{ID: 40, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "pricing-roundtrip", Platform: PlatformAnthropic, Status: StatusActive,
			LongContextPricingEnabled: true,
			ModelPricing: []ChannelModelPricing{{
				Models: []string{"claude-sonnet-*"}, BillingMode: BillingModeToken,
				InputPrice: &inputPrice, OutputPrice: &outputPrice,
			}},
		},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.Equal(t, APIKeyTypeLink, materialized.KeyType)
	require.Equal(t, LinkCardStateActive, materialized.LinkState)
	require.InDelta(t, 0.08, materialized.LinkRateMultiplier, 1e-12)
	require.True(t, materialized.Group.LongContextPricingEnabled)
	require.Equal(t, apiKey.Group.ModelPricing, materialized.Group.ModelPricing)

	billing := &BillingService{fallbackPrices: map[string]*ModelPricing{
		"claude-sonnet-4": {InputPricePerToken: 3e-6, OutputPricePerToken: 15e-6},
	}}
	resolver := NewModelPricingResolver(nil, billing)
	resolved := resolver.Resolve(context.Background(), PricingInput{Model: "claude-sonnet-4", Group: materialized.Group})
	require.Equal(t, PricingSourceGroup, resolved.Source)
	require.True(t, resolved.longContextPricingEnabled)
	require.InDelta(t, inputPrice, resolved.BasePricing.InputPricePerToken, 1e-12)
	require.InDelta(t, outputPrice, resolved.BasePricing.OutputPricePerToken, 1e-12)
}

func TestAPIKeyAuthSnapshotV20IsEvictedForPricingRefresh(t *testing.T) {
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), profitAuthTestAPIKey())
	require.NotNil(t, snapshot)
	snapshot.Version = 20

	materialized, used, err := svc.applyAuthCacheEntry("sk-v20-pricing", &APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	require.False(t, used, "v20 omitted group pricing and must be refreshed from the database")
	require.Nil(t, materialized)
}

func TestAPIKeyAuthSnapshotGPT56LongContextBillingRoundtrip(t *testing.T) {
	groupID := int64(20)
	apiKey := &APIKey{
		ID: 15, UserID: 1, GroupID: &groupID, Key: "sk-long-context-roundtrip", Status: StatusActive,
		User: &User{ID: 1, Status: StatusActive},
		Group: &Group{
			ID: groupID, Name: "long-context-roundtrip", Platform: PlatformOpenAI, Status: StatusActive,
			RateMultiplier: 0.12, LongContextPricingEnabled: true,
		},
	}
	svc := &APIKeyService{}

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: svc.snapshotFromAPIKey(context.Background(), apiKey)})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.LongContextPricingEnabled)

	billing := &BillingService{fallbackPrices: map[string]*ModelPricing{
		"gpt-5.6-sol": {
			InputPricePerToken:          5e-6,
			OutputPricePerToken:         30e-6,
			CacheReadPricePerToken:      0.5e-6,
			LongContextInputThreshold:   272000,
			LongContextInputMultiplier:  2,
			LongContextOutputMultiplier: 1.5,
		},
	}}
	resolver := NewModelPricingResolver(nil, billing)
	accountDisabled := false
	cost, err := billing.CalculateCostUnified(CostInput{
		Ctx:                       context.Background(),
		Model:                     "gpt-5.6-sol",
		Group:                     materialized.Group,
		Tokens:                    UsageTokens{InputTokens: 16169, CacheReadTokens: 281344, OutputTokens: 214},
		RateMultiplier:            materialized.Group.RateMultiplier,
		Resolver:                  resolver,
		LongContextBillingEnabled: &accountDisabled,
	})
	require.NoError(t, err)
	require.True(t, cost.LongContextBillingApplied)
	require.InDelta(t, 0.16169, cost.InputCost, 1e-12)
	require.InDelta(t, 0.281344, cost.CacheReadCost, 1e-12)
	require.InDelta(t, 0.00963, cost.OutputCost, 1e-12)
	require.InDelta(t, 0.452664, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.05431968, cost.ActualCost, 1e-12, "group multiplier must be applied exactly once")
}
