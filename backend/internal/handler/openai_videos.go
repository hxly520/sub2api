package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Videos handles OpenAI-compatible video task APIs.
//
// Supported creation/status/content surfaces:
//   - POST /v1/videos
//   - GET  /v1/videos/{id}
//   - GET  /v1/videos/{id}/content
//   - POST /v1/video/generations
//   - GET  /v1/video/generations/{id}
//   - POST /contents/generations/tasks
//   - GET  /contents/generations/tasks/{id}
func (h *OpenAIGatewayHandler) Videos(c *gin.Context) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	requestStart := time.Now()
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	reqLog := requestLogger(
		c,
		"handler.openai_gateway.videos",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	var body []byte
	if c.Request.Method != http.MethodGet {
		var err error
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			if maxErr, ok := extractMaxBytesError(err); ok {
				h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
				return
			}
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
			return
		}
	}

	parsed, err := h.gatewayService.ParseOpenAIVideoRequest(c, body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	requestModel := strings.TrimSpace(parsed.Model)
	requestFingerprint := service.HashMediaGenerationRequestFingerprint(parsed.Endpoint, body)
	reqLog = reqLog.With(
		zap.String("model", requestModel),
		zap.String("endpoint", parsed.Endpoint),
		zap.String("upstream_path", parsed.UpstreamPath),
		zap.Bool("generation", parsed.GenerationRequest),
		zap.Bool("content", parsed.ContentRequest),
		zap.String("request_id", parsed.RequestID),
	)

	idempotencyHash := ""
	replayIdempotentTask := func(replay *service.MediaGenerationTask) bool {
		if replay == nil {
			return false
		}
		if strings.TrimSpace(replay.RequestFingerprint) != requestFingerprint {
			h.errorResponse(c, http.StatusConflict, "idempotency_error", "Idempotency-Key was reused with a different video request")
			return true
		}
		statusCode := replay.ResponseStatus
		if statusCode <= 0 {
			statusCode = http.StatusOK
		}
		contentType := strings.TrimSpace(replay.ResponseContentType)
		if contentType == "" {
			contentType = "application/json"
		}
		body := service.RewriteOpenAIVideoClientResponseBody([]byte(replay.ResponseBody), replay.TaskID)
		c.Data(statusCode, contentType, body)
		reqLog.Debug("openai.videos.idempotency_replayed", zap.String("task_id", replay.TaskID))
		return true
	}
	if parsed.GenerationRequest {
		idempotencyHash = service.HashMediaGenerationIdempotencyKey(openAIVideoIdempotencyKey(c))
		if idempotencyHash != "" {
			replay, err := h.gatewayService.GetOpenAIVideoTaskByIdempotency(c.Request.Context(), apiKey.ID, idempotencyHash)
			if err == nil && replay != nil {
				if replayIdempotentTask(replay) {
					return
				}
				return
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				reqLog.Warn("openai.videos.idempotency_lookup_failed", zap.Error(err))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency lookup failed")
				return
			}
			releaseIdempotencyLock, err := h.gatewayService.AcquireOpenAIVideoIdempotencyLock(c.Request.Context(), apiKey.ID, idempotencyHash)
			if err != nil {
				reqLog.Warn("openai.videos.idempotency_lock_failed", zap.Error(err))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency lock failed")
				return
			}
			if releaseIdempotencyLock != nil {
				defer releaseIdempotencyLock()
			}
			replay, err = h.gatewayService.GetOpenAIVideoTaskByIdempotency(c.Request.Context(), apiKey.ID, idempotencyHash)
			if err == nil && replay != nil {
				if replayIdempotentTask(replay) {
					return
				}
				return
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				reqLog.Warn("openai.videos.idempotency_recheck_failed", zap.Error(err))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency lookup failed")
				return
			}
		}
	}

	if parsed.GenerationRequest {
		if !service.GroupAllowsImageGeneration(apiKey.Group) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
			return
		}
		if decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, parsed.ModerationBody()); decision != nil && decision.Blocked {
			h.errorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
			return
		}
		imageReleaseFunc, acquired := h.acquireImageGenerationSlot(c, streamStarted)
		if !acquired {
			return
		}
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
		}
	}

	setOpsRequestContext(c, requestModel, parsed.Stream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(parsed.Stream, false)))

	channelMapping := service.ChannelMappingResult{}
	if requestModel != "" {
		channelMapping, _ = h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, requestModel)
	}
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, parsed.Stream || parsed.ContentRequest, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if parsed.GenerationRequest {
		if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
			reqLog.Info("openai.videos.billing_eligibility_check_failed", zap.Error(err))
			status, code, message, retryAfter := billingErrorDetails(err)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.handleStreamingAwareError(c, status, code, message, streamStarted)
			return
		}
	}

	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, body)
	if parsed.GenerationRequest && sessionHash == "" {
		sessionHash = h.gatewayService.GenerateSessionHashWithFallback(c, body, parsed.StickySessionSeed())
	}
	var storedTask *service.MediaGenerationTask
	if !parsed.GenerationRequest && parsed.RequestID != "" {
		sessionHash = service.OpenAIVideoTaskSessionHash(parsed.RequestID)
		task, err := h.gatewayService.GetOpenAIVideoTaskByTaskID(c.Request.Context(), apiKey.ID, parsed.RequestID)
		if err == nil && task != nil {
			storedTask = task
			if err := h.gatewayService.BindOpenAIVideoTaskAccount(c.Request.Context(), apiKey.GroupID, task.TaskID, task.AccountID); err != nil {
				reqLog.Warn("openai.videos.bind_stored_task_account_failed",
					zap.String("request_id", task.TaskID),
					zap.Int64("account_id", task.AccountID),
					zap.Error(err),
				)
			}
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			reqLog.Warn("openai.videos.lookup_stored_task_failed", zap.String("request_id", parsed.RequestID), zap.Error(err))
		}
	}
	if parsed.ContentRequest && storedTask != nil {
		handled, err := h.gatewayService.StreamOpenAIVideoTaskContent(c.Request.Context(), c, storedTask)
		if handled {
			if err != nil {
				reqLog.Warn("openai.videos.stored_content_proxy_failed", zap.String("request_id", storedTask.TaskID), zap.Error(err))
				if c.Writer.Size() == -1 {
					h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Video content proxy failed")
				}
			} else {
				streamStarted = true
			}
			return
		}
	}

	requestCtx := c.Request.Context()
	routingStart := time.Now()
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	switchCount := 0
	failedAccountIDs := make(map[int64]struct{})
	sameAccountRetryCount := make(map[int64]int)
	var lastFailoverErr *service.UpstreamFailoverError

	for {
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			requestCtx,
			apiKey.GroupID,
			"",
			sessionHash,
			requestModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityVideos,
			false,
			false,
		)
		if err != nil {
			reqLog.Warn("openai.videos.account_select_failed", zap.Error(err), zap.Int("excluded_account_count", len(failedAccountIDs)))
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, requestModel, service.PlatformOpenAI)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				h.handleStreamingAwareError(c, cls.Status, cls.ErrType, "No available compatible video accounts", streamStarted)
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, streamStarted)
			} else {
				h.handleFailoverExhaustedSimple(c, 502, streamStarted)
			}
			return
		}
		if selection == nil || selection.Account == nil {
			markOpsRoutingCapacityLimited(c)
			h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available compatible video accounts", streamStarted)
			return
		}

		reqLog.Debug("openai.videos.account_schedule_decision",
			zap.String("layer", scheduleDecision.Layer),
			zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
			zap.Int("candidate_count", scheduleDecision.CandidateCount),
			zap.Int("top_k", scheduleDecision.TopK),
			zap.Int64("latency_ms", scheduleDecision.LatencyMs),
		)

		account := selection.Account
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		setOpsSelectedAccount(c, account.ID, account.Platform)

		accountReleaseFunc, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, parsed.Stream || parsed.ContentRequest, &streamStarted, reqLog)
		if !accountAcquired {
			return
		}
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())

		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.ForwardVideo(requestCtx, c, account, parsed, channelMapping.MappedModel)
		}()
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)
		if result != nil && result.FirstTokenMs != nil {
			service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, int64(*result.FirstTokenMs))
		}

		if err != nil {
			var upstreamUserErr *service.OpenAIImagesUpstreamError
			if errors.As(err, &upstreamUserErr) {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, !service.IsOpenAIImagesRetryableUpstreamError(upstreamUserErr), nil)
				reqLog.Warn("openai.videos.upstream_user_error",
					zap.Int64("account_id", account.ID),
					zap.Int("status_code", upstreamUserErr.StatusCode),
					zap.String("error_type", upstreamUserErr.ErrorType),
					zap.String("error_code", upstreamUserErr.Code),
					zap.Error(err),
				)
				return
			}
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) && parsed.GenerationRequest {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleFailoverExhausted(c, failoverErr, true)
					return
				}
				if failoverErr.RetryableOnSameAccount {
					retryLimit := account.GetPoolModeRetryCount()
					if sameAccountRetryCount[account.ID] < retryLimit {
						sameAccountRetryCount[account.ID]++
						reqLog.Warn("openai.videos.pool_mode_same_account_retry",
							zap.Int64("account_id", account.ID),
							zap.Int("upstream_status", failoverErr.StatusCode),
							zap.Int("retry_limit", retryLimit),
							zap.Int("retry_count", sameAccountRetryCount[account.ID]),
						)
						select {
						case <-requestCtx.Done():
							return
						case <-time.After(sameAccountRetryDelay):
						}
						continue
					}
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				failedAccountIDs[account.ID] = struct{}{}
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, streamStarted)
					return
				}
				switchCount++
				reqLog.Warn("openai.videos.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, false, nil)
			if c.Writer.Size() == writerSizeBeforeForward {
				h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed", streamStarted)
			}
			reqLog.Warn("openai.videos.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			return
		}

		if result != nil {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, result.FirstTokenMs)
		} else {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, true, nil)
		}
		if parsed.GenerationRequest && result != nil && strings.TrimSpace(result.ResponseID) != "" {
			if err := h.gatewayService.BindOpenAIVideoTaskAccount(requestCtx, apiKey.GroupID, result.ResponseID, account.ID); err != nil {
				reqLog.Warn("openai.videos.bind_task_account_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", result.ResponseID),
					zap.Error(err),
				)
			}
			upstreamModel := result.UpstreamModel
			usageFields := channelMapping.ToUsageFields(requestModel, upstreamModel)
			task := &service.MediaGenerationTask{
				TaskID:              result.ResponseID,
				APIKeyID:            apiKey.ID,
				UserID:              subject.UserID,
				AccountID:           account.ID,
				GroupID:             apiKey.GroupID,
				Model:               requestModel,
				RequestedModel:      requestModel,
				UpstreamModel:       upstreamModel,
				Endpoint:            parsed.Endpoint,
				InboundEndpoint:     GetInboundEndpoint(c),
				UpstreamEndpoint:    GetUpstreamEndpoint(c, account.Platform),
				RequestFingerprint:  requestFingerprint,
				RequestPayloadHash:  service.HashUsageRequestPayload(body),
				IdempotencyKeyHash:  idempotencyHash,
				ResponseStatus:      result.ResponseStatus,
				ResponseContentType: result.ResponseContentType,
				ResponseBody:        string(result.ResponseBody),
				Status:              result.VideoStatus,
				DurationSeconds:     result.MediaDurationSeconds,
				Resolution:          parsed.Resolution,
				SizeTier:            parsed.BillingSizeTier(),
				MediaType:           "video",
				ChannelMappedModel:  usageFields.ChannelMappedModel,
				BillingModelSource:  usageFields.BillingModelSource,
				ModelMappingChain:   usageFields.ModelMappingChain,
			}
			if usageFields.ChannelID > 0 {
				task.ChannelID = &usageFields.ChannelID
			}
			if subscription != nil {
				task.SubscriptionID = &subscription.ID
			}
			if err := h.gatewayService.CreateOpenAIVideoTask(requestCtx, task); err != nil {
				reqLog.Warn("openai.videos.store_task_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", result.ResponseID),
					zap.Error(err),
				)
			}
			if service.IsMediaGenerationSuccessStatus(result.VideoStatus) {
				finalizeOpenAIVideoTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, result, task)
			}
		}
		if !parsed.GenerationRequest && !parsed.ContentRequest && result != nil && parsed.RequestID != "" {
			if storedTask == nil {
				if task, err := h.gatewayService.GetOpenAIVideoTaskByTaskID(requestCtx, apiKey.ID, parsed.RequestID); err == nil {
					storedTask = task
				}
			}
			if len(result.ResponseBody) > 0 {
				if err := h.gatewayService.UpdateOpenAIVideoTaskResponse(requestCtx, apiKey.ID, parsed.RequestID, result); err != nil {
					reqLog.Warn("openai.videos.update_task_response_failed", zap.String("request_id", parsed.RequestID), zap.Error(err))
				} else if storedTask != nil {
					storedTask.ResponseStatus = result.ResponseStatus
					storedTask.ResponseContentType = result.ResponseContentType
					storedTask.ResponseBody = string(result.ResponseBody)
					storedTask.Status = result.VideoStatus
					if result.MediaDurationSeconds > 0 {
						storedTask.DurationSeconds = result.MediaDurationSeconds
					}
				}
			}
			finalizeOpenAIVideoTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, result, storedTask)
		}
		reqLog.Debug("openai.videos.request_completed", zap.Int64("account_id", account.ID), zap.Int("switch_count", switchCount))
		return
	}
}

