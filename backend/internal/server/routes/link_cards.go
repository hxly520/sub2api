package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	basemiddleware "github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RegisterLinkCardPublicRoutes(v1 *gin.RouterGroup, h *handler.Handlers, limiter *middleware.PanelRateLimiter, redisClient *redis.Client) {
	public := v1.Group("/public/link-cards")
	if limiter != nil {
		public.Use(limiter.PublicIP())
	}
	attemptLimiter := basemiddleware.NewRateLimiter(redisClient)
	failureLimiter := middleware.NewLinkCardActivationLimiter(redisClient)
	public.POST(
		"/activate",
		attemptLimiter.LimitWithOptions("link-card-activate", 30, time.Minute, basemiddleware.RateLimitOptions{
			FailureMode: basemiddleware.RateLimitFailClose,
		}),
		failureLimiter.Middleware(),
		h.LinkCard.Activate,
	)
	public.GET("/me", h.LinkCard.PublicProfile)
	public.GET("/usage", h.LinkCard.PublicUsage)
}
