package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoClientContentURLOriginKeepsExistingProxyURL(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	got, err := svc.OpenAIVideoClientContentURL(context.Background(), "https://api.52token.example/v1", "video-public-123", "https://1.1.1.1/private/video.mp4?signature=secret")
	require.NoError(t, err)
	require.Equal(t, "https://api.52token.example/v1/videos/video-public-123/content", got)
	require.NotContains(t, got, "1.1.1.1")
	require.NotContains(t, got, "signature")
}

func TestOpenAIVideoClientContentURLEdgeEncryptsExactSignedURL(t *testing.T) {
	keyHex := strings.Repeat("11", 32)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
		Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: keyHex, TokenTTLSeconds: 3600,
	}}}}
	upstreamURL := "https://1.1.1.1/signed/path/?asset=a%2Fb&signature=secret"
	first, err := svc.OpenAIVideoClientContentURL(context.Background(), "", "video-public-123", upstreamURL)
	require.NoError(t, err)
	second, err := svc.OpenAIVideoClientContentURL(context.Background(), "", "video-public-123", upstreamURL)
	require.NoError(t, err)
	prefix := "https://video.52token.example/v1/video-content/"
	require.True(t, strings.HasPrefix(first, prefix))
	require.True(t, strings.HasPrefix(second, prefix))
	require.NotEqual(t, first, second)
	require.NotContains(t, first, "1.1.1.1")
	require.NotContains(t, first, "signature")
	payload := decryptOpenAIVideoEdgeTokenForTest(t, keyHex, strings.TrimPrefix(first, prefix))
	require.Equal(t, 1, payload.Version)
	require.Equal(t, "video", payload.MediaType)
	require.Equal(t, upstreamURL, payload.URL)
	require.WithinDuration(t, time.Now().UTC().Add(time.Hour), time.Unix(payload.ExpiresAt, 0).UTC(), 5*time.Second)
}

func TestOpenAIVideoClientContentURLEdgeRejectsPrivateUpstream(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
		Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: strings.Repeat("11", 32), TokenTTLSeconds: 3600,
	}}}}
	got, err := svc.OpenAIVideoClientContentURL(context.Background(), "", "video-public-123", "https://127.0.0.1/private.mp4")
	require.Error(t, err)
	require.Empty(t, got)
}

func TestOpenAIVideoClientContentURLForTaskEncryptsAuthenticatedContentEndpoint(t *testing.T) {
	keyHex := strings.Repeat("22", 32)
	account := Account{
		ID:       17,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://1.1.1.1/v1",
			"api_key":  "upstream-secret",
		},
	}
	svc := &OpenAIGatewayService{
		accountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		cfg: &config.Config{Gateway: config.GatewayConfig{VideoProxy: config.VideoProxyConfig{
			Mode: config.VideoProxyModeEdge, EdgeBaseURL: "https://video.52token.example", EncryptionKey: keyHex, TokenTTLSeconds: 3600,
		}}},
	}
	task := &MediaGenerationTask{
		TaskID:            "video-public-123",
		PublicTaskID:      "video-public-123",
		UpstreamTaskID:    "provider-task-456",
		UpstreamEndpoint:  openAIVideosEndpoint,
		UpstreamResultURL: "/v1/videos/provider-task-456/content",
		AccountID:         account.ID,
	}

	got, err := svc.OpenAIVideoClientContentURLForTask(context.Background(), "", task)
	require.NoError(t, err)
	prefix := "https://video.52token.example/v1/video-content/"
	require.True(t, strings.HasPrefix(got, prefix))
	require.NotContains(t, got, "provider-task-456")
	require.NotContains(t, got, "upstream-secret")

	payload := decryptOpenAIVideoEdgeTokenForTest(t, keyHex, strings.TrimPrefix(got, prefix))
	require.Equal(t, "https://1.1.1.1/v1/videos/provider-task-456/content", payload.URL)
	require.Equal(t, "Bearer upstream-secret", payload.Headers["Authorization"])
}

func decryptOpenAIVideoEdgeTokenForTest(t *testing.T, keyHex, token string) openAIVideoEdgeTokenPayload {
	t.Helper()
	key, err := hex.DecodeString(keyHex)
	require.NoError(t, err)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	require.Greater(t, len(sealed), gcm.NonceSize())
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(openAIVideoEdgeTokenAAD))
	require.NoError(t, err)
	var payload openAIVideoEdgeTokenPayload
	require.NoError(t, json.Unmarshal(plain, &payload))
	return payload
}
