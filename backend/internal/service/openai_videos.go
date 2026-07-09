package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIVideosEndpoint              = "/v1/videos"
	openAIVideosGenerationsEndpoint   = "/v1/videos/generations"
	openAIVideoGenerationsEndpoint    = "/v1/video/generations"
	openAIContentsGenerationTasksPath = "/contents/generations/tasks"
)

type OpenAIVideoRequest struct {
	Endpoint          string
	UpstreamPath      string
	Method            string
	ContentType       string
	Multipart         bool
	Model             string
	ExplicitModel     bool
	Prompt            string
	Size              string
	Resolution        string
	DurationSeconds   int
	Stream            bool
	GenerationRequest bool
	ContentRequest    bool
	RequestID         string
	Body              []byte
	bodyHash          string
}

func (r *OpenAIVideoRequest) StickySessionSeed() string {
	if r == nil {
		return ""
	}
	parts := []string{
		"openai-video",
		strings.TrimSpace(r.Endpoint),
		strings.TrimSpace(r.Model),
		strings.TrimSpace(r.Prompt),
	}
	if r.bodyHash != "" {
		parts = append(parts, "body="+r.bodyHash)
	}
	return strings.Join(parts, "|")
}

func (r *OpenAIVideoRequest) BillingSizeTier() string {
	if r == nil {
		return NormalizeImageBillingTierOrDefault("")
	}
	if size := strings.TrimSpace(r.Size); size != "" {
		return NormalizeImageBillingTierOrDefault(size)
	}
	if resolution := strings.ToLower(strings.TrimSpace(r.Resolution)); resolution != "" {
		switch {
		case strings.Contains(resolution, "2160"), strings.Contains(resolution, "4k"):
			return ImageBillingSize4K
		case strings.Contains(resolution, "1080"), strings.Contains(resolution, "2k"):
			return ImageBillingSize2K
		case strings.Contains(resolution, "480"), strings.Contains(resolution, "720"), strings.Contains(resolution, "1k"):
			return ImageBillingSize1K
		}
	}
	return NormalizeImageBillingTierOrDefault("")
}

func (r *OpenAIVideoRequest) ModerationBody() []byte {
	if r == nil {
		return nil
	}
	prompt := strings.TrimSpace(r.Prompt)
	if prompt == "" {
		return nil
	}
	return []byte(fmt.Sprintf(`{"prompt":%q}`, prompt))
}

func (s *OpenAIGatewayService) ParseOpenAIVideoRequest(c *gin.Context, body []byte) (*OpenAIVideoRequest, error) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return nil, fmt.Errorf("missing request context")
	}
	req := normalizeOpenAIVideoEndpointPath(c.Request.Method, c.Request.URL.Path)
	if req == nil {
		return nil, fmt.Errorf("unsupported videos endpoint")
	}
	req.ContentType = strings.TrimSpace(c.GetHeader("Content-Type"))
	req.Body = body
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		req.bodyHash = hex.EncodeToString(sum[:8])
	}
	if req.GenerationRequest {
		if len(body) == 0 {
			return nil, fmt.Errorf("request body is empty")
		}
		if err := parseOpenAIVideoBody(req); err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Model) == "" {
			return nil, fmt.Errorf("model is required")
		}
	}
	return req, nil
}

