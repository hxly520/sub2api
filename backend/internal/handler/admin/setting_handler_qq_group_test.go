//go:build unit

package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newQQGroupSettingHandler(repo *settingHandlerRepoStub) *SettingHandler {
	svc := service.NewSettingService(repo, &config.Config{Default: config.DefaultConfig{UserConcurrency: 5}})
	return NewSettingHandler(svc, nil, nil, nil, nil, nil, nil)
}

func TestSettingHandler_GetSettings_ExposesQQGroupURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyPromoCodeEnabled: "true",
		service.SettingKeyQQGroupURL:       "  https://qm.qq.com/cgi-bin/qm/qr?k=admin  ",
	}}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	newQQGroupSettingHandler(repo).GetSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://qm.qq.com/cgi-bin/qm/qr?k=admin", data["qq_group_url"])
}

func TestSettingHandler_UpdateSettings_TrimsAndPersistsQQGroupURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyPromoCodeEnabled: "true",
	}}
	body, err := json.Marshal(map[string]any{
		"qq_group_url": "  https://qm.qq.com/cgi-bin/qm/qr?k=saved  ",
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	newQQGroupSettingHandler(repo).UpdateSettings(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://qm.qq.com/cgi-bin/qm/qr?k=saved", repo.values[service.SettingKeyQQGroupURL])
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data, ok := resp.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://qm.qq.com/cgi-bin/qm/qr?k=saved", data["qq_group_url"])
}

func TestSettingHandler_UpdateSettings_RejectsUnsafeQQGroupURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &settingHandlerRepoStub{values: map[string]string{
		service.SettingKeyPromoCodeEnabled: "true",
		service.SettingKeyQQGroupURL:       "https://qm.qq.com/existing",
	}}
	body, err := json.Marshal(map[string]any{"qq_group_url": "javascript:alert(1)"})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	newQQGroupSettingHandler(repo).UpdateSettings(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "https://qm.qq.com/existing", repo.values[service.SettingKeyQQGroupURL])
	require.Nil(t, repo.lastUpdates)
}

func TestDiffSettings_RecordsQQGroupURL(t *testing.T) {
	before := &service.SystemSettings{QQGroupURL: "https://qm.qq.com/old"}
	after := &service.SystemSettings{QQGroupURL: "https://qm.qq.com/new"}

	changed := diffSettings(before, after, nil, nil, UpdateSettingsRequest{})
	require.Contains(t, changed, "qq_group_url")
}
