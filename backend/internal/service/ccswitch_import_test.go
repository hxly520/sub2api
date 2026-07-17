package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCCSwitchUsageTemplateIsFixedToProviderUsageEndpoint(t *testing.T) {
	template := GetCCSwitchUsageTemplate()
	decoded, err := base64.StdEncoding.DecodeString(template.ScriptBase64)
	require.NoError(t, err)

	script := string(decoded)
	require.Equal(t, 1, template.Version)
	require.Equal(t, "/v1/usage", template.EndpointPath)
	require.Equal(t, CCSwitchUsageAutoIntervalMinutes, template.AutoIntervalMinutes)
	require.Contains(t, script, `url: "{{baseUrl}}/v1/usage"`)
	require.Contains(t, script, `"Authorization": "Bearer {{apiKey}}"`)
	require.NotContains(t, script, "http://")
	require.NotContains(t, script, "https://")

	for _, forbidden := range []string{
		"User-Agent",
		"fetch(",
		"XMLHttpRequest",
		"WebSocket",
		"eval(",
		"Function(",
		"require(",
		"import(",
	} {
		require.False(t, strings.Contains(script, forbidden), "unexpected executable capability %q", forbidden)
	}

	digest := sha256.Sum256(decoded)
	require.Equal(t, hex.EncodeToString(digest[:]), template.ScriptSHA256)
}