func normalizeOpenAIVideoEndpointPath(method, path string) *OpenAIVideoRequest {
	trimmed := "/" + strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "/" {
		return nil
	}
	method = strings.ToUpper(strings.TrimSpace(method))

	if strings.Contains(trimmed, openAIContentsGenerationTasksPath) {
		suffix := suffixAfterPath(trimmed, openAIContentsGenerationTasksPath)
		requestID, content := openAIVideoTaskIDAndContent(suffix)
		return &OpenAIVideoRequest{
			Endpoint:          openAIContentsGenerationTasksPath,
			UpstreamPath:      openAIContentsGenerationTasksPath + suffix,
			Method:            methodOrDefault(method),
			GenerationRequest: method == http.MethodPost && suffix == "",
			ContentRequest:    content,
			RequestID:         requestID,
		}
	}

	if strings.Contains(trimmed, openAIVideosGenerationsEndpoint) || strings.Contains(trimmed, "/videos/generations") {
		suffix := suffixAfterPath(trimmed, "/videos/generations")
		requestID, content := openAIVideoTaskIDAndContent(suffix)
		return &OpenAIVideoRequest{
			Endpoint:          openAIVideosGenerationsEndpoint,
			UpstreamPath:      openAIVideosGenerationsEndpoint + suffix,
			Method:            methodOrDefault(method),
			GenerationRequest: method == http.MethodPost && suffix == "",
			ContentRequest:    content,
			RequestID:         requestID,
		}
	}

	if strings.Contains(trimmed, openAIVideoGenerationsEndpoint) || strings.Contains(trimmed, "/video/generations") {
		suffix := suffixAfterPath(trimmed, "/video/generations")
		requestID, content := openAIVideoTaskIDAndContent(suffix)
		return &OpenAIVideoRequest{
			Endpoint:          openAIVideoGenerationsEndpoint,
			UpstreamPath:      openAIVideoGenerationsEndpoint + suffix,
			Method:            methodOrDefault(method),
			GenerationRequest: method == http.MethodPost && suffix == "",
			ContentRequest:    content,
			RequestID:         requestID,
		}
	}

	if strings.Contains(trimmed, openAIVideosEndpoint) || strings.HasPrefix(trimmed, "/videos") {
		suffix := suffixAfterPath(trimmed, "/videos")
		requestID, content := openAIVideoTaskIDAndContent(suffix)
		return &OpenAIVideoRequest{
			Endpoint:          openAIVideosEndpoint,
			UpstreamPath:      openAIVideosEndpoint + suffix,
			Method:            methodOrDefault(method),
			GenerationRequest: method == http.MethodPost && suffix == "",
			ContentRequest:    content,
			RequestID:         requestID,
		}
	}

	return nil
}

func methodOrDefault(method string) string {
	if method == "" {
		return http.MethodGet
	}
	return method
}

func suffixAfterPath(path, marker string) string {
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	suffix := strings.TrimRight(path[idx+len(marker):], "/")
	if suffix == "/" {
		return ""
	}
	return suffix
}

func openAIVideoTaskIDAndContent(suffix string) (string, bool) {
	trimmed := strings.Trim(suffix, "/")
	if trimmed == "" {
		return "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", false
	}
	return strings.TrimSpace(parts[0]), len(parts) > 1 && strings.EqualFold(parts[1], "content")
}

func parseOpenAIVideoBody(req *OpenAIVideoRequest) error {
	mediaType, _, err := mime.ParseMediaType(req.ContentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		req.Multipart = true
		return parseOpenAIVideoMultipartBody(req)
	}
	if !gjson.ValidBytes(req.Body) {
		return fmt.Errorf("failed to parse request body")
	}
	req.Model = strings.TrimSpace(gjson.GetBytes(req.Body, "model").String())
	req.ExplicitModel = req.Model != ""
	req.Prompt = strings.TrimSpace(gjson.GetBytes(req.Body, "prompt").String())
	if req.Prompt == "" {
		req.Prompt = strings.TrimSpace(gjson.GetBytes(req.Body, "content.#(type==\"text\").text").String())
	}
	req.Size = strings.TrimSpace(gjson.GetBytes(req.Body, "size").String())
	if req.Size == "" {
		req.Size = strings.TrimSpace(gjson.GetBytes(req.Body, "ratio").String())
	}
	req.Resolution = strings.TrimSpace(gjson.GetBytes(req.Body, "resolution_name").String())
	if req.Resolution == "" {
		req.Resolution = strings.TrimSpace(gjson.GetBytes(req.Body, "resolution").String())
	}
	req.DurationSeconds = extractOpenAIVideoDurationSeconds(req.Body)
	if stream := gjson.GetBytes(req.Body, "stream"); stream.Exists() {
		req.Stream = stream.Bool()
	}
	return nil
}

