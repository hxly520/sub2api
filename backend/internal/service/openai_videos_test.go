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
	"github.com/tidwall/gjson"
)

func TestForwardOpenAIVideoFormTaskRewritesModelAndReturnsTaskID(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("model", "omni-fast-public"))
	require.NoError(t, writer.WriteField("prompt", "waves"))
	require.NoError(t, writer.WriteField("seconds", "10"))
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
				"omni-fast-public": "omni-fast-upstream",
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
	require.Equal(t, "omni-fast-public", result.Model)
	require.Equal(t, "omni-fast-upstream", result.UpstreamModel)
	require.Equal(t, "omni-fast-upstream", result.BillingModel)
	require.Zero(t, result.ImageCount)
	require.Equal(t, 1, result.VideoCount)

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
	require.Equal(t, "omni-fast-upstream", foundModel)
	require.Empty(t, recorder.Body.String())
}

func TestForwardOpenAIVideoJSONTaskMapsCompatibilityAliasToUnifiedVideosAndDefaultsContentType(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"119337-grok-video","prompt":"city","duration":6}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(body))

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
	require.Equal(t, "application/json", parsed.ContentType)
	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)

	require.Equal(t, "https://video-upstream.test/v1/videos", upstream.lastReq.URL.String())
	require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
	require.JSONEq(t, string(body), string(upstream.lastBody))
	require.Equal(t, "video-request-456", result.ResponseID)
	require.Zero(t, result.ImageCount)
	require.Equal(t, 1, result.VideoCount)
}

