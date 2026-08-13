package handler

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestOpenAIFirstResponseHandlerNeverEnablesTimeoutReplay(t *testing.T) {
	source, err := os.ReadFile("openai_first_response_failover.go")
	require.NoError(t, err)
	require.Contains(t, string(source), "WithOpenAIFirstResponseEarlyFlush(ctx)")
	require.NotContains(t, string(source), "WithOpenAIFirstResponseTimeout(")
}

func TestOpenAIPoolModeHandlersDoNotContainLocalSameAccountReplay(t *testing.T) {
	for _, file := range []string{
		"openai_gateway_handler.go",
		"openai_chat_completions.go",
		"openai_images.go",
		"openai_image_tasks.go",
		"openai_videos.go",
		"openai_embeddings.go",
		"grok_media.go",
	} {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile(file)
			require.NoError(t, err)
			require.NotContains(t, string(source), "pool_mode_same_account_retry")
			require.NotContains(t, string(source), "sameAccountRetryCount")
			require.NotContains(t, string(source), "sameAccountRetries")
		})
	}
}

func TestOpenAIMediaCreationHandlersDoNotUseAutomaticReplayBudget(t *testing.T) {
	for _, file := range []string{
		"openai_images.go",
		"openai_image_tasks.go",
		"openai_videos.go",
		"grok_media.go",
	} {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile(file)
			require.NoError(t, err)
			require.NotContains(t, string(source), "retryBudget.tryConsume")
			require.NotContains(t, string(source), "upstream_failover_switching")
		})
	}
}

func TestOpenAIMediaCompositeModelsKeepRoutingAndBillingAttributionSeparate(t *testing.T) {
	imageSource, err := os.ReadFile("openai_image_tasks.go")
	require.NoError(t, err)
	require.Contains(t, string(imageSource), "requestModel := clientRequestedModel(c, routingModel)")
	require.Regexp(t, `SelectAccountWithSchedulerForImages\([\s\S]*?sessionHash,\s+routingModel,`, string(imageSource))

	videoSource, err := os.ReadFile("openai_videos.go")
	require.NoError(t, err)
	require.Contains(t, string(videoSource), "requestModel := clientRequestedModel(c, routingModel)")
	require.Regexp(t, `SelectAccountWithSchedulerForCapability\([\s\S]*?sessionHash,\s+routingModel,`, string(videoSource))
	require.Regexp(t, `Model:\s+routingModel,\s+RequestedModel:\s+requestModel,`, string(videoSource))
}

func TestOpenAIPoolTextHandlersReleaseFailedStickySession(t *testing.T) {
	for _, file := range []string{
		"openai_gateway_handler.go",
		"openai_chat_completions.go",
	} {
		t.Run(file, func(t *testing.T) {
			source, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Contains(t, string(source), "releaseOpenAIFailedPoolStickySession")
		})
	}
	helperSource, err := os.ReadFile("openai_account_schedule_profile.go")
	require.NoError(t, err)
	require.Contains(t, string(helperSource), "ReportOpenAIAccountRecentFailure")
}

func TestOpenAIRequestRetryBudget_PoolModeAllowsOnlyBoundedExplicitRejectionFailover(t *testing.T) {
	budget := openAIRequestRetryBudget{}
	account := &service.Account{
		ID:       1,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode": true,
		},
	}
	err := &service.UpstreamFailoverError{StatusCode: http.StatusForbidden, RetryableOnSameAccount: true}

	require.True(t, budget.tryConsume(account, err))
	require.True(t, budget.tryConsume(account, err))
	require.False(t, budget.tryConsume(account, err))
	require.Equal(t, openAIMaxAutomaticReplayAttempts, budget.used)
}

func TestOpenAIRequestRetryBudget_DirectAccountUsesSameBoundedFailoverBudget(t *testing.T) {
	budget := openAIRequestRetryBudget{}
	account := &service.Account{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	err := &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}

	require.True(t, budget.tryConsume(account, err))
	require.True(t, budget.tryConsume(account, err))
	require.False(t, budget.tryConsume(account, err))
	require.Equal(t, openAIMaxAutomaticReplayAttempts, budget.used)
}

func TestOpenAIRequestRetryBudget_DoesNotReplayUncertainAttempt(t *testing.T) {
	account := &service.Account{ID: 3, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}

	for _, err := range []*service.UpstreamFailoverError{
		{StatusCode: http.StatusInternalServerError},
		{StatusCode: http.StatusGatewayTimeout, FirstResponseTimeout: true},
		{StatusCode: http.StatusBadGateway},
	} {
		budget := openAIRequestRetryBudget{}
		require.False(t, budget.tryConsume(account, err))
		require.Zero(t, budget.used)
	}
}

