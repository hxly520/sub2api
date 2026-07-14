package service

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

type openAIAccountRuntimeBlock struct {
	Until  time.Time
	Reason string
}

const (
	openAIAccountStateUpdateTimeout       = 5 * time.Second
	openAIOAuth429FallbackCooldown        = 5 * time.Second
	openAIStopSchedulingBridgeCooldown    = 2 * time.Minute
	openAIOAuth429StormWindow             = 10 * time.Second
	openAIOAuth429StormThreshold          = 20
	openAIOAuth429StormMaxAccountSwitches = 1
)

func openAIAccountStateContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, openAIAccountStateUpdateTimeout)
}

func isOpenAIOAuthAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth
}

func isGrokOAuthAccount(account *Account) bool {
	return account != nil && account.Platform == PlatformGrok && account.Type == AccountTypeOAuth
}

func isOpenAIAccount(account *Account) bool {
	return account != nil && (account.Platform == PlatformOpenAI || account.Platform == PlatformGrok)
}

func (s *OpenAIGatewayService) handleOpenAIAccountUpstreamError(ctx context.Context, account *Account, statusCode int, headers http.Header, responseBody []byte, requestedModel ...string) bool {
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()

	if account != nil && account.Platform == PlatformOpenAI && isOpenAIContextWindowError("", responseBody) {
		return false
	}

	if isOpenAIImageRateLimitError(statusCode, responseBody) {
		if s != nil && s.rateLimitService != nil {
			_ = s.rateLimitService.HandleOpenAIImageRateLimit(stateCtx, account, statusCode, headers, responseBody)
		}
		return false
	}

	if statusCode == http.StatusTooManyRequests {
		s.markOpenAIOAuth429RateLimited(stateCtx, account, headers, responseBody)
	}
	if s == nil || account == nil || s.rateLimitService == nil {
		return false
	}
	if len(requestedModel) > 0 && s.rateLimitService.HandleUpstreamModelNotFound(stateCtx, account, requestedModel[0], statusCode, responseBody) {
		return true
	}
	shouldDisable := s.rateLimitService.HandleUpstreamError(stateCtx, account, statusCode, headers, responseBody)
	if shouldDisable {
		s.BlockAccountScheduling(account, time.Time{}, "upstream_disable")
	}
	return shouldDisable
}

func (s *OpenAIGatewayService) markOpenAIOAuth429RateLimited(ctx context.Context, account *Account, headers http.Header, responseBody []byte) {
	if s == nil || !isOpenAIOAuthAccount(account) {
		return
	}
	// Spark 影子：不按 /responses 429 的 global x-codex-* 信号做内存运行时熔断(同 handle429,外审第8轮 P1)。
	// 同时避免把 spark 的 429 计入全局 429 storm 计数(recordOpenAIOAuth429),否则会误伤母账号 failover 决策。
	if account.IsShadow() {
		return
	}
	s.recordOpenAIOAuth429()

	cooldownUntil := time.Now().Add(openAIOAuth429FallbackCooldown)
	if s.rateLimitService != nil {
		if resetAt := s.rateLimitService.calculateOpenAI429ResetTime(headers); resetAt != nil && resetAt.After(time.Now()) {
			cooldownUntil = *resetAt
		} else if resetUnix := parseOpenAIRateLimitResetTime(responseBody); resetUnix != nil {
			if resetAt := time.Unix(*resetUnix, 0); resetAt.After(time.Now()) {
				cooldownUntil = resetAt
			}
		} else if cooldown, ok := s.rateLimitService.get429FallbackCooldown(ctx, account); ok && cooldown > 0 {
			cooldownUntil = time.Now().Add(cooldown)
		}
	}
	s.BlockAccountScheduling(account, cooldownUntil, "429")
}