func TestForwardOpenAIVideoMinimaxH32KPassesUnifiedJSONProtocol(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"minimax-h3-2k","prompt":"city at night","duration":15,"aspect_ratio":"16:9","resolution":"2k","reference_image_urls":["https://media.test/1.png","https://media.test/2.png","https://media.test/3.png","https://media.test/4.png","https://media.test/5.png"],"reference_audios":["https://media.test/1.mp3","https://media.test/2.mp3","https://media.test/3.mp3"]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          721,
		Name:        "openai-minimax-video",
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
		Body:       io.NopCloser(strings.NewReader(`{"id":"minimax-video-task-123","status":"queued"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	require.Equal(t, "minimax-h3-2k", parsed.Model)
	require.Equal(t, "2k", parsed.Resolution)
	require.Equal(t, 15, parsed.DurationSeconds)
	require.Equal(t, 5, parsed.ReferenceImageCount)
	require.Equal(t, 3, parsed.ReferenceAudioCount)
	require.Zero(t, parsed.ReferenceVideoCount)

	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "https://video-upstream.test/v1/videos", upstream.lastReq.URL.String())
	require.Equal(t, http.MethodPost, upstream.lastReq.Method)
	require.JSONEq(t, string(body), string(upstream.lastBody))
	require.Equal(t, "minimax-video-task-123", result.ResponseID)
	require.Equal(t, "minimax-h3-2k", result.Model)
	require.Equal(t, "minimax-h3-2k", result.UpstreamModel)
	require.Equal(t, "2k", result.VideoResolution)
	require.Equal(t, 15, result.VideoDurationSeconds)
}

func TestForwardOpenAIVideoJSONTaskSupportsPluralVideosGenerationsPath(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"grok-imagine-video-1.5","prompt":"city","seconds":6,"image_url":"https://images.test/ref.png","aspect_ratio":"16:9","resolution":"720p"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          76,
		Name:        "openai-grok-compatible-video",
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
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"grok-video-request-789","status":"queued"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	require.True(t, parsed.GenerationRequest)
	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)

	require.Equal(t, "https://video-upstream.test/v1/videos", upstream.lastReq.URL.String())
	require.JSONEq(t, string(body), string(upstream.lastBody))
	require.Equal(t, "grok-video-request-789", result.ResponseID)
	require.Zero(t, result.ImageCount)
	require.Equal(t, 1, result.VideoCount)
}

func TestForwardOpenAIVideoMappedGrokUsesMappedContractAndStableIdempotency(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"public-video","prompt":"city","duration":6}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          77,
		Name:        "mapped-grok-video",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://video-upstream.test/v1",
			"model_mapping": map[string]any{
				"public-video": "grok-video",
			},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"mapped-grok-task","status":"queued"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	parsed.UpstreamIdempotencyKey = "video-public-task"
	result, err := svc.ForwardVideo(context.Background(), c, account, parsed, "")
	require.NoError(t, err)

	require.Equal(t, "https://video-upstream.test/v1/videos", upstream.lastReq.URL.String())
	require.Equal(t, "video-public-task", upstream.lastReq.Header.Get("Idempotency-Key"))
	require.Equal(t, "grok-video", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "mapped-grok-task", result.ResponseID)
}

func TestRewriteOpenAIVideoRequestClampsGrokMultiImageDuration(t *testing.T) {
	body := []byte(`{"model":"grok-video","prompt":"city","duration":15,"image_urls":["a","b"]}`)
	parsed := &OpenAIVideoRequest{
		Model:               "grok-video",
		DurationSeconds:     15,
		ReferenceImageCount: 2,
	}

	rewritten, contentType, err := rewriteOpenAIVideoRequest(body, "application/json", "grok-video", parsed)
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.Equal(t, int64(10), gjson.GetBytes(rewritten, "seconds").Int())
	require.Equal(t, int64(10), gjson.GetBytes(rewritten, "duration").Int())
}

func TestForwardOpenAIVideoCangyuanGrokContractLifecycle(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	createBody := []byte(`{"model":"grok-video","prompt":"city in rain","duration":15,"resolution":"720p","aspect_ratio":"16:9"}`)
	createCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	createCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(createBody))
	createCtx.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          77,
		Name:        "cangyuan-grok-video",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://cangyuan-upstream.test/v1",
		},
	}
	upstreamTaskID := "task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"created_at":1780000000,
				"id":"task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
				"model":"grok-video",
				"object":"video",
				"progress":0,
				"status":"queued",
				"task_id":"task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
			}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"code":"success",
				"data":{
					"fail_reason":"",
					"progress":"100%",
					"result_url":"https://upstream-media.test/generated-video.mp4",
					"status":"SUCCESS",
					"task_id":"task_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
				}
			}`)),
		},
		{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Type":   []string{"video/mp4"},
				"Content-Length": []string{"10"},
				"Content-Range":  []string{"bytes 0-9/10"},
			},
			Body: io.NopCloser(strings.NewReader("video-data")),
		},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(createCtx, createBody)
	require.NoError(t, err)
	created, err := svc.ForwardVideo(context.Background(), createCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "https://cangyuan-upstream.test/v1/videos", upstream.requests[0].URL.String())
	require.Equal(t, upstreamTaskID, created.ResponseID)
	require.Equal(t, MediaGenerationStatusPending, created.VideoStatus)
	require.Equal(t, 15, created.VideoDurationSeconds)

	statusCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	statusCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+upstreamTaskID, nil)
	parsed, err = svc.ParseOpenAIVideoRequest(statusCtx, nil)
	require.NoError(t, err)
	parsed.Model = "grok-video"
	require.NoError(t, parsed.UseUpstreamTaskIDAtEndpoint(upstreamTaskID, openAIVideosEndpoint))
	statusResult, err := svc.ForwardVideo(context.Background(), statusCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "https://cangyuan-upstream.test/v1/videos/"+upstreamTaskID, upstream.requests[1].URL.String())
	require.Equal(t, upstreamTaskID, statusResult.ResponseID)
	require.Equal(t, MediaGenerationStatusCompleted, statusResult.VideoStatus)
	require.Equal(t, "https://upstream-media.test/generated-video.mp4", statusResult.MediaResultURL)

	clientBody := RewriteOpenAIVideoClientResponseBodyWithBaseURL(
		statusResult.ResponseBody,
		"video-public-grok",
		"https://api.52token.org",
		upstreamTaskID,
	)
	require.Equal(t, "video-public-grok", gjson.GetBytes(clientBody, "data.task_id").String())
	require.Equal(t, "https://api.52token.org/v1/videos/video-public-grok/content", gjson.GetBytes(clientBody, "data.result_url").String())
	require.NotContains(t, string(clientBody), "cangyuan-upstream.test")
	require.NotContains(t, string(clientBody), "upstream-media.test")
	require.NotContains(t, string(clientBody), upstreamTaskID)

	contentRecorder := httptest.NewRecorder()
	contentCtx, _ := gin.CreateTestContext(contentRecorder)
	contentCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+upstreamTaskID+"/content", nil)
	contentCtx.Request.Header.Set("Range", "bytes=0-9")
	parsed, err = svc.ParseOpenAIVideoRequest(contentCtx, nil)
	require.NoError(t, err)
	parsed.Model = "grok-video"
	require.NoError(t, parsed.UseUpstreamTaskIDAtEndpoint(upstreamTaskID, openAIVideosEndpoint))
	_, err = svc.ForwardVideo(context.Background(), contentCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "https://cangyuan-upstream.test/v1/videos/"+upstreamTaskID+"/content", upstream.requests[2].URL.String())
	require.Equal(t, "bytes=0-9", upstream.requests[2].Header.Get("Range"))
	require.Equal(t, http.StatusPartialContent, contentRecorder.Code)
	require.Equal(t, "video/mp4", contentRecorder.Header().Get("Content-Type"))
	require.Equal(t, "video-data", contentRecorder.Body.String())
}

