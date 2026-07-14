package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamFailoverErrorCanSafelyReplayRequest(t *testing.T) {
	for _, statusCode := range []int{
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusTooManyRequests,
	} {
		err := &UpstreamFailoverError{StatusCode: statusCode}
		require.True(t, err.CanSafelyReplayRequest(), "status=%d", statusCode)
	}

	for _, statusCode := range []int{
		0,
		http.StatusBadRequest,
		http.StatusRequestTimeout,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		err := &UpstreamFailoverError{StatusCode: statusCode}
		require.False(t, err.CanSafelyReplayRequest(), "status=%d", statusCode)
	}

	require.False(t, (*UpstreamFailoverError)(nil).CanSafelyReplayRequest())
	require.False(t, (&UpstreamFailoverError{
		StatusCode:           http.StatusTooManyRequests,
		FirstResponseTimeout: true,
	}).CanSafelyReplayRequest())
}

func TestOpenAIWSAutomaticAttemptLimit(t *testing.T) {
	pool := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"pool_mode": true,
		},
	}
	direct := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	require.Equal(t, 1, openAIWSAutomaticAttemptLimit(pool))
	require.Equal(t, openAIWSReconnectRetryLimit+1, openAIWSAutomaticAttemptLimit(direct))
	require.Equal(t, openAIWSReconnectRetryLimit+1, openAIWSAutomaticAttemptLimit(nil))
}
