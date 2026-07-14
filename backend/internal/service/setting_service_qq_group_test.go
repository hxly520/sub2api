//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSettingService_GetPublicSettings_ExposesTrimmedQQGroupURL(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyQQGroupURL: "  https://qm.qq.com/cgi-bin/qm/qr?k=test  ",
	}}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://qm.qq.com/cgi-bin/qm/qr?k=test", settings.QQGroupURL)
}

func TestSettingService_GetPublicSettingsForInjection_ExposesQQGroupURL(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyQQGroupURL: "https://qm.qq.com/cgi-bin/qm/qr?k=injected",
	}}
	svc := NewSettingService(repo, &config.Config{})

	injected, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	payload, ok := injected.(*PublicSettingsInjectionPayload)
	require.True(t, ok)
	require.Equal(t, "https://qm.qq.com/cgi-bin/qm/qr?k=injected", payload.QQGroupURL)
}

func TestSettingService_UpdateSettings_TrimsQQGroupURL(t *testing.T) {
	repo := &settingUpdateRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		QQGroupURL: "  https://qm.qq.com/cgi-bin/qm/qr?k=stored  ",
	})
	require.NoError(t, err)
	require.Equal(t, "https://qm.qq.com/cgi-bin/qm/qr?k=stored", repo.updates[SettingKeyQQGroupURL])
}