func TestForwardOpenAIVideoCangyuanSeedanceContractLifecycle(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	createBody := []byte(`{"aspect_ratio":"16:9","duration":8,"model":"seedance-2.0-fast-720p","prompt":"rainy neon street"}`)
	createCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	createCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(createBody))
	createCtx.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          78,
		Name:        "cangyuan-seedance-video",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://cangyuan-upstream.test/v1",
		},
	}
	upstreamTaskID := "task_01HZX8A2"
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"created_at":"2026-05-17T08:00:00Z",
				"id":"task_01HZX8A2",
				"progress":0,
				"status":"queued"
			}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"task_01HZX8A2",
				"progress":100,
				"status":"completed",
				"video_url":"https://upstream-media.test/output.mp4"
			}`)),
		},
		{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"error":{"code":"400017","message":"reference image rejected"},
				"error_code":"400017",
				"id":"task_01HZX8A2",
				"status":"failed",
				"video_url":null
			}`)),
		},
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	parsed, err := svc.ParseOpenAIVideoRequest(createCtx, createBody)
	require.NoError(t, err)
	created, err := svc.ForwardVideo(context.Background(), createCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, "https://cangyuan-upstream.test/v1/videos", upstream.requests[0].URL.String())
	require.Equal(t, upstreamTaskID, created.ResponseID)
	require.Equal(t, MediaGenerationStatusPending, created.VideoStatus)
	require.Equal(t, VideoBillingResolution720P, created.VideoResolution)
	require.Equal(t, 8, created.VideoDurationSeconds)

	statusRequest := func() *gin.Context {
		statusCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		statusCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+upstreamTaskID, nil)
		return statusCtx
	}
	statusCtx := statusRequest()
	parsed, err = svc.ParseOpenAIVideoRequest(statusCtx, nil)
	require.NoError(t, err)
	statusResult, err := svc.ForwardVideo(context.Background(), statusCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, MediaGenerationStatusCompleted, statusResult.VideoStatus)
	require.Equal(t, "https://upstream-media.test/output.mp4", statusResult.MediaResultURL)
	clientBody := RewriteOpenAIVideoClientResponseBodyWithBaseURL(
		statusResult.ResponseBody,
		"video-public-seedance",
		"https://api.52token.org/v1",
		upstreamTaskID,
	)
	require.Equal(t, "video-public-seedance", gjson.GetBytes(clientBody, "id").String())
	require.Equal(t, "https://api.52token.org/v1/videos/video-public-seedance/content", gjson.GetBytes(clientBody, "video_url").String())
	require.NotContains(t, string(clientBody), "upstream-media.test")

	failedCtx := statusRequest()
	parsed, err = svc.ParseOpenAIVideoRequest(failedCtx, nil)
	require.NoError(t, err)
	failedResult, err := svc.ForwardVideo(context.Background(), failedCtx, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, MediaGenerationStatusFailed, failedResult.VideoStatus)
	require.Empty(t, failedResult.MediaResultURL)
	require.Zero(t, failedResult.VideoCount)
	safeFailure := SanitizeOpenAIVideoStoredResponseBody(failedResult.ResponseBody, failedResult.VideoStatus)
	require.NotContains(t, string(safeFailure), "reference image rejected")
	require.JSONEq(t, `{"status":"failed","error":{"message":"Video generation failed","type":"upstream_error","param":null,"code":null}}`, string(safeFailure))
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
	require.Zero(t, result.ImageCount)
	require.Equal(t, 1, result.VideoCount)
	require.JSONEq(t, string(responseBody), string(result.ResponseBody))
	require.Empty(t, recorder.Body.String())
	rewritten := RewriteOpenAIVideoClientResponseBody(result.ResponseBody, "video-public-123")
	require.NotContains(t, string(rewritten), "https://cdn.test/video.mp4")
	require.JSONEq(t, `{"id":"video-public-123","video_url":"/v1/videos/video-public-123/content","duration":8}`, string(rewritten))
}

func TestOpenAIVideoHasResultURLSupportsNestedOutput(t *testing.T) {
	require.True(t, openAIVideoHasResultURL([]byte(`{"output":[{"url":"https://cdn.test/video.mp4"}]}`)))
	require.True(t, openAIVideoHasResultURL([]byte(`{"result_url":"https://cdn.test/video.mp4"}`)))
	require.True(t, openAIVideoHasResultURL([]byte(`{"output":["https://cdn.test/video.mp4"]}`)))
	require.True(t, openAIVideoHasResultURL([]byte(`{"data":{"result":{"result_url":"https://cdn.test/video.mp4"}}}`)))
	require.True(t, openAIVideoHasResultURL([]byte(`{"data":{"content":{"video_url":"https://cdn.test/video.mp4"}}}`)))
	require.False(t, openAIVideoHasResultURL([]byte(`{"status":"completed"}`)))
}

func TestExtractOpenAIVideoTaskMetadataSupportsNestedResult(t *testing.T) {
	body := []byte(`{"data":{"result":{"task_id":"nested-task","status":"GENERATION_FAILED","duration_seconds":12}}}`)

	require.Equal(t, "nested-task", extractOpenAIVideoTaskID(body))
	require.Equal(t, MediaGenerationStatusFailed, extractOpenAIVideoStatus(body))
	require.Equal(t, 12, extractOpenAIVideoDurationSeconds(body))
}

func TestRewriteOpenAIVideoClientResponseBodyReplacesAllResultURLs(t *testing.T) {
	body := []byte(`{"result_url":"https://cdn.test/a.mp4","video_url":"https://cdn.test/b.mp4","content":{"url":"https://cdn.test/c.mp4","result_url":"https://cdn.test/d.mp4"},"output":["https://cdn.test/e.mp4",{"url":"https://cdn.test/f.mp4"},{"video_url":"https://cdn.test/g.mp4"},{"result_url":"https://cdn.test/h.mp4"}],"data":{"result":{"result_url":"https://cdn.test/i.mp4"},"output":[{"url":"https://cdn.test/j.mp4"}]}}`)

	rewritten := RewriteOpenAIVideoClientResponseBody(body, "task_123")

	require.NotContains(t, string(rewritten), "https://cdn.test")
	for _, path := range []string{
		"result_url",
		"video_url",
		"content.url",
		"content.result_url",
		"output.0",
		"output.1.url",
		"output.2.video_url",
		"output.3.result_url",
		"data.result.result_url",
		"data.output.0.url",
	} {
		require.Equal(t, "/v1/videos/task_123/content", gjson.GetBytes(rewritten, path).String(), path)
	}
}

