package service

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardOpenAIVideoFormTaskRewritesModelAndReturnsTaskID(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("model", "sora-public"))
	require.NoError(t, writer.WriteField("prompt", "waves"))
	require.NoError(t, writer.WriteField("seconds", "6"))
	part, err := writer.CreateFormFile("input_reference[]", "ref.png")
	require.NoError(t, err)
	_, _ = part.Write([]byte("png"))
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(buf.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	account := &Account{
		ID:          71,
		Name:        "openai-video",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://video-upstream.test/v1",
			"model_mapping": map[string]any{
				"sora-public": "sora-upstream",
			},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"upstream-req"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"video-task-123","status":"queued","usage":{"input_tokens":2,"output_tokens":0}}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(c, buf.Bytes())
	require.NoError(t, err)
	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)

	require.Equal(t, "https://video-upstream.test/v1/videos", upstream.lastReq.URL.String())
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.Equal(t, "Bearer api-key", upstream.lastReq.Header.Get("Authorization"))
	require.Contains(t, upstream.lastReq.Header.Get("Content-Type"), "multipart/form-data")
	require.Equal(t, "video-task-123", result.ResponseID)
	require.Equal(t, "sora-public", result.Model)
	require.Equal(t, "sora-upstream", result.UpstreamModel)
	require.Equal(t, "sora-upstream", result.BillingModel)
	require.Equal(t, 1, result.ImageCount)

	upstreamReader := multipart.NewReader(bytes.NewReader(upstream.lastBody), strings.TrimPrefix(strings.Split(upstream.lastReq.Header.Get("Content-Type"), "boundary=")[1], `"`))
	foundModel := ""
	for {
		part, err := upstreamReader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if part.FormName() == "model" {
			data, _ := io.ReadAll(part)
			foundModel = string(data)
		}
		_ = part.Close()
	}
	require.Equal(t, "sora-upstream", foundModel)
	require.JSONEq(t, `{"id":"video-task-123","status":"queued","usage":{"input_tokens":2,"output_tokens":0}}`, recorder.Body.String())
}

func TestForwardOpenAIVideoJSONTaskSupportsSingularGenerationsPath(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"119337-grok-video","prompt":"city","duration":5}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          72,
		Name:        "openai-video",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://video-upstream.test/v1",
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"video-request-456","status":"queued"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)

	require.Equal(t, "https://video-upstream.test/v1/video/generations", upstream.lastReq.URL.String())
	require.JSONEq(t, string(body), string(upstream.lastBody))
	require.Equal(t, "video-request-456", result.ResponseID)
	require.Equal(t, 1, result.ImageCount)
}

