package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RegisterPointsInternalRoutes(r *gin.Engine, h *handler.Handlers, redisClient *redis.Client) {
	rateLimiter := middleware.NewRateLimiter(redisClient)
	r.POST("/api/internal/points/credits",
		rateLimiter.LimitWithOptions("points-credit", 120, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}),
		h.Points.Credit,
	)
}
