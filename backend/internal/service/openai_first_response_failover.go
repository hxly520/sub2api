package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAIFirstResponseTimeoutContextKey struct{}

// openAIFirstResponseBudget is shared by the upstream round trip and the SSE
// reader. The deadline starts lazily immediately before the selected account's
// first upstream request, so parsing and account selection do not consume the
// failover window. One deadline also prevents a slow response-header phase and
// a slow SSE preamble phase from each receiving a full timeout.
type openAIFirstResponseBudget struct {
	timeout  time.Duration
	mu       sync.Mutex
	started  time.Time
	deadline time.Time
}

func (b *openAIFirstResponseBudget) remaining() (time.Duration, time.Duration) {
	if b == nil || b.timeout <= 0 {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started.IsZero() {
		b.started = time.Now()
		b.deadline = b.started.Add(b.timeout)
	}
	return b.timeout, time.Until(b.deadline)
}

func WithOpenAIFirstResponseTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx
	}
	return context.WithValue(ctx, openAIFirstResponseTimeoutContextKey{}, &openAIFirstResponseBudget{timeout: timeout})
}

func openAIFirstResponseBudgetFromContext(ctx context.Context) *openAIFirstResponseBudget {
	if ctx == nil {
		return nil
	}
	budget, _ := ctx.Value(openAIFirstResponseTimeoutContextKey{}).(*openAIFirstResponseBudget)
	if budget == nil || budget.timeout <= 0 {
		return nil
	}
	return budget
}

const (
	openAIFirstResponseWaiting int32 = iota
	openAIFirstResponseObserved
	openAIFirstResponseTimedOut
)

// A Responses preamble is normally tiny. Keep an explicit bounded queue before
// semantic output so bufio.Writer cannot auto-flush a large response.created
// event and make a later account retry visible to the client. If an unusual
// upstream exceeds the bound, the caller commits that stream instead of
// retaining unbounded memory or attempting an unsafe failover.
const openAIFirstResponseMaxBufferedBytes = 2 * 1024 * 1024

// reserveOpenAIFirstResponseBuffer keeps every pre-output queue bounded by the
// same limit. Callers pass the serialized byte size before retaining an item.
// Once the next item would exceed the cap, the current account is committed:
// the timeout watch is stopped and the caller must flush its queued items plus
// the current item instead of attempting a later account failover.
func reserveOpenAIFirstResponseBuffer(watch *openAIFirstResponseTimeoutWatch, bufferedBytes *int, itemBytes int) bool {
	if bufferedBytes == nil {
		if watch != nil {
			watch.observe()
		}
		return false
	}
	if itemBytes < 0 {
		itemBytes = 0
	}
	if *bufferedBytes >= 0 && itemBytes <= openAIFirstResponseMaxBufferedBytes-*bufferedBytes {
		*bufferedBytes += itemBytes
		return true
	}
	if watch != nil {
		watch.observe()
	}
	return false
}

type openAIFirstResponseTimeoutWatch struct {
	timeout time.Duration
	body    io.Closer
	timer   *time.Timer
	state   atomic.Int32
}

func newOpenAIFirstResponseTimeoutWatch(ctx context.Context, body io.Closer) *openAIFirstResponseTimeoutWatch {
	budget := openAIFirstResponseBudgetFromContext(ctx)
	if budget == nil || body == nil {
		return nil
	}
	timeout, remaining := budget.remaining()
	watch := &openAIFirstResponseTimeoutWatch{timeout: timeout, body: body}
	if remaining <= 0 {
		watch.state.Store(openAIFirstResponseTimedOut)
		_ = body.Close()
		return watch
	}
	watch.timer = time.AfterFunc(remaining, func() {
		if watch.state.CompareAndSwap(openAIFirstResponseWaiting, openAIFirstResponseTimedOut) {
			_ = body.Close()
		}
	})
	return watch
}

type openAIFirstResponseHeaderGuard struct {
	timeout time.Duration
	timer   *time.Timer
	cancel  context.CancelFunc
	state   atomic.Int32
}

const (
	openAIFirstResponseHeaderWaiting int32 = iota
	openAIFirstResponseHeaderCompleted
	openAIFirstResponseHeaderTimedOut
)

