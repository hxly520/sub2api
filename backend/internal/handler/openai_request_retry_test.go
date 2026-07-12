package handler

import (
	"net/http"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
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
