package service

import (
	"bytes"
	"context"
	"encoding/json"
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

type geminiChatFailingWriter struct {
	gin.ResponseWriter
	failAfter int
	writes    int
}

func (w *geminiChatFailingWriter) Write(p []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed: client disconnected")
	}
	w.writes++
	return w.ResponseWriter.Write(p)
}

func (w *geminiChatFailingWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func TestGeminiResponseToChatCompletionsPreservesInlineData(t *testing.T) {
	tests := []struct {
		name  string
		parts []any
		want  string
	}{
		{
			name: "image only",
			parts: []any{
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}},
			},
			want: "![image](data:image/png;base64,aW1hZ2U=)",
		},
		{
			name: "text and image",
			parts: []any{
				map[string]any{"text": "rendered image:\n"},
				map[string]any{"inlineData": map[string]any{"mimeType": "image/webp", "data": "d2VicA=="}},
			},
			want: "rendered image:\n![image](data:image/webp;base64,d2VicA==)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": tt.parts},
					"finishReason": "STOP",
				}},
			}
			rawData, err := json.Marshal(geminiResp)
			require.NoError(t, err)

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
			require.NoError(t, err)
			require.Len(t, got.Choices, 1)

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, tt.want, content)
			require.Equal(t, "stop", got.Choices[0].FinishReason)
		})
	}
}

func TestGeminiResponseToChatCompletionsOmitsInvalidInlineData(t *testing.T) {
	tests := []struct {
		name       string
		inlineData map[string]any
	}{
		{
			name:       "unsupported MIME type",
			inlineData: map[string]any{"mimeType": "image/svg+xml", "data": "PHN2Zz48L3N2Zz4="},
		},
		{
			name:       "malformed MIME type",
			inlineData: map[string]any{"mimeType": "image/png; charset=utf-8", "data": "aW1hZ2U="},
		},
		{
			name:       "malformed base64",
			inlineData: map[string]any{"mimeType": "image/png", "data": "not-valid-base64!!!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geminiResp := map[string]any{
				"candidates": []any{map[string]any{
					"content":      map[string]any{"parts": []any{map[string]any{"text": "before"}, map[string]any{"inlineData": tt.inlineData}, map[string]any{"text": "after"}}},
					"finishReason": "STOP",
				}},
			}
			rawData, err := json.Marshal(geminiResp)
			require.NoError(t, err)

			got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
			require.NoError(t, err)

			var content string
			require.NoError(t, json.Unmarshal(got.Choices[0].Message.Content, &content))
			require.Equal(t, "beforeafter", content)
		})
	}
}

func TestConvertGeminiToClaudeMessageOmitsInlineDataForAnthropicMessages(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "before"},
				map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}},
				map[string]any{"functionCall": map[string]any{"name": "get_weather", "args": map[string]any{"city": "Paris"}}},
				map[string]any{"text": "after"},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	withInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, true)
	contentWithInlineData, ok := withInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithInlineData, 4)
	require.Equal(t, map[string]any{"type": "text", "text": "before"}, contentWithInlineData[0])
	require.Equal(t, map[string]any{"type": "text", "text": "![image](data:image/png;base64,aW1hZ2U=)"}, contentWithInlineData[1])
	toolUse, ok := contentWithInlineData[2].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUse["type"])
	require.Equal(t, "get_weather", toolUse["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"}, contentWithInlineData[3])

	withoutInlineData, _ := convertGeminiToClaudeMessage(geminiResp, "gemini-test", rawData, false)
	contentWithoutInlineData, ok := withoutInlineData["content"].([]any)
	require.True(t, ok)
	require.Len(t, contentWithoutInlineData, 3)
	require.Equal(t, map[string]any{"type": "text", "text": "before"}, contentWithoutInlineData[0])
	toolUseWithoutInlineData, ok := contentWithoutInlineData[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "tool_use", toolUseWithoutInlineData["type"])
	require.Equal(t, "get_weather", toolUseWithoutInlineData["name"])
	require.Equal(t, map[string]any{"type": "text", "text": "after"}, contentWithoutInlineData[2])
}