func parseOpenAIVideoMultipartBody(req *OpenAIVideoRequest) error {
	_, params, err := mime.ParseMediaType(req.ContentType)
	if err != nil {
		return fmt.Errorf("parse multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return fmt.Errorf("multipart boundary is required")
	}
	reader := multipart.NewReader(bytes.NewReader(req.Body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read multipart body: %w", err)
		}
		name := strings.TrimSpace(part.FormName())
		fileName := strings.TrimSpace(part.FileName())
		if fileName != "" {
			_ = part.Close()
			continue
		}
		valueBytes, readErr := io.ReadAll(io.LimitReader(part, 1<<20))
		_ = part.Close()
		if readErr != nil {
			return fmt.Errorf("read multipart field: %w", readErr)
		}
		value := strings.TrimSpace(string(valueBytes))
		switch name {
		case "model":
			req.Model = value
			req.ExplicitModel = value != ""
		case "prompt":
			req.Prompt = value
		case "size":
			req.Size = value
		case "resolution", "resolution_name":
			req.Resolution = value
		case "duration", "seconds", "video_duration", "sora2_duration":
			req.DurationSeconds = normalizeOpenAIVideoDurationSeconds(value)
		case "stream":
			req.Stream = strings.EqualFold(value, "true")
		}
	}
	return nil
}

func (s *OpenAIGatewayService) ForwardVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIVideoRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed video request is required")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	if account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("videos endpoint requires an OpenAI API key account")
	}

	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	upstreamModel := account.GetMappedModel(requestModel)

	forwardBody := parsed.Body
	forwardContentType := parsed.ContentType
	var err error
	if parsed.GenerationRequest && strings.TrimSpace(upstreamModel) != "" {
		forwardBody, forwardContentType, err = rewriteOpenAIVideoModel(parsed.Body, parsed.ContentType, upstreamModel)
		if err != nil {
			return nil, err
		}
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, parsed.Stream || parsed.ContentRequest)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIVideoRequest(upstreamCtx, c, account, parsed, forwardBody, forwardContentType, token)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:    account.Platform,
			AccountID:   account.ID,
			AccountName: account.Name,
			Kind:        "request_error",
			Message:     safeErr,
			UpstreamURL: safeUpstreamURL(upstreamReq.URL.String()),
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if parsed.GenerationRequest && s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleOpenAIVideoErrorResponse(upstreamCtx, resp, c, account, upstreamModel)
	}
	defer func() { _ = resp.Body.Close() }()

	if parsed.ContentRequest {
		if err := s.streamOpenAIVideoContentResponse(resp, c); err != nil {
			return nil, err
		}
		return &OpenAIForwardResult{
			RequestID:       resp.Header.Get("x-request-id"),
			Model:           requestModel,
			UpstreamModel:   upstreamModel,
			ResponseHeaders: resp.Header.Clone(),
			Duration:        time.Since(startTime),
			ResponseStatus:  resp.StatusCode,
			MediaType:       "video",
		}, nil
	}

	if parsed.Stream && isEventStreamResponse(resp.Header) {
		usage, _, _, ttft, err := s.handleOpenAIImagesStreamingResponse(resp, c, startTime)
		return &OpenAIForwardResult{
			RequestID:            resp.Header.Get("x-request-id"),
			Usage:                usage,
			Model:                requestModel,
			BillingModel:         upstreamModel,
			UpstreamModel:        upstreamModel,
			Stream:               true,
			ResponseHeaders:      resp.Header.Clone(),
			Duration:             time.Since(startTime),
			FirstTokenMs:         ttft,
			ImageCount:           boolToMediaCount(parsed.GenerationRequest),
			ImageSize:            parsed.BillingSizeTier(),
			MediaDurationSeconds: parsed.DurationSeconds,
			MediaType:            "video",
			ResponseStatus:       resp.StatusCode,
		}, err
	}

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	responseID := extractOpenAIVideoTaskID(body)
	videoStatus := extractOpenAIVideoStatus(body)
	if videoStatus == "" && openAIVideoHasResultURL(body) {
		videoStatus = MediaGenerationStatusCompleted
	}
	if responseID == "" && parsed.GenerationRequest && IsMediaGenerationSuccessStatus(videoStatus) {
		responseID = localOpenAIVideoSynchronousTaskID(resp.Header.Get("x-request-id"), body)
	}
	durationSeconds := parsed.DurationSeconds
	if durationSeconds <= 0 {
		durationSeconds = extractOpenAIVideoDurationSeconds(body)
	}

	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}
	clientTaskID := responseID
	if strings.TrimSpace(parsed.RequestID) != "" {
		clientTaskID = parsed.RequestID
	}
	clientBody := RewriteOpenAIVideoClientResponseBody(body, clientTaskID)
	c.Data(resp.StatusCode, contentType, clientBody)

	return &OpenAIForwardResult{
		RequestID:            resp.Header.Get("x-request-id"),
		ResponseID:           responseID,
		Usage:                usage,
		Model:                requestModel,
		BillingModel:         upstreamModel,
		UpstreamModel:        upstreamModel,
		Stream:               false,
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		ImageCount:           boolToMediaCount(parsed.GenerationRequest),
		ImageSize:            parsed.BillingSizeTier(),
		MediaDurationSeconds: durationSeconds,
		MediaType:            "video",
		ResponseStatus:       resp.StatusCode,
		ResponseBody:         body,
		ResponseContentType:  contentType,
		VideoStatus:          videoStatus,
	}, nil
}

