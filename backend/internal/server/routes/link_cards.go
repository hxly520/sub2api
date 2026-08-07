package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func RegisterLinkCardPublicRoutes(v1 *gin.RouterGroup, h *handler.Handlers, limiter *middleware.PanelRateLimiter) {
	public := v1.Group("/public/link-cards")
	if limiter != nil {
		public.Use(limiter.PublicIP())
	}
	public.POST("/activate", h.LinkCard.Activate)
	public.GET("/me", h.LinkCard.PublicProfile)
	public.GET("/usage", h.LinkCard.PublicUsage)
}
