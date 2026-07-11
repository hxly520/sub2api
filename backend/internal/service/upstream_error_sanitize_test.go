//go:build unit

package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeClientUpstreamErrorMessage_RedactsRoutingSecretsAndStack(t *testing.T) {
	input := "model not found at https://private-upstream.example/v1/models?key=secret-value " +
		"POST /v1/chat/completions api_key=sk-private-1234567890123456 Bearer eyJhbGciOiJIUzI1NiJ9.payload123.signature123\n" +
		"at upstream.handler (/srv/app/index.js:42:9)"

	got := sanitizeClientUpstreamErrorMessage(input)

	require.Contains(t, got, "model not found")
	require.Contains(t, got, "[upstream URL]")
	require.Contains(t, got, "[upstream path]")
	require.NotContains(t, got, "private-upstream.example")
	require.NotContains(t, got, "secret-value")
	require.NotContains(t, got, "sk-private")
	require.NotContains(t, got, "eyJhbGci")
	require.NotContains(t, got, "/srv/app")
}

func TestSanitizeUpstreamErrorMessage_PreservesNormalDiagnostic(t *testing.T) {
	const input = "Your input exceeds the context window of this model. Please reduce the prompt."
	require.Equal(t, input, sanitizeUpstreamErrorMessage(input))
	require.Equal(t, input, sanitizeClientUpstreamErrorMessage(input))
}

func TestSanitizeClientUpstreamErrorMessage_StackOnlyFallsBackToStableMessage(t *testing.T) {
	require.Equal(t, "Upstream request failed", sanitizeClientUpstreamErrorMessage("panic: upstream crashed\n at handler (/srv/app.js:1:1)"))
	require.Empty(t, sanitizeClientUpstreamErrorMessage(""))
}

func TestSanitizeOpenAIResponseFailedEventForClient_RedactsErrorMessageOnly(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"id":"resp_1","model":"public-model","status":"failed","error":{"type":"invalid_request_error","code":"bad_request","message":"failed at https://private-upstream.example/v1/responses?key=secret"},"usage":{"input_tokens":3,"output_tokens":0}}}`)

	got, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "response.failed", false)

	require.True(t, changed)
	require.Equal(t, "response.failed", gjson.GetBytes(got, "type").String())
	require.Equal(t, "invalid_request_error", gjson.GetBytes(got, "response.error.type").String())
	require.Equal(t, "bad_request", gjson.GetBytes(got, "response.error.code").String())
	require.Contains(t, gjson.GetBytes(got, "response.error.message").String(), "[upstream URL]")
	require.NotContains(t, string(got), "private-upstream.example")
	require.False(t, gjson.GetBytes(got, "response.usage").Exists())
}

func TestOpenAIImagesUpstreamError_ClientMessageRedactsUpstreamRouting(t *testing.T) {
	err := &OpenAIImagesUpstreamError{Message: "invalid image request at https://private-upstream.example/v1/images/generations?key=secret"}

	got := err.clientMessage()

	require.Contains(t, got, "invalid image request")
	require.Contains(t, got, "[upstream URL]")
	require.NotContains(t, got, "private-upstream.example")
}

func TestSanitizeUpstreamErrorMessage_InternalKeepsRedactedURLForOps(t *testing.T) {
	got := sanitizeUpstreamErrorMessage("upstream failed: https://example.com/v1/responses?access_token=secret-value")
	require.Contains(t, got, "https://example.com/v1/responses")
	require.Contains(t, got, "access_token=***")
	require.NotContains(t, got, "secret-value")
}

func TestSanitizeUpstreamErrorResponseBody_PreservesSchemaAndNumericFields(t *testing.T) {
	body := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"invalid model; see https://upstream.example/v1/models","code":400},"request_id":"req_public"}`)

	got := sanitizeUpstreamErrorResponseBody(body)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(got, &payload))
	require.Equal(t, "error", payload["type"])
	errorPayload, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "invalid_request_error", errorPayload["type"])
	require.Equal(t, float64(400), errorPayload["code"])
	require.Contains(t, errorPayload["message"], "[upstream URL]")
	require.NotContains(t, string(got), "upstream.example")
	require.Equal(t, "req_public", payload["request_id"])
}
