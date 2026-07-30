//go:build unit

package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIModelCapacityRetryUpstream struct {
	service.HTTPUpstream
	mu            sync.Mutex
	mode          string
	accountIDs    []int64
	requestBodies [][]byte
}

func (u *openAIModelCapacityRetryUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.requestBodies = append(u.requestBodies, append([]byte(nil), body...))
	call := len(u.accountIDs)
	u.mu.Unlock()

	if call == 1 {
		switch u.mode {
		case "http_exact":
			return openAIModelCapacityHTTPResponse(service.OpenAIModelAtCapacityMessage), nil
		case "http_near_match":
			return openAIModelCapacityHTTPResponse(service.OpenAIModelAtCapacityMessage + " Retry later."), nil
		case "sse_before_output":
			return openAIModelCapacitySSEResponse(false), nil
		case "sse_error_before_output":
			return openAIModelCapacitySSEErrorResponse(), nil
		case "sse_after_output":
			return openAIModelCapacitySSEResponse(true), nil
		}
	}
	if u.mode == "http_exact_always" {
		return openAIModelCapacityHTTPResponse(service.OpenAIModelAtCapacityMessage), nil
	}
	return openAIModelCapacitySuccessSSEResponse(), nil
}

func (u *openAIModelCapacityRetryUpstream) snapshot() ([]int64, [][]byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	ids := append([]int64(nil), u.accountIDs...)
	bodies := make([][]byte, len(u.requestBodies))
	for i := range u.requestBodies {
		bodies[i] = append([]byte(nil), u.requestBodies[i]...)
	}
	return ids, bodies
}

func openAIModelCapacityHTTPResponse(message string) *http.Response {
	body := `{"error":{"type":"invalid_request_error","message":` + strconv.Quote(message) + `}}`
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func openAIModelCapacitySSEResponse(afterOutput bool) *http.Response {
	lines := []string{
		`data: {"type":"response.created","response":{"id":"resp_capacity","status":"in_progress"}}`,
		"",
	}
	if afterOutput {
		lines = append(lines,
			`data: {"type":"response.output_text.delta","response_id":"resp_capacity","output_index":0,"content_index":0,"delta":"partial"}`,
			"",
		)
	}
	lines = append(lines,
		`data: {"type":"response.failed","response":{"id":"resp_capacity","status":"failed","error":{"type":"invalid_request_error","message":"Selected model is at capacity. Please try a different model."},"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":1}}}`,
		"",
	)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req-capacity"}},
		Body:       io.NopCloser(bytes.NewBufferString(strings.Join(lines, "\n"))),
	}
}

func openAIModelCapacitySuccessSSEResponse() *http.Response {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_healthy","object":"response","status":"completed","model":"gpt-5.1","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req-healthy"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func openAIModelCapacitySSEErrorResponse() *http.Response {
	body := strings.Join([]string{
		"event: error",
		`data: {"type":"error","error":{"type":"server_error","message":"Selected model is at capacity. Please try a different model."}}`,
		"",
	}, "\n")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req-capacity-error"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func setOpenAIModelCapacityRequest(t *testing.T, c *gin.Context, stream bool) {
	t.Helper()
	body := `{"model":"gpt-5.1","stream":false,"input":"keep this request unchanged"}`
	if stream {
		body = `{"model":"gpt-5.1","stream":true,"input":"keep this request unchanged"}`
	}
	setOpenAIModelCapacityRawRequest(t, c, "/v1/responses", body)
}

func setOpenAIModelCapacityRawRequest(t *testing.T, c *gin.Context, path, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
}

func TestOpenAIResponsesModelCapacityHTTPRetriesWithUnchangedBody(t *testing.T) {
	upstream := &openAIModelCapacityRetryUpstream{mode: "http_exact"}
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	setOpenAIModelCapacityRequest(t, c, false)
	started := time.Now()

	handler.Responses(c)

	accountIDs, requestBodies := upstream.snapshot()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []int64{1, 2}, accountIDs)
	require.Len(t, requestBodies, 2)
	require.Equal(t, requestBodies[0], requestBodies[1])
	require.GreaterOrEqual(t, time.Since(started), openAIModelCapacityRetryBaseDelay)
}

