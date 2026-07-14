package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIFirstResponseCommitWriter struct {
	gin.ResponseWriter
	committed chan struct{}
	once      sync.Once
}

func (w *openAIFirstResponseCommitWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.committed) })
	return w.ResponseWriter.Write(p)
}

func (w *openAIFirstResponseCommitWriter) WriteString(s string) (int, error) {
	w.once.Do(func() { close(w.committed) })
	return w.ResponseWriter.WriteString(s)
}

func newOpenAIFirstResponseOverflowContext(t *testing.T, path string) (*gin.Context, <-chan struct{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	writer := &openAIFirstResponseCommitWriter{
		ResponseWriter: c.Writer,
		committed:      make(chan struct{}),
	}
	c.Writer = writer
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c, writer.committed
}

func openAIFirstResponseOverflowPadding() string {
	return strings.Repeat("x", openAIFirstResponseMaxBufferedBytes+1024)
}

func awaitOpenAIFirstResponseOverflowCommit(
	t *testing.T,
	committed <-chan struct{},
	body io.Closer,
	done <-chan error,
) error {
	t.Helper()
	select {
	case <-committed:
	case <-time.After(2 * time.Second):
		_ = body.Close()
		t.Fatal("oversized first-response preamble was not committed before the failover timeout")
	}
	require.NoError(t, body.Close())
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("stream handler did not stop after the upstream body was closed")
		return nil
	}
}

func requireNoFirstResponseFailover(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	var failoverErr *UpstreamFailoverError
	require.NotErrorAs(t, err, &failoverErr, "visible output must commit the current account")
}

func TestOpenAIFirstResponseOverflow_CommitsResponsesStream(t *testing.T) {
	c, committed := newOpenAIFirstResponseOverflowContext(t, "/v1/responses")
	body := newOpenAICompatBlockingReadCloser([]byte(
		`data: {"type":"response.created","response":{"id":"resp_overflow","status":"in_progress","metadata":{"padding":"` +
			openAIFirstResponseOverflowPadding() + `"}}}` + "\n\n",
	))
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	done := make(chan error, 1)
	go func() {
		_, err := (&OpenAIGatewayService{}).handleStreamingResponse(
			WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Second),
			resp,
			c,
			&Account{ID: 301, Name: "responses-overflow", Platform: PlatformOpenAI},
			time.Now(),
			"gpt-5.6-luna",
			"gpt-5.6-luna",
		)
		done <- err
	}()

	requireNoFirstResponseFailover(t, awaitOpenAIFirstResponseOverflowCommit(t, committed, body, done))
}

func TestOpenAIFirstResponseOverflow_CommitsResponsesPassthrough(t *testing.T) {
	c, committed := newOpenAIFirstResponseOverflowContext(t, "/v1/responses")
	body := newOpenAICompatBlockingReadCloser([]byte(
		`data: {"type":"response.created","response":{"id":"resp_overflow","status":"in_progress","metadata":{"padding":"` +
			openAIFirstResponseOverflowPadding() + `"}}}` + "\n\n",
	))
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	done := make(chan error, 1)
	go func() {
		_, err := (&OpenAIGatewayService{}).handleStreamingResponsePassthrough(
			WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Second),
			resp,
			c,
			&Account{ID: 302, Name: "passthrough-overflow", Platform: PlatformOpenAI},
			time.Now(),
			"gpt-5.6-luna",
			"gpt-5.6-luna",
		)
		done <- err
	}()

	requireNoFirstResponseFailover(t, awaitOpenAIFirstResponseOverflowCommit(t, committed, body, done))
}

