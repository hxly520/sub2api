package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const openAIImageIdempotencyKeyMaxLength = 255

type openAIImageAsyncCreation struct {
	publicTaskID           string
	idempotencyHash        string
	requestFingerprint     string
	upstreamIdempotencyKey string
	resumedTask            *service.MediaGenerationTask
	release                func()
}

func (h *OpenAIGatewayHandler) prepareOpenAIImageAsyncCreation(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	body []byte,
	parsed *service.OpenAIImagesRequest,
) (*openAIImageAsyncCreation, bool) {
	if h == nil || h.gatewayService == nil || c == nil || apiKey == nil || parsed == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task service is unavailable")
		return nil, true
	}
	state := &openAIImageAsyncCreation{
		requestFingerprint: service.HashMediaGenerationRequestFingerprint(parsed.Endpoint, body),
	}
	idempotencyKey := openAIImageIdempotencyKey(c)
	if len(idempotencyKey) > openAIImageIdempotencyKeyMaxLength {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Idempotency-Key is too long")
		return nil, true
	}
	state.idempotencyHash = service.HashMediaGenerationIdempotencyKey(idempotencyKey)
	state.upstreamIdempotencyKey = idempotencyKey

	inspect := func(task *service.MediaGenerationTask) (handled bool, resumable bool) {
		if task == nil {
			return false, false
		}
		if strings.TrimSpace(task.MediaType) != "" && !strings.EqualFold(strings.TrimSpace(task.MediaType), "image") {
			h.errorResponse(c, http.StatusConflict, "idempotency_error", "Idempotency-Key belongs to a different media request")
			return true, false
		}
		if strings.TrimSpace(task.RequestFingerprint) != state.requestFingerprint {
			h.errorResponse(c, http.StatusConflict, "idempotency_error", "Idempotency-Key was reused with a different image request")
			return true, false
		}
		publicTaskID := task.ClientTaskID()
		if publicTaskID == "" {
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task is unavailable")
			return true, false
		}
		if service.NormalizeMediaGenerationStatus(task.Status) == service.MediaGenerationStatusCreating && task.ProviderTaskID() == "" {
			state.publicTaskID = publicTaskID
			state.resumedTask = task
			return false, true
		}
		if strings.TrimSpace(task.ResponseBody) == "" {
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task is unavailable")
			return true, false
		}
		if err := writeOpenAIImageStoredTaskResponse(c, h, task); err != nil {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Image result delivery failed")
		}
		return true, false
	}

	if state.idempotencyHash != "" {
		task, err := h.gatewayService.GetMediaGenerationTaskByIdempotency(c.Request.Context(), apiKey.ID, state.idempotencyHash)
		if err == nil && task != nil {
			if handled, _ := inspect(task); handled {
				return nil, true
			}
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			reqLog.Warn("openai.images.idempotency_lookup_failed", zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image idempotency lookup failed")
			return nil, true
		}

		release, err := h.gatewayService.AcquireMediaGenerationIdempotencyLock(c.Request.Context(), apiKey.ID, state.idempotencyHash)
		if err != nil {
			reqLog.Warn("openai.images.idempotency_lock_failed", zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image idempotency lock failed")
			return nil, true
		}
		state.release = release
		task, err = h.gatewayService.GetMediaGenerationTaskByIdempotency(c.Request.Context(), apiKey.ID, state.idempotencyHash)
		if err == nil && task != nil {
			if handled, resumable := inspect(task); handled {
				state.close()
				return nil, true
			} else if resumable {
				return state, false
			}
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			state.close()
			reqLog.Warn("openai.images.idempotency_recheck_failed", zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image idempotency lookup failed")
			return nil, true
		}
	}

	state.publicTaskID = service.NewOpenAIImagePublicTaskID()
	if state.upstreamIdempotencyKey == "" {
		state.upstreamIdempotencyKey = state.publicTaskID
	}
	return state, false
}

func (s *openAIImageAsyncCreation) close() {
	if s == nil || s.release == nil {
		return
	}
	s.release()
	s.release = nil
}

func (h *OpenAIGatewayHandler) handleOpenAIImageAsyncCreation(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	body []byte,
	parsed *service.OpenAIImagesRequest,
	channelMapping service.ChannelMappingResult,
	state *openAIImageAsyncCreation,
	routingStart time.Time,
) {
	if state == nil || state.publicTaskID == "" {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task initialization failed")
		return
	}
	requestModel := strings.TrimSpace(parsed.Model)
	sessionHash := service.OpenAIImageTaskSessionHash(state.publicTaskID)
	if state.resumedTask != nil && state.resumedTask.AccountID > 0 {
		if err := h.gatewayService.BindStickySession(c.Request.Context(), apiKey.GroupID, sessionHash, state.resumedTask.AccountID); err != nil {
			reqLog.Warn("openai.images.bind_creation_intent_account_failed", zap.String("request_id", state.publicTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task routing is unavailable")
			return
		}
	}
	requestCtx := withOpenAIAccountScheduleProfile(
		service.WithOpenAIImageGenerationIntent(c.Request.Context()),
		c,
		requestModel,
	)

	selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForImages(
		requestCtx,
		apiKey.GroupID,
		sessionHash,
		requestModel,
		nil,
		service.OpenAIImagesCapabilityAsync,
	)
	if err != nil || selection == nil || selection.Account == nil {
		reqLog.Warn("openai.images.async_account_select_failed", zap.Error(err))
		markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available compatible async image accounts")
		return
	}
	reqLog.Debug("openai.images.async_account_schedule_decision",
		zap.String("layer", scheduleDecision.Layer),
		zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
		zap.Int("candidate_count", scheduleDecision.CandidateCount),
		zap.Int64("latency_ms", scheduleDecision.LatencyMs),
	)
	account := selection.Account
	if state.resumedTask != nil && state.resumedTask.AccountID > 0 && account.ID != state.resumedTask.AccountID {
		reqLog.Warn("openai.images.creation_intent_account_mismatch",
			zap.String("request_id", state.publicTaskID),
			zap.Int64("expected_account_id", state.resumedTask.AccountID),
			zap.Int64("selected_account_id", account.ID),
		)
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task routing is unavailable")
		return
	}

	mappedModel := requestModel
	if value := strings.TrimSpace(channelMapping.MappedModel); value != "" {
		mappedModel = value
	}
	upstreamModel := account.GetMappedModel(mappedModel)
	usageFields := channelMapping.ToUsageFields(requestModel, upstreamModel)
	billingInputSize := strings.TrimSpace(parsed.ImageSize)
	if billingInputSize == "" {
		billingInputSize = strings.TrimSpace(parsed.Size)
	}
	if billingInputSize == "" || strings.Contains(billingInputSize, ":") || strings.EqualFold(billingInputSize, "auto") {
		billingInputSize = parsed.SizeTier
	}
	intentBody := service.RewriteOpenAIImageClientResponseBody([]byte(`{"object":"image.generation","status":"creating"}`), state.publicTaskID)
	intent := &service.MediaGenerationTask{
		TaskID:              state.publicTaskID,
		PublicTaskID:        state.publicTaskID,
		APIKeyID:            apiKey.ID,
		UserID:              subject.UserID,
		AccountID:           account.ID,
		GroupID:             apiKey.GroupID,
		Model:               requestModel,
		RequestedModel:      requestModel,
		UpstreamModel:       upstreamModel,
		Endpoint:            parsed.Endpoint,
		InboundEndpoint:     GetInboundEndpoint(c),
		UpstreamEndpoint:    service.OpenAIImageTaskPollEndpoint(upstreamModel, parsed.Endpoint),
		ChannelMappedModel:  usageFields.ChannelMappedModel,
		BillingModelSource:  usageFields.BillingModelSource,
		ModelMappingChain:   usageFields.ModelMappingChain,
		RequestFingerprint:  state.requestFingerprint,
		RequestPayloadHash:  service.HashUsageRequestPayload(body),
		IdempotencyKeyHash:  state.idempotencyHash,
		ResponseStatus:      http.StatusAccepted,
		ResponseContentType: "application/json",
		ResponseBody:        string(intentBody),
		Status:              service.MediaGenerationStatusCreating,
		Resolution:          billingInputSize,
		SizeTier:            parsed.SizeTier,
		MediaType:           "image",
	}
	if usageFields.ChannelID > 0 {
		intent.ChannelID = &usageFields.ChannelID
	}
	if subscription != nil {
		intent.SubscriptionID = &subscription.ID
	}
	if err := h.gatewayService.CreateMediaGenerationTask(requestCtx, intent); err != nil {
		reqLog.Warn("openai.images.store_creation_intent_failed", zap.String("request_id", state.publicTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task persistence failed")
		return
	}
	parsed.UpstreamIdempotencyKey = state.upstreamIdempotencyKey
	setOpsSelectedAccount(c, account.ID, account.Platform)
	service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())

	for {
		accountRelease, acquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, new(bool), reqLog)
		if !acquired {
			return
		}
		writerSizeBeforeForward := c.Writer.Size()
		forwardStart := time.Now()
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountRelease != nil {
					accountRelease()
				}
			}()
			return h.gatewayService.ForwardImages(requestCtx, c, account, body, parsed, channelMapping.MappedModel)
		}()
		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, forwardDurationMs-upstreamLatencyMs)
		}
		if forwardErr != nil {
			var upstreamUserErr *service.OpenAIImagesUpstreamError
			if errors.As(forwardErr, &upstreamUserErr) {
				h.reportOpenAIAccountScheduleResult(c, account, requestModel, !service.IsOpenAIImagesRetryableUpstreamError(upstreamUserErr), nil)
				markOpenAIImageTaskTerminalDetached(h, apiKey.ID, state.publicTaskID, service.MediaGenerationStatusFailed, "upstream_rejected")
				return
			}
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) {
				h.reportOpenAIAccountScheduleResult(c, account, requestModel, false, nil)
				if c.Writer.Size() != writerSizeBeforeForward {
					c.Abort()
					return
				}
				// Do not switch accounts after dispatch: an accepted async task may
				// otherwise be created and billed twice on different upstreams.
				h.handleFailoverExhausted(c, failoverErr, false)
				return
			}
			h.reportOpenAIAccountScheduleResult(c, account, requestModel, false, nil)
			if !openAIForwardErrorAlreadyCommunicated(c, writerSizeBeforeForward, forwardErr) {
				h.ensureForwardErrorResponse(c, false)
			}
			reqLog.Warn("openai.images.async_forward_failed", zap.Int64("account_id", account.ID), zap.Error(forwardErr))
			return
		}
		if result == nil || strings.TrimSpace(result.ResponseID) == "" || len(result.ResponseBody) == 0 {
			markOpenAIImageTaskTerminalDetached(h, apiKey.ID, state.publicTaskID, service.MediaGenerationStatusFailed, "invalid_upstream_task_response")
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream image task response is invalid")
			return
		}
		h.reportOpenAIAccountScheduleResult(c, account, requestModel, true, result.FirstTokenMs)
		status := service.NormalizeMediaGenerationStatus(result.MediaStatus)
		if status == "" || status == service.MediaGenerationStatusCreating {
			status = service.MediaGenerationStatusPending
		}
		rawPublicBody := service.RewriteOpenAIImageClientResponseBody(result.ResponseBody, state.publicTaskID)
		result.MediaStatus = status
		result.ImageSize = parsed.SizeTier
		result.ImageInputSize = billingInputSize
		outcome := *intent
		outcome.UpstreamTaskID = strings.TrimSpace(result.ResponseID)
		outcome.ResponseStatus = result.ResponseStatus
		outcome.ResponseContentType = result.ResponseContentType
		outcome.ResponseBody = string(rawPublicBody)
		outcome.Status = status
		if err := persistOpenAIImageTaskDetached(requestCtx, h, &outcome); err != nil {
			reqLog.Warn("openai.images.store_task_outcome_failed", zap.String("request_id", state.publicTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task persistence failed")
			return
		}
		if err := h.gatewayService.BindStickySession(requestCtx, apiKey.GroupID, sessionHash, account.ID); err != nil {
			reqLog.Warn("openai.images.bind_task_account_failed", zap.String("request_id", state.publicTaskID), zap.Error(err))
		}
		clientBody, err := h.gatewayService.PrepareOpenAIImageClientResponseBody(requestCtx, rawPublicBody)
		if err != nil {
			reqLog.Warn("openai.images.prepare_client_response_failed", zap.String("request_id", state.publicTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Image result delivery failed")
			return
		}
		result.ResponseBody = clientBody
		finalizeOpenAIImageTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, result, &outcome)
		writeOpenAIImageForwardResponse(c, result)
		return
	}
}

func (h *OpenAIGatewayHandler) handleOpenAIImageTaskStatus(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	requestStart time.Time,
) {
	if !service.GroupAllowsImageGeneration(apiKey.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	publicTaskID := strings.TrimSpace(c.Param("request_id"))
	if publicTaskID == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Image task id is required")
		return
	}
	task, err := h.gatewayService.GetMediaGenerationTaskByTaskID(c.Request.Context(), apiKey.ID, publicTaskID)
	if errors.Is(err, sql.ErrNoRows) || task == nil || !strings.EqualFold(strings.TrimSpace(task.MediaType), "image") {
		h.errorResponse(c, http.StatusNotFound, "invalid_request_error", "Image task not found")
		return
	}
	if err != nil {
		reqLog.Warn("openai.images.lookup_task_failed", zap.String("request_id", publicTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task lookup failed")
		return
	}
	providerTaskID := task.ProviderTaskID()
	if task.ClientTaskID() == "" || providerTaskID == "" || task.AccountID <= 0 {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task is unavailable")
		return
	}
	requestModel := strings.TrimSpace(task.RequestedModel)
	if requestModel == "" {
		requestModel = strings.TrimSpace(task.Model)
	}
	reqLog = reqLog.With(zap.String("model", requestModel), zap.String("request_id", publicTaskID))
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, strings.TrimSpace(task.UpstreamEndpoint), int16(service.RequestTypeSync))
	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	streamStarted := false
	userRelease, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	account, err := h.gatewayService.GetOpenAIImageTaskAccount(c.Request.Context(), task)
	if err != nil {
		reqLog.Warn("openai.images.load_task_account_failed", zap.String("request_id", publicTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task account is unavailable")
		return
	}
	setOpsSelectedAccount(c, account.ID, account.Platform)

	if service.IsMediaGenerationSuccessStatus(task.Status) || service.IsMediaGenerationFailureStatus(task.Status) {
		storedResult := service.OpenAIImageForwardResultFromStoredTask(task)
		clientBody, deliveryErr := h.gatewayService.PrepareOpenAIImageClientResponseBody(c.Request.Context(), storedResult.ResponseBody)
		if deliveryErr != nil {
			reqLog.Warn("openai.images.prepare_stored_client_response_failed", zap.String("request_id", publicTaskID), zap.Error(deliveryErr))
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Image result delivery failed")
			return
		}
		storedResult.ResponseBody = clientBody
		finalizeOpenAIImageTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, storedResult, task)
		writeOpenAIImageForwardResponse(c, storedResult)
		return
	}

	accountRelease, err := h.concurrencyHelper.AcquireAccountSlotWithWait(c, account.ID, account.Concurrency, false, &streamStarted)
	if err != nil {
		reqLog.Warn("openai.images.task_account_slot_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		h.handleConcurrencyError(c, err, "account", false)
		return
	}
	if accountRelease != nil {
		defer accountRelease()
	}
	upstreamModel := strings.TrimSpace(task.UpstreamModel)
	result, err := h.gatewayService.ForwardOpenAIImageTaskStatus(
		c.Request.Context(),
		c,
		account,
		task.UpstreamEndpoint,
		providerTaskID,
		requestModel,
		upstreamModel,
	)
	if err != nil {
		var upstreamUserErr *service.OpenAIImagesUpstreamError
		if errors.As(err, &upstreamUserErr) {
			return
		}
		var failoverErr *service.UpstreamFailoverError
		if errors.As(err, &failoverErr) {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Image task status is temporarily unavailable")
			return
		}
		reqLog.Warn("openai.images.task_status_forward_failed", zap.String("request_id", publicTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Image task status request failed")
		return
	}
	if result == nil || len(result.ResponseBody) == 0 {
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream image task response is invalid")
		return
	}
	if responseID := strings.TrimSpace(result.ResponseID); responseID != "" && responseID != providerTaskID {
		reqLog.Warn("openai.images.task_status_id_mismatch", zap.String("request_id", publicTaskID))
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream image task response is invalid")
		return
	}
	status := service.NormalizeMediaGenerationStatus(result.MediaStatus)
	rawPublicBody := service.RewriteOpenAIImageClientResponseBody(result.ResponseBody, publicTaskID)
	result.MediaStatus = status
	result.ImageSize = task.SizeTier
	result.ImageInputSize = task.Resolution
	if err := h.gatewayService.UpdateMediaGenerationTaskResponse(
		c.Request.Context(),
		apiKey.ID,
		publicTaskID,
		result.ResponseStatus,
		result.ResponseContentType,
		rawPublicBody,
		"",
		status,
		0,
	); err != nil {
		reqLog.Warn("openai.images.update_task_response_failed", zap.String("request_id", publicTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task update failed")
		return
	}
	freshTask, err := h.gatewayService.GetMediaGenerationTaskByTaskID(c.Request.Context(), apiKey.ID, publicTaskID)
	if err != nil || freshTask == nil {
		reqLog.Warn("openai.images.reload_task_failed", zap.String("request_id", publicTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Image task update verification failed")
		return
	}
	clientBody, deliveryErr := h.gatewayService.PrepareOpenAIImageClientResponseBody(c.Request.Context(), rawPublicBody)
	if deliveryErr != nil {
		reqLog.Warn("openai.images.prepare_client_response_failed", zap.String("request_id", publicTaskID), zap.Error(deliveryErr))
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Image result delivery failed")
		return
	}
	result.ResponseBody = clientBody
	finalizeOpenAIImageTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, result, freshTask)
	writeOpenAIImageForwardResponse(c, result)
}

func finalizeOpenAIImageTaskFromStatus(
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
	if result == nil || task == nil || task.ClientTaskID() == "" {
		return
	}
	status := service.NormalizeMediaGenerationStatus(result.MediaStatus)
	if service.IsMediaGenerationFailureStatus(status) {
		markOpenAIImageTaskTerminalDetached(h, apiKey.ID, task.ClientTaskID(), status, "image_task_failed")
		return
	}
	if !service.IsMediaGenerationSuccessStatus(status) || result.ImageCount <= 0 || task.UsageRecordedAt != nil {
		return
	}
	leaseToken := uuid.NewString()
	acquired, err := h.gatewayService.TryAcquireMediaGenerationFinalization(
		c.Request.Context(),
		apiKey.ID,
		task.ClientTaskID(),
		leaseToken,
		time.Now().UTC().Add(2*time.Minute),
	)
	if err != nil {
		reqLog.Warn("openai.images.acquire_finalization_failed", zap.String("request_id", task.ClientTaskID()), zap.Error(err))
		return
	}
	if !acquired {
		return
	}
	recordOpenAIImageFinalUsage(c, h, reqLog, apiKey, subject, subscription, account, result, task, leaseToken)
}

func recordOpenAIImageFinalUsage(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	statusResult *service.OpenAIForwardResult,
	task *service.MediaGenerationTask,
	leaseToken string,
) {
	publicTaskID := task.ClientTaskID()
	requestModel := strings.TrimSpace(task.RequestedModel)
	if requestModel == "" {
		requestModel = strings.TrimSpace(task.Model)
	}
	upstreamModel := strings.TrimSpace(task.UpstreamModel)
	if upstreamModel == "" {
		upstreamModel = strings.TrimSpace(statusResult.UpstreamModel)
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
		RequestID:           publicTaskID,
		ResponseID:          publicTaskID,
		Model:               requestModel,
		BillingModel:        upstreamModel,
		UpstreamModel:       upstreamModel,
		Usage:               statusResult.Usage,
		Duration:            statusResult.Duration,
		ImageCount:          statusResult.ImageCount,
		ImageSize:           strings.TrimSpace(task.SizeTier),
		ImageInputSize:      strings.TrimSpace(task.Resolution),
		ImageOutputSizes:    statusResult.ImageOutputSizes,
		MediaType:           "image",
		MediaStatus:         service.MediaGenerationStatusCompleted,
		ResponseStatus:      statusResult.ResponseStatus,
		ResponseContentType: statusResult.ResponseContentType,
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
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
				zap.String("component", "handler.openai_gateway.images"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.String("model", requestModel),
				zap.Int64("account_id", account.ID),
				zap.String("request_id", publicTaskID),
			).Error("openai.images.record_final_usage_failed", zap.Error(err))
			if releaseErr := h.gatewayService.ReleaseMediaGenerationFinalization(ctx, apiKey.ID, publicTaskID, leaseToken, "usage_record_failed"); releaseErr != nil {
				reqLog.Warn("openai.images.release_finalization_failed", zap.String("request_id", publicTaskID), zap.Error(releaseErr))
			}
			return
		}
		completed, err := h.gatewayService.CompleteMediaGenerationFinalization(ctx, apiKey.ID, publicTaskID, leaseToken)
		if err != nil || !completed {
			reqLog.Warn("openai.images.complete_finalization_failed", zap.String("request_id", publicTaskID), zap.Bool("completed", completed), zap.Error(err))
			if releaseErr := h.gatewayService.ReleaseMediaGenerationFinalization(ctx, apiKey.ID, publicTaskID, leaseToken, "finalization_complete_failed"); releaseErr != nil {
				reqLog.Warn("openai.images.release_finalization_failed", zap.String("request_id", publicTaskID), zap.Error(releaseErr))
			}
		}
	})
}

func persistOpenAIImageTaskDetached(ctx context.Context, h *OpenAIGatewayHandler, task *service.MediaGenerationTask) error {
	if h == nil || h.gatewayService == nil {
		return errors.New("image task service is unavailable")
	}
	persistCtx := context.Background()
	if ctx != nil {
		persistCtx = context.WithoutCancel(ctx)
	}
	persistCtx, cancel := context.WithTimeout(persistCtx, 5*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := h.gatewayService.CreateMediaGenerationTask(persistCtx, task); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	return lastErr
}

func markOpenAIImageTaskTerminalDetached(h *OpenAIGatewayHandler, apiKeyID int64, taskID, status, finalizationError string) {
	if h == nil || h.gatewayService == nil || strings.TrimSpace(taskID) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = h.gatewayService.MarkMediaGenerationTaskTerminal(ctx, apiKeyID, taskID, status, finalizationError)
}

func writeOpenAIImageForwardResponse(c *gin.Context, result *service.OpenAIForwardResult) {
	if c == nil || result == nil {
		return
	}
	statusCode := result.ResponseStatus
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}
	contentType := strings.TrimSpace(result.ResponseContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	service.PrepareOpenAIImageClientResponseHeaders(c.Writer.Header())
	c.Writer.Header().Del("Content-Length")
	c.Writer.Header().Del("Content-Encoding")
	c.Data(statusCode, contentType, result.ResponseBody)
}

func writeOpenAIImageStoredTaskResponse(c *gin.Context, h *OpenAIGatewayHandler, task *service.MediaGenerationTask) error {
	result := service.OpenAIImageForwardResultFromStoredTask(task)
	if result == nil {
		return errors.New("image task response is unavailable")
	}
	result.ResponseBody = service.RewriteOpenAIImageClientResponseBody(result.ResponseBody, task.ClientTaskID())
	if h == nil || h.gatewayService == nil {
		return errors.New("image task service is unavailable")
	}
	clientBody, err := h.gatewayService.PrepareOpenAIImageClientResponseBody(c.Request.Context(), result.ResponseBody)
	if err != nil {
		return err
	}
	result.ResponseBody = clientBody
	writeOpenAIImageForwardResponse(c, result)
	return nil
}

func openAIImageIdempotencyKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value := strings.TrimSpace(c.GetHeader("Idempotency-Key")); value != "" {
		return value
	}
	return strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
}
