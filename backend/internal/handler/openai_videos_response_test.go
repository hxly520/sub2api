package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIVideoResponseBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("configured base wins", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "http://internal.local/v1/videos/task", nil)
		c.Request.Host = "internal.local"

		got := resolveOpenAIVideoResponseBaseURL(c, "https://api.52token.example")
		require.Equal(t, "https://api.52token.example", got)
	})

	t.Run("request origin fallback", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "http://sub2api.example/v1/videos/task", nil)
		c.Request.Host = "sub2api.example"
		c.Request.Header.Set("X-Forwarded-Proto", "https")

		got := resolveOpenAIVideoResponseBaseURL(c, "")
		require.Equal(t, "https://sub2api.example", got)
	})

	t.Run("invalid configured base falls back", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "http://sub2api.example/v1/videos/task", nil)
		c.Request.Host = "sub2api.example"

		got := resolveOpenAIVideoResponseBaseURL(c, "javascript:alert(1)")
		require.Equal(t, "http://sub2api.example", got)
	})
}