func (s *OpenAIGatewayService) buildOpenAIVideoRequest(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIVideoRequest, body []byte, contentType, token string) (*http.Request, error) {
	baseURL := account.GetOpenAIBaseURL()
	targetURL := buildOpenAIEndpointURL("https://api.openai.com", parsed.UpstreamPath)
	if baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = buildOpenAIEndpointURL(validatedURL, parsed.UpstreamPath)
	}

	var reader io.Reader
	if parsed.Method != http.MethodGet && len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, parsed.Method, targetURL, reader)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	applyOpenAICompatibleAPIKeyAuth(req, account, token)
	for key, values := range c.Request.Header {
		if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	if parsed.Method != http.MethodGet && strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func rewriteOpenAIVideoModel(body []byte, contentType string, model string) ([]byte, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return rewriteOpenAIImagesMultipartModel(body, contentType, model)
	}
	rewritten, err := sjson.SetBytes(body, "model", model)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite video request model: %w", err)
	}
	return rewritten, contentType, nil
}

func (s *OpenAIGatewayService) streamOpenAIVideoContentResponse(resp *http.Response, c *gin.Context) error {
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fmt.Errorf("video content redirect is unavailable")
	}
	headers := resp.Header.Clone()
	headers.Del("Location")
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), headers, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "video/mp4"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", contentType)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		return err
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func (s *OpenAIGatewayService) StreamOpenAIVideoTaskContent(ctx context.Context, c *gin.Context, task *MediaGenerationTask) (bool, error) {
	if task == nil {
		return false, nil
	}
	mediaURL := OpenAIVideoResultURLFromBody([]byte(task.ResponseBody))
	if strings.TrimSpace(mediaURL) == "" {
		return false, nil
	}
	if strings.HasPrefix(strings.TrimSpace(mediaURL), "/") {
		return false, nil
	}
	validatedURL, err := validateOpenAIVideoContentProxyURL(mediaURL)
	if err != nil {
		return true, err
	}
	if s == nil || s.httpUpstream == nil {
		return true, fmt.Errorf("video content proxy is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validatedURL, nil)
	if err != nil {
		return true, fmt.Errorf("video content proxy request failed")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	if c != nil && c.Request != nil {
		for _, header := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
			if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
				req.Header.Set(header, value)
			}
		}
	}
	resp, err := s.httpUpstream.Do(req, "", task.AccountID, 1)
	if err != nil {
		return true, fmt.Errorf("video content proxy request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return true, fmt.Errorf("video content is unavailable")
	}
	if err := s.streamOpenAIVideoContentResponse(resp, c); err != nil {
		return true, err
	}
	return true, nil
}

func validateOpenAIVideoContentProxyURL(rawURL string) (string, error) {
	normalized, err := urlvalidator.ValidateHTTPSURL(rawURL, urlvalidator.ValidationOptions{})
	if err != nil {
		return "", fmt.Errorf("invalid video content url")
	}
	return normalized, nil
}

func (s *OpenAIGatewayService) handleOpenAIVideoErrorResponse(ctx context.Context, resp *http.Response, c *gin.Context, account *Account, upstreamModel string) (*OpenAIForwardResult, error) {
	statusCode := resp.StatusCode
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}
	c.Data(statusCode, contentType, body)
	s.handleOpenAIAccountUpstreamError(ctx, account, statusCode, resp.Header, body, upstreamModel)
	return nil, &OpenAIImagesUpstreamError{
		StatusCode: statusCode,
		ErrorType:  strings.TrimSpace(gjson.GetBytes(body, "error.type").String()),
		Code:       strings.TrimSpace(gjson.GetBytes(body, "error.code").String()),
		Message:    sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body))),
	}
}