func TestOpenAIResponsesModelCapacityRetriesSingleAccountWithinBudget(t *testing.T) {
	account := service.Account{
		ID: 1, Name: "single-capacity-account", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "token-1"},
	}

	t.Run("recovers_on_same_account", func(t *testing.T) {
		upstream := &openAIModelCapacityRetryUpstream{mode: "http_exact"}
		handler := newOpenAIResponsesFailoverTestHandlerWithAccounts(t, upstream, []service.Account{account})
		c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
		setOpenAIModelCapacityRequest(t, c, false)

		handler.Responses(c)

		accountIDs, requestBodies := upstream.snapshot()
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, []int64{1, 1}, accountIDs)
		require.Len(t, requestBodies, 2)
		require.Equal(t, requestBodies[0], requestBodies[1])
	})

	t.Run("stops_after_two_replays", func(t *testing.T) {
		upstream := &openAIModelCapacityRetryUpstream{mode: "http_exact_always"}
		handler := newOpenAIResponsesFailoverTestHandlerWithAccounts(t, upstream, []service.Account{account})
		c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
		setOpenAIModelCapacityRequest(t, c, false)

		handler.Responses(c)

		accountIDs, requestBodies := upstream.snapshot()
		require.Equal(t, []int64{1, 1, 1}, accountIDs)
		require.Len(t, requestBodies, 3)
		require.Equal(t, requestBodies[0], requestBodies[1])
		require.Equal(t, requestBodies[1], requestBodies[2])
		require.NotEqual(t, http.StatusOK, rec.Code)
	})
}

