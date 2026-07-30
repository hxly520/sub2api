package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIsOpenAIModelAtCapacityErrorExactMatchOnly(t *testing.T) {
	exact := OpenAIModelAtCapacityMessage
	require.True(t, isOpenAIModelAtCapacityError(exact, nil))
	require.True(t, isOpenAIModelAtCapacityError("", []byte(`{"error":{"message":"Selected model is at capacity. Please try a different model."}}`)))
	require.True(t, isOpenAIModelAtCapacityError("", []byte(`{"response":{"error":{"message":"Selected model is at capacity. Please try a different model."}}}`)))

	for _, message := range []string{
		"Selected model is at capacity",
		"selected model is at capacity. Please try a different model.",
		"Selected model is at capacity. Please try a different model. Retry later.",
		"Relay error: Selected model is at capacity. Please try a different model.",
	} {
		require.False(t, isOpenAIModelAtCapacityError(message, nil), "message=%q", message)
		require.False(t, isOpenAIModelAtCapacityError("", []byte(`{"error":{"message":`+strconv.Quote(message)+`}}`)), "message=%q", message)
	}
}

func TestOpenAIModelAtCapacityFailoverIsExplicitlyReplaySafe(t *testing.T) {
	body := []byte(`{"error":{"message":"Selected model is at capacity. Please try a different model."}}`)
	for _, statusCode := range []int{http.StatusBadRequest, http.StatusTeapot, http.StatusServiceUnavailable} {
		failoverErr := newOpenAIUpstreamFailoverError(statusCode, http.Header{}, body, OpenAIModelAtCapacityMessage, false)
		require.True(t, failoverErr.IsOpenAIModelAtCapacity())
		require.True(t, failoverErr.CanSafelyReplayRequest())
		require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
		require.Equal(t, NextAccountRetry, failoverErr.NextAccountAction)
		require.False(t, failoverErr.ShouldReportAccountScheduleFailure())
	}

	nonExact := newOpenAIUpstreamFailoverError(
		http.StatusServiceUnavailable,
		http.Header{},
		[]byte(`{"error":{"message":"Selected model is at capacity. Retry later."}}`),
		"Selected model is at capacity. Retry later.",
		false,
	)
	require.False(t, nonExact.IsOpenAIModelAtCapacity())
	require.False(t, nonExact.CanSafelyReplayRequest())
}

func TestOpenAIModelAtCapacityDoesNotRuntimeBlockAccount(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 71, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	failoverErr := newOpenAIUpstreamFailoverError(
		http.StatusServiceUnavailable,
		http.Header{},
		[]byte(`{"error":{"message":"Selected model is at capacity. Please try a different model."}}`),
		OpenAIModelAtCapacityMessage,
		false,
	)

	require.False(t, svc.MaybeBlockOpenAIAccountAfterFailoverError(account, failoverErr))
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestOpenAIStreamModelAtCapacityFailoverIsReplaySafeBeforeOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	payload := []byte(`{"type":"response.failed","response":{"status":"failed","error":{"message":"Selected model is at capacity. Please try a different model."}}}`)

	failoverErr := svc.newOpenAIStreamFailoverError(c, &Account{ID: 1, Platform: PlatformOpenAI}, false, "req-capacity", payload, OpenAIModelAtCapacityMessage)

	require.True(t, failoverErr.IsOpenAIModelAtCapacity())
	require.True(t, failoverErr.CanSafelyReplayRequest())
	require.False(t, c.Writer.Written())
}

func TestHandleNonStreamingResponseModelCapacityFailedJSONReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"id":"resp_capacity","status":"failed","error":{"message":"Selected model is at capacity. Please try a different model."},"usage":{"input_tokens":0,"output_tokens":0}}`
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req-capacity-json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	_, err := svc.handleNonStreamingResponse(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, "gpt-test", "gpt-test")

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.True(t, failoverErr.IsOpenAIModelAtCapacity())
	require.False(t, c.Writer.Written())
}

func TestHandleNonStreamingResponseModelCapacityFailedSSEReturnsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		body        string
		passthrough bool
	}{
		{
			name: "response_failed",
			body: strings.Join([]string{
				`data: {"type":"response.created","response":{"id":"resp_capacity","status":"in_progress"}}`,
				"",
				`data: {"type":"response.failed","response":{"id":"resp_capacity","status":"failed","error":{"message":"Selected model is at capacity. Please try a different model."}}}`,
				"",
			}, "\n"),
		},
		{
			name: "sse_error",
			body: strings.Join([]string{
				"event: error",
				`data: {"type":"error","error":{"message":"Selected model is at capacity. Please try a different model."}}`,
				"",
			}, "\n"),
		},
		{
			name:        "passthrough_sse_error",
			passthrough: true,
			body: strings.Join([]string{
				"event: error",
				`data: {"type":"error","error":{"message":"Selected model is at capacity. Please try a different model."}}`,
				"",
			}, "\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req-capacity-sse"}},
				Body:       io.NopCloser(bytes.NewBufferString(tt.body)),
			}
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			account := &Account{ID: 1, Platform: PlatformOpenAI}

			var err error
			if tt.passthrough {
				_, err = svc.handleNonStreamingResponsePassthrough(c.Request.Context(), resp, c, "gpt-test", "gpt-test", account)
			} else {
				_, err = svc.handleNonStreamingResponse(c.Request.Context(), resp, c, account, "gpt-test", "gpt-test")
			}

			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr))
			require.True(t, failoverErr.IsOpenAIModelAtCapacity())
			require.False(t, c.Writer.Written())
		})
	}
}