func TestStreamOpenAIVideoTaskContentProxiesStoredResultURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_123/content", nil)
	c.Request.Header.Set("Range", "bytes=0-3")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Content-Type":  []string{"video/mp4"},
			"Content-Range": []string{"bytes 0-3/9"},
		},
		Body: io.NopCloser(strings.NewReader("mp4!")),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	task := &MediaGenerationTask{
		TaskID:       "task_123",
		AccountID:    73,
		ResponseBody: `{"result_url":"https://cdn.test/video.mp4"}`,
	}

	handled, err := svc.StreamOpenAIVideoTaskContent(context.Background(), c, task)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "https://cdn.test/video.mp4", upstream.lastReq.URL.String())
	require.Equal(t, "bytes=0-3", upstream.lastReq.Header.Get("Range"))
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "mp4!", recorder.Body.String())
	require.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
}

func TestStreamOpenAIVideoTaskContentDoesNotExposeRedirectLocation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_123/content", nil)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusFound,
		Header: http.Header{
			"Location": []string{"https://cdn.test/redirected-video.mp4"},
		},
		Body: io.NopCloser(strings.NewReader("")),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	task := &MediaGenerationTask{
		TaskID:       "task_123",
		AccountID:    73,
		ResponseBody: `{"video_url":"https://cdn.test/video.mp4"}`,
	}

	handled, err := svc.StreamOpenAIVideoTaskContent(context.Background(), c, task)
	require.Error(t, err)
	require.True(t, handled)
	require.Empty(t, recorder.Header().Get("Location"))
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestOpenAIVideoTaskExtractorsSupportNestedTaskShapes(t *testing.T) {
	body := []byte(`{"data":{"task":{"task_id":"task_nested_123","state":"succeeded","duration_seconds":"12s"}}}`)

	require.Equal(t, "task_nested_123", extractOpenAIVideoTaskID(body))
	require.Equal(t, MediaGenerationStatusCompleted, extractOpenAIVideoStatus(body))
	require.Equal(t, 12, extractOpenAIVideoDurationSeconds(body))
}

