//go:build unit

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

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamRawChatCompletions_FirstResponseTimeoutBeforeDataTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	reader, writer := io.Pipe()
	defer writer.Close()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_first_timeout_raw"}},
		Body:       reader,
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Millisecond)

	result, err := svc.streamRawChatCompletions(
		ctx,
		c,
		resp,
		rawChatCompletionsTestAccount(),
		"gpt-5.5",
		"gpt-5.5",
		"gpt-5.5",
		nil,
		nil,
		time.Now(),
		0,
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.True(t, failoverErr.FirstResponseTimeout)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.Equal(t, 10, failoverErr.FirstResponseTimeoutMs)
	require.Empty(t, rec.Body.String())
}

func TestHandleStreamingResponsePassthrough_FirstResponseTimeoutBeforeDataTriggersFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	reader, writer := io.Pipe()
	defer writer.Close()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_first_timeout_responses"}},
		Body:       reader,
	}
	account := &Account{ID: 202, Name: "responses-account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Millisecond)

	result, err := svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")

	require.NotNil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.True(t, failoverErr.FirstResponseTimeout)
	require.Equal(t, http.StatusGatewayTimeout, failoverErr.StatusCode)
	require.Equal(t, 10, failoverErr.FirstResponseTimeoutMs)
	require.Empty(t, rec.Body.String())
}

func TestHandleStreamingResponsePassthrough_EarlyFlushContextFlushesPreamble(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	reader, writer := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_first_early_flush"}},
		Body:       reader,
	}
	account := &Account{ID: 203, Name: "single-account", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	ctx := WithOpenAIFirstResponseEarlyFlush(
		WithOpenAIFirstResponseTimeout(context.Background(), time.Second),
	)

	resultCh := make(chan struct {
		result *openaiStreamingResultPassthrough
		err    error
	}, 1)
	go func() {
		result, err := svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
		resultCh <- struct {
			result *openaiStreamingResultPassthrough
			err    error
		}{result: result, err: err}
	}()

	_, err := writer.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_early\"}}\n\n"))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return strings.Contains(rec.Body.String(), "response.created")
	}, time.Second, 10*time.Millisecond)

	_, err = writer.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_early\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, "resp_early", got.result.responseID)
	case <-time.After(time.Second):
		t.Fatal("stream did not finish")
	}
}

func TestHandleStreamingResponsePassthrough_EarlyFlushContextFlushesEventLinePreamble(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	reader, writer := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_first_event_line_early_flush"}},
		Body:       reader,
	}
	account := &Account{ID: 204, Name: "single-account-event-line", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	ctx := WithOpenAIFirstResponseEarlyFlush(
		WithOpenAIFirstResponseTimeout(context.Background(), time.Second),
	)

	resultCh := make(chan struct {
		result *openaiStreamingResultPassthrough
		err    error
	}, 1)
	go func() {
		result, err := svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "gpt-5.5", "gpt-5.5")
		resultCh <- struct {
			result *openaiStreamingResultPassthrough
			err    error
		}{result: result, err: err}
	}()

	_, err := writer.Write([]byte("event: response.created\n"))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return strings.Contains(rec.Body.String(), "event: response.created")
	}, time.Second, 10*time.Millisecond)

	_, err = writer.Write([]byte("data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_event_line\"}}\n\n"))
	require.NoError(t, err)
	_, err = writer.Write([]byte("event: response.completed\n"))
	require.NoError(t, err)
	_, err = writer.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_event_line\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.NotNil(t, got.result.firstTokenMs)
		require.Equal(t, "resp_event_line", got.result.responseID)
	case <-time.After(time.Second):
		t.Fatal("stream did not finish")
	}
}

func TestOpenAIFirstResponseTimeoutWatch_EventLineStopsTimeout(t *testing.T) {
	body := io.NopCloser(strings.NewReader(""))
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Millisecond)
	watch := newOpenAIFirstResponseTimeoutWatch(ctx, body)
	defer watch.Stop()

	watch.ObserveLine("event: response.created")
	time.Sleep(25 * time.Millisecond)

	require.False(t, watch.TimedOut())
}

func TestStreamRawChatCompletions_FirstDataStopsFirstResponseTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","model":"gpt-5.5","choices":[{"index":0,"delta":{"content":"ok"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_first_seen"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
	ctx := WithOpenAIFirstResponseTimeout(context.Background(), time.Second)

	result, err := svc.streamRawChatCompletions(
		ctx,
		c,
		resp,
		rawChatCompletionsTestAccount(),
		"gpt-5.5",
		"gpt-5.5",
		"gpt-5.5",
		nil,
		nil,
		time.Now(),
		0,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), `"content":"ok"`)
}

func TestOpenAIFirstResponseRuntimeConfigUsesSettings(t *testing.T) {
	resetOpenAIFirstResponseRuntimeConfigCacheForTest()
	t.Cleanup(resetOpenAIFirstResponseRuntimeConfigCacheForTest)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIFirstResponse.Enabled = false
	cfg.Gateway.OpenAIFirstResponse.TimeoutMS = 5000
	cfg.Gateway.OpenAIFirstResponse.MaxAttempts = 2

	repo := &openAIAdvancedSchedulerSettingRepoStub{
		values: map[string]string{
			openAIFirstResponseEnabledSettingKey:   "true",
			openAIFirstResponseTimeoutMSSettingKey: "3000",
		},
	}
	svc := &OpenAIGatewayService{
		cfg:            cfg,
		settingService: NewSettingService(repo, cfg),
	}

	got := svc.GetOpenAIFirstResponseRuntimeConfig(context.Background())
	require.True(t, got.Enabled)
	require.Equal(t, 3000, got.TimeoutMS)
	require.Equal(t, 2, got.MaxAttempts)
}

func TestOpenAIFirstResponseRuntimeConfigFallsBackToYAML(t *testing.T) {
	resetOpenAIFirstResponseRuntimeConfigCacheForTest()
	t.Cleanup(resetOpenAIFirstResponseRuntimeConfigCacheForTest)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIFirstResponse.Enabled = true
	cfg.Gateway.OpenAIFirstResponse.TimeoutMS = 2500
	cfg.Gateway.OpenAIFirstResponse.MaxAttempts = 3
	cfg.Gateway.OpenAIFirstResponse.CountAsError = true

	repo := &openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{}}
	svc := &OpenAIGatewayService{
		cfg:            cfg,
		settingService: NewSettingService(repo, cfg),
	}

	got := svc.GetOpenAIFirstResponseRuntimeConfig(context.Background())
	require.True(t, got.Enabled)
	require.Equal(t, 2500, got.TimeoutMS)
	require.Equal(t, 3, got.MaxAttempts)
	require.True(t, got.CountAsError)
}
