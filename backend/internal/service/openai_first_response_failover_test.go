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

type openAIFirstResponseStatusUpstream struct {
	statusCode int
}

func (u openAIFirstResponseStatusUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{
		StatusCode: u.statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
	}, nil
}

func (u openAIFirstResponseStatusUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
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

func TestOpenAIFirstResponseTimeout_PassthroughDoesNotAbortAcceptedStream(t *testing.T) {
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
	releaseAfter := 80 * time.Millisecond
	releaseTimer := time.AfterFunc(releaseAfter, func() {
		_ = upstreamBody.Close()
	})
	t.Cleanup(func() {
		releaseTimer.Stop()
		_ = upstreamBody.Close()
	})

	started := time.Now()
	result, err := svc.handleStreamingResponsePassthrough(
		ctx,
		resp,
		c,
		&Account{ID: 101, Name: "slow", Platform: PlatformOpenAI},
		time.Now(),
		"gpt-5.6-luna",
		"gpt-5.6-luna",
	)
	elapsed := time.Since(started)

	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.False(t, failoverErr.FirstResponseTimeout)
	require.GreaterOrEqual(t, elapsed, releaseAfter-20*time.Millisecond,
		"an accepted upstream stream must not be canceled by the legacy first-output timeout")
	require.Empty(t, recorder.Body.String(), "structural preamble must remain buffered when the stream ends without semantic output")
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

func TestOpenAIFirstTokenTimingStartsAtUpstreamDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{httpUpstream: openAIFirstResponseImmediateUpstream{}}
	account := &Account{ID: 204, Name: "timing", Platform: PlatformOpenAI, Concurrency: 1}
	req := httptest.NewRequest(http.MethodPost, "https://upstream.test/v1/responses", nil)
	totalRequestStart := time.Now().Add(-5 * time.Second)

	fastTimingCtx := WithOpenAIFastFirstTokenTiming(context.Background())
	resp, err := svc.doOpenAIUpstreamWithFirstResponseBudget(fastTimingCtx, c, account, req, "", false)
	require.NoError(t, err)
	require.NotNil(t, resp)

	firstTokenStart := openAIFirstTokenStart(c, totalRequestStart)
	require.WithinDuration(t, time.Now(), firstTokenStart, time.Second)
	require.Greater(t, firstTokenStart.Sub(totalRequestStart), 4*time.Second)
	require.NoError(t, resp.Body.Close())
}

func TestOpenAIFirstTokenTimingUsesSuccessfulResponseHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{httpUpstream: openAIFirstResponseImmediateUpstream{}}
	account := &Account{ID: 205, Name: "accepted", Platform: PlatformOpenAI, Concurrency: 1}
	req := httptest.NewRequest(http.MethodPost, "https://upstream.test/v1/responses", nil)

	fastTimingCtx := WithOpenAIFastFirstTokenTiming(context.Background())
	resp, err := svc.doOpenAIUpstreamWithFirstResponseBudget(fastTimingCtx, c, account, req, "", false)
	require.NoError(t, err)
	require.NotNil(t, resp)
	accepted := openAIFirstTokenAccepted(c)
	require.NotNil(t, accepted)
	require.GreaterOrEqual(t, *accepted, 0)
	require.Less(t, *accepted, 1000)

	// A later semantic token must not replace the faster accepted-response
	// sample used for user-visible TTFT and scheduler scoring.
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, resp.Body.Close())
	resp.Body = io.NopCloser(strings.NewReader(
		`data: {"type":"response.created","response":{"id":"resp_accepted"}}` + "\n\n" +
			`data: {"type":"response.output_text.delta","delta":"ok"}` + "\n\n" +
			`data: {"type":"response.completed","response":{"id":"resp_accepted","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n",
	))
	result, err := svc.handleStreamingResponse(
		context.Background(),
		resp,
		c,
		account,
		time.Now().Add(-time.Second),
		"gpt-5.6-sol",
		"gpt-5.6-sol",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, accepted, result.firstTokenMs)
}

func TestOpenAIFirstTokenTimingRejectsErrorHeadersAndClearsPriorAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{ID: 206, Name: "retry", Platform: PlatformOpenAI, Concurrency: 1}
	req := httptest.NewRequest(http.MethodPost, "https://upstream.test/v1/responses", nil)

	successSvc := &OpenAIGatewayService{httpUpstream: openAIFirstResponseImmediateUpstream{}}
	fastTimingCtx := WithOpenAIFastFirstTokenTiming(context.Background())
	successResp, err := successSvc.doOpenAIUpstreamWithFirstResponseBudget(fastTimingCtx, c, account, req, "", false)
	require.NoError(t, err)
	require.NotNil(t, openAIFirstTokenAccepted(c))
	require.NoError(t, successResp.Body.Close())

	errorSvc := &OpenAIGatewayService{httpUpstream: openAIFirstResponseStatusUpstream{statusCode: http.StatusBadGateway}}
	errorReq := httptest.NewRequest(http.MethodPost, "https://upstream.test/v1/responses", nil)
	errorResp, err := errorSvc.doOpenAIUpstreamWithFirstResponseBudget(fastTimingCtx, c, account, errorReq, "", false)
	require.NoError(t, err)
	require.NotNil(t, errorResp)
	require.Nil(t, openAIFirstTokenAccepted(c), "an error attempt must clear the prior success sample")
	require.NoError(t, errorResp.Body.Close())
}

func TestOpenAIFirstTokenTimingFallsBackWithoutDispatchMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	fallback := time.Now().Add(-time.Second)

	require.Equal(t, fallback, openAIFirstTokenStart(c, fallback))
}

func TestOpenAIFirstTokenTimingGlobalOptimizationDisabledKeepsSemanticFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{httpUpstream: openAIFirstResponseImmediateUpstream{}}
	account := &Account{
		ID:          207,
		Name:        "global-off-relay-account",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Extra:       map[string]any{"openai_upstream_relay": true},
	}
	req := httptest.NewRequest(http.MethodPost, "https://upstream.test/v1/responses", nil)
	require.True(t, account.IsOpenAIUpstreamRelay())

	resp, err := svc.doOpenAIUpstreamWithFirstResponseBudget(context.Background(), c, account, req, "", false)
	require.NoError(t, err)
	require.Nil(t, openAIFirstTokenAccepted(c), "relay account mode must not enable fast TTFT when the global setting is off")
	require.NoError(t, resp.Body.Close())

	// With the global optimization disabled, a successful response header is
	// not the TTFT endpoint. Delay the first semantic event to prove the
	// semantic fallback remains active even for an upstream-relay account.
	time.Sleep(30 * time.Millisecond)
	resp.Body = io.NopCloser(strings.NewReader(
		`data: {"type":"response.created","response":{"id":"resp_global_off"}}` + "\n\n" +
			`data: {"type":"response.output_text.delta","delta":"ok"}` + "\n\n" +
			`data: {"type":"response.completed","response":{"id":"resp_global_off","usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n",
	))
	semanticStart := time.Now().Add(-250 * time.Millisecond)
	result, err := svc.handleStreamingResponse(
		context.Background(),
		resp,
		c,
		account,
		semanticStart,
		"gpt-5.6-sol",
		"gpt-5.6-sol",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.firstTokenMs)
	require.GreaterOrEqual(t, *result.firstTokenMs, 20)
}
