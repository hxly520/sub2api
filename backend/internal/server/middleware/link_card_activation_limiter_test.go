package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLinkCardActivationLimiterLocksOnTenthInvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	router := linkCardActivationLimiterTestRouter(client)
	for attempt := 1; attempt < 10; attempt++ {
		response := performLinkCardActivationAttempt(router, false)
		require.Equal(t, http.StatusNotFound, response.Code, "attempt %d", attempt)
	}

	locked := performLinkCardActivationAttempt(router, false)
	require.Equal(t, http.StatusTooManyRequests, locked.Code)
	require.Equal(t, "300", locked.Header().Get("Retry-After"))
	require.Contains(t, locked.Body.String(), "LINK_CARD_ACTIVATION_LOCKED")

	blocked := performLinkCardActivationAttempt(router, true)
	require.Equal(t, http.StatusTooManyRequests, blocked.Code)
	require.NotContains(t, blocked.Body.String(), "activated")

	server.FastForward(5*time.Minute + time.Second)
	afterExpiry := performLinkCardActivationAttempt(router, false)
	require.Equal(t, http.StatusNotFound, afterExpiry.Code)
}

func TestLinkCardActivationLimiterSuccessClearsConsecutiveFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	router := linkCardActivationLimiterTestRouter(client)
	for range 9 {
		require.Equal(t, http.StatusNotFound, performLinkCardActivationAttempt(router, false).Code)
	}
	require.Equal(t, http.StatusOK, performLinkCardActivationAttempt(router, true).Code)
	for range 9 {
		require.Equal(t, http.StatusNotFound, performLinkCardActivationAttempt(router, false).Code)
	}
	require.Equal(t, http.StatusTooManyRequests, performLinkCardActivationAttempt(router, false).Code)
}

func TestLinkCardActivationLimiterCountsMalformedCredentialAttempts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	router := linkCardActivationLimiterMalformedRequestTestRouter(client)
	for attempt := 1; attempt < 10; attempt++ {
		response := performLinkCardMalformedAttempt(router)
		require.Equal(t, http.StatusBadRequest, response.Code, "attempt %d", attempt)
	}

	locked := performLinkCardMalformedAttempt(router)
	require.Equal(t, http.StatusTooManyRequests, locked.Code)
	require.Equal(t, "300", locked.Header().Get("Retry-After"))
	require.Contains(t, locked.Body.String(), "LINK_CARD_ACTIVATION_LOCKED")
}

func TestLinkCardActivationLimiterFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 10 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })

	response := performLinkCardActivationAttempt(linkCardActivationLimiterTestRouter(client), false)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "LINK_CARD_ACTIVATION_GUARD_UNAVAILABLE")
}

func linkCardActivationLimiterTestRouter(client *redis.Client) *gin.Engine {
	router := gin.New()
	limiter := NewLinkCardActivationLimiter(client)
	router.POST("/activate", limiter.Middleware(), func(c *gin.Context) {
		if c.GetHeader("X-Test-Valid") == "1" {
			ResetLinkCardActivationFailures(c)
			c.JSON(http.StatusOK, gin.H{"status": "activated"})
			return
		}
		if HandleLinkCardActivationFailure(c) {
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"status": "invalid"})
	})
	return router
}

func linkCardActivationLimiterMalformedRequestTestRouter(client *redis.Client) *gin.Engine {
	router := gin.New()
	limiter := NewLinkCardActivationLimiter(client)
	router.POST("/activate", limiter.Middleware(), func(c *gin.Context) {
		if HandleLinkCardActivationFailure(c) {
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"status": "invalid"})
	})
	return router
}

func performLinkCardActivationAttempt(router http.Handler, valid bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/activate", nil)
	request.RemoteAddr = "203.0.113.10:12345"
	if valid {
		request.Header.Set("X-Test-Valid", "1")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func performLinkCardMalformedAttempt(router http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/activate", nil)
	request.RemoteAddr = "203.0.113.10:12345"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
