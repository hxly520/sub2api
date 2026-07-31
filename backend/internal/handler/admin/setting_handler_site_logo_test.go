package admin

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateSettingsRejectsInvalidSiteLogo(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	dataURI := func(mediaType string, body []byte) string {
		return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(body)
	}
	invalidValues := map[string]string{
		"unsupported svg":  dataURI("image/svg+xml", []byte("<svg/>")),
		"forged signature": dataURI("image/png", []byte("not an image")),
		"invalid base64":   "data:image/png;base64,%%%",
		"oversized": dataURI("image/png", append(
			pngHeader,
			[]byte(strings.Repeat("x", service.SiteLogoMaxDecodedBytes))...,
		)),
	}

	for name, value := range invalidValues {
		t.Run(name, func(t *testing.T) {
			stored := dataURI("image/png", pngHeader)
			h, repo := newStepUpSwitchTestHandler(t, map[string]string{
				service.SettingKeySiteLogo: stored,
			})

			recorder := doUpdateSettings(t, h, map[string]any{"site_logo": value}, nil)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), "INVALID_SITE_LOGO")
			require.Equal(t, stored, repo.values[service.SettingKeySiteLogo])
		})
	}
}