func newOpenAIFirstResponseHeaderGuard(ctx context.Context, req *http.Request) (*http.Request, *openAIFirstResponseHeaderGuard) {
	budget := openAIFirstResponseBudgetFromContext(ctx)
	if budget == nil || req == nil {
		return req, nil
	}
	timeout, remaining := budget.remaining()
	reqCtx, cancel := context.WithCancel(req.Context())
	guard := &openAIFirstResponseHeaderGuard{timeout: timeout, cancel: cancel}
	if remaining <= 0 {
		guard.state.Store(openAIFirstResponseHeaderTimedOut)
		cancel()
		return req.WithContext(reqCtx), guard
	}
	guard.timer = time.AfterFunc(remaining, func() {
		if guard.state.CompareAndSwap(openAIFirstResponseHeaderWaiting, openAIFirstResponseHeaderTimedOut) {
			cancel()
		}
	})
	return req.WithContext(reqCtx), guard
}

func (g *openAIFirstResponseHeaderGuard) finish(resp *http.Response) bool {
	if g == nil {
		return false
	}
	completed := g.state.CompareAndSwap(openAIFirstResponseHeaderWaiting, openAIFirstResponseHeaderCompleted)
	if completed && g.timer != nil {
		g.timer.Stop()
	}
	if !completed && g.state.Load() == openAIFirstResponseHeaderTimedOut {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		g.cancel()
		return true
	}
	if resp == nil || resp.Body == nil {
		g.cancel()
		return false
	}
	resp.Body = &openAICancelOnCloseReadCloser{ReadCloser: resp.Body, cancel: g.cancel}
	return false
}

type openAICancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (r *openAICancelOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.once.Do(r.cancel)
	return err
}

// doOpenAIUpstreamWithFirstResponseBudget covers response-header wait with the
// same budget later consumed by the SSE semantic-output watcher. Requests that
// have no first-response budget retain the existing transport behavior.
func (s *OpenAIGatewayService) doOpenAIUpstreamWithFirstResponseBudget(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	req *http.Request,
	proxyURL string,
	passthrough bool,
) (*http.Response, error) {
	if s == nil || s.httpUpstream == nil || account == nil {
		return nil, errors.New("OpenAI upstream transport is unavailable")
	}
	guardedReq, guard := newOpenAIFirstResponseHeaderGuard(ctx, req)
	setOpenAIFirstTokenStart(c, time.Now())
	resp, err := s.httpUpstream.Do(guardedReq, proxyURL, account.ID, account.Concurrency)
	if guard != nil && guard.finish(resp) {
		return nil, s.newOpenAIFirstResponseTimeoutFailoverError(c, account, passthrough, "", guard.timeout)
	}
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, passthrough)
	}
	if resp != nil {
		markOpenAIFirstTokenAccepted(ctx, c, resp.StatusCode, time.Now())
	}
	return resp, nil
}

func (w *openAIFirstResponseTimeoutWatch) Stop() {
	if w == nil || w.timer == nil {
		return
	}
	w.timer.Stop()
}

func (w *openAIFirstResponseTimeoutWatch) TimedOut() bool {
	return w != nil && w.state.Load() == openAIFirstResponseTimedOut
}

func (w *openAIFirstResponseTimeoutWatch) Waiting() bool {
	return w != nil && w.state.Load() == openAIFirstResponseWaiting
}

func (w *openAIFirstResponseTimeoutWatch) observe() {
	if w == nil {
		return
	}
	if w.state.CompareAndSwap(openAIFirstResponseWaiting, openAIFirstResponseObserved) && w.timer != nil {
		w.timer.Stop()
	}
}

func (w *openAIFirstResponseTimeoutWatch) ObservePayload(payload string) {
	if w != nil && openAIStreamPayloadCountsAsFirstResponse(payload) {
		w.observe()
	}
}

func (w *openAIFirstResponseTimeoutWatch) ObserveLine(line string) {
	if w == nil {
		return
	}
	if payload, ok := extractOpenAISSEDataLine(line); ok {
		w.ObservePayload(payload)
		return
	}
	if openAIStreamEventLineCountsAsFirstResponse(line) {
		w.observe()
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
	if w == nil || s == nil || !w.TimedOut() || openAIStreamClientOutputStarted(c, clientOutputStarted) {
		return nil
	}
	return s.newOpenAIFirstResponseTimeoutFailoverError(c, account, passthrough, upstreamRequestID, w.timeout)
}

func openAIStreamPayloadCountsAsFirstResponse(payload string) bool {
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" || trimmed == "[DONE]" || isOpenAIChatUsageOnlyStreamChunk(trimmed) {
		return false
	}
	eventType := strings.TrimSpace(gjson.Get(trimmed, "type").String())
	if openAIStreamEventIsPreamble(eventType) {
		return false
	}
	// A failed terminal response is still an observed upstream response. Its
	// retryability is decided by the normal response.failed path, not by a timer
	// racing while that error is being parsed.
	if eventType == "response.failed" {
		return true
	}
	if choices := gjson.Get(trimmed, "choices"); choices.Exists() && choices.IsArray() {
		for _, choice := range choices.Array() {
			if finishReason := choice.Get("finish_reason"); openAIStreamJSONValueHasContent(finishReason) {
				return true
			}
			delta := choice.Get("delta")
			if openAIStreamJSONValueHasContent(delta.Get("content")) ||
				openAIStreamJSONValueHasContent(delta.Get("reasoning")) ||
				openAIStreamJSONValueHasContent(delta.Get("reasoning_content")) ||
				openAIStreamJSONValueHasContent(delta.Get("reasoning_summary")) ||
				openAIStreamJSONValueHasContent(delta.Get("refusal")) ||
				openAIStreamJSONValueHasContent(delta.Get("audio")) ||
				openAIStreamJSONValueHasContent(delta.Get("tool_calls")) ||
				openAIStreamJSONValueHasContent(delta.Get("function_call")) {
				return true
			}
		}
		return false
	}
	if eventType == "" {
		return openAIStreamJSONValueHasContent(gjson.Get(trimmed, "error"))
	}
	return openAIResponsesPayloadStartsSemanticOutput(trimmed, eventType)
}