func extractOpenAIVideoTaskID(body []byte) string {
	for _, path := range []string{
		"id",
		"request_id",
		"task_id",
		"data.id",
		"data.request_id",
		"data.task_id",
		"data.task.id",
		"data.task.request_id",
		"data.task.task_id",
		"data.video.id",
		"result.id",
		"result.request_id",
		"result.task_id",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func extractOpenAIVideoStatus(body []byte) string {
	for _, path := range []string{
		"status",
		"state",
		"data.status",
		"data.state",
		"data.task.status",
		"data.task.state",
		"data.video.status",
		"data.video.state",
		"result.status",
		"result.state",
		"video.status",
		"video.state",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return NormalizeMediaGenerationStatus(value)
		}
	}
	return ""
}

func openAIVideoHasResultURL(body []byte) bool {
	return OpenAIVideoResultURLFromBody(body) != ""
}

func OpenAIVideoResultURLFromBody(body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range openAIVideoResultURLPaths() {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	for _, path := range openAIVideoResultURLArrayPaths() {
		for _, item := range gjson.GetBytes(body, path).Array() {
			if value := strings.TrimSpace(item.Get("video_url").String()); value != "" {
				return value
			}
			if value := strings.TrimSpace(item.Get("url").String()); value != "" {
				return value
			}
		}
	}
	return ""
}

func RewriteOpenAIVideoClientResponseBody(body []byte, taskID string) []byte {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || !gjson.ValidBytes(body) || OpenAIVideoResultURLFromBody(body) == "" {
		return body
	}
	contentURL := openAIVideoContentProxyPath(taskID)
	rewritten := append([]byte(nil), body...)
	for _, path := range openAIVideoResultURLPaths() {
		if strings.TrimSpace(gjson.GetBytes(rewritten, path).String()) == "" {
			continue
		}
		next, err := sjson.SetBytes(rewritten, path, contentURL)
		if err == nil {
			rewritten = next
		}
	}
	for _, path := range openAIVideoResultURLArrayPaths() {
		for index, item := range gjson.GetBytes(rewritten, path).Array() {
			for _, field := range []string{"video_url", "url"} {
				if strings.TrimSpace(item.Get(field).String()) == "" {
					continue
				}
				next, err := sjson.SetBytes(rewritten, fmt.Sprintf("%s.%d.%s", path, index, field), contentURL)
				if err == nil {
					rewritten = next
				}
			}
		}
	}
	return rewritten
}

func openAIVideoResultURLPaths() []string {
	return []string{
		"url",
		"video_url",
		"content.url",
		"content.video_url",
		"data.url",
		"data.video_url",
		"data.content.url",
		"data.content.video_url",
		"data.video.url",
		"data.video.video_url",
		"result.url",
		"result.video_url",
		"video.url",
		"video.video_url",
	}
}

func openAIVideoResultURLArrayPaths() []string {
	return []string{"output", "data.output", "result.output", "data.result.output"}
}

func openAIVideoContentProxyPath(taskID string) string {
	return "/v1/videos/" + url.PathEscape(strings.TrimSpace(taskID)) + "/content"
}

func localOpenAIVideoSynchronousTaskID(upstreamRequestID string, body []byte) string {
	seed := strings.TrimSpace(upstreamRequestID)
	if seed == "" {
		seed = fmt.Sprintf("local:%d:%s", time.Now().UnixNano(), string(body))
	} else {
		seed = "upstream:" + seed
	}
	sum := sha256.Sum256([]byte(seed))
	return "video-sync-" + hex.EncodeToString(sum[:])[:24]
}

func extractOpenAIVideoDurationSeconds(body []byte) int {
	for _, path := range []string{
		"duration",
		"seconds",
		"duration_seconds",
		"video_duration",
		"sora2_duration",
		"data.duration",
		"data.seconds",
		"data.duration_seconds",
		"data.task.duration",
		"data.task.seconds",
		"data.task.duration_seconds",
		"data.video.duration",
		"data.video.seconds",
		"data.video.duration_seconds",
		"video.duration",
		"video.seconds",
		"video.duration_seconds",
		"result.duration",
		"result.seconds",
		"result.duration_seconds",
	} {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		if seconds := normalizeOpenAIVideoDurationSeconds(value.String()); seconds > 0 {
			return seconds
		}
	}
	return 0
}

func normalizeOpenAIVideoDurationSeconds(value string) int {
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "s"))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	seconds := int(parsed)
	if seconds < 0 {
		return 0
	}
	if seconds > 3600 {
		return 3600
	}
	return seconds
}