func TestForwardOpenAIVideoLegacyContentTaskUsesRelayVideosPath(t *testing.T) {
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

	require.Equal(t, "https://video-upstream.test/api/plan/v3/videos", upstream.lastReq.URL.String())
	require.JSONEq(t, `{"model":"seedance-upstream","prompt":"city","stream":false}`, string(upstream.lastBody))
	require.Equal(t, "plan-task-789", result.ResponseID)
	require.Zero(t, result.ImageCount)
	require.Equal(t, 1, result.VideoCount)
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
	require.Empty(t, statusRecorder.Body.String())
	require.JSONEq(t, `{"id":"video-task-123","status":"completed"}`, string(result.ResponseBody))

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

func TestOpenAIVideoUseUpstreamTaskIDPrefersStoredProtocol(t *testing.T) {
	req := &OpenAIVideoRequest{
		Endpoint:       openAIVideosEndpoint,
		Model:          "opaque-upstream-model",
		ContentRequest: true,
	}

	err := req.UseUpstreamTaskIDAtEndpoint("provider-task-123", openAIVideoGenerationsEndpoint)
	require.NoError(t, err)
	require.Equal(t, "/v1/video/generations/provider-task-123/content", req.UpstreamPath)
}

func TestOpenAIVideoUseUpstreamTaskIDPreservesStoredGrokUnifiedEndpoint(t *testing.T) {
	req := &OpenAIVideoRequest{
		Endpoint: openAIVideosEndpoint,
		Model:    "grok-video",
	}

	err := req.UseUpstreamTaskIDAtEndpoint("provider-task-123", openAIVideosEndpoint)
	require.NoError(t, err)
	require.Equal(t, "/v1/videos/provider-task-123", req.UpstreamPath)
}

func TestOpenAIVideoUseUpstreamTaskIDRejectsUnknownStoredProtocol(t *testing.T) {
	req := &OpenAIVideoRequest{Endpoint: openAIVideosEndpoint, Model: "seedance-2.0"}

	err := req.UseUpstreamTaskIDAtEndpoint("provider-task-123", "https://unexpected.example/private")
	require.NoError(t, err)
	require.Equal(t, "/v1/videos/provider-task-123", req.UpstreamPath)
}

func TestParseOpenAIVideoRequestRejectsAmbiguousEndpointPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"seedance-pro","prompt":"city","duration":6}`)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "similar prefix", method: http.MethodPost, path: "/v1/videos-evil"},
		{name: "duplicate separator", method: http.MethodGet, path: "/v1/videos//task-123"},
		{name: "extra segment", method: http.MethodGet, path: "/v1/videos/task-123/content/extra"},
		{name: "unsupported method", method: http.MethodDelete, path: "/v1/videos/task-123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(tt.method, tt.path, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			_, err := svc.ParseOpenAIVideoRequest(c, body)
			require.Error(t, err)
		})
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-123/", nil)
	parsed, err := svc.ParseOpenAIVideoRequest(c, nil)
	require.NoError(t, err)
	require.Equal(t, "task-123", parsed.RequestID)
}

func TestParseOpenAIVideoRequestRejectsStreamingGeneration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"seedance-pro","prompt":"city","stream":true}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	_, err := (&OpenAIGatewayService{}).ParseOpenAIVideoRequest(c, body)
	require.EqualError(t, err, "streaming video generation is not supported")
}

func TestParseOpenAIVideoRequestTreatsAudioToggleSeparatelyFromAudioReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, audio := range []string{"true", "false"} {
		t.Run(audio, func(t *testing.T) {
			body := []byte(`{"model":"seedance-2.0","prompt":"city","duration":6,"resolution":"720p","audio":` + audio + `}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			parsed, err := (&OpenAIGatewayService{}).ParseOpenAIVideoRequest(c, body)
			require.NoError(t, err)
			require.Zero(t, parsed.ReferenceAudioCount)
		})
	}

	body := []byte(`{"model":"seedance-2.0","prompt":"city","duration":6,"resolution":"720p","image_url":"https://media.test/ref.png","audio":"https://media.test/ref.mp3"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	parsed, err := (&OpenAIGatewayService{}).ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	require.Equal(t, 1, parsed.ReferenceAudioCount)

	body = []byte(`{"model":"seedance-2.0","prompt":"city","duration":6,"resolution":"720p","reference_audios":["https://media.test/ref.mp3"]}`)
	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	_, err = (&OpenAIGatewayService{}).ParseOpenAIVideoRequest(c, body)
	require.ErrorContains(t, err, "require at least one image")
}

func TestParseOpenAIVideoMultipartDoesNotCountAudioToggleAsReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "seedance-2.0"))
	require.NoError(t, writer.WriteField("prompt", "city"))
	require.NoError(t, writer.WriteField("duration", "6"))
	require.NoError(t, writer.WriteField("resolution", "720p"))
	require.NoError(t, writer.WriteField("audio", "true"))
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	parsed, err := (&OpenAIGatewayService{}).ParseOpenAIVideoRequest(c, body.Bytes())
	require.NoError(t, err)
	require.Zero(t, parsed.ReferenceAudioCount)
}

func TestValidateOpenAIVideoModelRequestMatrix(t *testing.T) {
	tests := []struct {
		name    string
		request OpenAIVideoRequest
		wantErr string
	}{
		{name: "grok valid", request: OpenAIVideoRequest{Model: "grok-video", Prompt: "motion", DurationSeconds: 15}},
		{name: "grok invalid duration", request: OpenAIVideoRequest{Model: "grok-video", Prompt: "motion", DurationSeconds: 7}, wantErr: "duration"},
		{name: "grok multi image clamps upstream", request: OpenAIVideoRequest{Model: "grok-video", Prompt: "motion", DurationSeconds: 12, ReferenceImageCount: 2}},
		{name: "grok invalid ratio", request: OpenAIVideoRequest{Model: "grok-video", Prompt: "motion", AspectRatio: "21:9"}, wantErr: "aspect_ratio"},
		{name: "grok 1.5 missing image", request: OpenAIVideoRequest{Model: "grok-imagine-video-1.5", Prompt: "motion", DurationSeconds: 6, AspectRatio: "16:9", Resolution: "720p"}, wantErr: "exactly one"},
		{name: "grok 1.5 invalid ratio", request: OpenAIVideoRequest{Model: "grok-imagine-video-1.5", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 1, AspectRatio: "1:1", Resolution: "720p"}, wantErr: "aspect_ratio"},
		{name: "grok 1.5 invalid resolution", request: OpenAIVideoRequest{Model: "grok-imagine-video-1.5", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 1, AspectRatio: "16:9", Resolution: "1080p"}, wantErr: "resolution"},
		{name: "grok 1.5 valid", request: OpenAIVideoRequest{Model: "grok-imagine-video-1.5", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 1, AspectRatio: "9:16", Resolution: "480p"}},
		{name: "seedance invalid duration", request: OpenAIVideoRequest{Model: "seedance-2.0", Prompt: "motion", DurationSeconds: 16}, wantErr: "between 4 and 15"},
		{name: "seedance invalid resolution", request: OpenAIVideoRequest{Model: "seedance-2.0", Prompt: "motion", DurationSeconds: 6, Resolution: "360p"}, wantErr: "resolution"},
		{name: "seedance per-second valid 4k", request: OpenAIVideoRequest{Model: "seedance-2.0-4k", Prompt: "motion", DurationSeconds: 6, Resolution: "4K"}},
		{name: "seedance per-second requires duration", request: OpenAIVideoRequest{Model: "seedance-2.0-720p", Prompt: "motion", Resolution: "720p"}, wantErr: "duration is required"},
		{name: "omni v2v missing video", request: OpenAIVideoRequest{Model: "omni-v2v", Prompt: "motion"}, wantErr: "exactly one"},
		{name: "omni v2v multiple videos", request: OpenAIVideoRequest{Model: "omni-v2v", Prompt: "motion", ReferenceVideoCount: 2}, wantErr: "exactly one"},
		{name: "omni v2v valid", request: OpenAIVideoRequest{Model: "omni-v2v", Prompt: "motion", ReferenceVideoCount: 1}},
		{name: "omni too many images", request: OpenAIVideoRequest{Model: "omni-fast", Prompt: "motion", ReferenceImageCount: 6}, wantErr: "at most five"},
		{name: "sora invalid duration", request: OpenAIVideoRequest{Model: "sora-2", Prompt: "motion", DurationSeconds: 10}, wantErr: "4, 8, or 12"},
		{name: "sora invalid size", request: OpenAIVideoRequest{Model: "sora-2", Prompt: "motion", DurationSeconds: 8, Size: "1920x1080"}, wantErr: "size"},
		{name: "sora accepts frame reference", request: OpenAIVideoRequest{Model: "sora-2", Prompt: "motion", DurationSeconds: 8, ReferenceImageCount: 1, ReferenceMode: "frame"}},
		{name: "sora rejects multiple frames", request: OpenAIVideoRequest{Model: "sora-2", Prompt: "motion", DurationSeconds: 8, ReferenceImageCount: 2, ReferenceMode: "frame"}, wantErr: "at most one"},
		{name: "sora rejects image mode", request: OpenAIVideoRequest{Model: "sora-2", Prompt: "motion", DurationSeconds: 8, ReferenceImageCount: 1, ReferenceMode: "image"}, wantErr: "must be frame"},
		{name: "sora valid", request: OpenAIVideoRequest{Model: "sora-2", Prompt: "motion", DurationSeconds: 4, AspectRatio: "9:16"}},
		{name: "veo valid", request: OpenAIVideoRequest{Model: "veo-3-1", Prompt: "motion", DurationSeconds: 6, AspectRatio: "16:9", Resolution: "1080p"}},
		{name: "veo standard accepts two frames", request: OpenAIVideoRequest{Model: "veo-3-1", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 2, ReferenceMode: "frame"}},
		{name: "veo standard rejects three frames", request: OpenAIVideoRequest{Model: "veo-3-1", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 3, ReferenceMode: "frame"}, wantErr: "at most 2"},
		{name: "veo standard alias containing preferred remains frame mode", request: OpenAIVideoRequest{Model: "preferred-veo-3-1", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 3, ReferenceMode: "frame"}, wantErr: "at most 2"},
		{name: "veo ref accepts three images", request: OpenAIVideoRequest{Model: "veo-3-1-ref", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 3, ReferenceMode: "image"}},
		{name: "veo ref prefixed alias accepts three images", request: OpenAIVideoRequest{Model: "public-veo-3-1-ref-v2", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 3, ReferenceMode: "image"}},
		{name: "veo ref rejects frame mode", request: OpenAIVideoRequest{Model: "veo-3-1-ref", Prompt: "motion", DurationSeconds: 6, ReferenceImageCount: 1, ReferenceMode: "frame"}, wantErr: "must be image"},
		{name: "veo invalid duration", request: OpenAIVideoRequest{Model: "veo-3-1", Prompt: "motion", DurationSeconds: 5}, wantErr: "duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOpenAIVideoModelRequest(&tt.request)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestOpenAIVideoCangyuanModelRoutingAndDerivedResolution(t *testing.T) {
	require.Equal(t, openAIVideosEndpoint, OpenAIVideoUpstreamEndpointForModel("grok-video", openAIVideoGenerationsEndpoint))
	require.Equal(t, openAIVideosEndpoint, OpenAIVideoUpstreamEndpointForModel("veo-3-1", openAIVideoGenerationsEndpoint))
	require.Equal(t, VideoBillingResolution4K, NormalizeVideoBillingResolutionOrDefault("2160p"))

	req := &OpenAIVideoRequest{Model: "seedance-2.0-fast-720p", Prompt: "motion", DurationSeconds: 6}
	normalizeOpenAIVideoDerivedFields(req)
	require.Equal(t, VideoBillingResolution720P, req.Resolution)
	require.NoError(t, validateOpenAIVideoModelRequest(req))
}

func TestOpenAIVideoCangyuanPublishedModelProfiles(t *testing.T) {
	models := []string{
		"seedance-2.0-mini",
		"omni-fast",
		"omni-fast-no-water",
		"seedance-2.0-fast-480p",
		"sora-2",
		"seedance-2.0-mini-720p",
		"seedance-2.0-720p",
		"grok-video-1.5",
		"seedance-2.0",
		"seedance-2.0-fast",
		"seedance-2.0-mini-480p",
		"seedance-2.0-fast-720p",
		"veo-3-1",
		"seedance-2.0-4k",
		"seedance-2.0-1080p",
		"sora-2-pro",
		"omni-v2v-no-water",
		"seedance-2.0-480p",
		"grok-video",
		"omni-v2v",
		"veo-3-1-ref",
		"veo-3-1-fast",
	}
	require.Len(t, models, 22)

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			profile := classifyOpenAIVideoModel(model)
			require.NotEqual(t, openAIVideoModelUnknown, profile)
			req := &OpenAIVideoRequest{Model: model, Prompt: "motion"}
			switch profile {
			case openAIVideoModelGrok15:
				req.ReferenceImageCount = 1
				req.DurationSeconds = 4
			case openAIVideoModelGrok:
				req.DurationSeconds = 4
			case openAIVideoModelSeedanceStandard:
				req.DurationSeconds = 4
			case openAIVideoModelSeedancePerSecond:
				req.DurationSeconds = 4
			case openAIVideoModelOmniV2V:
				req.ReferenceVideoCount = 1
			case openAIVideoModelSora, openAIVideoModelVeo:
				req.DurationSeconds = 4
			}
			normalizeOpenAIVideoDerivedFields(req)
			require.NoError(t, validateOpenAIVideoModelRequest(req))
			require.Equal(t, openAIVideosEndpoint, OpenAIVideoUpstreamEndpointForModel(model, "/unexpected"))
		})
	}
}

func TestParseOpenAIVideoRequestValidatesJSONProtocolProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "grok accepts seven images",
			body: `{"model":"grok-video","prompt":"motion","duration":10,"resolution":"720p","image_urls":["1","2","3","4","5","6","7"]}`,
		},
		{
			name:    "grok rejects eight images",
			body:    `{"model":"grok-video","prompt":"motion","duration":10,"image_urls":["1","2","3","4","5","6","7","8"]}`,
			wantErr: "at most seven",
		},
		{
			name:    "grok 1.5 rejects video reference",
			body:    `{"model":"grok-imagine-video-1.5","prompt":"motion","duration":6,"resolution":"720p","aspect_ratio":"16:9","image_url":"image","video_url":"video"}`,
			wantErr: "does not accept a video reference",
		},
		{
			name: "seedance per-second suffix accepts 4k",
			body: `{"model":"seedance-2.0-4k","prompt":"motion","duration":15,"resolution":"4k","image_urls":["1","2","3","4","5","6","7","8","9"],"audio_urls":["1","2","3"]}`,
		},
		{
			name:    "seedance standard rejects unpaired frame",
			body:    `{"model":"seedance-2.0","prompt":"motion","duration":6,"resolution":"720p","first_frame_url":"image"}`,
			wantErr: "must be provided together",
		},
		{
			name:    "seedance standard rejects frames with other references",
			body:    `{"model":"seedance-2.0","prompt":"motion","duration":6,"resolution":"720p","first_frame_url":"first","last_frame_url":"last","audio_url":"audio"}`,
			wantErr: "cannot be combined",
		},
		{
			name: "omni root profile accepts one image",
			body: `{"model":"omni-fast","prompt":"motion","duration":10,"resolution":"720p","aspect_ratio":"16:9","image_url":"image"}`,
		},
		{
			name: "omni root profile accepts one frame",
			body: `{"model":"omni-fast","prompt":"motion","aspect_ratio":"16:9","first_image_url":"image"}`,
		},
		{
			name:    "seedance per-second media reference requires image",
			body:    `{"model":"seedance-2.0-fast-720p","prompt":"motion","duration":6,"reference_videos":["video"]}`,
			wantErr: "require at least one image",
		},
		{
			name: "sora accepts documented shape",
			body: `{"model":"sora-2","prompt":"motion","seconds":12,"size":"1024x1024","aspect_ratio":"16:9"}`,
		},
		{
			name: "sora accepts one frame reference",
			body: `{"model":"sora-2","prompt":"motion","seconds":8,"reference_mode":"frame","images":["image"]}`,
		},
		{
			name: "veo accepts three reference images",
			body: `{"model":"veo-3-1-ref","prompt":"motion","duration":6,"resolution":"1080p","aspect_ratio":"16:9","reference_mode":"image","images":["1","2","3"]}`,
		},
		{
			name:    "veo rejects four reference images",
			body:    `{"model":"veo-3-1-ref","prompt":"motion","duration":6,"reference_mode":"image","images":["1","2","3","4"]}`,
			wantErr: "at most 3",
		},
		{
			name:    "veo rejects video reference",
			body:    `{"model":"veo-3-1","prompt":"motion","duration":6,"video_url":"video"}`,
			wantErr: "does not accept video or audio",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			parsed, err := (&OpenAIGatewayService{}).ParseOpenAIVideoRequest(c, body)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, parsed)
		})
	}
}

func TestParseOpenAIVideoRequestAcceptsOmniMultipartInputReferenceArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "omni-fast"))
	require.NoError(t, writer.WriteField("prompt", "motion"))
	require.NoError(t, writer.WriteField("duration", "10"))
	require.NoError(t, writer.WriteField("resolution", "720p"))
	require.NoError(t, writer.WriteField("aspect_ratio", "16:9"))
	for _, name := range []string{"first.png", "second.png"} {
		part, err := writer.CreateFormFile("input_reference[]", name)
		require.NoError(t, err)
		_, err = part.Write([]byte("png"))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	parsed, err := (&OpenAIGatewayService{}).ParseOpenAIVideoRequest(c, body.Bytes())
	require.NoError(t, err)
	require.Equal(t, 2, parsed.ReferenceImageCount)
	require.Equal(t, 2, parsed.InputReferenceFileCount)
}

func TestForwardOpenAIVideoRequiresConfiguredAccountBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"seedance-pro","prompt":"city","duration":6}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{}}
	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	_, err = svc.ForwardVideo(context.Background(), c, &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "secret"},
	}, parsed, "")
	require.ErrorContains(t, err, "base_url is required")
	require.Empty(t, recorder.Body.String())
}

func TestForwardOpenAIVideoRejectsNilUpstreamResponse(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"seedance-pro","prompt":"city","duration":6}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	svc := &OpenAIGatewayService{httpUpstream: &httpUpstreamRecorder{}}
	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	_, err = svc.ForwardVideo(context.Background(), c, &Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "secret",
			"base_url": "https://video-upstream.test/v1",
		},
	}, parsed, "")
	require.EqualError(t, err, "video upstream response is unavailable")
	require.Empty(t, recorder.Body.String())
}

func TestForwardOpenAIVideoUpstreamErrorDoesNotWriteProviderDetails(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"seedance-pro","prompt":"city","duration":6}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusBadRequest,
		Header: http.Header{
			"Location":     []string{"https://upstream.test/private/task"},
			"X-Request-Id": []string{"upstream-request-id"},
		},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"https://upstream.test/v1/videos upstream-task-id"}}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	parsed, err := svc.ParseOpenAIVideoRequest(c, body)
	require.NoError(t, err)
	_, err = svc.ForwardVideo(context.Background(), c, &Account{
		ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "secret", "base_url": "https://video-upstream.test/v1"},
	}, parsed, "")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "upstream.test")
	require.NotContains(t, err.Error(), "upstream-task-id")
	require.Empty(t, recorder.Body.String())
	require.Empty(t, recorder.Header().Get("Location"))
	require.Empty(t, recorder.Header().Get("X-Request-Id"))
}

func TestRewriteOpenAIVideoClientResponseBodyAddsPublicIDAndRewritesRawData(t *testing.T) {
	body := []byte(`{"status":"succeeded","raw_data":{"video_url":"https://cdn.test/private.mp4"},"data":{"raw_data":{"content_url":"https://cdn.test/private-2.mp4"}}}`)
	rewritten := RewriteOpenAIVideoClientResponseBody(body, "video-public-123")

	require.Equal(t, "video-public-123", gjson.GetBytes(rewritten, "id").String())
	require.Equal(t, "/v1/videos/video-public-123/content", gjson.GetBytes(rewritten, "raw_data.video_url").String())
	require.Equal(t, "/v1/videos/video-public-123/content", gjson.GetBytes(rewritten, "data.raw_data.content_url").String())
	require.NotContains(t, string(rewritten), "cdn.test")
}

func TestRewriteOpenAIVideoClientResponseBodyUsesPublicBaseURL(t *testing.T) {
	body := []byte(`{"id":"upstream-task","data":{"result_url":"https://signed.upstream.test/video.mp4"}}`)

	rewritten := RewriteOpenAIVideoClientResponseBodyWithBaseURL(
		body,
		"video-public-123",
		"https://api.52token.example/v1",
		"upstream-task",
	)

	require.Equal(t, "video-public-123", gjson.GetBytes(rewritten, "id").String())
	require.Equal(t, "https://api.52token.example/v1/videos/video-public-123/content", gjson.GetBytes(rewritten, "data.result_url").String())
	require.NotContains(t, string(rewritten), "signed.upstream.test")
	require.NotContains(t, string(rewritten), "upstream-task")
}

func TestRewriteOpenAIVideoClientResponseBodyRejectsUnsafePublicBaseURL(t *testing.T) {
	body := []byte(`{"video_url":"https://signed.upstream.test/video.mp4"}`)

	rewritten := RewriteOpenAIVideoClientResponseBodyWithBaseURL(body, "video-public-123", "javascript:alert(1)")

	require.Equal(t, "/v1/videos/video-public-123/content", gjson.GetBytes(rewritten, "video_url").String())
}

func TestStreamOpenAIVideoTaskContentRejectsUnsafeMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		header http.Header
	}{
		{name: "unsupported mime", header: http.Header{"Content-Type": []string{"text/html"}}},
		{name: "oversized content length", header: http.Header{"Content-Type": []string{"video/mp4"}, "Content-Length": []string{"4294967297"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/video-public/content", nil)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     tt.header,
				Body:       io.NopCloser(strings.NewReader("provider-body")),
			}}
			svc := &OpenAIGatewayService{httpUpstream: upstream}
			handled, err := svc.StreamOpenAIVideoTaskContent(context.Background(), c, &MediaGenerationTask{
				PublicTaskID: "video-public",
				AccountID:    7,
				ResponseBody: `{"video_url":"https://cdn.test/private.mp4"}`,
			})
			require.True(t, handled)
			require.Error(t, err)
			require.Empty(t, recorder.Body.String())
			require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))
		})
	}
}
