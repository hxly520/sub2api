package service

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	openAIFirstTokenStartContextKey    = "openai_first_token_start"
	openAIFirstTokenAcceptedContextKey = "openai_first_token_accepted_ms"
)

type openAIFastFirstTokenTimingContextKey struct{}

// WithOpenAIFastFirstTokenTiming is applied only when the global
// openai_first_response_enabled runtime setting is on. Account-level relay
// options must never enable or disable this timing policy.
func WithOpenAIFastFirstTokenTiming(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIFastFirstTokenTimingContextKey{}, true)
}

func openAIFastFirstTokenTimingFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	enabled, _ := ctx.Value(openAIFastFirstTokenTimingContextKey{}).(bool)
	return enabled
}

func setOpenAIFirstTokenStart(c *gin.Context, startedAt time.Time) {
	if c == nil || startedAt.IsZero() {
		return
	}
	c.Set(openAIFirstTokenStartContextKey, startedAt)
	// Every upstream attempt owns its timing sample. Clear a previous accepted
	// response so a failed attempt can never leak into the final account's TTFT.
	c.Set(openAIFirstTokenAcceptedContextKey, nil)
}

func openAIFirstTokenStart(c *gin.Context, fallback time.Time) time.Time {
	if c != nil {
		if value, ok := c.Get(openAIFirstTokenStartContextKey); ok {
			if startedAt, ok := value.(time.Time); ok && !startedAt.IsZero() {
				return startedAt
			}
		}
	}
	return fallback
}

// markOpenAIFirstTokenAccepted records the earliest reliable success signal:
// the final upstream attempt returned a 2xx response header. This matches the
// first-response convention used by upstream relays without counting account
// selection, queueing, or failed attempts.
func markOpenAIFirstTokenAccepted(ctx context.Context, c *gin.Context, statusCode int, acceptedAt time.Time) {
	if !openAIFastFirstTokenTimingFromContext(ctx) || c == nil || statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices || acceptedAt.IsZero() {
		return
	}
	startedAt := openAIFirstTokenStart(c, time.Time{})
	if startedAt.IsZero() || acceptedAt.Before(startedAt) {
		return
	}
	elapsed := int(acceptedAt.Sub(startedAt).Milliseconds())
	c.Set(openAIFirstTokenAcceptedContextKey, elapsed)
}

func openAIFirstTokenAccepted(c *gin.Context) *int {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIFirstTokenAcceptedContextKey)
	if !ok {
		return nil
	}
	elapsed, ok := value.(int)
	if !ok || elapsed < 0 {
		return nil
	}
	return &elapsed
}

// doOpenAIUpstreamWithFirstTokenTiming wraps the official transport without
// changing its retry, plugin, protocol, or error semantics. When the global
// TTFT option marked the request context, it records only the final attempt's
// real upstream round-trip to a successful 2xx response header.
func (s *OpenAIGatewayService) doOpenAIUpstreamWithFirstTokenTiming(
	ctx context.Context,
	c *gin.Context,
	request *http.Request,
	proxyURL string,
	account *Account,
) (*http.Response, error) {
	if request == nil {
		return s.doOpenAIUpstream(request, proxyURL, account)
	}

	// Detect the opt-in marker without modifying request.Context. Streaming
	// requests deliberately use context.WithoutCancel in several official
	// forwarding paths so usage can be drained after a client disconnect.
	// Replacing that context here would change transport and billing semantics.
	timingCtx := ctx
	if openAIFastFirstTokenTimingFromContext(request.Context()) {
		timingCtx = request.Context()
	}
	if !openAIFastFirstTokenTimingFromContext(timingCtx) {
		return s.doOpenAIUpstream(request, proxyURL, account)
	}

	startedAt := time.Now()
	setOpenAIFirstTokenStart(c, startedAt)
	resp, err := s.doOpenAIUpstream(request, proxyURL, account)
	if err == nil && resp != nil {
		markOpenAIFirstTokenAccepted(timingCtx, c, resp.StatusCode, time.Now())
	}
	return resp, err
}