func (s *OpenAIGatewayService) openAIMediaTaskRepo() (MediaGenerationTaskRepository, bool) {
	if s == nil || s.usageBillingRepo == nil {
		return nil, false
	}
	repo, ok := s.usageBillingRepo.(MediaGenerationTaskRepository)
	return repo, ok
}

func (s *OpenAIGatewayService) GetOpenAIVideoTaskByTaskID(ctx context.Context, apiKeyID int64, taskID string) (*MediaGenerationTask, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, nil
	}
	return repo.GetMediaGenerationTaskByTaskID(ctx, apiKeyID, taskID)
}

func (s *OpenAIGatewayService) GetOpenAIVideoTaskByIdempotency(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (*MediaGenerationTask, error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil, nil
	}
	return repo.GetMediaGenerationTaskByIdempotency(ctx, apiKeyID, idempotencyKeyHash)
}

func (s *OpenAIGatewayService) AcquireOpenAIVideoIdempotencyLock(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (func(), error) {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return func() {}, nil
	}
	return repo.AcquireMediaGenerationIdempotencyLock(ctx, apiKeyID, idempotencyKeyHash)
}

func (s *OpenAIGatewayService) CreateOpenAIVideoTask(ctx context.Context, task *MediaGenerationTask) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil
	}
	return repo.CreateMediaGenerationTask(ctx, task)
}

func (s *OpenAIGatewayService) UpdateOpenAIVideoTaskResponse(ctx context.Context, apiKeyID int64, taskID string, result *OpenAIForwardResult) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok || result == nil {
		return nil
	}
	return repo.UpdateMediaGenerationTaskResponse(
		ctx,
		apiKeyID,
		taskID,
		result.ResponseStatus,
		result.ResponseContentType,
		string(result.ResponseBody),
		result.VideoStatus,
		result.MediaDurationSeconds,
	)
}

func (s *OpenAIGatewayService) MarkOpenAIVideoTaskTerminal(ctx context.Context, apiKeyID int64, taskID, status, finalizationError string) error {
	repo, ok := s.openAIMediaTaskRepo()
	if !ok {
		return nil
	}
	return repo.MarkMediaGenerationTaskTerminal(ctx, apiKeyID, taskID, status, finalizationError)
}

func OpenAIVideoTaskSessionHash(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ""
	}
	return "openai-video:" + DeriveSessionHashFromSeed(requestID)
}

func (s *OpenAIGatewayService) BindOpenAIVideoTaskAccount(ctx context.Context, groupID *int64, requestID string, accountID int64) error {
	return s.BindStickySession(ctx, groupID, OpenAIVideoTaskSessionHash(requestID), accountID)
}

func boolToMediaCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
