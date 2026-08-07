package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	videoResponseBaseURL := resolveOpenAIVideoResponseBaseURL(c, h.gatewayService.OpenAIVideoPublicBaseURL(c.Request.Context()))

	var body []byte
	if c.Request.Method != http.MethodGet {
		var err error
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			logRequestBodyReadFailure(reqLog, c.Request, err)
			if maxErr, ok := extractMaxBytesError(err); ok {
				h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
				return
			}
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", requestBodyReadFailureMessage(err))
			return
		}
	}

	parsed, err := h.gatewayService.ParseOpenAIVideoRequest(c, body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	routingModel := strings.TrimSpace(parsed.Model)
	requestModel := clientRequestedModel(c, routingModel)
	requestFingerprint := service.HashMediaGenerationRequestFingerprint(parsed.Endpoint, body)
	reqLog = reqLog.With(
		zap.String("model", requestModel),
		zap.String("routing_model", routingModel),
		zap.String("endpoint", parsed.Endpoint),
		zap.String("upstream_path", parsed.UpstreamPath),
		zap.Bool("generation", parsed.GenerationRequest),
		zap.Bool("content", parsed.ContentRequest),
		zap.String("request_id", parsed.RequestID),
	)

	idempotencyKey := ""
	idempotencyHash := ""
	generationPublicTaskID := ""
	var generationIntent *service.MediaGenerationTask
	var generationPricingSnapshot *service.MediaGenerationPricingSnapshot
	var mediaHold *service.MediaBalanceHoldCommand
	mediaHoldTransferred := false
	defer func() {
		if !mediaHoldTransferred {
			releaseMediaBalanceHold(h, mediaHold)
		}
	}()
	resumedGenerationIntent := false
	inspectIdempotentTask := func(replay *service.MediaGenerationTask) (handled bool, resumable bool) {
		if replay == nil {
			return false, false
		}
		if strings.TrimSpace(replay.RequestFingerprint) != requestFingerprint {
			h.errorResponse(c, http.StatusConflict, "idempotency_error", "Idempotency-Key was reused with a different video request")
			return true, false
		}
		clientTaskID := replay.ClientTaskID()
		if clientTaskID == "" {
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task is unavailable")
			return true, false
		}
		if service.NormalizeMediaGenerationStatus(replay.Status) == service.MediaGenerationStatusCreating && replay.ProviderTaskID() == "" {
			generationPublicTaskID = clientTaskID
			generationIntent = replay
			generationPricingSnapshot = replay.PricingSnapshot()
			resumedGenerationIntent = true
			return false, true
		}
		statusCode := replay.ResponseStatus
		if statusCode <= 0 {
			statusCode = http.StatusOK
		}
		contentType := strings.TrimSpace(replay.ResponseContentType)
		if contentType == "" {
			contentType = "application/json"
		}
		body, err := openAIVideoClientResponseBody(
			c.Request.Context(),
			h,
			[]byte(replay.ResponseBody),
			replay.Status,
			clientTaskID,
			replay.UpstreamResultURL,
			videoResponseBaseURL,
			replay,
			replay.ProviderTaskID(),
		)
		if err != nil {
			reqLog.Warn("openai.videos.edge_url_failed", zap.String("task_id", clientTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video content URL is unavailable")
			return true, false
		}
		c.Data(statusCode, contentType, body)
		reqLog.Debug("openai.videos.idempotency_replayed", zap.String("task_id", clientTaskID))
		return true, false
	}
	if parsed.GenerationRequest {
		idempotencyKey = openAIVideoIdempotencyKey(c)
		idempotencyHash = service.HashMediaGenerationIdempotencyKey(idempotencyKey)
		if idempotencyHash != "" {
			replay, err := h.gatewayService.GetOpenAIVideoTaskByIdempotency(c.Request.Context(), apiKey.ID, idempotencyHash)
			if err == nil && replay != nil {
				handled, _ := inspectIdempotentTask(replay)
				if handled {
					return
				}
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
				handled, resumable := inspectIdempotentTask(replay)
				if handled {
					return
				}
				if !resumable {
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task is unavailable")
					return
				}
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				reqLog.Warn("openai.videos.idempotency_recheck_failed", zap.Error(err))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video idempotency lookup failed")
				return
			}
		}
		if generationPublicTaskID == "" {
			generationPublicTaskID = service.NewOpenAIVideoPublicTaskID()
		}
		if idempotencyKey == "" {
			idempotencyKey = generationPublicTaskID
		}
		parsed.UpstreamIdempotencyKey = idempotencyKey
	}

	if parsed.GenerationRequest {
		if !service.GroupAllowsImageGeneration(apiKey.Group) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
			return
		}
		if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, parsed.ModerationBody()); decision != nil && !decision.AllowNextStage {
			h.openAISecurityAuditError(c, decision)
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
	if routingModel != "" {
		channelMapping, _ = h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, routingModel)
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
	if resumedGenerationIntent && generationIntent != nil && generationIntent.AccountID > 0 {
		sessionHash = service.OpenAIVideoTaskSessionHash(generationIntent.ClientTaskID())
		if err := h.gatewayService.BindOpenAIVideoTaskAccount(c.Request.Context(), apiKey.GroupID, generationIntent.ClientTaskID(), generationIntent.AccountID); err != nil {
			reqLog.Warn("openai.videos.bind_creation_intent_account_failed", zap.String("request_id", generationIntent.ClientTaskID()), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task routing is unavailable")
			return
		}
	}
	var storedTask *service.MediaGenerationTask
	if !parsed.GenerationRequest {
		if strings.TrimSpace(parsed.RequestID) == "" {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Video task id is required")
			return
		}
		task, lookupErr := h.gatewayService.GetOpenAIVideoTaskByTaskID(c.Request.Context(), apiKey.ID, parsed.RequestID)
		if errors.Is(lookupErr, sql.ErrNoRows) || task == nil {
			h.errorResponse(c, http.StatusNotFound, "invalid_request_error", "Video task not found")
			return
		}
		if lookupErr != nil {
			reqLog.Warn("openai.videos.lookup_stored_task_failed", zap.String("request_id", parsed.RequestID), zap.Error(lookupErr))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task lookup failed")
			return
		}
		storedTask = task
		clientTaskID := storedTask.ClientTaskID()
		providerTaskID := storedTask.ProviderTaskID()
		if clientTaskID == "" || providerTaskID == "" || storedTask.AccountID <= 0 {
			reqLog.Warn("openai.videos.stored_task_incomplete", zap.String("request_id", parsed.RequestID))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task is unavailable")
			return
		}
		requestModel = strings.TrimSpace(storedTask.RequestedModel)
		if requestModel == "" {
			requestModel = strings.TrimSpace(storedTask.Model)
		}
		routingModel = strings.TrimSpace(storedTask.Model)
		if routingModel == "" {
			routingModel = strings.TrimSpace(storedTask.UpstreamModel)
		}
		if routingModel == "" {
			routingModel = requestModel
		}
		parsed.Model = strings.TrimSpace(storedTask.UpstreamModel)
		if parsed.Model == "" {
			parsed.Model = routingModel
		}
		if err := parsed.UseUpstreamTaskIDAtEndpoint(providerTaskID, storedTask.UpstreamEndpoint); err != nil {
			reqLog.Warn("openai.videos.stored_task_upstream_id_invalid", zap.String("request_id", clientTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task is unavailable")
			return
		}
		sessionHash = service.OpenAIVideoTaskSessionHash(clientTaskID)
		if err := h.gatewayService.BindOpenAIVideoTaskAccount(c.Request.Context(), apiKey.GroupID, clientTaskID, storedTask.AccountID); err != nil {
			reqLog.Warn("openai.videos.bind_stored_task_account_failed",
				zap.String("request_id", clientTaskID),
				zap.Int64("account_id", storedTask.AccountID),
				zap.Error(err),
			)
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task routing is unavailable")
			return
		}

		if parsed.ContentRequest {
			if !service.IsMediaGenerationSuccessStatus(storedTask.Status) {
				h.errorResponse(c, http.StatusConflict, "invalid_request_error", "Video content is unavailable")
				return
			}
			if h.gatewayService.OpenAIVideoEdgeProxyEnabled() {
				edgeURL, edgeErr := h.gatewayService.OpenAIVideoClientContentURLForTask(c.Request.Context(), videoResponseBaseURL, storedTask)
				if edgeErr != nil || edgeURL == "" {
					reqLog.Warn("openai.videos.edge_content_url_failed", zap.String("request_id", clientTaskID), zap.Error(edgeErr))
					h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Video content is unavailable")
					return
				}
				c.Redirect(http.StatusTemporaryRedirect, edgeURL)
				return
			}
			handled, proxyErr := h.gatewayService.StreamOpenAIVideoTaskContent(c.Request.Context(), c, storedTask)
			if handled {
				if proxyErr != nil {
					reqLog.Warn("openai.videos.stored_content_proxy_failed", zap.String("request_id", clientTaskID), zap.Error(proxyErr))
					if c.Writer.Size() == -1 {
						h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Video content proxy failed")
					}
				} else {
					streamStarted = true
				}
				return
			}
		} else if isOpenAIVideoTerminalStatus(storedTask.Status) && strings.TrimSpace(storedTask.ResponseBody) != "" && !openAIVideoTaskNeedsFinalization(storedTask) {
			if err := writeOpenAIVideoStoredResponse(c, h, storedTask, videoResponseBaseURL); err != nil {
				reqLog.Warn("openai.videos.stored_edge_url_failed", zap.String("request_id", clientTaskID), zap.Error(err))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video content URL is unavailable")
			}
			return
		}
	}

	requestCtx := withOpenAIAccountScheduleProfile(c.Request.Context(), c, routingModel)
	routingStart := time.Now()
	selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		requestCtx,
		apiKey.GroupID,
		"",
		sessionHash,
		routingModel,
		nil,
		service.OpenAIUpstreamTransportHTTPSSE,
		service.OpenAIEndpointCapabilityVideos,
		false,
		false,
		false,
	)
	if err != nil {
		reqLog.Warn("openai.videos.account_select_failed", zap.Error(err))
		cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, routingModel, service.PlatformOpenAI)
		if !cls.ModelNotFound {
			markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		}
		h.handleStreamingAwareError(c, cls.Status, cls.ErrType, "No available compatible video accounts", streamStarted)
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
	if storedTask != nil && account.ID != storedTask.AccountID {
		reqLog.Warn("openai.videos.stored_task_account_mismatch",
			zap.String("request_id", storedTask.ClientTaskID()),
			zap.Int64("expected_account_id", storedTask.AccountID),
			zap.Int64("selected_account_id", account.ID),
		)
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task routing is unavailable")
		return
	}
	if storedTask != nil && !parsed.ContentRequest && openAIVideoTaskNeedsFinalization(storedTask) {
		storedResult := openAIVideoForwardResultFromStoredTask(storedTask)
		finalizeOpenAIVideoTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, storedResult, storedTask)
		if !ensureOpenAIVideoTaskResultDeliverable(c, h, reqLog, apiKey.ID, storedResult, storedTask) {
			return
		}
		if err := writeOpenAIVideoStoredResponse(c, h, storedTask, videoResponseBaseURL); err != nil {
			reqLog.Warn("openai.videos.recovered_edge_url_failed", zap.String("request_id", storedTask.ClientTaskID()), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video content URL is unavailable")
		}
		return
	}
	if parsed.GenerationRequest {
		if resumedGenerationIntent && generationIntent != nil && generationIntent.AccountID > 0 && account.ID != generationIntent.AccountID {
			reqLog.Warn("openai.videos.creation_intent_account_mismatch",
				zap.String("request_id", generationIntent.ClientTaskID()),
				zap.Int64("expected_account_id", generationIntent.AccountID),
				zap.Int64("selected_account_id", account.ID),
			)
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task routing is unavailable")
			return
		}
		mappedModel := routingModel
		if value := strings.TrimSpace(channelMapping.MappedModel); value != "" {
			mappedModel = value
		}
		upstreamModel := account.GetMappedModel(mappedModel)
		upstreamRequest, prepareErr := service.PrepareOpenAIVideoRequestForUpstream(parsed, upstreamModel)
		if prepareErr != nil {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", prepareErr.Error())
			return
		}
		usageFields := channelMapping.ToUsageFields(requestModel, upstreamModel)
		if !resumedGenerationIntent || generationPricingSnapshot == nil {
			generationPricingSnapshot, prepareErr = h.gatewayService.CaptureOpenAIVideoPricingSnapshot(
				requestCtx,
				apiKey,
				subject.UserID,
				requestModel,
				upstreamModel,
				upstreamRequest.Resolution,
				upstreamRequest.DurationSeconds,
				1,
				usageFields,
			)
			if prepareErr != nil {
				reqLog.Warn("openai.videos.capture_pricing_snapshot_failed", zap.String("model", requestModel), zap.Error(prepareErr))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video pricing is unavailable")
				return
			}
		}
		if h.gatewayService.MediaGenerationBalanceHoldRequired(apiKey, subscription) && generationPricingSnapshot != nil {
			holdAmount := generationPricingSnapshot.EstimatedCost(1, upstreamRequest.DurationSeconds)
			if holdAmount > 0 {
				mediaHold = markLinkCardMediaBalanceHold(newMediaBalanceHoldCommand(
					apiKey.ID,
					subject.UserID,
					service.MediaBalanceHoldRequestID(generationPublicTaskID),
					requestFingerprint,
					service.HashUsageRequestPayload(body),
					holdAmount,
				), apiKey)
				if err := h.gatewayService.ReserveMediaBalance(requestCtx, mediaHold); err != nil {
					status, code, message, retryAfter := billingErrorDetails(err)
					if retryAfter > 0 {
						c.Header("Retry-After", strconv.Itoa(retryAfter))
					}
					h.errorResponse(c, status, code, message)
					return
				}
			}
		}
		intentBody := service.RewriteOpenAIVideoClientResponseBody(
			[]byte(`{"status":"creating"}`),
			generationPublicTaskID,
		)
		intent := &service.MediaGenerationTask{
			TaskID:              generationPublicTaskID,
			PublicTaskID:        generationPublicTaskID,
			APIKeyID:            apiKey.ID,
			UserID:              subject.UserID,
			AccountID:           account.ID,
			GroupID:             apiKey.GroupID,
			Model:               routingModel,
			RequestedModel:      requestModel,
			UpstreamModel:       upstreamModel,
			Endpoint:            parsed.Endpoint,
			InboundEndpoint:     GetInboundEndpoint(c),
			UpstreamEndpoint:    upstreamRequest.UpstreamPath,
			RequestFingerprint:  requestFingerprint,
			RequestPayloadHash:  service.HashUsageRequestPayload(body),
			IdempotencyKeyHash:  idempotencyHash,
			ResponseStatus:      http.StatusAccepted,
			ResponseContentType: "application/json",
			ResponseBody:        string(intentBody),
			Status:              service.MediaGenerationStatusCreating,
			DurationSeconds:     upstreamRequest.DurationSeconds,
			RequestCount:        1,
			Resolution:          upstreamRequest.Resolution,
			SizeTier:            upstreamRequest.BillingSizeTier(),
			MediaType:           "video",
			ChannelMappedModel:  usageFields.ChannelMappedModel,
			BillingModelSource:  usageFields.BillingModelSource,
			ModelMappingChain:   usageFields.ModelMappingChain,
		}
		if generationPricingSnapshot != nil {
			intent.BillingMode = string(generationPricingSnapshot.Mode)
			intent.BillingUnitPrice = &generationPricingSnapshot.UnitPrice
			intent.BillingRateMultiplier = &generationPricingSnapshot.RateMultiplier
		}
		if usageFields.ChannelID > 0 {
			intent.ChannelID = &usageFields.ChannelID
		}
		if subscription != nil {
			intent.SubscriptionID = &subscription.ID
		}
		if err := h.gatewayService.CreateOpenAIVideoTask(requestCtx, intent); err != nil {
			reqLog.Warn("openai.videos.store_creation_intent_failed", zap.String("request_id", generationPublicTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task persistence failed")
			return
		}
		generationIntent = intent
	}

	sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
	setOpsSelectedAccount(c, account.ID, account.Platform)

	accountReleaseFunc, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, parsed.Stream || parsed.ContentRequest, &streamStarted, reqLog)
	if !accountAcquired {
		if parsed.GenerationRequest {
			markMediaGenerationTaskTerminalDetached(h, apiKey.ID, generationPublicTaskID, service.MediaGenerationStatusFailed, "account_slot_unavailable")
		}
		return
	}
	service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
	if parsed.GenerationRequest {
		if err := markMediaBalanceHoldDispatched(h, mediaHold); err != nil {
			markMediaGenerationTaskTerminalDetached(h, apiKey.ID, generationPublicTaskID, service.MediaGenerationStatusFailed, "hold_dispatch_failed")
			reqLog.Warn("openai.videos.mark_balance_dispatched_failed", zap.String("request_id", generationPublicTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "billing_service_error", "Video billing reservation is unavailable")
			return
		}
	}

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
			if parsed.GenerationRequest {
				mediaHoldTransferred = mediaHold != nil && shouldRetainMediaBalanceHoldAfterDispatch(err)
			}
			h.reportOpenAIAccountScheduleResult(c, account, routingModel, !service.IsOpenAIImagesRetryableUpstreamError(upstreamUserErr), nil)
			reqLog.Warn("openai.videos.upstream_user_error",
				zap.Int64("account_id", account.ID),
				zap.Int("status_code", upstreamUserErr.StatusCode),
				zap.String("error_type", upstreamUserErr.ErrorType),
				zap.String("error_code", upstreamUserErr.Code),
				zap.Error(err),
			)
			writeOpenAIVideoSafeUpstreamError(c)
			return
		}
		var failoverErr *service.UpstreamFailoverError
		if errors.As(err, &failoverErr) && parsed.GenerationRequest {
			mediaHoldTransferred = mediaHold != nil && shouldRetainMediaBalanceHoldAfterDispatch(err)
			h.reportOpenAIAccountScheduleResult(c, account, routingModel, false, nil)
			if c.Writer.Size() != writerSizeBeforeForward {
				c.Abort()
				return
			}
			// Once a media request has been dispatched, switching accounts can
			// create a second long-running task if the first upstream accepted it
			// before returning an ambiguous gateway error. Keep the local intent
			// resumable with the same account and idempotency key instead.
			writeOpenAIVideoSafeUpstreamError(c)
			return
		}
		if parsed.GenerationRequest {
			mediaHoldTransferred = mediaHold != nil && shouldRetainMediaBalanceHoldAfterDispatch(err)
		}
		h.reportOpenAIAccountScheduleResult(c, account, routingModel, false, nil)
		if c.Writer.Size() == writerSizeBeforeForward {
			writeOpenAIVideoSafeUpstreamError(c)
		} else {
			c.Abort()
		}
		reqLog.Warn("openai.videos.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return
	}

	if result != nil {
		h.reportOpenAIAccountScheduleResult(c, account, routingModel, true, result.FirstTokenMs)
	} else {
		h.reportOpenAIAccountScheduleResult(c, account, routingModel, true, nil)
	}
	if parsed.GenerationRequest {
		if result == nil || strings.TrimSpace(result.ResponseID) == "" || len(result.ResponseBody) == 0 {
			markMediaGenerationTaskTerminalDetached(h, apiKey.ID, generationPublicTaskID, service.MediaGenerationStatusFailed, "invalid_upstream_task_response")
			reqLog.Warn("openai.videos.invalid_generation_response", zap.Int64("account_id", account.ID))
			writeOpenAIVideoSafeUpstreamError(c)
			return
		}
		mediaHoldTransferred = mediaHold != nil
		upstreamTaskID := strings.TrimSpace(result.ResponseID)
		publicTaskID := generationPublicTaskID
		status := service.NormalizeMediaGenerationStatus(result.VideoStatus)
		storedBody := service.SanitizeOpenAIVideoStoredResponseBody(result.ResponseBody, status)
		storedBody = service.RewriteOpenAIVideoClientResponseBody(storedBody, publicTaskID, upstreamTaskID)
		upstreamModel := result.UpstreamModel
		usageFields := channelMapping.ToUsageFields(requestModel, upstreamModel)
		task := &service.MediaGenerationTask{
			TaskID:              publicTaskID,
			PublicTaskID:        publicTaskID,
			UpstreamTaskID:      upstreamTaskID,
			APIKeyID:            apiKey.ID,
			UserID:              subject.UserID,
			AccountID:           account.ID,
			GroupID:             apiKey.GroupID,
			Model:               routingModel,
			RequestedModel:      requestModel,
			UpstreamModel:       upstreamModel,
			Endpoint:            parsed.Endpoint,
			InboundEndpoint:     GetInboundEndpoint(c),
			UpstreamEndpoint:    service.OpenAIVideoUpstreamEndpointForModel(upstreamModel, parsed.UpstreamPath),
			RequestFingerprint:  requestFingerprint,
			RequestPayloadHash:  service.HashUsageRequestPayload(body),
			IdempotencyKeyHash:  idempotencyHash,
			ResponseStatus:      result.ResponseStatus,
			ResponseContentType: result.ResponseContentType,
			ResponseBody:        string(storedBody),
			UpstreamResultURL:   result.MediaResultURL,
			Status:              status,
			DurationSeconds:     result.MediaDurationSeconds,
			RequestCount:        1,
			Resolution:          result.VideoResolution,
			SizeTier:            result.ImageSize,
			MediaType:           "video",
			ChannelMappedModel:  usageFields.ChannelMappedModel,
			BillingModelSource:  usageFields.BillingModelSource,
			ModelMappingChain:   usageFields.ModelMappingChain,
		}
		if generationPricingSnapshot != nil {
			task.BillingMode = string(generationPricingSnapshot.Mode)
			task.BillingUnitPrice = &generationPricingSnapshot.UnitPrice
			task.BillingRateMultiplier = &generationPricingSnapshot.RateMultiplier
		}
		if usageFields.ChannelID > 0 {
			task.ChannelID = &usageFields.ChannelID
		}
		if subscription != nil {
			task.SubscriptionID = &subscription.ID
		}
		if err := persistOpenAIVideoTaskOutcome(requestCtx, h, task); err != nil {
			reqLog.Warn("openai.videos.store_task_failed",
				zap.Int64("account_id", account.ID),
				zap.String("request_id", publicTaskID),
				zap.Error(err),
			)
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task persistence failed")
			return
		}
		mediaHoldTransferred = mediaHold != nil && !service.IsMediaGenerationFailureStatus(status)
		if err := h.gatewayService.BindOpenAIVideoTaskAccount(requestCtx, apiKey.GroupID, publicTaskID, account.ID); err != nil {
			reqLog.Warn("openai.videos.bind_task_account_failed",
				zap.Int64("account_id", account.ID),
				zap.String("request_id", publicTaskID),
				zap.Error(err),
			)
		}
		if service.IsMediaGenerationSuccessStatus(status) {
			finalizeOpenAIVideoTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, result, task)
		}
		if !ensureOpenAIVideoTaskResultDeliverable(c, h, reqLog, apiKey.ID, result, task) {
			return
		}
		if err := writeOpenAIVideoForwardResponse(c, h, result, task, videoResponseBaseURL); err != nil {
			reqLog.Warn("openai.videos.forward_edge_url_failed", zap.String("request_id", publicTaskID), zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video content URL is unavailable")
		}
		reqLog.Debug("openai.videos.request_completed", zap.Int64("account_id", account.ID))
		return
	}

	if parsed.ContentRequest {
		streamStarted = result != nil
		reqLog.Debug("openai.videos.request_completed", zap.Int64("account_id", account.ID))
		return
	}

	if result == nil || storedTask == nil || len(result.ResponseBody) == 0 {
		writeOpenAIVideoSafeUpstreamError(c)
		return
	}
	clientTaskID := storedTask.ClientTaskID()
	result.ResponseBody = service.SanitizeOpenAIVideoStoredResponseBody(result.ResponseBody, result.VideoStatus)
	result.ResponseBody = service.RewriteOpenAIVideoClientResponseBody(result.ResponseBody, clientTaskID, storedTask.ProviderTaskID())
	if err := h.gatewayService.UpdateOpenAIVideoTaskResponse(requestCtx, apiKey.ID, clientTaskID, result); err != nil {
		reqLog.Warn("openai.videos.update_task_response_failed", zap.String("request_id", clientTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task update failed")
		return
	}
	freshTask, err := h.gatewayService.GetOpenAIVideoTaskByTaskID(requestCtx, apiKey.ID, clientTaskID)
	if err != nil || freshTask == nil {
		reqLog.Warn("openai.videos.reload_task_after_update_failed", zap.String("request_id", clientTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video task update verification failed")
		return
	}
	storedTask = freshTask
	applyOpenAIVideoStoredTaskToForwardResult(result, storedTask)
	finalizeOpenAIVideoTaskFromStatus(c, h, reqLog, apiKey, subject, subscription, account, result, storedTask)
	if !ensureOpenAIVideoTaskResultDeliverable(c, h, reqLog, apiKey.ID, result, storedTask) {
		return
	}
	if err := writeOpenAIVideoForwardResponse(c, h, result, storedTask, videoResponseBaseURL); err != nil {
		reqLog.Warn("openai.videos.status_edge_url_failed", zap.String("request_id", clientTaskID), zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Video content URL is unavailable")
	}
	reqLog.Debug("openai.videos.request_completed", zap.Int64("account_id", account.ID))
}

func ensureOpenAIVideoTaskResultDeliverable(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKeyID int64,
	result *service.OpenAIForwardResult,
	task *service.MediaGenerationTask,
) bool {
	if result == nil || !service.IsMediaGenerationSuccessStatus(result.VideoStatus) {
		return true
	}
	if h == nil || h.gatewayService == nil || task == nil {
		return false
	}
	freshTask, err := h.gatewayService.GetOpenAIVideoTaskByTaskID(c.Request.Context(), apiKeyID, task.ClientTaskID())
	if err == nil && freshTask != nil && freshTask.UsageRecordedAt != nil && service.IsMediaGenerationSuccessStatus(freshTask.Status) {
		return true
	}
	reqLog.Warn("openai.videos.result_delivery_blocked_unsettled",
		zap.String("request_id", task.ClientTaskID()),
		zap.Error(err),
	)
	h.errorResponse(c, http.StatusServiceUnavailable, "billing_service_error", "Video result settlement is incomplete")
	return false
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
	if result == nil || task == nil || strings.TrimSpace(task.ClientTaskID()) == "" {
		return
	}
	status := service.NormalizeMediaGenerationStatus(result.VideoStatus)
	if status == service.MediaGenerationStatusPending || status == service.MediaGenerationStatusRunning || status == "" {
		return
	}
	clientTaskID := task.ClientTaskID()
	if service.IsMediaGenerationFailureStatus(status) {
		if err := h.gatewayService.MarkOpenAIVideoTaskTerminal(c.Request.Context(), apiKey.ID, clientTaskID, status, "video_task_failed"); err != nil {
			reqLog.Warn("openai.videos.mark_task_failed_status_failed", zap.String("request_id", clientTaskID), zap.String("status", status), zap.Error(err))
		}
		releaseMediaBalanceHold(h, mediaBalanceHoldCommandForTask(task, apiKey))
		return
	}
	if !service.IsMediaGenerationSuccessStatus(status) || task.UsageRecordedAt != nil {
		return
	}
	leaseToken := uuid.NewString()
	acquired, err := h.gatewayService.TryAcquireOpenAIVideoFinalization(
		c.Request.Context(),
		apiKey.ID,
		clientTaskID,
		leaseToken,
		time.Now().UTC().Add(2*time.Minute),
	)
	if err != nil {
		reqLog.Warn("openai.videos.acquire_finalization_failed", zap.String("request_id", clientTaskID), zap.Error(err))
		return
	}
	if !acquired {
		return
	}
	recordOpenAIVideoFinalUsage(c, h, reqLog, apiKey, subject, subscription, account, result, task, leaseToken)
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
	leaseToken string,
) {
	if statusResult == nil || task == nil {
		return
	}
	clientTaskID := task.ClientTaskID()
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	durationSeconds := task.DurationSeconds
	if statusResult.MediaDurationSeconds > 0 {
		durationSeconds = statusResult.MediaDurationSeconds
	}
	requestModel := strings.TrimSpace(task.RequestedModel)
	if requestModel == "" {
		requestModel = strings.TrimSpace(task.Model)
	}
	upstreamModel := strings.TrimSpace(task.UpstreamModel)
	if upstreamModel == "" {
		upstreamModel = strings.TrimSpace(statusResult.UpstreamModel)
	}
	videoResolution := strings.TrimSpace(task.Resolution)
	if videoResolution == "" {
		videoResolution = statusResult.VideoResolution
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
	videoCount := task.RequestCount
	if videoCount <= 0 {
		videoCount = 1
	}
	recordResult := &service.OpenAIForwardResult{
		RequestID:            clientTaskID,
		ResponseID:           clientTaskID,
		Model:                requestModel,
		BillingModel:         upstreamModel,
		UpstreamModel:        upstreamModel,
		Usage:                statusResult.Usage,
		Stream:               false,
		Duration:             statusResult.Duration,
		ImageCount:           0,
		VideoCount:           videoCount,
		VideoResolution:      videoResolution,
		VideoDurationSeconds: durationSeconds,
		MediaDurationSeconds: durationSeconds,
		MediaType:            "video",
		ResponseStatus:       statusResult.ResponseStatus,
		VideoStatus:          statusResult.VideoStatus,
	}
	mediaHold := mediaBalanceHoldCommandForTask(task, apiKey)
	actualMediaCost := mediaBalanceSettledCost(mediaHold, mediaBalanceActualCost(task.PricingSnapshot(), recordResult, videoCount, durationSeconds, true))
	if markErr := markMediaBalanceHoldForCapture(h, mediaHold, actualMediaCost); markErr != nil {
		reqLog.Warn("openai.videos.mark_balance_capture_pending_failed", zap.String("request_id", clientTaskID), zap.Error(markErr))
	}
	if err := recordMediaUsageWithRetry(func(ctx context.Context) error {
		return h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:               recordResult,
			APIKey:               apiKey,
			User:                 apiKey.User,
			Account:              account,
			Subscription:         subscription,
			InboundEndpoint:      strings.TrimSpace(task.InboundEndpoint),
			UpstreamEndpoint:     strings.TrimSpace(task.UpstreamEndpoint),
			UserAgent:            userAgent,
			IPAddress:            clientIP,
			RequestPayloadHash:   strings.TrimSpace(task.RequestPayloadHash),
			APIKeyService:        h.apiKeyService,
			QuotaPlatform:        quotaPlatform,
			MediaPricingSnapshot: task.PricingSnapshot(),
			MediaBalanceHoldRequestID: func() string {
				if mediaHold := mediaBalanceHoldCommandForTask(task, apiKey); mediaHold != nil {
					return mediaHold.RequestID
				}
				return ""
			}(),
			MediaBalanceHoldAmount: func() float64 {
				if mediaHold == nil {
					return 0
				}
				return mediaHold.HoldAmount
			}(),
			ChannelUsageFields: usageFields,
		})
	}); err != nil {
		logger.L().With(
			zap.String("component", "handler.openai_gateway.videos"),
			zap.Int64("user_id", subject.UserID),
			zap.Int64("api_key_id", apiKey.ID),
			zap.Any("group_id", apiKey.GroupID),
			zap.String("model", requestModel),
			zap.Int64("account_id", account.ID),
			zap.String("request_id", clientTaskID),
		).Error("openai.videos.record_final_usage_failed", zap.Error(err))
		if releaseErr := h.gatewayService.ReleaseOpenAIVideoFinalization(context.Background(), apiKey.ID, clientTaskID, leaseToken, "usage_record_failed"); releaseErr != nil {
			reqLog.Warn("openai.videos.release_finalization_failed", zap.String("request_id", clientTaskID), zap.Error(releaseErr))
		}
		return
	}
	finalizeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completed, err := h.gatewayService.CompleteOpenAIVideoFinalization(finalizeCtx, apiKey.ID, clientTaskID, leaseToken)
	if err != nil || !completed {
		reqLog.Warn("openai.videos.complete_finalization_failed", zap.String("request_id", clientTaskID), zap.Bool("completed", completed), zap.Error(err))
		if releaseErr := h.gatewayService.ReleaseOpenAIVideoFinalization(finalizeCtx, apiKey.ID, clientTaskID, leaseToken, "finalization_complete_failed"); releaseErr != nil {
			reqLog.Warn("openai.videos.release_finalization_failed", zap.String("request_id", clientTaskID), zap.Error(releaseErr))
		}
	}
}

func persistOpenAIVideoTaskOutcome(ctx context.Context, h *OpenAIGatewayHandler, task *service.MediaGenerationTask) error {
	if h == nil || h.gatewayService == nil {
		return errors.New("video gateway service is unavailable")
	}
	persistCtx := context.Background()
	if ctx != nil {
		persistCtx = context.WithoutCancel(ctx)
	}
	persistCtx, cancel := context.WithTimeout(persistCtx, 5*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := h.gatewayService.CreateOpenAIVideoTask(persistCtx, task); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-persistCtx.Done():
			timer.Stop()
			return lastErr
		case <-timer.C:
		}
	}
	return lastErr
}

func isOpenAIVideoTerminalStatus(status string) bool {
	return service.IsMediaGenerationSuccessStatus(status) || service.IsMediaGenerationFailureStatus(status)
}

func openAIVideoTaskNeedsFinalization(task *service.MediaGenerationTask) bool {
	return task != nil && service.IsMediaGenerationSuccessStatus(task.Status) && task.UsageRecordedAt == nil
}

func openAIVideoForwardResultFromStoredTask(task *service.MediaGenerationTask) *service.OpenAIForwardResult {
	if task == nil {
		return nil
	}
	return &service.OpenAIForwardResult{
		RequestID:            task.ClientTaskID(),
		ResponseID:           task.ProviderTaskID(),
		Model:                task.RequestedModel,
		BillingModel:         task.UpstreamModel,
		UpstreamModel:        task.UpstreamModel,
		Duration:             0,
		VideoCount:           1,
		VideoResolution:      task.Resolution,
		VideoDurationSeconds: task.DurationSeconds,
		MediaDurationSeconds: task.DurationSeconds,
		MediaType:            "video",
		ResponseStatus:       task.ResponseStatus,
		ResponseBody:         []byte(task.ResponseBody),
		ResponseContentType:  task.ResponseContentType,
		MediaResultURL:       task.UpstreamResultURL,
		VideoStatus:          task.Status,
	}
}

func applyOpenAIVideoStoredTaskToForwardResult(result *service.OpenAIForwardResult, task *service.MediaGenerationTask) {
	if result == nil || task == nil {
		return
	}
	result.ResponseID = task.ProviderTaskID()
	result.Model = task.RequestedModel
	result.BillingModel = task.UpstreamModel
	result.UpstreamModel = task.UpstreamModel
	result.VideoResolution = task.Resolution
	result.VideoDurationSeconds = task.DurationSeconds
	result.MediaDurationSeconds = task.DurationSeconds
	result.MediaType = "video"
	result.ResponseStatus = task.ResponseStatus
	result.ResponseBody = []byte(task.ResponseBody)
	result.ResponseContentType = task.ResponseContentType
	result.MediaResultURL = task.UpstreamResultURL
	result.VideoStatus = task.Status
}

func writeOpenAIVideoStoredResponse(c *gin.Context, h *OpenAIGatewayHandler, task *service.MediaGenerationTask, publicBaseURL string) error {
	if c == nil || task == nil {
		return nil
	}
	statusCode := task.ResponseStatus
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}
	contentType := strings.TrimSpace(task.ResponseContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	body, err := openAIVideoClientResponseBody(c.Request.Context(), h, []byte(task.ResponseBody), task.Status, task.ClientTaskID(), task.UpstreamResultURL, publicBaseURL, task, task.ProviderTaskID())
	if err != nil {
		return err
	}
	c.Data(statusCode, contentType, body)
	return nil
}

func writeOpenAIVideoForwardResponse(c *gin.Context, h *OpenAIGatewayHandler, result *service.OpenAIForwardResult, task *service.MediaGenerationTask, publicBaseURL string) error {
	if c == nil || result == nil || task == nil {
		return nil
	}
	statusCode := result.ResponseStatus
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}
	contentType := strings.TrimSpace(result.ResponseContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	body, err := openAIVideoClientResponseBody(c.Request.Context(), h, result.ResponseBody, result.VideoStatus, task.ClientTaskID(), result.MediaResultURL, publicBaseURL, task, result.ResponseID)
	if err != nil {
		return err
	}
	c.Data(statusCode, contentType, body)
	return nil
}

func openAIVideoClientResponseBody(
	ctx context.Context,
	h *OpenAIGatewayHandler,
	body []byte,
	status string,
	clientTaskID string,
	upstreamResultURL string,
	publicBaseURL string,
	task *service.MediaGenerationTask,
	upstreamTaskIDs ...string,
) ([]byte, error) {
	if h == nil || h.gatewayService == nil {
		return nil, errors.New("video gateway service is unavailable")
	}
	if strings.TrimSpace(upstreamResultURL) == "" {
		upstreamResultURL = service.OpenAIVideoResultURLFromBody(body)
	}
	var contentURL string
	var err error
	if strings.TrimSpace(upstreamResultURL) == "" {
		contentURL = ""
	} else if task != nil {
		contentURL, err = h.gatewayService.OpenAIVideoClientContentURLForTask(ctx, publicBaseURL, task)
	} else {
		contentURL, err = h.gatewayService.OpenAIVideoClientContentURL(ctx, publicBaseURL, clientTaskID, upstreamResultURL)
	}
	if err != nil {
		return nil, err
	}
	body = service.SanitizeOpenAIVideoStoredResponseBody(body, status)
	return service.RewriteOpenAIVideoClientResponseBodyWithContentURL(body, clientTaskID, contentURL, upstreamTaskIDs...), nil
}

func resolveOpenAIVideoResponseBaseURL(c *gin.Context, configuredBaseURL string) string {
	if value := normalizeOpenAIVideoResponseBaseURL(configuredBaseURL); value != "" {
		return value
	}
	if c == nil || c.Request == nil {
		return ""
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if forwardedProto := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]); forwardedProto == "http" || forwardedProto == "https" {
		scheme = forwardedProto
	}
	return normalizeOpenAIVideoResponseBaseURL(scheme + "://" + strings.TrimSpace(c.Request.Host))
}

func normalizeOpenAIVideoResponseBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return raw
}

func writeOpenAIVideoSafeUpstreamError(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	for _, header := range []string{"Location", "X-Request-ID", "X-Upstream-Request-ID", "Server", "Via"} {
		c.Writer.Header().Del(header)
	}
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"message": "Video upstream request failed",
			"type":    "upstream_error",
			"param":   nil,
			"code":    nil,
		},
	})
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
