package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func openAIAccountScheduleSelectionContext(c *gin.Context, requestedModel string) context.Context {
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	inboundEndpoint := ""
	if c != nil {
		inboundEndpoint = GetInboundEndpoint(c)
	}
	return service.WithOpenAIAccountScheduleProfile(ctx, service.NewOpenAIAccountScheduleProfile(requestedModel, inboundEndpoint, ""))
}

func openAIAccountScheduleResultProfile(c *gin.Context, requestedModel string, account *service.Account) service.OpenAIAccountScheduleProfile {
	inboundEndpoint := ""
	upstreamEndpoint := ""
	if c != nil {
		inboundEndpoint = GetInboundEndpoint(c)
		if account != nil {
			upstreamEndpoint = resolveOpenAIUpstreamEndpoint(c, account)
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
