package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterPointsInternalRoutes(r *gin.Engine, h *handler.Handlers) {
	r.POST("/api/internal/points/credits", h.Points.Credit)
}
