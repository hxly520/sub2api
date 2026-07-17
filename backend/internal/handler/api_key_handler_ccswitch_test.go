package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyHandlerGetCCSwitchUsageTemplateRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/keys/ccswitch-usage-template", nil)

	(&APIKeyHandler{}).GetCCSwitchUsageTemplate(context)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Equal(t, "no-store, max-age=0", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", recorder.Header().Get("Pragma"))
}

func TestAPIKeyHandlerGetCCSwitchUsageTemplateReturnsFixedBalanceTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/keys/ccswitch-usage-template", nil)
	context.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1})

	(&APIKeyHandler{}).GetCCSwitchUsageTemplate(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store, max-age=0", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", recorder.Header().Get("Pragma"))

	var envelope struct {
		Code int                           `json:"code"`
		Data service.CCSwitchUsageTemplate `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.Zero(t, envelope.Code)
	require.NotEmpty(t, envelope.Data.ScriptBase64)
	require.Equal(t, "/v1/usage", envelope.Data.EndpointPath)
	require.Equal(t, service.CCSwitchUsageAutoIntervalMinutes, envelope.Data.AutoIntervalMinutes)
}
