//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestSettingServiceUpdateSettingsValidatesSiteLogo(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}
	valid := siteLogoDataURI("image/png", pngHeader)
	invalidValues := map[string]string{
		"unsupported svg":  siteLogoDataURI("image/svg+xml", []byte("<svg/>")),
		"forged signature": siteLogoDataURI("image/png", []byte("not an image")),
		"invalid base64":   "data:image/png;base64,%%%",
		"oversized": siteLogoDataURI("image/png", append(
			pngHeader,
			[]byte(strings.Repeat("x", SiteLogoMaxDecodedBytes))...,
		)),
	}

	for name, value := range invalidValues {
		t.Run(name, func(t *testing.T) {
			repo := &settingUpdateRepoStub{}
			svc := NewSettingService(repo, &config.Config{})

			err := svc.UpdateSettings(context.Background(), &SystemSettings{SiteLogo: value})

			require.Error(t, err)
			require.Equal(t, "INVALID_SITE_LOGO", infraerrors.Reason(err))
			require.Nil(t, repo.updates)
		})
	}

	t.Run("valid png is persisted", func(t *testing.T) {
		repo := &settingUpdateRepoStub{}
		svc := NewSettingService(repo, &config.Config{})

		err := svc.UpdateSettings(context.Background(), &SystemSettings{
			SiteLogo: " DATA:IMAGE/PNG;BASE64," + base64.StdEncoding.EncodeToString(pngHeader) + " ",
		})

		require.NoError(t, err)
		require.Equal(t, valid, repo.updates[SettingKeySiteLogo])
	})
}
