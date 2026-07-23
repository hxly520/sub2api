//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAccountScheduleProfileKeepsGPT56TiersDistinct(t *testing.T) {
	sol := NewOpenAIAccountScheduleProfile("openai/gpt-5.6-sol-max", "/v1/responses", "/v1/responses")
	terra := NewOpenAIAccountScheduleProfile("gpt-5.6-terra", "/v1/responses", "/v1/responses")
	luna := NewOpenAIAccountScheduleProfile("gpt-5.6-luna-2026-07-09", "/v1/responses", "/v1/responses")

	require.Equal(t, "gpt-5.6-sol", sol.ModelFamily)
	require.Equal(t, "gpt-5.6-terra", terra.ModelFamily)
	require.Equal(t, "gpt-5.6-luna", luna.ModelFamily)
	require.NotEqual(t, sol.ModelFamily, terra.ModelFamily)
	require.NotEqual(t, terra.ModelFamily, luna.ModelFamily)
}

func TestOpenAIAccountScheduleProfileFromContextRequiresExplicitProfile(t *testing.T) {
	require.Equal(
		t,
		OpenAIAccountScheduleProfile{},
		openAIAccountScheduleProfileFromContext(context.Background(), "gpt-5.6-luna"),
	)

	ctx := WithOpenAIAccountScheduleProfile(
		context.Background(),
		NewOpenAIAccountScheduleProfile("", "/v1/responses", ""),
	)
	profile := openAIAccountScheduleProfileFromContext(ctx, "gpt-5.6-luna")
	require.Equal(t, "gpt-5.6-luna", profile.ModelFamily)
	require.Equal(t, "/v1/responses", profile.InboundEndpoint)
}

func TestOpenAIAccountRuntimeStatsRequiresWarmProfileBeforeUsingTTFT(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	profile := NewOpenAIAccountScheduleProfile("gpt-5.6-luna", "/v1/responses", "/v1/responses")
	ttft := 900

	stats.report(9001, true, &ttft, profile)
	_, _, hasTTFT := stats.snapshotForSchedule(9001, profile)
	require.False(t, hasTTFT)

	stats.report(9001, true, &ttft, profile)
	stats.report(9001, true, &ttft, profile)
	_, observed, hasTTFT := stats.snapshotForSchedule(9001, profile)
	require.True(t, hasTTFT)
	require.InDelta(t, 900, observed, 0.01)
	require.Equal(t, 1, stats.profileSize())
}

func TestOpenAIAccountRuntimeStatsStickyPerformanceRequiresSuccessfulTTFT(t *testing.T) {
	stats := newOpenAIAccountRuntimeStats()
	profile := NewOpenAIAccountScheduleProfile("gpt-5.6-sol", "/v1/responses", "/v1/responses")
	failedTTFT := 200
	for i := 0; i < int(openAIAccountProfileMinTTFTSamples); i++ {
		stats.report(9002, false, &failedTTFT, profile)
	}

	errorRate, _, hasTTFT := stats.snapshotForStickyPerformance(9002, profile)
	require.False(t, hasTTFT)
	require.Greater(t, errorRate, 0.0)

	successTTFT := 750
	for i := 0; i < int(openAIAccountProfileMinTTFTSamples); i++ {
		stats.report(9002, true, &successTTFT, profile)
	}

	_, observed, hasTTFT := stats.snapshotForStickyPerformance(9002, profile)
	require.True(t, hasTTFT)
	require.InDelta(t, successTTFT, observed, 1e-9)
}

func TestDefaultOpenAIAccountSchedulerProfilesDoNotBleedAcrossEndpoints(t *testing.T) {
	accounts := []*Account{
		{ID: 9101, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3, Priority: 1},
		{ID: 9102, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3, Priority: 1},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 1

	stats := newOpenAIAccountRuntimeStats()
	chatProfile := NewOpenAIAccountScheduleProfile("gpt-5.6-luna", "/v1/chat/completions", "/v1/responses")
	responsesProfile := NewOpenAIAccountScheduleProfile("gpt-5.6-luna", "/v1/responses", "/v1/responses")
	fast := 700
	slow := 9000
	for i := 0; i < int(openAIAccountProfileMinTTFTSamples); i++ {
		stats.report(9101, true, &fast, chatProfile)
		stats.report(9102, true, &slow, chatProfile)
		stats.report(9101, true, &slow, responsesProfile)
		stats.report(9102, true, &fast, responsesProfile)
	}

	scheduler := &defaultOpenAIAccountScheduler{
		service: &OpenAIGatewayService{cfg: cfg},
		stats:   stats,
	}
	loadMap := map[int64]*AccountLoadInfo{
		9101: {AccountID: 9101},
		9102: {AccountID: 9102},
	}
	chatPlan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{
		RequestedModel: "gpt-5.6-luna",
		Profile:        NewOpenAIAccountScheduleProfile("gpt-5.6-luna", "/v1/chat/completions", ""),
	}, accounts, loadMap)
	responsesPlan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{
		RequestedModel: "gpt-5.6-luna",
		Profile:        NewOpenAIAccountScheduleProfile("gpt-5.6-luna", "/v1/responses", ""),
	}, accounts, loadMap)

	require.Equal(t, int64(9101), chatPlan.selectionOrder[0].account.ID)
	require.Equal(t, int64(9102), responsesPlan.selectionOrder[0].account.ID)
}