func TestGeminiResponseToChatCompletionsRetainsTextAndToolBehavior(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{
				map[string]any{"text": "checking"},
				map[string]any{"functionCall": map[string]any{
					"name": "get_weather",
					"args": map[string]any{"city": "Paris"},
				}},
			}},
			"finishReason": "STOP",
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	got, _, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.NoError(t, err)
	require.Len(t, got.Choices, 1)

	choice := got.Choices[0]
	var content string
	require.NoError(t, json.Unmarshal(choice.Message.Content, &content))
	require.Equal(t, "checking", content)
	require.Equal(t, "tool_calls", choice.FinishReason)
	require.Len(t, choice.Message.ToolCalls, 1)
	require.Equal(t, "get_weather", choice.Message.ToolCalls[0].Function.Name)
	require.JSONEq(t, `{"city":"Paris"}`, choice.Message.ToolCalls[0].Function.Arguments)
}

func TestGeminiResponseToChatCompletionsRejectsMissingFinishReason(t *testing.T) {
	geminiResp := map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{map[string]any{"text": "partial"}}},
		}},
	}
	rawData, err := json.Marshal(geminiResp)
	require.NoError(t, err)

	got, usage, err := geminiResponseToChatCompletions(geminiResp, "gemini-test", rawData, nil)
	require.ErrorIs(t, err, errGeminiStreamMissingTerminal)
	require.Nil(t, got)
	require.Nil(t, usage)
}

func TestCollectGeminiSSERequiresFinishReasonEvenWithDone(t *testing.T) {
	body := strings.NewReader("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\ndata: [DONE]\n\n")

	result, usage, err := collectGeminiSSE(body, false)
	require.ErrorIs(t, err, errGeminiStreamMissingTerminal)
	require.Nil(t, result)
	require.NotNil(t, usage)
}

func TestCollectGeminiSSEReturnsOnFinishReasonWithoutEOF(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = writer.Close()
		_ = reader.Close()
	})

	type outcome struct {
		result map[string]any
		usage  *ClaudeUsage
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, usage, err := collectGeminiSSE(reader, false)
		done <- outcome{result: result, usage: usage, err: err}
	}()

	_, err := io.WriteString(writer, strings.Join([]string{
		`data: {"candidates":[{"content":{"parts":[{"text":"hello "}]}}]}`,
		"",
		`data: {"candidates":[{"content":{"parts":[{"text":"world"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":13,"candidatesTokenCount":6}}`,
		"",
	}, "\n"))
	require.NoError(t, err)

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.NotNil(t, got.usage)
		require.Equal(t, 13, got.usage.InputTokens)
		require.Equal(t, 6, got.usage.OutputTokens)
		require.Equal(t, "STOP", extractGeminiFinishReason(got.result))
		parts := extractGeminiParts(got.result)
		require.NotEmpty(t, parts)
		require.Equal(t, "hello world", parts[0]["text"])
	case <-time.After(time.Second):
		t.Fatal("collectGeminiSSE waited for [DONE] or EOF after finishReason")
	}
}

func TestGeminiChatStreamingPartialEOFEmitsExplicitErrorWithoutDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n",
		)),
	}
	svc := &GeminiMessagesCompatService{cfg: &config.Config{}}

	result, err := svc.handleChatCompletionsStreamingResponseFromGemini(
		context.Background(), c, resp, &Account{ID: 7, Platform: PlatformGemini},
		"gemini-request", time.Now(), "gemini-test", false, false, nil,
	)

	require.ErrorIs(t, err, errGeminiStreamMissingTerminal)
	require.NotNil(t, result)
	require.False(t, result.clientDisconnected)
	require.Contains(t, rec.Body.String(), `"content":"partial"`)
	require.Contains(t, rec.Body.String(), `"type":"upstream_error"`)
	require.NotContains(t, rec.Body.String(), "data: [DONE]")
}

func TestGeminiChatStreamingClientDisconnectStillCollectsTerminalUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &geminiChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`,
			"",
			`data: {"candidates":[{"content":{"parts":[{"text":"partial"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":4}}`,
			"",
		}, "\n"))),
	}
	svc := &GeminiMessagesCompatService{cfg: &config.Config{}}

	result, err := svc.handleChatCompletionsStreamingResponseFromGemini(
		context.Background(), c, resp, &Account{ID: 8, Platform: PlatformGemini},
		"gemini-request", time.Now(), "gemini-test", false, true, nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.clientDisconnected)
	require.Equal(t, 9, result.usage.InputTokens)
	require.Equal(t, 4, result.usage.OutputTokens)
	require.Empty(t, rec.Body.String())
}

