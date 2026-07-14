package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func (s *OpenAIGatewayService) GetOpenAIImageTaskAccount(ctx context.Context, task *MediaGenerationTask) (*Account, error) {
	if s == nil || s.accountRepo == nil || task == nil || task.AccountID <= 0 {
		return nil, fmt.Errorf("image task account is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, task.AccountID)
	if err != nil || account == nil || !account.IsOpenAI() || account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("image task account is unavailable")
	}
	if strings.TrimSpace(account.GetCredential("base_url")) == "" {
		return nil, fmt.Errorf("image task account base_url is unavailable")
	}
	return account, nil
}

func (s *OpenAIGatewayService) ForwardOpenAIImageTaskStatus(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	pollEndpoint string,
	providerTaskID string,
	requestModel string,
	upstreamModel string,
) (*OpenAIForwardResult, error) {
	if c == nil || account == nil || account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("image task status request is unavailable")
	}
	pollEndpoint = normalizeOpenAIImageTaskEndpoint(pollEndpoint)
	providerTaskID = strings.TrimSpace(providerTaskID)
	if pollEndpoint == "" || providerTaskID == "" {
		return nil, fmt.Errorf("image task status request is unavailable")
	}

	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if baseURL == "" {
		return nil, fmt.Errorf("image task account base_url is required")
	}
	validatedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	targetURL := strings.TrimRight(buildOpenAIImagesURL(validatedBaseURL, pollEndpoint), "/") + "/" + url.PathEscape(providerTaskID)
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	applyOpenAICompatibleAPIKeyAuth(upstreamReq, account, token)
	for key, values := range c.Request.Header {
		if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			upstreamReq.Header.Add(key, value)
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("User-Agent", customUA)
	}
	account.ApplyHeaderOverrides(upstreamReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	startedAt := time.Now()
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
			UpstreamURL: safeUpstreamURL(upstreamReq.URL.String()),
			Kind:        "request_error",
			Message:     safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		respBody := s.readUpstreamErrorBody(resp)
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMessage := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMessage, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMessage,
			})
			s.handleFailoverSideEffects(ctx, resp, account, respBody, requestModel)
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: respBody}
		}
		return s.handleOpenAIImagesErrorResponse(ctx, resp, c, account, requestModel)
	}

	usage, imageCount, outputSizes, responseBody, responseContentType, err := s.handleOpenAIImagesNonStreamingResponse(resp, c, false)
	if err != nil {
		return nil, err
	}
	status := extractOpenAIImageTaskStatus(responseBody)
	if status == "" && imageCount > 0 {
		status = MediaGenerationStatusCompleted
	}
	return &OpenAIForwardResult{
		RequestID:           strings.TrimSpace(resp.Header.Get("x-request-id")),
		ResponseID:          extractOpenAIImageTaskID(responseBody),
		Usage:               usage,
		Model:               strings.TrimSpace(requestModel),
		UpstreamModel:       strings.TrimSpace(upstreamModel),
		ResponseHeaders:     resp.Header.Clone(),
		Duration:            time.Since(startedAt),
		ImageCount:          imageCount,
		ImageOutputSizes:    outputSizes,
		MediaType:           "image",
		MediaStatus:         status,
		ResponseStatus:      resp.StatusCode,
		ResponseBody:        responseBody,
		ResponseContentType: responseContentType,
	}, nil
}

func RewriteOpenAIImageClientResponseBody(body []byte, publicTaskID string) []byte {
	publicTaskID = strings.TrimSpace(publicTaskID)
	if publicTaskID == "" || len(body) == 0 || !gjson.ValidBytes(body) {
		return body
	}
	rewritten := append([]byte(nil), body...)
	found := false
	for _, path := range []string{
		"id",
		"task_id",
		"request_id",
		"data.id",
		"data.task_id",
		"data.request_id",
		"data.task.id",
		"result.id",
		"result.task_id",
		"result.request_id",
	} {
		if !gjson.GetBytes(rewritten, path).Exists() {
			continue
		}
		updated, err := sjson.SetBytes(rewritten, path, publicTaskID)
		if err != nil {
			continue
		}
		rewritten = updated
		found = true
	}
	if !found {
		if updated, err := sjson.SetBytes(rewritten, "id", publicTaskID); err == nil {
			rewritten = updated
		}
	}
	return rewritten
}

func OpenAIImageForwardResultFromStoredTask(task *MediaGenerationTask) *OpenAIForwardResult {
	if task == nil {
		return nil
	}
	body := []byte(task.ResponseBody)
	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	statusCode := task.ResponseStatus
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}
	contentType := strings.TrimSpace(task.ResponseContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	return &OpenAIForwardResult{
		RequestID:           task.ClientTaskID(),
		ResponseID:          task.ProviderTaskID(),
		Usage:               usage,
		Model:               strings.TrimSpace(task.RequestedModel),
		UpstreamModel:       strings.TrimSpace(task.UpstreamModel),
		ImageCount:          extractOpenAIImageCountFromJSONBytes(body),
		ImageSize:           strings.TrimSpace(task.SizeTier),
		ImageInputSize:      strings.TrimSpace(task.Resolution),
		ImageOutputSizes:    collectOpenAIResponseImageOutputSizesFromJSONBytes(body),
		MediaType:           "image",
		MediaStatus:         NormalizeMediaGenerationStatus(task.Status),
		ResponseStatus:      statusCode,
		ResponseBody:        body,
		ResponseContentType: contentType,
	}
}
