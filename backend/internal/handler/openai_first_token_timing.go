package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// enableOpenAIFastFirstTokenTiming keeps the official forwarding path intact
// and only changes the recorded TTFT origin when the administrator enables the
// private option. Non-streaming requests retain the official metric behavior.
func (h *OpenAIGatewayHandler) enableOpenAIFastFirstTokenTiming(c *gin.Context, stream bool) {
	if !stream || c == nil || c.Request == nil || h == nil || h.gatewayService == nil {
		return
	}
	ctx := c.Request.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if !h.gatewayService.GetOpenAIFirstResponseRuntimeConfig(ctx).Enabled {
		return
	}
	c.Request = c.Request.WithContext(service.WithOpenAIFastFirstTokenTiming(ctx))
}
