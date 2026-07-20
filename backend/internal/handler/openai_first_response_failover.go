package handler

import (
	"context"

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
	stream bool,
) context.Context {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	runtimeCfg := h.openAIFirstResponseRuntimeConfig(ctx)
	if !runtimeCfg.Enabled || !stream || h == nil || h.gatewayService == nil {
		return ctx
	}
	if reqLog != nil {
		reqLog.Debug("openai.first_response_early_flush_enabled")
	}
	return service.WithOpenAIFastFirstTokenTiming(
		service.WithOpenAIFirstResponseEarlyFlush(ctx),
	)
}

func (h *OpenAIGatewayHandler) reportOpenAIAccountFailoverScheduleResult(
	c *gin.Context,
	account *service.Account,
	requestedModel string,
	failoverErr *service.UpstreamFailoverError,
) {
	if failoverErr != nil && !failoverErr.ShouldReportAccountScheduleFailure() {
		return
	}
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