func TestOpenAIRequestRetryBudget_DisabledMediaReplayDoesNotConsumeBudget(t *testing.T) {
	budget := openAIRequestRetryBudget{}
	account := &service.Account{ID: 4, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	err := &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}

	require.False(t, budget.tryConsumeIfAllowed(false, account, err))
	require.Zero(t, budget.used)
	require.True(t, budget.tryConsumeIfAllowed(true, account, err))
	require.Equal(t, 1, budget.used)
}

func TestOpenAIRequestRetryBudget_ModelCapacityUsesBoundedExponentialBackoff(t *testing.T) {
	budget := openAIRequestRetryBudget{}
	account := &service.Account{ID: 5, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:        http.StatusServiceUnavailable,
		Reason:            service.GatewayFailureReason("openai_model_at_capacity"),
		NextAccountAction: service.NextAccountRetry,
	}

	require.True(t, budget.tryConsume(account, failoverErr))
	require.Equal(t, openAIModelCapacityRetryBaseDelay, budget.modelCapacityBackoff())
	require.True(t, budget.tryConsume(account, failoverErr))
	require.Equal(t, 2*openAIModelCapacityRetryBaseDelay, budget.modelCapacityBackoff())
	require.False(t, budget.tryConsume(account, failoverErr))
	require.Equal(t, openAIMaxAutomaticReplayAttempts, budget.used)
	require.Equal(t, openAIMaxAutomaticReplayAttempts, budget.modelCapacityRetries)
}

func TestOpenAIRequestRetryBudget_ModelCapacityBackoffHonorsCancellation(t *testing.T) {
	budget := openAIRequestRetryBudget{}
	account := &service.Account{ID: 6, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		Reason:            service.GatewayFailureReason("openai_model_at_capacity"),
		NextAccountAction: service.NextAccountRetry,
	}
	require.True(t, budget.tryConsume(account, failoverErr))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	require.False(t, budget.waitBeforeReplay(ctx, nil, "responses", account, failoverErr))
	require.Less(t, time.Since(started), 50*time.Millisecond)
}

func TestOpenAIRequestRetryBudget_ModelCapacityWritesStructuredRetryLog(t *testing.T) {
	budget := openAIRequestRetryBudget{}
	account := &service.Account{ID: 8, Name: "capacity-account", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	failoverErr := &service.UpstreamFailoverError{
		StatusCode:        http.StatusTeapot,
		Reason:            service.GatewayFailureReason("openai_model_at_capacity"),
		NextAccountAction: service.NextAccountRetry,
	}
	require.True(t, budget.tryConsume(account, failoverErr))
	core, observed := observer.New(zap.WarnLevel)
	reqLog := zap.New(core)

	require.True(t, budget.waitBeforeReplay(context.Background(), reqLog, "responses", account, failoverErr))

	entries := observed.FilterMessage("openai.model_capacity_retry_scheduled").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.EqualValues(t, account.ID, fields["account_id"])
	require.Equal(t, account.Name, fields["account_name"])
	require.Equal(t, "responses", fields["route"])
	require.Equal(t, "openai_model_at_capacity", fields["failure_reason"])
	require.EqualValues(t, 1, fields["retry_attempt"])
	require.EqualValues(t, openAIMaxAutomaticReplayAttempts, fields["retry_max"])
	require.EqualValues(t, openAIModelCapacityRetryBaseDelay.Milliseconds(), fields["backoff_ms"])
	require.NotContains(t, fields, "request_body")
	require.NotContains(t, fields, "response_body")
}

func TestOpenAIRequestRetryBudget_PoolRetryOnlyHandlesCapacityRejections(t *testing.T) {
	account := &service.Account{
		ID:       9,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode":             true,
			"pool_mode_retry_count": 1,
		},
	}

	capacity := &service.UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		Reason:                 service.GatewayFailureReason("openai_model_at_capacity"),
		NextAccountAction:      service.NextAccountRetry,
		RetryableOnSameAccount: true,
	}
	genericTransient := &service.UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		NextAccountAction:      service.NextAccountRetry,
		RetryableOnSameAccount: true,
	}

	budget := openAIRequestRetryBudget{}
	require.True(t, budget.tryPoolRetry(context.Background(), nil, account, capacity))
	require.False(t, budget.tryPoolRetry(context.Background(), nil, account, capacity))
	require.False(t, budget.tryPoolRetry(context.Background(), nil, account, genericTransient))
}

func TestOpenAIRequestRetryBudget_NonExactServiceUnavailableRemainsUnsafe(t *testing.T) {
	budget := openAIRequestRetryBudget{}
	account := &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	failoverErr := &service.UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}

	require.False(t, budget.tryConsume(account, failoverErr))
	require.Zero(t, budget.used)
}
