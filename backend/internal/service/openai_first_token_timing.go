package service

import (
	"context"
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
	if !openAIFastFirstTokenTimingFromContext(ctx) || c == nil || statusCode < 200 || statusCode >= 300 || acceptedAt.IsZero() {
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
