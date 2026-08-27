package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceRecordUsage_GPT56OfficialLongContextAcrossResponsesTransports(t *testing.T) {
	const (
		groupRateMultiplier = 0.12
		totalInputTokens    = 297713
		cacheWriteTokens    = 200
		cacheReadTokens     = 281344
		outputTokens        = 214
	)

	tests := []struct {
		name         string
		openAIWSMode bool
	}{
		{name: "http responses"},
		{name: "websocket http bridge", openAIWSMode: true},
		{name: "websocket v2", openAIWSMode: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			userRepo := &openAIRecordUsageUserRepoStub{}
			svc := newOpenAIRecordUsageServiceForTest(
				usageRepo,
				userRepo,
				&openAIRecordUsageSubRepoStub{},
				nil,
			)
			svc.resolver = NewModelPricingResolver(nil, svc.billingService)

			groupID := int64(5600 + i)
			apiKey := &APIKey{
				ID:      int64(15600 + i),
				GroupID: &groupID,
				Group: &Group{
					ID:                        groupID,
					Platform:                  PlatformOpenAI,
					RateMultiplier:            groupRateMultiplier,
					LongContextPricingEnabled: false,
				},
			}
			account := &Account{
				ID:       int64(25600 + i),
				Platform: PlatformOpenAI,
				Extra: map[string]any{
					openAILongContextBillingEnabledKey: true,
				},
			}

			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID: fmt.Sprintf("resp_gpt56_long_context_%d", i),
					Usage: OpenAIUsage{
						InputTokens:              totalInputTokens,
						OutputTokens:             outputTokens,
						CacheCreationInputTokens: cacheWriteTokens,
						CacheReadInputTokens:     cacheReadTokens,
					},
					Model:        "gpt-5.6-sol",
					OpenAIWSMode: tt.openAIWSMode,
					Duration:     time.Second,
				},
				APIKey:  apiKey,
				User:    &User{ID: int64(35600 + i)},
				Account: account,
			})
			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)

			uncachedInputTokens := totalInputTokens - cacheWriteTokens - cacheReadTokens
			expectedInputCost := float64(uncachedInputTokens) * 5e-6 * 2
			expectedCacheWriteCost := float64(cacheWriteTokens) * 6.25e-6 * 2
			expectedCacheReadCost := float64(cacheReadTokens) * 0.5e-6 * 2
			expectedOutputCost := float64(outputTokens) * 30e-6 * 1.5
			expectedTotalCost := expectedInputCost + expectedCacheWriteCost + expectedCacheReadCost + expectedOutputCost

			require.True(t, usageRepo.lastLog.LongContextBillingApplied)
			require.InDelta(t, expectedInputCost, usageRepo.lastLog.InputCost, 1e-12)
			require.InDelta(t, expectedCacheWriteCost, usageRepo.lastLog.CacheCreationCost, 1e-12)
			require.InDelta(t, expectedCacheReadCost, usageRepo.lastLog.CacheReadCost, 1e-12)
			require.InDelta(t, expectedOutputCost, usageRepo.lastLog.OutputCost, 1e-12)
			require.InDelta(t, expectedTotalCost, usageRepo.lastLog.TotalCost, 1e-12)
			require.InDelta(t, expectedTotalCost*groupRateMultiplier, usageRepo.lastLog.ActualCost, 1e-12)
			require.Equal(t, 1, userRepo.deductCalls)
			require.InDelta(t, expectedTotalCost*groupRateMultiplier, userRepo.lastAmount, 1e-12)
		})
	}
}