func TestBuildOpenAIWeightedSelectionOrderKeepsClearlyBetterCandidateFirst(t *testing.T) {
	candidates := []openAIAccountCandidateScore{
		{account: &Account{ID: 9202}, loadInfo: &AccountLoadInfo{}, score: 4.0},
		{account: &Account{ID: 9201}, loadInfo: &AccountLoadInfo{}, score: 8.0},
		{account: &Account{ID: 9203}, loadInfo: &AccountLoadInfo{}, score: 3.0},
	}

	for i := 0; i < 50; i++ {
		order := buildOpenAIWeightedSelectionOrder(candidates, OpenAIAccountScheduleRequest{SessionHash: string(rune(i + 1))})
		require.Equal(t, int64(9201), order[0].account.ID)
	}
}

func TestDefaultOpenAIAccountSchedulerSlowPenaltyCanOverrideStickyBonus(t *testing.T) {
	accounts := []*Account{
		{ID: 9251, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3},
		{ID: 9252, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.7
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 0.8
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 0.5
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.SessionSticky = 3

	stats := newOpenAIAccountRuntimeStats()
	profile := NewOpenAIAccountScheduleProfile("gpt-5.6-luna", "/v1/responses", "/v1/responses")
	slow := openAIAccountSlowSuccessTTFTThresholdMs + 1
	fast := 800
	for i := 0; i < openAIAccountSlowSuccessConsecutiveThreshold; i++ {
		stats.report(accounts[0].ID, true, &slow, profile)
		stats.report(accounts[1].ID, true, &fast, profile)
	}

	scheduler := &defaultOpenAIAccountScheduler{
		service: &OpenAIGatewayService{cfg: cfg},
		stats:   stats,
	}
	plan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{
		RequestedModel:  "gpt-5.6-luna",
		SessionHash:     "sticky-slow-session",
		StickyWeighted:  true,
		StickyAccountID: accounts[0].ID,
		Profile:         profile,
	}, accounts, map[int64]*AccountLoadInfo{
		accounts[0].ID: {AccountID: accounts[0].ID},
		accounts[1].ID: {AccountID: accounts[1].ID},
	})

	require.Len(t, plan.selectionOrder, 2)
	require.Equal(t, accounts[1].ID, plan.selectionOrder[0].account.ID)
}

func TestDefaultOpenAIAccountSchedulerRecentFailurePenaltyPrefersHealthyPeer(t *testing.T) {
	accounts := []*Account{
		{ID: 9253, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3},
		{ID: 9254, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3},
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 0.2
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 0.8
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Queue = 0.8
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.ErrorRate = 1.5
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.TTFT = 2

	stats := newOpenAIAccountRuntimeStats()
	profile := NewOpenAIAccountScheduleProfile("gpt-5.6-sol", "/v1/responses", "/v1/responses")
	fast := 500
	slower := 3000
	for i := 0; i < int(openAIAccountProfileMinTTFTSamples); i++ {
		stats.report(accounts[0].ID, true, &fast, profile)
		stats.report(accounts[1].ID, true, &slower, profile)
	}
	stats.markRecentFailure(accounts[0].ID, time.Now())

	scheduler := &defaultOpenAIAccountScheduler{
		service: &OpenAIGatewayService{cfg: cfg},
		stats:   stats,
	}
	plan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{
		RequestedModel: "gpt-5.6-sol",
		SessionHash:    "failed-pool-session",
		Profile:        profile,
	}, accounts, map[int64]*AccountLoadInfo{
		accounts[0].ID: {AccountID: accounts[0].ID},
		accounts[1].ID: {AccountID: accounts[1].ID},
	})

	require.Len(t, plan.selectionOrder, 2)
	require.Equal(t, accounts[1].ID, plan.selectionOrder[0].account.ID)

	require.True(t, stats.recentFailurePenaltyActive(accounts[0].ID, time.Now()))
	require.False(t, stats.recentFailurePenaltyActive(
		accounts[0].ID,
		time.Now().Add(openAIAccountRecentFailurePenaltyDuration+time.Second),
	))
}

func TestDefaultOpenAIAccountSchedulerRecentFailurePenaltyKeepsOnlyCandidateAvailable(t *testing.T) {
	account := &Account{ID: 9255, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 3}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Load = 1

	stats := newOpenAIAccountRuntimeStats()
	stats.markRecentFailure(account.ID, time.Now())
	scheduler := &defaultOpenAIAccountScheduler{
		service: &OpenAIGatewayService{cfg: cfg},
		stats:   stats,
	}
	plan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), OpenAIAccountScheduleRequest{}, []*Account{account}, map[int64]*AccountLoadInfo{
		account.ID: {AccountID: account.ID},
	})

	require.Len(t, plan.selectionOrder, 1)
	require.Equal(t, account.ID, plan.selectionOrder[0].account.ID)
}

