package service

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func siteLogoDataURI(mediaType string, body []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(body)
}

func TestParseSiteLogoDataURIAcceptsSupportedSignatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mediaType string
		body      []byte
	}{
		{name: "png", mediaType: "image/png", body: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}},
		{name: "jpeg", mediaType: "image/jpeg", body: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01}},
		{name: "gif", mediaType: "image/gif", body: []byte("GIF89a\x01\x00\x01\x00")},
		{name: "webp", mediaType: "image/webp", body: []byte{'R', 'I', 'F', 'F', 0x08, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw := siteLogoDataURI(tt.mediaType, tt.body)
			logo, err := ParseSiteLogoDataURI(raw)
			require.NoError(t, err)
			require.NotNil(t, logo)
			require.Equal(t, tt.mediaType, logo.MediaType)
			require.Equal(t, tt.body, logo.Body)
			require.Equal(t, raw, logo.DataURI)
		})
	}
}

func TestParseSiteLogoDataURIRejectsUnsafeAndMalformedValues(t *testing.T) {
	t.Parallel()

	pngHeader := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unsupported svg", raw: siteLogoDataURI("image/svg+xml", []byte("<svg/>"))},
		{name: "forged signature", raw: siteLogoDataURI("image/png", []byte("not an image"))},
		{name: "mismatched signature", raw: siteLogoDataURI("image/jpeg", pngHeader)},
		{name: "invalid base64", raw: "data:image/png;base64,%%%"},
		{name: "missing base64 marker", raw: "data:image/png," + base64.StdEncoding.EncodeToString(pngHeader)},
		{name: "oversized", raw: siteLogoDataURI("image/png", append(pngHeader, []byte(strings.Repeat("x", SiteLogoMaxDecodedBytes))...))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logo, err := ParseSiteLogoDataURI(tt.raw)
			require.Error(t, err)
			require.Nil(t, logo)
		})
	}
}

func TestParseSiteLogoDataURIAllowsEmptyClear(t *testing.T) {
	t.Parallel()

	logo, err := ParseSiteLogoDataURI("  ")
	require.NoError(t, err)
	require.Nil(t, logo)
}