func (s *OpenAIGatewayService) BlockAccountScheduling(account *Account, until time.Time, reason string) {
	if s == nil || !isOpenAIAccount(account) {
		return
	}
	now := time.Now()
	blockUntil := until
	if blockUntil.IsZero() || !blockUntil.After(now) {
		blockUntil = now.Add(openAIStopSchedulingBridgeCooldown)
	}
	nextBlock := openAIAccountRuntimeBlock{
		Until:  blockUntil,
		Reason: normalizeOpenAIRuntimeBlockReason(reason),
	}

	for {
		current, loaded := s.openaiAccountRuntimeBlockUntil.Load(account.ID)
		if !loaded {
			actual, stored := s.openaiAccountRuntimeBlockUntil.LoadOrStore(account.ID, nextBlock)
			if !stored {
				return
			}
			current = actual
		}

		currentBlock, ok := openAIAccountRuntimeBlockFromValue(current)
		if !ok || currentBlock.Until.IsZero() {
			if s.openaiAccountRuntimeBlockUntil.CompareAndSwap(account.ID, current, nextBlock) {
				return
			}
			continue
		}
		if currentBlock.Until.After(blockUntil) {
			return
		}
		if s.openaiAccountRuntimeBlockUntil.CompareAndSwap(account.ID, current, nextBlock) {
			return
		}
	}
}

func (s *OpenAIGatewayService) ClearAccountSchedulingBlock(accountID int64) {
	if s == nil || accountID <= 0 {
		return
	}
	s.openaiAccountRuntimeBlockUntil.Delete(accountID)
}

func (s *OpenAIGatewayService) isOpenAIAccountRuntimeBlocked(account *Account) bool {
	_, ok := s.openAIAccountRuntimeBlock(account)
	return ok
}

func (s *OpenAIGatewayService) openAIAccountRuntimeBlock(account *Account) (openAIAccountRuntimeBlock, bool) {
	if s == nil || !isOpenAIAccount(account) {
		return openAIAccountRuntimeBlock{}, false
	}
	value, ok := s.openaiAccountRuntimeBlockUntil.Load(account.ID)
	if !ok {
		return openAIAccountRuntimeBlock{}, false
	}
	block, ok := openAIAccountRuntimeBlockFromValue(value)
	if !ok || block.Until.IsZero() {
		s.openaiAccountRuntimeBlockUntil.Delete(account.ID)
		return openAIAccountRuntimeBlock{}, false
	}
	if time.Now().Before(block.Until) {
		return block, true
	}
	s.openaiAccountRuntimeBlockUntil.Delete(account.ID)
	return openAIAccountRuntimeBlock{}, false
}

func openAIAccountRuntimeBlockFromValue(value any) (openAIAccountRuntimeBlock, bool) {
	switch v := value.(type) {
	case openAIAccountRuntimeBlock:
		if v.Reason == "" {
			v.Reason = "upstream_failure"
		}
		return v, true
	case *openAIAccountRuntimeBlock:
		if v == nil {
			return openAIAccountRuntimeBlock{}, false
		}
		out := *v
		if out.Reason == "" {
			out.Reason = "upstream_failure"
		}
		return out, true
	case time.Time:
		return openAIAccountRuntimeBlock{Until: v, Reason: "upstream_failure"}, true
	default:
		return openAIAccountRuntimeBlock{}, false
	}
}

func (s *OpenAIGatewayService) MaybeBlockOpenAIAccountAfterFailoverError(account *Account, failoverErr *UpstreamFailoverError) bool {
	if s == nil || account == nil || failoverErr == nil || !isOpenAIAccount(account) {
		return false
	}
	// Pool-mode endpoints already schedule their own upstream account pool. A
	// local transient block removes the whole pool and can exhaust the group.
	// Slow first output is also not evidence that an account is unhealthy.
	if account.IsPoolMode() || failoverErr.FirstResponseTimeout {
		return false
	}
	reason := ""
	if openAIStatusShouldRuntimeBlockAfterFailure(failoverErr.StatusCode) {
		if text := http.StatusText(failoverErr.StatusCode); text != "" {
			reason = "upstream_status_" + text
		} else {
			reason = "upstream_status_" + strconv.Itoa(failoverErr.StatusCode)
		}
	}
	if reason == "" {
		return false
	}
	s.blockOpenAIAccountSchedulingAfterFailure(account, normalizeOpenAIRuntimeBlockReason(reason))
	return true
}

func (s *OpenAIGatewayService) MaybeBlockOpenAIAccountAfterForwardError(account *Account, err error) bool {
	if s == nil || account == nil || err == nil || !isOpenAIAccount(account) {
		return false
	}
	if account.IsPoolMode() {
		return false
	}
	reason := openAIForwardErrorRuntimeBlockReason(err)
	if reason == "" {
		return false
	}
	s.blockOpenAIAccountSchedulingAfterFailure(account, reason)
	return true
}