func TestOpenAIConversationEntrypointsReuseSingleAccountAfterCapacity(t *testing.T) {
	account := service.Account{
		ID: 1, Name: "single-conversation-account", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"access_token": "token-1"},
	}
	tests := []struct {
		name     string
		path     string
		body     string
		messages bool
		invoke   func(*OpenAIGatewayHandler, *gin.Context)
	}{
		{
			name: "responses", path: "/v1/responses",
			body:   `{"model":"gpt-5.1","stream":false,"input":"keep this request unchanged"}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) { handler.Responses(c) },
		},
		{
			name: "chat_completions", path: "/v1/chat/completions",
			body:   `{"model":"gpt-5.1","stream":false,"messages":[{"role":"user","content":"keep this request unchanged"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) { handler.ChatCompletions(c) },
		},
		{
			name: "messages", path: "/v1/messages", messages: true,
			body:   `{"model":"gpt-5.1","stream":false,"max_tokens":32,"messages":[{"role":"user","content":"keep this request unchanged"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) { handler.Messages(c) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &openAIModelCapacityRetryUpstream{mode: "http_exact"}
			handler := newOpenAIResponsesFailoverTestHandlerWithAccounts(t, upstream, []service.Account{account})
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			setOpenAIModelCapacityRawRequest(t, c, tt.path, tt.body)
			if tt.messages {
				apiKey, ok := middleware2.GetAPIKeyFromContext(c)
				require.True(t, ok)
				apiKey.Group.AllowMessagesDispatch = true
			}

			tt.invoke(handler, c)

			accountIDs, requestBodies := upstream.snapshot()
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, []int64{1, 1}, accountIDs)
			require.Len(t, requestBodies, 2)
			require.Equal(t, requestBodies[0], requestBodies[1])
		})
	}
}

func TestOpenAICompatibleTextEntrypointsModelCapacityHTTPRetries(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		invoke func(*OpenAIGatewayHandler, *gin.Context)
	}{
		{
			name: "chat_completions",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5.1","stream":false,"messages":[{"role":"user","content":"keep this request unchanged"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.ChatCompletions(c)
			},
		},
		{
			name: "messages",
			path: "/v1/messages",
			body: `{"model":"gpt-5.1","stream":false,"max_tokens":32,"messages":[{"role":"user","content":"keep this request unchanged"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.Messages(c)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &openAIModelCapacityRetryUpstream{mode: "http_exact"}
			handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			setOpenAIModelCapacityRawRequest(t, c, tt.path, tt.body)
			if tt.name == "messages" {
				apiKey, ok := middleware2.GetAPIKeyFromContext(c)
				require.True(t, ok)
				apiKey.Group.AllowMessagesDispatch = true
			}

			tt.invoke(handler, c)

			accountIDs, requestBodies := upstream.snapshot()
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, []int64{1, 2}, accountIDs)
			require.Len(t, requestBodies, 2)
			require.Equal(t, requestBodies[0], requestBodies[1])
		})
	}
}

func TestOpenAICompatibleTextEntrypointsModelCapacitySSEErrorRetries(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		body   string
		invoke func(*OpenAIGatewayHandler, *gin.Context)
	}{
		{
			name: "chat_completions",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5.1","stream":true,"messages":[{"role":"user","content":"keep this request unchanged"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.ChatCompletions(c)
			},
		},
		{
			name: "messages",
			path: "/v1/messages",
			body: `{"model":"gpt-5.1","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"keep this request unchanged"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.Messages(c)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &openAIModelCapacityRetryUpstream{mode: "sse_error_before_output"}
			handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			setOpenAIModelCapacityRawRequest(t, c, tt.path, tt.body)
			if tt.name == "messages" {
				apiKey, ok := middleware2.GetAPIKeyFromContext(c)
				require.True(t, ok)
				apiKey.Group.AllowMessagesDispatch = true
			}

			tt.invoke(handler, c)

			accountIDs, requestBodies := upstream.snapshot()
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, []int64{1, 2}, accountIDs)
			require.Len(t, requestBodies, 2)
			require.Equal(t, requestBodies[0], requestBodies[1])
			require.NotContains(t, rec.Body.String(), service.OpenAIModelAtCapacityMessage)
		})
	}
}

func TestOpenAIResponsesModelCapacitySSEBeforeOutputRetries(t *testing.T) {
	for _, mode := range []string{"sse_before_output", "sse_error_before_output"} {
		t.Run(mode, func(t *testing.T) {
			upstream := &openAIModelCapacityRetryUpstream{mode: mode}
			handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			setOpenAIModelCapacityRequest(t, c, true)

			handler.Responses(c)

			accountIDs, requestBodies := upstream.snapshot()
			require.Equal(t, []int64{1, 2}, accountIDs)
			require.Len(t, requestBodies, 2)
			require.Equal(t, requestBodies[0], requestBodies[1])
			require.Contains(t, rec.Body.String(), "resp_healthy")
			require.NotContains(t, rec.Body.String(), "resp_capacity")
			require.NotContains(t, rec.Body.String(), service.OpenAIModelAtCapacityMessage)
		})
	}
}

func TestOpenAIResponsesModelCapacityAfterOutputDoesNotReplay(t *testing.T) {
	upstream := &openAIModelCapacityRetryUpstream{mode: "sse_after_output"}
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	setOpenAIModelCapacityRequest(t, c, true)

	handler.Responses(c)

	accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{1}, accountIDs)
	require.Contains(t, rec.Body.String(), "partial")
	require.NotContains(t, rec.Body.String(), "resp_healthy")
}

func TestOpenAICompatibleModelCapacityAfterGeneratedOutputDoesNotReplay(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		body     string
		messages bool
		invoke   func(*OpenAIGatewayHandler, *gin.Context)
	}{
		{
			name: "chat_completions_streaming",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5.1","stream":true,"messages":[{"role":"user","content":"hello"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.ChatCompletions(c)
			},
		},
		{
			name: "chat_completions_buffered",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5.1","stream":false,"messages":[{"role":"user","content":"hello"}]}`,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.ChatCompletions(c)
			},
		},
		{
			name:     "messages_streaming",
			path:     "/v1/messages",
			body:     `{"model":"gpt-5.1","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`,
			messages: true,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.Messages(c)
			},
		},
		{
			name:     "messages_buffered",
			path:     "/v1/messages",
			body:     `{"model":"gpt-5.1","stream":false,"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`,
			messages: true,
			invoke: func(handler *OpenAIGatewayHandler, c *gin.Context) {
				handler.Messages(c)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &openAIModelCapacityRetryUpstream{mode: "sse_after_output"}
			handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			setOpenAIModelCapacityRawRequest(t, c, tt.path, tt.body)
			if tt.messages {
				apiKey, ok := middleware2.GetAPIKeyFromContext(c)
				require.True(t, ok)
				apiKey.Group.AllowMessagesDispatch = true
			}

			tt.invoke(handler, c)

			accountIDs, _ := upstream.snapshot()
			require.Equal(t, []int64{1}, accountIDs)
			require.NotContains(t, rec.Body.String(), "resp_healthy")
		})
	}
}

func TestOpenAIResponsesModelCapacityNearMatchDoesNotReplay(t *testing.T) {
	upstream := &openAIModelCapacityRetryUpstream{mode: "http_near_match"}
	handler := newOpenAIResponsesFailoverTestHandler(t, upstream)
	c, _ := newOpenAIResponsesFailoverTestContext(t, nil)
	setOpenAIModelCapacityRequest(t, c, false)

	handler.Responses(c)

	accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{1}, accountIDs)
}