func openAIStreamJSONValueHasContent(value gjson.Result) bool {
	if !value.Exists() || value.Type == gjson.Null {
		return false
	}
	switch value.Type {
	case gjson.String:
		return strings.TrimSpace(value.String()) != ""
	case gjson.JSON:
		trimmed := strings.TrimSpace(value.Raw)
		return trimmed != "" && trimmed != "[]" && trimmed != "{}" && trimmed != "null"
	default:
		return true
	}
}

func openAIResponsesPayloadStartsSemanticOutput(payload, eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	switch eventType {
	case "response.completed", "response.done", "response.incomplete",
		"response.cancelled", "response.canceled", "error":
		return true
	case "response.output_text.delta", "response.output_text.done",
		"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done",
		"response.reasoning_text.delta", "response.refusal.delta", "response.refusal.done",
		"response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.custom_tool_call_input.delta", "response.custom_tool_call_input.done",
		"response.output_audio.delta", "response.audio_transcript.delta":
		for _, path := range []string{"delta", "text", "arguments", "input", "transcript"} {
			if openAIStreamJSONValueHasContent(gjson.Get(payload, path)) {
				return true
			}
		}
		return false
	case "response.output_item.added", "response.output_item.done":
		return openAIResponsesOutputItemIsSemantic(gjson.Get(payload, "item"))
	case "response.image_generation_call.partial_image":
		return openAIStreamJSONValueHasContent(gjson.Get(payload, "partial_image_b64")) ||
			openAIStreamJSONValueHasContent(gjson.Get(payload, "b64_json"))
	}

	// Preserve compatibility with future delta events without treating structural
	// lifecycle metadata as user-visible output.
	if strings.HasSuffix(eventType, ".delta") {
		return openAIStreamJSONValueHasContent(gjson.Get(payload, "delta"))
	}
	return false
}

func openAIResponsesOutputItemIsSemantic(item gjson.Result) bool {
	if !item.Exists() || !item.IsObject() {
		return false
	}
	itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
	if strings.Contains(itemType, "call") || strings.Contains(itemType, "tool") {
		return itemType != ""
	}
	for _, path := range []string{"arguments", "input", "text", "output", "result"} {
		if openAIStreamJSONValueHasContent(item.Get(path)) {
			return true
		}
	}
	for _, path := range []string{"content", "summary"} {
		parts := item.Get(path)
		if !parts.Exists() || !parts.IsArray() {
			continue
		}
		for _, part := range parts.Array() {
			if openAIStreamJSONValueHasContent(part.Get("text")) ||
				openAIStreamJSONValueHasContent(part.Get("refusal")) ||
				openAIStreamJSONValueHasContent(part.Get("output")) {
				return true
			}
		}
	}
	return false
}

func openAIStreamEventLineCountsAsFirstResponse(line string) bool {
	// An event field without its data field is not a semantic response and must
	// not consume the failover opportunity.
	return false
}

func (s *OpenAIGatewayService) newOpenAIFirstResponseTimeoutFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	timeout time.Duration,
) *UpstreamFailoverError {
	timeoutMS := int(timeout.Milliseconds())
	if timeoutMS <= 0 {
		timeoutMS = 1
	}
	message := fmt.Sprintf("OpenAI upstream did not emit the first stream event within %dms", timeoutMS)
	body, _ := json.Marshal(gin.H{"error": gin.H{"type": "upstream_timeout", "message": message}})
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
		FirstResponseTimeoutMs: timeoutMS,
	}
}
