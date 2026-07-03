package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) openAIFirstResponseRuntimeConfig(ctx context.Context) service.OpenAIFirstResponseRuntimeConfig {
	if h != nil && h.gatewayService != nil {
		return h.gatewayService.GetOpenAIFirstResponseRuntimeConfig(ctx)
	}
	if h == nil || h.cfg == nil {
		return service.OpenAIFirstResponseRuntimeConfig{
			Enabled:     false,
			TimeoutMS:   5000,
			MaxAttempts: 2,
		}
	}
	timeoutMS := h.cfg.Gateway.OpenAIFirstResponse.TimeoutMS
	if timeoutMS <= 0 {
		timeoutMS = 5000
	}
	maxAttempts := h.cfg.Gateway.OpenAIFirstResponse.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 2
	}
	return service.OpenAIFirstResponseRuntimeConfig{
		Enabled:      h.cfg.Gateway.OpenAIFirstResponse.Enabled,
		TimeoutMS:    timeoutMS,
		MaxAttempts:  maxAttempts,
		CountAsError: h.cfg.Gateway.OpenAIFirstResponse.CountAsError,
	}
}

func (h *OpenAIGatewayHandler) openAIFirstResponseForwardContext(
	c *gin.Context,
	reqLog *zap.Logger,
	groupID *int64,
	requestedModel string,
	failedAccountIDs map[int64]struct{},
	switchCount int,
	maxAccountSwitches int,
	stream bool,
	requiredTransport service.OpenAIUpstreamTransport,
	requiredCapability service.OpenAIEndpointCapability,
	requireCompact bool,
	platform string,
) context.Context {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	runtimeCfg := h.openAIFirstResponseRuntimeConfig(ctx)
	timeout := time.Duration(runtimeCfg.TimeoutMS) * time.Millisecond
	if !runtimeCfg.Enabled || timeout <= 0 || !stream || h == nil || h.gatewayService == nil {
		return ctx
	}

	eligibleCount, err := h.gatewayService.CountOpenAIEligibleAccountsForCapability(
		ctx,
		groupID,
		requestedModel,
		failedAccountIDs,
		requiredTransport,
		requiredCapability,
		requireCompact,
		platform,
	)
	if err != nil {
		if reqLog != nil {
			reqLog.Debug("openai.first_response_candidate_count_failed", zap.Error(err))
		}
		return ctx
	}
	if eligibleCount <= 1 {
		if reqLog != nil {
			reqLog.Debug("openai.first_response_early_flush_enabled_single_candidate",
				zap.Int("eligible_account_count", eligibleCount),
			)
		}
		return service.WithOpenAIFirstResponseEarlyFlush(ctx)
	}
	if maxAccountSwitches <= 0 || switchCount >= maxAccountSwitches {
		if reqLog != nil {
			reqLog.Debug("openai.first_response_early_flush_enabled_no_switch_budget",
				zap.Int("eligible_account_count", eligibleCount),
				zap.Int("switch_count", switchCount),
				zap.Int("max_switches", maxAccountSwitches),
			)
		}
		return service.WithOpenAIFirstResponseEarlyFlush(ctx)
	}
	if switchCount+1 >= runtimeCfg.MaxAttempts {
		if reqLog != nil {
			reqLog.Debug("openai.first_response_early_flush_enabled_attempt_budget",
				zap.Int("eligible_account_count", eligibleCount),
				zap.Int("switch_count", switchCount),
				zap.Int("max_attempts", runtimeCfg.MaxAttempts),
			)
		}
		return service.WithOpenAIFirstResponseEarlyFlush(ctx)
	}
	if reqLog != nil {
		reqLog.Debug("openai.first_response_timeout_enabled",
			zap.Int("eligible_account_count", eligibleCount),
			zap.Duration("timeout", timeout),
			zap.Int("switch_count", switchCount),
			zap.Int("max_switches", maxAccountSwitches),
		)
	}
	return service.WithOpenAIFirstResponseEarlyFlush(
		service.WithOpenAIFirstResponseTimeout(ctx, timeout),
	)
}

func (h *OpenAIGatewayHandler) reportOpenAIAccountFailoverScheduleResult(
	c *gin.Context,
	account *service.Account,
	requestedModel string,
	failoverErr *service.UpstreamFailoverError,
) {
	if failoverErr == nil || !failoverErr.FirstResponseTimeout {
		h.reportOpenAIAccountScheduleResult(c, account, requestedModel, false, nil)
		return
	}
	timeoutMs := failoverErr.FirstResponseTimeoutMs
	if timeoutMs <= 0 {
		ctx := context.Background()
		if c != nil && c.Request != nil {
			ctx = c.Request.Context()
		}
		timeoutMs = h.openAIFirstResponseRuntimeConfig(ctx).TimeoutMS
	}
	if timeoutMs <= 0 {
		timeoutMs = 1
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	countAsError := h.openAIFirstResponseRuntimeConfig(ctx).CountAsError
	failoverErr.CountAsError = countAsError
	h.reportOpenAIAccountScheduleResult(c, account, requestedModel, !countAsError, &timeoutMs)
}