func TestGeminiChatStreamingReturnsOnFinishReasonWithoutEOF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamReader, upstreamWriter := io.Pipe()
	t.Cleanup(func() {
		_ = upstreamWriter.Close()
		_ = upstreamReader.Close()
	})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: upstreamReader}
	svc := &GeminiMessagesCompatService{cfg: &config.Config{}}

	type outcome struct {
		result *geminiStreamResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.handleChatCompletionsStreamingResponseFromGemini(
			context.Background(), c, resp, &Account{ID: 8, Platform: PlatformGemini},
			"gemini-request", time.Now(), "gemini-test", false, true, nil,
		)
		done <- outcome{result: result, err: err}
	}()

	_, err := io.WriteString(upstreamWriter, `data: {"candidates":[{"content":{"parts":[{"text":"complete"}]} ,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":5}}`+"\n\n")
	require.NoError(t, err)
	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		require.Equal(t, 11, got.result.usage.InputTokens)
		require.Equal(t, 5, got.result.usage.OutputTokens)
		require.Contains(t, rec.Body.String(), "data: [DONE]")
	case <-time.After(time.Second):
		t.Fatal("Gemini chat stream waited for EOF after finishReason")
	}
}

func TestGeminiChatStreamingWriteFailureStartsBoundedDrain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstreamReader, upstreamWriter := io.Pipe()
	t.Cleanup(func() {
		_ = upstreamWriter.Close()
		_ = upstreamReader.Close()
	})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Writer = &geminiChatFailingWriter{ResponseWriter: c.Writer, failAfter: 0}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: upstreamReader}
	guard := startClientDisconnectDrainGuardWithTimeout(context.Background(), upstreamReader, 20*time.Millisecond)
	defer guard.Stop()
	svc := &GeminiMessagesCompatService{cfg: &config.Config{}}

	type outcome struct {
		result *geminiStreamResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := svc.handleChatCompletionsStreamingResponseFromGemini(
			context.Background(), c, resp, &Account{ID: 8, Platform: PlatformGemini},
			"gemini-request", time.Now(), "gemini-test", false, false, guard,
		)
		done <- outcome{result: result, err: err}
	}()

	_, err := io.WriteString(upstreamWriter, `data: {"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`+"\n\n")
	require.NoError(t, err)
	select {
	case got := <-done:
		require.Error(t, got.err)
		require.NotNil(t, got.result)
		require.True(t, got.result.clientDisconnected)
	case <-time.After(time.Second):
		t.Fatal("Gemini chat stream remained blocked after downstream write failure")
	}
}

func TestGeminiForwardAsChatCompletionsMissingFinishReasonDoesNotRetryAfterPartialOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gemini-test","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n",
		)),
	}}
	svc := &GeminiMessagesCompatService{cfg: &config.Config{}, httpUpstream: httpStub}
	account := &Account{ID: 9, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "test"}}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body)

	require.ErrorIs(t, err, errGeminiStreamMissingTerminal)
	require.NotNil(t, result)
	require.False(t, result.ClientDisconnect)
	require.Equal(t, 1, httpStub.calls)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestGeminiForwardAsChatCompletionsClientCancelKeepsUpstreamDrainAlive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	body := []byte(`{"model":"gemini-test","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(reqCtx)
	httpStub := &geminiCompatHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`data: {"candidates":[{"content":{"parts":[{"text":"done"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":2}}` + "\n\n",
		)),
	}}
	svc := &GeminiMessagesCompatService{cfg: &config.Config{}, httpUpstream: httpStub}
	account := &Account{ID: 10, Platform: PlatformGemini, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "test"}}

	result, err := svc.ForwardAsChatCompletions(reqCtx, c, account, body)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 7, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.NotNil(t, httpStub.lastReq)
	require.NoError(t, httpStub.lastReq.Context().Err())
}