func TestForwardOpenAIVideoSynchronousURLResponseGetsLocalTaskID(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"sync-video","prompt":"city","seconds":8}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          75,
		Name:        "openai-sync-video",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://video-upstream.test/v1",
		},
	}
	responseBody := []byte(`{"video_url":"https://cdn.test/video.mp4","duration":8}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"sync-upstream-req"},
		},
		Body: io.NopCloser(bytes.NewReader(responseBody)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)

	require.Equal(t, "https://video-upstream.test/v1/videos", upstream.lastReq.URL.String())
	require.Equal(t, localOpenAIVideoSynchronousTaskID("sync-upstream-req", responseBody), result.ResponseID)
	require.Equal(t, MediaGenerationStatusCompleted, result.VideoStatus)
	require.Equal(t, 8, result.MediaDurationSeconds)
	require.Equal(t, 1, result.ImageCount)
	require.JSONEq(t, string(responseBody), recorder.Body.String())
}

func TestOpenAIVideoHasResultURLSupportsNestedOutput(t *testing.T) {
	require.True(t, openAIVideoHasResultURL([]byte(`{"output":[{"url":"https://cdn.test/video.mp4"}]}`)))
	require.True(t, openAIVideoHasResultURL([]byte(`{"data":{"content":{"video_url":"https://cdn.test/video.mp4"}}}`)))
	require.False(t, openAIVideoHasResultURL([]byte(`{"status":"completed"}`)))
}

func TestOpenAIVideoTaskExtractorsSupportNestedTaskShapes(t *testing.T) {
	body := []byte(`{"data":{"task":{"task_id":"task_nested_123","state":"succeeded","duration_seconds":"12s"}}}`)

	require.Equal(t, "task_nested_123", extractOpenAIVideoTaskID(body))
	require.Equal(t, MediaGenerationStatusCompleted, extractOpenAIVideoStatus(body))
	require.Equal(t, 12, extractOpenAIVideoDurationSeconds(body))
}

func TestForwardOpenAIVideoContentTaskUsesVersionedPlanBaseURL(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"seedance-pro","prompt":"city","stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/contents/generations/tasks", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          74,
		Name:        "openai-video-plan",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://video-upstream.test/api/plan/v3",
			"model_mapping": map[string]any{
				"seedance-pro": "seedance-upstream",
			},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"task_id":"plan-task-789","status":"queued"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)

	require.Equal(t, "https://video-upstream.test/api/plan/v3/contents/generations/tasks", upstream.lastReq.URL.String())
	require.JSONEq(t, `{"model":"seedance-upstream","prompt":"city","stream":false}`, string(upstream.lastBody))
	require.Equal(t, "plan-task-789", result.ResponseID)
	require.Equal(t, 1, result.ImageCount)
}

func TestForwardOpenAIVideoStatusAndContentUseGETWithoutBillingUnit(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	account := &Account{
		ID:          73,
		Name:        "openai-video",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://video-upstream.test/v1",
		},
	}

	statusRecorder := httptest.NewRecorder()
	statusCtx, _ := gin.CreateTestContext(statusRecorder)
	statusCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video-task-123", nil)
	statusUpstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"video-task-123","status":"completed"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: statusUpstream}
	parsed, err := svc.ParseOpenAIVideoRequest(statusCtx, nil)
	require.NoError(t, err)
	result, err := svc.ForwardVideo(context.Background(), statusCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "https://video-upstream.test/v1/videos/video-task-123", statusUpstream.lastReq.URL.String())
	require.Equal(t, http.MethodGet, statusUpstream.lastReq.Method)
	require.Empty(t, statusUpstream.lastBody)
	require.Zero(t, result.ImageCount)
	require.JSONEq(t, `{"id":"video-task-123","status":"completed"}`, statusRecorder.Body.String())

	contentRecorder := httptest.NewRecorder()
	contentCtx, _ := gin.CreateTestContext(contentRecorder)
	contentCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video-task-123/content", nil)
	contentUpstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"video/mp4"}},
		Body:       io.NopCloser(strings.NewReader("mp4-bytes")),
	}}
	svc.httpUpstream = contentUpstream
	parsed, err = svc.ParseOpenAIVideoRequest(contentCtx, nil)
	require.NoError(t, err)
	result, err = svc.ForwardVideo(context.Background(), contentCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "https://video-upstream.test/v1/videos/video-task-123/content", contentUpstream.lastReq.URL.String())
	require.Equal(t, http.MethodGet, contentUpstream.lastReq.Method)
	require.Zero(t, result.ImageCount)
	require.Equal(t, "mp4-bytes", contentRecorder.Body.String())
}

func TestOpenAIVideoTaskSessionHashAndBind(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9)
	cache := &stubGatewayCache{}
	svc := &OpenAIGatewayService{cache: cache}

	hash := OpenAIVideoTaskSessionHash("video-task-123")
	require.NotEmpty(t, hash)
	require.NoError(t, svc.BindOpenAIVideoTaskAccount(ctx, &groupID, "video-task-123", 73))

	accountID, err := svc.getStickySessionAccountID(ctx, &groupID, hash)
	require.NoError(t, err)
	require.Equal(t, int64(73), accountID)
}
