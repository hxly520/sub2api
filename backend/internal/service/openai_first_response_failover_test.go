package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIFirstResponseBlockingUpstream struct{}

func (openAIFirstResponseBlockingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func (u openAIFirstResponseBlockingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

type openAIFirstResponseImmediateUpstream struct{}

func (openAIFirstResponseImmediateUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func (u openAIFirstResponseImmediateUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func TestOpenAIStreamPayloadCountsAsFirstResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "empty", payload: "", want: false},
		{name: "done sentinel", payload: "[DONE]", want: false},
		{name: "responses created", payload: `{"type":"response.created","response":{"id":"resp_1"}}`, want: false},
		{name: "responses in progress", payload: `{"type":"response.in_progress","response":{"id":"resp_1"}}`, want: false},
		{name: "responses message item metadata", payload: `{"type":"response.output_item.added","item":{"type":"message","id":"msg_1"}}`, want: false},
		{name: "responses content part metadata", payload: `{"type":"response.content_part.added","part":{"type":"output_text","text":""}}`, want: false},
		{name: "responses annotation metadata", payload: `{"type":"response.output_text.annotation.added","annotation":{"type":"url_citation"}}`, want: false},
		{name: "responses text delta", payload: `{"type":"response.output_text.delta","delta":"ok"}`, want: true},
		{name: "responses empty text delta", payload: `{"type":"response.output_text.delta","delta":""}`, want: false},
		{name: "responses reasoning delta", payload: `{"type":"response.reasoning_summary_text.delta","delta":"thinking"}`, want: true},
		{name: "responses function call starts", payload: `{"type":"response.output_item.added","item":{"type":"function_call","call_id":"call_1","name":"search"}}`, want: true},
		{name: "responses custom tool starts", payload: `{"type":"response.output_item.added","item":{"type":"custom_tool_call","call_id":"call_1","name":"exec"}}`, want: true},
		{name: "responses function arguments", payload: `{"type":"response.function_call_arguments.delta","delta":"{\\\"q\\\":"}`, want: true},
		{name: "responses completed", payload: `{"type":"response.completed","response":{"id":"resp_1"}}`, want: true},
		{name: "responses incomplete", payload: `{"type":"response.incomplete","response":{"id":"resp_1"}}`, want: true},
		{name: "responses failed", payload: `{"type":"response.failed","response":{"error":{"message":"failed"}}}`, want: true},
		{name: "chat role preamble", payload: `{"object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}`, want: false},
		{name: "chat empty content", payload: `{"object":"chat.completion.chunk","choices":[{"delta":{"content":""},"finish_reason":null}]}`, want: false},
		{name: "chat usage only", payload: `{"object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":10}}`, want: false},
		{name: "chat content", payload: `{"object":"chat.completion.chunk","choices":[{"delta":{"content":"ok"},"finish_reason":null}]}`, want: true},
		{name: "chat tool call", payload: `{"object":"chat.completion.chunk","choices":[{"delta":{"tool_calls":[{"id":"call_1"}]},"finish_reason":null}]}`, want: true},
		{name: "chat empty tool call", payload: `{"object":"chat.completion.chunk","choices":[{"delta":{"tool_calls":[]},"finish_reason":null}]}`, want: false},
		{name: "chat terminal", payload: `{"object":"chat.completion.chunk","choices":[{"delta":{},"finish_reason":"stop"}]}`, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, openAIStreamPayloadCountsAsFirstResponse(tt.payload))
		})
	}
}

func TestOpenAIFirstResponseTimeout_PreambleIsWithheldAndFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	upstreamBody := newOpenAICompatBlockingReadCloser([]byte(
		"event: response.created\n" +
			`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}` + "\n\n" +
			`data: {"type":"response.in_progress","response":{"id":"resp_1","status":"in_progress"}}` + "\n\n",
	))
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_first_timeout"}},
		Body:       upstreamBody,
	}
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), 30*time.Millisecond)
	svc := &OpenAIGatewayService{}

	result, err := svc.handleStreamingResponsePassthrough(
		ctx,
		resp,
		c,
		&Account{ID: 101, Name: "slow", Platform: PlatformOpenAI},
		time.Now(),
		"gpt-5.6-luna",
		"gpt-5.6-luna",
	)

	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.True(t, failoverErr.FirstResponseTimeout)
	require.Equal(t, 30, failoverErr.FirstResponseTimeoutMs)
	require.Empty(t, recorder.Body.String(), "preamble bytes must remain buffered so another account can retry")
}

