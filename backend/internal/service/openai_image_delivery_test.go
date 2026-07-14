package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareOpenAIImageClientResponseBodyUsesEncryptedImageEdgeURL(t *testing.T) {
	keyHex := strings.Repeat("33", 32)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
		Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: keyHex, TokenTTLSeconds: 3600,
	}}}}
	rawURL := "http://1.1.1.1/generated.png?signature=secret"

	got, err := svc.PrepareOpenAIImageClientResponseBody(
		context.Background(),
		[]byte(`{"data":[{"url":"`+rawURL+`"}]}`),
	)

	require.NoError(t, err)
	clientURL := gjson.GetBytes(got, "data.0.url").String()
	prefix := "https://video.52token.example/v1/image-content/"
	require.True(t, strings.HasPrefix(clientURL, prefix))
	require.NotContains(t, clientURL, "1.1.1.1")
	require.NotContains(t, clientURL, "signature")
	payload := decryptOpenAIVideoEdgeTokenForTest(t, keyHex, strings.TrimPrefix(clientURL, prefix))
	require.Equal(t, "image", payload.MediaType)
	require.Equal(t, rawURL, payload.URL)
}

func TestPrepareOpenAIImageClientResponseBodyRequiresEdgeForRemoteURL(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	got, err := svc.PrepareOpenAIImageClientResponseBody(
		context.Background(),
		[]byte(`{"data":[{"url":"https://1.1.1.1/generated.png"}]}`),
	)

	require.Error(t, err)
	require.Nil(t, got)
}

func TestPrepareOpenAIImageClientResponseBodyRejectsPrivateURL(t *testing.T) {
	keyHex := strings.Repeat("33", 32)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
		Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: keyHex, TokenTTLSeconds: 3600,
	}}}}

	got, err := svc.PrepareOpenAIImageClientResponseBody(
		context.Background(),
		[]byte(`{"data":[{"url":"http://127.0.0.1/generated.png"}]}`),
	)

	require.Error(t, err)
	require.Nil(t, got)
}

func TestPrepareOpenAIImageClientResponseBodyPreservesEmbeddedImageData(t *testing.T) {
	body := []byte(`{"data":[{"b64_json":"aW1hZ2U="},{"url":"data:image/png;base64,aW1hZ2U="}]}`)

	got, err := (&OpenAIGatewayService{}).PrepareOpenAIImageClientResponseBody(context.Background(), body)

	require.NoError(t, err)
	require.JSONEq(t, string(body), string(got))
}

func TestPrepareOpenAIImageClientResponseBodyRemovesUnknownProviderURLs(t *testing.T) {
	body := []byte(`{"id":"task-public","status":"queued","status_url":"https://provider.example/tasks/private","message":"see https://provider.example/docs"}`)

	got, err := (&OpenAIGatewayService{}).PrepareOpenAIImageClientResponseBody(context.Background(), body)

	require.NoError(t, err)
	require.Equal(t, "", gjson.GetBytes(got, "status_url").String())
	require.Equal(t, "", gjson.GetBytes(got, "message").String())
	require.NotContains(t, string(got), "provider.example")
}

func TestPrepareOpenAIImageClientResponseBodyRewritesOutputURLArrays(t *testing.T) {
	keyHex := strings.Repeat("33", 32)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
		Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: keyHex, TokenTTLSeconds: 3600,
	}}}}

	got, err := svc.PrepareOpenAIImageClientResponseBody(
		context.Background(),
		[]byte(`{"output":["https://1.1.1.1/generated.png"]}`),
	)

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(gjson.GetBytes(got, "output.0").String(), "https://video.52token.example/v1/image-content/"))
}

func TestPrepareOpenAIImageClientResponseBodyDoesNotProxyPromptURL(t *testing.T) {
	keyHex := strings.Repeat("33", 32)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
		Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: keyHex, TokenTTLSeconds: 3600,
	}}}}

	got, err := svc.PrepareOpenAIImageClientResponseBody(
		context.Background(),
		[]byte(`{"data":[{"url":"https://1.1.1.1/generated.png","revised_prompt":"https://prompt.example/reference.png"}]}`),
	)

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(gjson.GetBytes(got, "data.0.url").String(), "https://video.52token.example/v1/image-content/"))
	require.Empty(t, gjson.GetBytes(got, "data.0.revised_prompt").String())
	require.NotContains(t, string(got), "prompt.example")
}

func TestPrepareOpenAIImageClientStreamLineRewritesRemoteURL(t *testing.T) {
	keyHex := strings.Repeat("33", 32)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
		Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: keyHex, TokenTTLSeconds: 3600,
	}}}}

	got, err := svc.PrepareOpenAIImageClientStreamLine(
		context.Background(),
		[]byte("data: {\"data\":[{\"url\":\"https://1.1.1.1/generated.png?secret=1\"}]}\n"),
	)

	require.NoError(t, err)
	require.Contains(t, string(got), "https://video.52token.example/v1/image-content/")
	require.NotContains(t, string(got), "1.1.1.1")
	require.NotContains(t, string(got), "secret=1")
}

func TestPrepareOpenAIImageClientStreamLineRejectsExternalURLInMalformedData(t *testing.T) {
	got, err := (&OpenAIGatewayService{}).PrepareOpenAIImageClientStreamLine(
		context.Background(),
		[]byte("data: {\"status_url\":\"https://provider.example/task\"\n"),
	)

	require.Error(t, err)
	require.Nil(t, got)
}

func TestPrepareOpenAIImageClientResponseHeadersRemovesProviderLocations(t *testing.T) {
	header := http.Header{
		"Location":      []string{"https://provider.example/private.png"},
		"Set-Cookie":    []string{"provider=secret"},
		"ETag":          []string{"provider-etag"},
		"Cache-Control": []string{"public, max-age=86400"},
		"X-Request-Id":  []string{"request-123"},
	}

	PrepareOpenAIImageClientResponseHeaders(header)

	require.Empty(t, header.Get("Location"))
	require.Empty(t, header.Get("Set-Cookie"))
	require.Empty(t, header.Get("ETag"))
	require.Equal(t, "no-store", header.Get("Cache-Control"))
	require.Equal(t, "request-123", header.Get("X-Request-Id"))
}
