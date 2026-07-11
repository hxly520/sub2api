package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveOpenAIVideoResponseBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("configured base wins", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "http://internal.local/v1/videos/task", nil)
		c.Request.Host = "internal.local"

		got := resolveOpenAIVideoResponseBaseURL(c, "https://api.52token.example")
		require.Equal(t, "https://api.52token.example", got)
	})

	t.Run("request origin fallback", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "http://sub2api.example/v1/videos/task", nil)
		c.Request.Host = "sub2api.example"
		c.Request.Header.Set("X-Forwarded-Proto", "https")

		got := resolveOpenAIVideoResponseBaseURL(c, "")
		require.Equal(t, "https://sub2api.example", got)
	})

	t.Run("invalid configured base falls back", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "http://sub2api.example/v1/videos/task", nil)
		c.Request.Host = "sub2api.example"

		got := resolveOpenAIVideoResponseBaseURL(c, "javascript:alert(1)")
		require.Equal(t, "http://sub2api.example", got)
	})
}

func TestOpenAIVideoTaskNeedsFinalization(t *testing.T) {
	task := &service.MediaGenerationTask{Status: service.MediaGenerationStatusCompleted}
	require.True(t, openAIVideoTaskNeedsFinalization(task))

	now := time.Now().UTC()
	task.UsageRecordedAt = &now
	require.False(t, openAIVideoTaskNeedsFinalization(task))

	task.UsageRecordedAt = nil
	task.Status = service.MediaGenerationStatusFailed
	require.False(t, openAIVideoTaskNeedsFinalization(task))
}

func TestOpenAIVideoForwardResultFromStoredTaskKeepsBillingDimensions(t *testing.T) {
	task := &service.MediaGenerationTask{
		PublicTaskID:        "video-public",
		UpstreamTaskID:      "provider-task",
		RequestedModel:      "seedance-2.0-fast-720p",
		UpstreamModel:       "cy-seedance-fast-720p",
		DurationSeconds:     12,
		Resolution:          service.VideoBillingResolution720P,
		Status:              service.MediaGenerationStatusCompleted,
		ResponseStatus:      http.StatusOK,
		ResponseContentType: "application/json",
	}

	result := openAIVideoForwardResultFromStoredTask(task)
	require.Equal(t, "video-public", result.RequestID)
	require.Equal(t, "provider-task", result.ResponseID)
	require.Equal(t, 1, result.VideoCount)
	require.Equal(t, 12, result.VideoDurationSeconds)
	require.Equal(t, service.VideoBillingResolution720P, result.VideoResolution)
	require.Equal(t, "video", result.MediaType)
}

func TestApplyOpenAIVideoStoredTaskToForwardResultUsesPersistedTerminalState(t *testing.T) {
	result := &service.OpenAIForwardResult{
		ResponseID:     "provider-late",
		ResponseBody:   []byte(`{"status":"pending"}`),
		VideoStatus:    service.MediaGenerationStatusPending,
		MediaResultURL: "https://late.example/video.mp4",
	}
	task := &service.MediaGenerationTask{
		PublicTaskID:        "video-public",
		UpstreamTaskID:      "provider-first",
		RequestedModel:      "grok-video",
		UpstreamModel:       "grok-upstream",
		Status:              service.MediaGenerationStatusCompleted,
		ResponseStatus:      http.StatusOK,
		ResponseContentType: "application/json",
		ResponseBody:        `{"id":"video-public","status":"completed","video_url":"/v1/videos/video-public/content"}`,
		UpstreamResultURL:   "https://first.example/video.mp4",
		DurationSeconds:     15,
		Resolution:          service.VideoBillingResolution720P,
	}

	applyOpenAIVideoStoredTaskToForwardResult(result, task)

	require.Equal(t, "provider-first", result.ResponseID)
	require.Equal(t, service.MediaGenerationStatusCompleted, result.VideoStatus)
	require.Equal(t, task.ResponseBody, string(result.ResponseBody))
	require.Equal(t, "https://first.example/video.mp4", result.MediaResultURL)
	require.Equal(t, 15, result.MediaDurationSeconds)
	require.Equal(t, service.VideoBillingResolution720P, result.VideoResolution)
}
