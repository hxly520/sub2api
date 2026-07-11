package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) openAIFirstResponseRuntimeConfig(ctx context.Context) service.OpenAIFirstResponseRuntimeConfig {
	if h == nil || h.gatewayService == nil {
		return service.OpenAIFirstResponseRuntimeConfig{}
	}
	runtimeCfg := h.gatewayService.GetOpenAIFirstResponseRuntimeConfig(ctx)
	if runtimeCfg.MaxAttempts <= 0 {
		runtimeCfg.MaxAttempts = 1
	}
	return runtimeCfg
}

func (h *OpenAIGatewayHandler) openAIFirstResponseForwardContext(
	c *gin.Context,
	reqLog *zap.Logger,
	groupID *int64,
	requestedModel string,
	selectedAccountID int64,
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

	hasAlternative, err := h.gatewayService.HasOpenAIAlternativeAccountForCapability(
		ctx,
		groupID,
		requestedModel,
		selectedAccountID,
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
	if !hasAlternative || maxAccountSwitches <= 0 || switchCount >= maxAccountSwitches || switchCount+1 >= runtimeCfg.MaxAttempts {
		return service.WithOpenAIFirstResponseEarlyFlush(ctx)
	}
	if reqLog != nil {
		reqLog.Debug("openai.first_response_timeout_enabled",
			zap.Duration("timeout", timeout),
			zap.Int("switch_count", switchCount),
			zap.Int("max_switches", maxAccountSwitches),
		)
	}
	return service.WithOpenAIFirstResponseTimeout(ctx, timeout)
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
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	runtimeCfg := h.openAIFirstResponseRuntimeConfig(ctx)
	failoverErr.CountAsError = runtimeCfg.CountAsError
	h.gatewayService.ReportOpenAIAccountFirstResponseTimeout(
		account.ID,
		runtimeCfg.CountAsError,
		openAIAccountScheduleResultProfile(c, requestedModel, account),
	)
}
