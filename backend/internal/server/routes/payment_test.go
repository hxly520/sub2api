//go:build unit

package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterPaymentRoutesIncludesKeyingPayWebhook(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	noAuth := func(c *gin.Context) { c.Next() }

	RegisterPaymentRoutes(
		v1,
		&handler.PaymentHandler{},
		&handler.PaymentWebhookHandler{},
		&adminhandler.PaymentHandler{},
		middleware.JWTAuthMiddleware(noAuth),
		middleware.AdminAuthMiddleware(noAuth),
		middleware.AuditLogMiddleware(noAuth),
		nil,
		nil,
	)

	routes := router.Routes()
	methods := make(map[string]bool)
	for _, route := range routes {
		if route.Path == "/api/v1/payment/webhook/keyingpay" {
			methods[route.Method] = true
		}
	}
	require.True(t, methods[http.MethodGet])
	require.True(t, methods[http.MethodPost])
}