func finalizeOpenAIVideoTaskFromStatus(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	task *service.MediaGenerationTask,
) {
	if result == nil || task == nil || strings.TrimSpace(task.TaskID) == "" {
		return
	}
	status := service.NormalizeMediaGenerationStatus(result.VideoStatus)
	if status == service.MediaGenerationStatusPending || status == service.MediaGenerationStatusRunning || status == "" {
		return
	}
	if service.IsMediaGenerationFailureStatus(status) {
		errMsg := ""
		if len(result.ResponseBody) > 0 {
			errMsg = openAIVideoFinalizationErrorMessage(result.ResponseBody)
		}
		if err := h.gatewayService.MarkOpenAIVideoTaskTerminal(c.Request.Context(), apiKey.ID, task.TaskID, status, errMsg); err != nil {
			reqLog.Warn("openai.videos.mark_task_failed_status_failed", zap.String("request_id", task.TaskID), zap.String("status", status), zap.Error(err))
		}
		return
	}
	if !service.IsMediaGenerationSuccessStatus(status) || task.FinalizedAt != nil {
		return
	}
	recordOpenAIVideoFinalUsage(c, h, reqLog, apiKey, subject, subscription, account, result, task)
}

func recordOpenAIVideoFinalUsage(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	statusResult *service.OpenAIForwardResult,
	task *service.MediaGenerationTask,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	durationSeconds := task.DurationSeconds
	if statusResult != nil && statusResult.MediaDurationSeconds > 0 {
		durationSeconds = statusResult.MediaDurationSeconds
	}
	requestModel := strings.TrimSpace(task.RequestedModel)
	if requestModel == "" {
		requestModel = strings.TrimSpace(task.Model)
	}
	upstreamModel := strings.TrimSpace(task.UpstreamModel)
	if upstreamModel == "" && statusResult != nil {
		upstreamModel = strings.TrimSpace(statusResult.UpstreamModel)
	}
	imageSize := strings.TrimSpace(task.SizeTier)
	if imageSize == "" && statusResult != nil {
		imageSize = statusResult.ImageSize
	}
	usageFields := service.ChannelUsageFields{
		OriginalModel:      requestModel,
		ChannelMappedModel: strings.TrimSpace(task.ChannelMappedModel),
		BillingModelSource: strings.TrimSpace(task.BillingModelSource),
		ModelMappingChain:  strings.TrimSpace(task.ModelMappingChain),
	}
	if task.ChannelID != nil {
		usageFields.ChannelID = *task.ChannelID
	}
	if usageFields.ChannelMappedModel == "" {
		usageFields.ChannelMappedModel = requestModel
	}
	recordResult := &service.OpenAIForwardResult{
		RequestID:            "video:" + strings.TrimSpace(task.TaskID),
		ResponseID:           task.TaskID,
		Model:                requestModel,
		BillingModel:         upstreamModel,
		UpstreamModel:        upstreamModel,
		Usage:                statusResult.Usage,
		Stream:               false,
		Duration:             statusResult.Duration,
		ImageCount:           1,
		ImageSize:            imageSize,
		MediaDurationSeconds: durationSeconds,
		MediaType:            "video",
		ResponseStatus:       statusResult.ResponseStatus,
		VideoStatus:          statusResult.VideoStatus,
	}
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             recordResult,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    strings.TrimSpace(task.InboundEndpoint),
			UpstreamEndpoint:   strings.TrimSpace(task.UpstreamEndpoint),
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: strings.TrimSpace(task.RequestPayloadHash),
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			ChannelUsageFields: usageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.videos"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", requestModel),
				zap.Int64("account_id", account.ID),
				zap.String("request_id", task.TaskID),
			).Error("openai.videos.record_final_usage_failed", zap.Error(err))
			reqLog.Debug("openai.videos.record_final_usage_failed", zap.Error(err))
			return
		}
		if err := h.gatewayService.MarkOpenAIVideoTaskTerminal(ctx, apiKey.ID, task.TaskID, service.MediaGenerationStatusCompleted, ""); err != nil {
			reqLog.Warn("openai.videos.mark_task_completed_failed", zap.String("request_id", task.TaskID), zap.Error(err))
		}
	})
}

func openAIVideoFinalizationErrorMessage(body []byte) string {
	msg := strings.TrimSpace(string(body))
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}

func openAIVideoIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if key := strings.TrimSpace(c.GetHeader("Idempotency-Key")); key != "" {
		return key
	}
	return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
}

func recordOpenAIVideoUsage(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	requestModel string,
	body []byte,
	channelMapping service.ChannelMappingResult,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	upstreamModel := ""
	if result != nil {
		upstreamModel = result.UpstreamModel
	}
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: service.HashUsageRequestPayload(body),
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			ChannelUsageFields: channelMapping.ToUsageFields(requestModel, upstreamModel),
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.videos"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", requestModel),
				zap.Int64("account_id", account.ID),
			).Error("openai.videos.record_usage_failed", zap.Error(err))
			reqLog.Debug("openai.videos.record_usage_failed", zap.Error(err))
		}
	})
}