func (s *OpenAIGatewayService) blockOpenAIAccountSchedulingAfterFailure(account *Account, reason string) {
	if s == nil || account == nil {
		return
	}
	s.BlockAccountScheduling(account, time.Time{}, reason)
	logger.L().With(zap.String("component", "service.openai_gateway")).Warn(
		"openai.account_runtime_block_failure",
		zap.Int64("account_id", account.ID),
		zap.String("account_name", account.Name),
		zap.String("platform", account.Platform),
		zap.String("reason", reason),
		zap.Duration("cooldown", openAIStopSchedulingBridgeCooldown),
	)
}

func openAIStatusShouldRuntimeBlockAfterFailure(statusCode int) bool {
	if statusCode == 0 || statusCode == http.StatusTooManyRequests {
		return false
	}
	return statusCode == 529 || statusCode >= http.StatusInternalServerError
}

func openAIForwardErrorRuntimeBlockReason(err error) string {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" || strings.Contains(msg, "client disconnected") || strings.Contains(msg, "after disconnect") ||
		strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded") {
		return ""
	}
	switch {
	case strings.Contains(msg, "stream read error"),
		strings.Contains(msg, "http2: response body closed"),
		strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, "connection reset by peer"):
		return "stream_read_error"
	case strings.Contains(msg, "stream data interval timeout"):
		return "stream_data_interval_timeout"
	case strings.Contains(msg, "missing terminal event"):
		return "missing_terminal_event"
	default:
		return ""
	}
}

// IsOpenAIUpstreamFailureForStickyRelease identifies failures where retaining
// a pool-mode session binding would route the client's next retry back to the
// same failed upstream. Client cancellation and downstream disconnects remain
// excluded by openAIForwardErrorRuntimeBlockReason.
func IsOpenAIUpstreamFailureForStickyRelease(err error) bool {
	if openAIForwardErrorRuntimeBlockReason(err) != "" {
		return true
	}
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "upstream response failed") ||
		strings.Contains(message, "stream usage incomplete after timeout")
}

func normalizeOpenAIRuntimeBlockReason(reason string) string {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return "upstream_failure"
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_", "/", "_", ":", "_", "(", "", ")", "")
	return replacer.Replace(reason)
}

func openAIRuntimeBlockReasonAllowsSingleCandidateFailOpen(reason string) bool {
	reason = normalizeOpenAIRuntimeBlockReason(reason)
	switch reason {
	case "first_response_timeout", "stream_read_error", "stream_data_interval_timeout", "missing_terminal_event":
		return true
	default:
		return strings.HasPrefix(reason, "upstream_status_")
	}
}

func (s *OpenAIGatewayService) recordOpenAIOAuth429() {
	if s == nil {
		return
	}
	now := time.Now()
	windowStart := s.openaiOAuth429WindowStartUnixNano.Load()
	if windowStart == 0 || now.Sub(time.Unix(0, windowStart)) >= openAIOAuth429StormWindow {
		if s.openaiOAuth429WindowStartUnixNano.CompareAndSwap(windowStart, now.UnixNano()) {
			s.openaiOAuth429WindowCount.Store(1)
			return
		}
	}
	s.openaiOAuth429WindowCount.Add(1)
}

func (s *OpenAIGatewayService) isOpenAIOAuth429Storm() bool {
	if s == nil {
		return false
	}
	windowStart := s.openaiOAuth429WindowStartUnixNano.Load()
	if windowStart == 0 || time.Since(time.Unix(0, windowStart)) >= openAIOAuth429StormWindow {
		return false
	}
	return s.openaiOAuth429WindowCount.Load() >= openAIOAuth429StormThreshold
}

func (s *OpenAIGatewayService) ShouldStopOpenAIOAuth429Failover(account *Account, statusCode int, failedSwitches int) bool {
	if statusCode != http.StatusTooManyRequests || failedSwitches < openAIOAuth429StormMaxAccountSwitches {
		return false
	}
	if isGrokOAuthAccount(account) {
		return true
	}
	if !isOpenAIOAuthAccount(account) {
		return false
	}
	return s.isOpenAIOAuth429Storm()
}