func TestOpenAIGatewayServiceFirstResponseTimeoutDoesNotCreateTTFTSample(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)

	stats := newOpenAIAccountRuntimeStats()
	svc := &OpenAIGatewayService{
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		openaiAccountStats: stats,
	}
	profile := NewOpenAIAccountScheduleProfile("gpt-5.6-luna", "/v1/responses", "/v1/responses")

	svc.ReportOpenAIAccountFirstResponseTimeout(9261, false, profile)
	_, _, hasTTFT := stats.snapshot(9261, profile)
	require.False(t, hasTTFT)
	require.Zero(t, stats.size(), "a neutral timeout must not create an account runtime sample")

	svc.ReportOpenAIAccountFirstResponseTimeout(9261, true, profile)
	errorRate, _, hasTTFT := stats.snapshot(9261, profile)
	require.False(t, hasTTFT, "a timeout is not a first-token observation")
	require.Greater(t, errorRate, 0.0)
}

func TestOpenAIGatewayServiceRuntimeBlockedSingleTemporaryCandidateFailsOpen(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	groupID := int64(9300)
	account := Account{
		ID: 9301, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{account}}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                &config.Config{},
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	svc.BlockAccountScheduling(&account, time.Now().Add(time.Minute), "stream_read_error")

	selection, _, err := svc.SelectAccountWithScheduler(
		context.Background(), &groupID, "", "", "gpt-5.6-luna", nil, OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, account.ID, selection.Account.ID)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(&account))
}

func TestOpenAIGatewayServiceRuntimeBlockedPermanentCandidateStaysBlocked(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	groupID := int64(9400)
	account := Account{
		ID: 9401, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
	}
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{accounts: []Account{account}}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                &config.Config{},
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	svc.BlockAccountScheduling(&account, time.Now().Add(time.Minute), "missing_refresh_token")

	selection, _, err := svc.SelectAccountWithScheduler(
		context.Background(), &groupID, "", "", "gpt-5.6-luna", nil, OpenAIUpstreamTransportAny, false,
	)

	require.Error(t, err)
	require.Nil(t, selection)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(&account))
}

func TestOpenAIGatewayServiceRuntimeBlockedCandidateDoesNotDisplaceHealthyAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
	groupID := int64(9500)
	blocked := Account{
		ID: 9501, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
	}
	healthy := Account{
		ID: 9502, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID},
	}
	svc := &OpenAIGatewayService{
		accountRepo: schedulerGroupAwareOpenAIAccountRepo{schedulerTestOpenAIAccountRepo{
			accounts: []Account{blocked, healthy},
		}},
		cache:              &schedulerTestGatewayCache{},
		cfg:                &config.Config{},
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	svc.BlockAccountScheduling(&blocked, time.Now().Add(time.Minute), "stream_read_error")

	selection, _, err := svc.SelectAccountWithScheduler(
		context.Background(), &groupID, "", "", "gpt-5.6-luna", nil, OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, healthy.ID, selection.Account.ID)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(&blocked))
}

