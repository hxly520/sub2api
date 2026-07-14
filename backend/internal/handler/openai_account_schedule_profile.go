package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func withOpenAIAccountScheduleProfile(ctx context.Context, c *gin.Context, requestedModel string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	inboundEndpoint := ""
	if c != nil {
		inboundEndpoint = GetInboundEndpoint(c)
	}
	return service.WithOpenAIAccountScheduleProfile(
		ctx,
		service.NewOpenAIAccountScheduleProfile(requestedModel, inboundEndpoint, ""),
	)
}

func openAIAccountScheduleSelectionContext(c *gin.Context, requestedModel string) context.Context {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	return withOpenAIAccountScheduleProfile(ctx, c, requestedModel)
}

func openAIAccountScheduleResultProfile(c *gin.Context, requestedModel string, account *service.Account) service.OpenAIAccountScheduleProfile {
	inboundEndpoint := ""
	upstreamEndpoint := ""
	if c != nil {
		inboundEndpoint = GetInboundEndpoint(c)
		if account != nil {
			upstreamEndpoint = resolveOpenAIUpstreamEndpoint(c, account, nil)
		}
	}
	return service.NewOpenAIAccountScheduleProfile(requestedModel, inboundEndpoint, upstreamEndpoint)
}

func (h *OpenAIGatewayHandler) reportOpenAIAccountScheduleResult(c *gin.Context, account *service.Account, requestedModel string, success bool, firstTokenMs *int) {
	if h == nil || h.gatewayService == nil || account == nil {
		return
	}
	h.gatewayService.ReportOpenAIAccountScheduleResultWithProfile(
		account.ID,
		success,
		firstTokenMs,
		openAIAccountScheduleResultProfile(c, requestedModel, account),
	)
}

func (h *OpenAIGatewayHandler) releaseOpenAIFailedPoolStickySession(
	c *gin.Context,
	reqLog *zap.Logger,
	groupID *int64,
	sessionHash string,
	account *service.Account,
) {
	if h == nil || h.gatewayService == nil || account == nil || !account.IsPoolMode() {
		return
	}
	h.gatewayService.ReportOpenAIAccountRecentFailure(account.ID)
	if sessionHash == "" {
		return
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	released, err := h.gatewayService.ReleaseOpenAIStickySessionAfterFailure(ctx, groupID, sessionHash, account.ID)
	if err != nil {
		if reqLog != nil {
			reqLog.Warn("openai.failed_pool_sticky_release_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		}
		return
	}
	if released && reqLog != nil {
		reqLog.Info("openai.failed_pool_sticky_released", zap.Int64("account_id", account.ID))
	}
}