func TestOpenAIFirstResponseOverflow_CommitsConvertedChatStream(t *testing.T) {
	c, committed := newOpenAIFirstResponseOverflowContext(t, "/v1/chat/completions")
	body := newOpenAICompatBlockingReadCloser([]byte(
		`data: {"type":"response.created","response":{"id":"resp_overflow","status":"in_progress","model":"` +
			openAIFirstResponseOverflowPadding() + `"}}` + "\n\n",
	))
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	done := make(chan error, 1)
	go func() {
		_, err := (&OpenAIGatewayService{}).handleChatStreamingResponse(
			WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Second),
			resp,
			c,
			&Account{ID: 303, Name: "chat-overflow", Platform: PlatformOpenAI},
			"",
			"gpt-5.6-luna",
			"gpt-5.6-luna",
			time.Now(),
			0,
		)
		done <- err
	}()

	requireNoFirstResponseFailover(t, awaitOpenAIFirstResponseOverflowCommit(t, committed, body, done))
}

func TestOpenAIFirstResponseOverflow_CommitsRawChatStream(t *testing.T) {
	c, committed := newOpenAIFirstResponseOverflowContext(t, "/v1/chat/completions")
	model := openAIFirstResponseOverflowPadding()
	body := newOpenAICompatBlockingReadCloser([]byte(
		`data: {"id":"chatcmpl_overflow","object":"chat.completion.chunk","model":"` + model +
			`","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n",
	))
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	done := make(chan error, 1)
	go func() {
		_, err := (&OpenAIGatewayService{}).streamRawChatCompletions(
			WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Second),
			c,
			resp,
			&Account{ID: 304, Name: "raw-chat-overflow", Platform: PlatformOpenAI},
			model,
			model,
			model,
			nil,
			nil,
			time.Now(),
			0,
		)
		done <- err
	}()

	requireNoFirstResponseFailover(t, awaitOpenAIFirstResponseOverflowCommit(t, committed, body, done))
}

func TestOpenAIFirstResponseOverflow_CommitsAnthropicStream(t *testing.T) {
	c, committed := newOpenAIFirstResponseOverflowContext(t, "/v1/messages")
	body := newOpenAICompatBlockingReadCloser([]byte(
		`data: {"type":"response.created","response":{"id":"` + openAIFirstResponseOverflowPadding() +
			`","status":"in_progress","model":"gpt-5.6-luna"}}` + "\n\n",
	))
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	done := make(chan error, 1)
	go func() {
		_, err := (&OpenAIGatewayService{}).handleAnthropicStreamingResponse(
			WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Second),
			resp,
			c,
			&Account{ID: 305, Name: "anthropic-overflow", Platform: PlatformOpenAI},
			"gpt-5.6-luna",
			"gpt-5.6-luna",
			"gpt-5.6-luna",
			time.Now(),
		)
		done <- err
	}()

	requireNoFirstResponseFailover(t, awaitOpenAIFirstResponseOverflowCommit(t, committed, body, done))
}

func TestOpenAIFirstResponseOverflow_CommitsChatFallbackChunk(t *testing.T) {
	c, _ := newOpenAIFirstResponseOverflowContext(t, "/v1/responses")
	body := newOpenAICompatBlockingReadCloser([]byte(
		`data: {"id":"chatcmpl_overflow","object":"chat.completion.chunk","model":"` +
			openAIFirstResponseOverflowPadding() +
			`","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n",
	))
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body}
	emitted := make(chan struct{}, 1)
	done := make(chan ccStreamScanState, 1)
	go func() {
		state := (&OpenAIGatewayService{}).scanCCStream(
			WithOpenAIFirstResponseTimeout(context.Background(), 10*time.Second),
			c,
			&Account{ID: 306, Name: "chat-fallback-overflow", Platform: PlatformOpenAI},
			resp,
			"first response overflow test",
			"rid_overflow",
			time.Now(),
			func(*apicompat.ChatCompletionsChunk) {
				select {
				case emitted <- struct{}{}:
				default:
				}
			},
		)
		done <- state
	}()

	select {
	case <-emitted:
	case <-time.After(2 * time.Second):
		_ = body.Close()
		t.Fatal("oversized fallback chunk was not emitted before the failover timeout")
	}
	require.NoError(t, body.Close())
	select {
	case state := <-done:
		requireNoFirstResponseFailover(t, state.Err)
	case <-time.After(2 * time.Second):
		t.Fatal("fallback stream scanner did not stop after the upstream body was closed")
	}
}