func TestOpenAIGatewayServiceHasAlternativeExcludesUnavailableCandidates(t *testing.T) {
	groupID := int64(9600)
	accounts := []Account{
		{ID: 9601, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}},
		{ID: 9602, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusDisabled, Schedulable: true, GroupIDs: []int64{groupID}},
		{ID: 9603, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false, GroupIDs: []int64{groupID}},
		{ID: 9604, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID + 1}},
	}
	svc := &OpenAIGatewayService{
		accountRepo: groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: accounts}},
	}

	hasAlternative, err := svc.HasOpenAIAlternativeAccountForCapability(
		context.Background(), &groupID, "gpt-5.6-luna", 9601, nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
		false, PlatformOpenAI,
	)

	require.NoError(t, err)
	require.False(t, hasAlternative)
}

func TestOpenAIGatewayServiceHasAlternativeExcludesRuntimeBlocked(t *testing.T) {
	groupID := int64(9700)
	blocked := Account{ID: 9701, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	healthy := Account{ID: 9702, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	svc := &OpenAIGatewayService{
		accountRepo: groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{blocked, healthy}}},
	}
	svc.BlockAccountScheduling(&blocked, time.Now().Add(time.Minute), "stream_read_error")

	hasAlternative, err := svc.HasOpenAIAlternativeAccountForCapability(
		context.Background(), &groupID, "gpt-5.6-luna", healthy.ID, nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
		false, PlatformOpenAI,
	)

	require.NoError(t, err)
	require.False(t, hasAlternative)
}

func TestOpenAIGatewayServiceHasAlternativeExcludesModelRuntimeBlocked(t *testing.T) {
	groupID := int64(9750)
	selected := Account{ID: 9751, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	blocked := Account{ID: 9752, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	svc := &OpenAIGatewayService{
		accountRepo: groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{selected, blocked}}},
	}
	svc.recordOpenAIAccountModelTransientFailure(&blocked, "gpt-5.6-luna", time.Now())
	svc.recordOpenAIAccountModelTransientFailure(&blocked, "gpt-5.6-luna", time.Now())

	hasAlternative, err := svc.HasOpenAIAlternativeAccountForCapability(
		context.Background(), &groupID, "gpt-5.6-luna", selected.ID, nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
		false, PlatformOpenAI,
	)

	require.NoError(t, err)
	require.False(t, hasAlternative)
}

func TestOpenAIGatewayServiceHasAlternativeExcludesProxyStreamQuarantined(t *testing.T) {
	groupID := int64(9760)
	proxyID := int64(9769)
	selected := Account{ID: 9761, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	quarantined := Account{ID: 9762, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}, ProxyID: &proxyID}
	svc := &OpenAIGatewayService{
		accountRepo: groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{selected, quarantined}}},
		cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIProxyStreamCircuit: config.GatewayOpenAIProxyStreamCircuitConfig{
			FailureThreshold: 1,
			WindowSeconds:    60,
			TTLSeconds:       60,
		}}},
	}
	svc.recordOpenAIProxyStreamDisconnect(&quarantined, errors.New("unexpected EOF"), "req-test")

	hasAlternative, err := svc.HasOpenAIAlternativeAccountForCapability(
		context.Background(), &groupID, "gpt-5.6-luna", selected.ID, nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
		false, PlatformOpenAI,
	)

	require.NoError(t, err)
	require.False(t, hasAlternative)
}

func TestOpenAIGatewayServiceHasAlternativeFindsSameGroupCandidate(t *testing.T) {
	groupID := int64(9800)
	selected := Account{ID: 9801, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	alternative := Account{ID: 9802, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	svc := &OpenAIGatewayService{
		accountRepo: groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{selected, alternative}}},
	}

	hasAlternative, err := svc.HasOpenAIAlternativeAccountForCapability(
		context.Background(), &groupID, "gpt-5.6-luna", selected.ID, nil,
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
		false, PlatformOpenAI,
	)

	require.NoError(t, err)
	require.True(t, hasAlternative)
}

func TestOpenAIGatewayServiceHasAlternativeHonorsExcludedAccounts(t *testing.T) {
	groupID := int64(9900)
	selected := Account{ID: 9901, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	alternative := Account{ID: 9902, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID}}
	svc := &OpenAIGatewayService{
		accountRepo: groupAwareStubOpenAIAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{selected, alternative}}},
	}

	hasAlternative, err := svc.HasOpenAIAlternativeAccountForCapability(
		context.Background(), &groupID, "gpt-5.6-luna", selected.ID,
		map[int64]struct{}{alternative.ID: {}},
		OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions,
		false, PlatformOpenAI,
	)

	require.NoError(t, err)
	require.False(t, hasAlternative)
}
