package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// A gateway retry is useful only when the upstream explicitly rejected the
// request before generation. Two bounded cross-account attempts let a
// three-account group skip two definitively unavailable accounts without ever
// replaying an ambiguous, potentially billable request.
const openAIMaxAutomaticReplayAttempts = 2

const openAIModelCapacityRetryBaseDelay = 100 * time.Millisecond

type openAIRequestRetryBudget struct {
	used                            int
	modelCapacityRetries            int
	modelCapacitySelectionPending   bool
	modelCapacityCandidateReuseUsed bool
	poolRetryCounts                 map[int64]int
}

// tryPoolRetry preserves the account-local retry contract for explicit,
// pre-generation upstream rejections. It is separate from the cross-account
// replay budget because retrying the same account does not consume an account
// switch and must remain bounded by that account's own setting.
func (b *openAIRequestRetryBudget) tryPoolRetry(
	ctx context.Context,
	logger *zap.Logger,
	account *service.Account,
	failoverErr *service.UpstreamFailoverError,
) bool {
	if b == nil || account == nil || failoverErr == nil || !failoverErr.RetryableOnSameAccount ||
		(!failoverErr.CanSafelyReplayRequest() && !failoverErr.RequestScopedTransient) {
		return false
	}
	limit := account.GetPoolModeRetryCount()
	if limit <= 0 {
		return false
	}
	if b.poolRetryCounts == nil {
		b.poolRetryCounts = make(map[int64]int)
	}
	if b.poolRetryCounts[account.ID] >= limit {
		return false
	}
	b.poolRetryCounts[account.ID]++
	if logger != nil {
		logger.Warn("openai.pool_mode_retry",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.Int("retry_count", b.poolRetryCounts[account.ID]),
			zap.Int("retry_limit", limit),
		)
	}
	return sleepWithContext(ctx, sameAccountRetryDelay)
}

func (b *openAIRequestRetryBudget) tryConsumeIfAllowed(allowed bool, account *service.Account, failoverErr *service.UpstreamFailoverError) bool {
	if !allowed {
		return false
	}
	return b.tryConsume(account, failoverErr)
}

func (b *openAIRequestRetryBudget) tryConsume(account *service.Account, failoverErr *service.UpstreamFailoverError) bool {
	if b == nil || account == nil ||
		!failoverErr.CanSafelyReplayRequest() || b.used >= openAIMaxAutomaticReplayAttempts {
		return false
	}
	b.used++
	if failoverErr.IsOpenAIModelAtCapacity() {
		b.modelCapacityRetries++
		b.modelCapacitySelectionPending = true
		b.modelCapacityCandidateReuseUsed = false
	}
	return true
}

// retryModelCapacitySelection permits an already-approved replay to reuse the
// excluded candidates only when no alternate account can be selected. This
// keeps alternate-account failover first while making one-account groups useful.
func (b *openAIRequestRetryBudget) retryModelCapacitySelection(
	failedAccountIDs map[int64]struct{},
	lastFailoverErr *service.UpstreamFailoverError,
) bool {
	if b == nil || !b.modelCapacitySelectionPending || b.modelCapacityCandidateReuseUsed ||
		lastFailoverErr == nil || !lastFailoverErr.IsOpenAIModelAtCapacity() ||
		len(failedAccountIDs) == 0 {
		return false
	}
	clear(failedAccountIDs)
	b.modelCapacityCandidateReuseUsed = true
	return true
}

func (b *openAIRequestRetryBudget) markReplaySelectionSucceeded() {
	if b == nil || !b.modelCapacitySelectionPending {
		return
	}
	b.modelCapacitySelectionPending = false
	b.modelCapacityCandidateReuseUsed = false
}

func retryOpenAIModelCapacitySelection(
	reqLog *zap.Logger,
	route string,
	budget *openAIRequestRetryBudget,
	failedAccountIDs map[int64]struct{},
	lastFailoverErr *service.UpstreamFailoverError,
) bool {
	excluded := len(failedAccountIDs)
	if !budget.retryModelCapacitySelection(failedAccountIDs, lastFailoverErr) {
		return false
	}
	if reqLog != nil {
		reqLog.Warn("openai.model_capacity_retry_reusing_candidates",
			zap.String("route", route),
			zap.Int("previously_excluded_account_count", excluded),
			zap.Int("retry_attempt", budget.used),
			zap.Int("retry_max", openAIMaxAutomaticReplayAttempts),
		)
	}
	return true
}

func (b *openAIRequestRetryBudget) modelCapacityBackoff() time.Duration {
	if b == nil || b.modelCapacityRetries <= 0 {
		return 0
	}
	shift := b.modelCapacityRetries - 1
	if shift > 1 {
		shift = 1
	}
	return openAIModelCapacityRetryBaseDelay * time.Duration(1<<shift)
}

// waitBeforeReplay applies a short, context-aware exponential backoff only to
// the exact model-capacity rejection. Other established failover classes keep
// their existing latency and retry behavior.
func (b *openAIRequestRetryBudget) waitBeforeReplay(
	ctx context.Context,
	reqLog *zap.Logger,
	route string,
	account *service.Account,
	failoverErr *service.UpstreamFailoverError,
) bool {
	if failoverErr == nil || !failoverErr.IsOpenAIModelAtCapacity() {
		return true
	}
	delay := b.modelCapacityBackoff()
	if ctx == nil {
		ctx = context.Background()
	}
	fields := []zap.Field{
		zap.String("route", route),
		zap.Int("upstream_status", failoverErr.StatusCode),
		zap.String("failure_reason", string(failoverErr.Reason)),
		zap.Int("retry_attempt", b.used),
		zap.Int("retry_max", openAIMaxAutomaticReplayAttempts),
		zap.Int("capacity_retry_attempt", b.modelCapacityRetries),
		zap.Int64("backoff_ms", delay.Milliseconds()),
	}
	if account != nil {
		fields = append(fields,
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
		)
	}
	if reqLog != nil {
		reqLog.Warn("openai.model_capacity_retry_scheduled", fields...)
	}
	if sleepWithContext(ctx, delay) {
		return true
	}
	if reqLog != nil {
		reqLog.Info("openai.model_capacity_retry_aborted", fields...)
	}
	return false
}
