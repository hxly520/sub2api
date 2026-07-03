package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type openAIFirstResponseTimeoutContextKey struct{}

// WithOpenAIFirstResponseTimeout enables a first-SSE/data-payload timeout for a
// single upstream attempt. Handlers should only set it when a same-group
// alternative account is actually available.
func WithOpenAIFirstResponseTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, openAIFirstResponseTimeoutContextKey{}, timeout)
}

func openAIFirstResponseTimeoutFromContext(ctx context.Context) time.Duration {
	if ctx == nil {
		return 0
	}
	timeout, _ := ctx.Value(openAIFirstResponseTimeoutContextKey{}).(time.Duration)
	if timeout <= 0 {
		return 0
	}
	return timeout
}

type openAIFirstResponseTimeoutWatch struct {
	timeout  time.Duration
	body     io.Closer
	timer    *time.Timer
	observed atomic.Bool
	timedOut atomic.Bool
}

func newOpenAIFirstResponseTimeoutWatch(ctx context.Context, body io.Closer) *openAIFirstResponseTimeoutWatch {
	timeout := openAIFirstResponseTimeoutFromContext(ctx)
	if timeout <= 0 || body == nil {
		return nil
	}
	w := &openAIFirstResponseTimeoutWatch{
		timeout: timeout,
		body:    body,
	}
	w.timer = time.AfterFunc(timeout, func() {
		if w.observed.Load() {
			return
		}
		w.timedOut.Store(true)
		_ = body.Close()
	})
	return w
}

func (w *openAIFirstResponseTimeoutWatch) Stop() {
	if w == nil || w.timer == nil {
		return
	}
	w.timer.Stop()
}

func (w *openAIFirstResponseTimeoutWatch) TimedOut() bool {
	return w != nil && w.timedOut.Load()
}

func (w *openAIFirstResponseTimeoutWatch) ObservePayload(payload string) {
	if w == nil || !openAIStreamPayloadCountsAsFirstResponse(payload) {
		return
	}
	if w.observed.CompareAndSwap(false, true) && w.timer != nil {
		w.timer.Stop()
	}
}

func (w *openAIFirstResponseTimeoutWatch) ObserveLine(line string) {
	if w == nil {
		return
	}
	if payload, ok := extractOpenAISSEDataLine(line); ok {
		w.ObservePayload(payload)
	}
}

func (w *openAIFirstResponseTimeoutWatch) failoverErrorIfTimedOut(
	s *OpenAIGatewayService,
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	clientOutputStarted bool,
) *UpstreamFailoverError {
	if w == nil || s == nil || !w.TimedOut() {
		return nil
	}
	if openAIStreamClientOutputStarted(c, clientOutputStarted) {
		return nil
	}
	return s.newOpenAIFirstResponseTimeoutFailoverError(
		c,
		account,
		passthrough,
		upstreamRequestID,
		w.timeout,
	)
}

func openAIStreamPayloadCountsAsFirstResponse(payload string) bool {
	trimmed := strings.TrimSpace(payload)
	return trimmed != "" && trimmed != "[DONE]"
}

func (s *OpenAIGatewayService) newOpenAIFirstResponseTimeoutFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	timeout time.Duration,
) *UpstreamFailoverError {
	timeoutMs := int(timeout.Milliseconds())
	if timeoutMs <= 0 {
		timeoutMs = 1
	}
	message := fmt.Sprintf("OpenAI upstream did not emit the first stream event within %dms", timeoutMs)
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_timeout",
			"message": message,
		},
	})
	if c != nil {
		setOpsUpstreamError(c, http.StatusGatewayTimeout, message, "")
		event := OpsUpstreamErrorEvent{
			Platform:           PlatformOpenAI,
			UpstreamStatusCode: http.StatusGatewayTimeout,
			UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
			Passthrough:        passthrough,
			Kind:               "first_response_timeout",
			Message:            message,
		}
		if account != nil {
			event.Platform = account.Platform
			event.AccountID = account.ID
			event.AccountName = account.Name
		}
		appendOpsUpstreamError(c, event)
	}
	return &UpstreamFailoverError{
		StatusCode:             http.StatusGatewayTimeout,
		ResponseBody:           body,
		FirstResponseTimeout:   true,
		FirstResponseTimeoutMs: timeoutMs,
	}
}
