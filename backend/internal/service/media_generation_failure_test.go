//go:build unit

package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsDefinitiveMediaGenerationFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{name: "explicit forbidden", statusCode: http.StatusForbidden, body: `{"message":"insufficient balance"}`, want: true},
		{name: "explicit rate limit", statusCode: http.StatusTooManyRequests, want: true},
		{name: "provider queue cancellation", statusCode: http.StatusBadGateway, body: `{"message":"管理员取消：生图池排队任务已清空"}`, want: true},
		{name: "structured failed terminal", statusCode: http.StatusBadGateway, body: `{"task":{"status":"failed"}}`, want: true},
		{name: "generic bad gateway", statusCode: http.StatusBadGateway, body: `{"message":"service temporarily unavailable"}`},
		{name: "gateway timeout", statusCode: http.StatusGatewayTimeout},
		{name: "transport error", statusCode: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsDefinitiveMediaGenerationFailure(tt.statusCode, []byte(tt.body)))
		})
	}
}