func TestOpenAIFirstResponseTimeout_LargePreambleCannotAutoFlushBeforeFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	// This exceeds the old 4 KiB bufio.Writer capacity but remains below the
	// explicit bounded first-response queue.
	largeMetadata := strings.Repeat("x", 32*1024)
	upstreamBody := newOpenAICompatBlockingReadCloser([]byte(
		`data: {"type":"response.created","response":{"id":"resp_large","status":"in_progress","metadata":{"padding":"` +
			largeMetadata + `"}}}` + "\n\n",
	))
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       upstreamBody,
	}
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), 30*time.Millisecond)
	svc := &OpenAIGatewayService{}

	_, err := svc.handleStreamingResponse(
		ctx,
		resp,
		c,
		&Account{ID: 102, Name: "large-preamble", Platform: PlatformOpenAI},
		time.Now(),
		"gpt-5.6-luna",
		"gpt-5.6-luna",
	)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.FirstResponseTimeout)
	require.Empty(t, recorder.Body.String(), "large structural preamble must not leak before an account retry")
}

func TestOpenAIFirstResponseTimeout_SemanticPayloadStopsWatch(t *testing.T) {
	t.Parallel()
	body := newOpenAICompatBlockingReadCloser(nil)
	watch := newOpenAIFirstResponseTimeoutWatch(
		WithOpenAIFirstResponseTimeout(context.Background(), 20*time.Millisecond),
		body,
	)
	require.NotNil(t, watch)
	t.Cleanup(watch.Stop)

	watch.ObservePayload(`{"type":"response.created"}`)
	require.True(t, watch.Waiting())
	watch.ObservePayload(`{"type":"response.output_text.delta","delta":"ok"}`)
	require.False(t, watch.Waiting())

	time.Sleep(30 * time.Millisecond)
	require.False(t, watch.TimedOut())
	require.NoError(t, body.Close())
}

func TestReserveOpenAIFirstResponseBuffer_CommitsBeforeOverflow(t *testing.T) {
	t.Parallel()
	watch := &openAIFirstResponseTimeoutWatch{}
	bufferedBytes := openAIFirstResponseMaxBufferedBytes - 2

	require.True(t, reserveOpenAIFirstResponseBuffer(watch, &bufferedBytes, 2))
	require.Equal(t, openAIFirstResponseMaxBufferedBytes, bufferedBytes)
	require.True(t, watch.Waiting())

	require.False(t, reserveOpenAIFirstResponseBuffer(watch, &bufferedBytes, 1))
	require.Equal(t, openAIFirstResponseMaxBufferedBytes, bufferedBytes)
	require.False(t, watch.Waiting(), "overflow must commit the current account and stop failover timing")
	require.False(t, watch.TimedOut())
}

func TestOpenAIFirstResponseTimeout_CoversResponseHeaderWait(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{httpUpstream: openAIFirstResponseBlockingUpstream{}}
	account := &Account{ID: 202, Name: "slow-headers", Platform: PlatformOpenAI, Concurrency: 1}
	req := httptest.NewRequest(http.MethodPost, "https://upstream.test/v1/responses", nil)
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), 30*time.Millisecond)

	started := time.Now()
	resp, err := svc.doOpenAIUpstreamWithFirstResponseBudget(ctx, c, account, req, "", false)
	elapsed := time.Since(started)

	require.Nil(t, resp)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.FirstResponseTimeout)
	require.Equal(t, 30, failoverErr.FirstResponseTimeoutMs)
	require.GreaterOrEqual(t, elapsed, 20*time.Millisecond)
	require.Less(t, elapsed, 500*time.Millisecond)
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIFirstResponseTimeout_ResponseHeaderSuccessKeepsRemainingSSEBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{httpUpstream: openAIFirstResponseImmediateUpstream{}}
	account := &Account{ID: 203, Name: "fast-headers", Platform: PlatformOpenAI, Concurrency: 1}
	req := httptest.NewRequest(http.MethodPost, "https://upstream.test/v1/responses", nil)
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), 200*time.Millisecond)

	resp, err := svc.doOpenAIUpstreamWithFirstResponseBudget(ctx, c, account, req, "", false)
	require.NoError(t, err)
	require.NotNil(t, resp)
	watch := newOpenAIFirstResponseTimeoutWatch(ctx, resp.Body)
	require.NotNil(t, watch)
	watch.ObservePayload(`{"type":"response.output_text.delta","delta":"ok"}`)
	require.False(t, watch.Waiting())
	require.NoError(t, resp.Body.Close())
}
