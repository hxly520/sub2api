package handler

import (
	"context"
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

// GrokImages handles xAI image generation/editing through Grok groups.
func (h *OpenAIGatewayHandler) GrokImages(c *gin.Context) {
	endpoint := service.GrokMediaEndpointImagesGenerations
	if strings.Contains(c.Request.URL.Path, "/images/edits") {
		endpoint = service.GrokMediaEndpointImagesEdits
	}
	h.handleGrokMedia(c, endpoint, "")
}

// GrokVideoGeneration handles xAI video generation through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoGeneration(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosGenerations, "")
}

// GrokVideoEdit handles asynchronous xAI video edits through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoEdit(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosEdits, "")
}

// GrokVideoExtension handles asynchronous xAI video extensions through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoExtension(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideosExtensions, "")
}

// GrokVideoStatus handles xAI video status retrieval through Grok groups.
func (h *OpenAIGatewayHandler) GrokVideoStatus(c *gin.Context) {
	h.handleGrokMedia(c, service.GrokMediaEndpointVideoStatus, c.Param("request_id"))
}

func (h *OpenAIGatewayHandler) handleGrokMedia(c *gin.Context, endpoint service.GrokMediaEndpoint, requestID string) {
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
		"handler.openai_gateway.grok_media",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("endpoint", string(endpoint)),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	var body []byte
	var err error
	if endpoint.RequiresRequestBody() {
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
		if len(body) == 0 {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
			return
		}
	}

	contentType := c.GetHeader("Content-Type")
	requestInfo := service.ParseGrokMediaRequest(contentType, body)
	requestModel := requestInfo.Model
	if endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if endpoint == service.GrokMediaEndpointVideoStatus && strings.TrimSpace(requestID) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "request_id is required")
		return
	}

	reqLog = reqLog.With(zap.String("model", requestModel))
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	if endpoint.IsGenerationRequest() {
		if !service.GroupAllowsImageGeneration(apiKey.Group) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
			return
		}
		if moderationBody := requestInfo.ModerationBody(); len(moderationBody) > 0 {
			decision := h.checkContentModeration(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, moderationBody)
			if decision != nil && decision.Blocked {
				h.errorResponse(c, contentModerationStatus(decision), contentModerationErrorCode(decision), decision.Message)
				return
			}
		}
		imageReleaseFunc, acquired := h.acquireImageGenerationSlot(c, streamStarted)
		if !acquired {
			return
		}
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
		}
	}

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	var mediaPricingSnapshot *service.MediaGenerationPricingSnapshot
	var mediaHold *service.MediaBalanceHoldCommand
	mediaHoldTransferred := false
	defer func() {
		if !mediaHoldTransferred {
			releaseMediaBalanceHold(h, mediaHold)
		}
	}()

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("grok_media.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	sessionSeed := body
	if len(sessionSeed) == 0 && strings.TrimSpace(requestID) != "" {
		sessionSeed = []byte(requestID)
	}
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, sessionSeed)
	if endpoint == service.GrokMediaEndpointVideoStatus {
		sessionHash = service.GrokMediaVideoRequestSessionHash(requestID)
	}
	requestCtx := withOpenAIAccountScheduleProfile(c.Request.Context(), c, requestModel)
	routingStart := time.Now()

	selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		requestCtx,
		apiKey.GroupID,
		"",
		sessionHash,
		requestModel,
		nil,
		service.OpenAIUpstreamTransportHTTPSSE,
		"",
		false,
		false,
		service.PlatformGrok,
	)
	if err != nil {
		reqLog.Warn("grok_media.account_select_failed",
			zap.Error(err),
		)
		cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, requestModel, service.PlatformGrok)
		if !cls.ModelNotFound {
			markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		}
		h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
		return
	}
	if selection == nil || selection.Account == nil {
		cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, requestModel, service.PlatformGrok)
		if !cls.ModelNotFound {
			markOpsRoutingCapacityLimited(c)
		}
		h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
		return
	}

	reqLog.Debug("grok_media.account_schedule_decision",
		zap.String("layer", scheduleDecision.Layer),
		zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
		zap.Int("candidate_count", scheduleDecision.CandidateCount),
		zap.Int("top_k", scheduleDecision.TopK),
		zap.Int64("latency_ms", scheduleDecision.LatencyMs),
		zap.Float64("load_skew", scheduleDecision.LoadSkew),
	)

	account := selection.Account
	sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
	setOpsSelectedAccount(c, account.ID, account.Platform)
	if endpoint.IsGenerationRequest() && h.gatewayService.MediaGenerationBalanceHoldRequired(apiKey, subscription) {
		mappedModel := account.GetMappedModel(requestModel)
		if strings.TrimSpace(mappedModel) == "" {
			mappedModel = requestModel
		}
		usageFields := service.ChannelUsageFields{
			OriginalModel:      requestModel,
			ChannelMappedModel: mappedModel,
		}
		var pricingErr error
		if endpoint.IsVideoGenerationRequest() {
			mediaPricingSnapshot, pricingErr = h.gatewayService.CaptureOpenAIVideoPricingSnapshot(
				requestCtx,
				apiKey,
				subject.UserID,
				requestModel,
				mappedModel,
				requestInfo.Resolution,
				requestInfo.DurationSeconds,
				1,
				usageFields,
			)
		} else {
			mediaPricingSnapshot, pricingErr = h.gatewayService.CaptureOpenAIImagePricingSnapshot(
				requestCtx,
				apiKey,
				subject.UserID,
				requestModel,
				mappedModel,
				requestInfo.SizeTier,
				requestInfo.N,
				usageFields,
			)
		}
		if pricingErr != nil {
			reqLog.Warn("grok_media.capture_pricing_snapshot_failed", zap.Error(pricingErr))
			h.errorResponse(c, http.StatusServiceUnavailable, "billing_service_error", "Media pricing is unavailable")
			return
		}
		if mediaPricingSnapshot != nil {
			holdAmount := mediaPricingSnapshot.EstimatedCost(requestInfo.N, requestInfo.DurationSeconds)
			if endpoint.IsVideoGenerationRequest() {
				holdAmount = mediaPricingSnapshot.EstimatedCost(1, requestInfo.DurationSeconds)
			}
			if holdAmount > 0 {
				mediaHold = newMediaBalanceHoldCommand(
					apiKey.ID,
					subject.UserID,
					service.NewMediaBalanceHoldRequestID(),
					service.HashMediaGenerationRequestFingerprint(string(endpoint), body),
					service.HashUsageRequestPayload(body),
					holdAmount,
				)
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
	}

	accountReleaseFunc, accountAcquired := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
	if !accountAcquired {
		return
	}

	service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
	if endpoint.IsGenerationRequest() {
		if err := markMediaBalanceHoldDispatched(h, mediaHold); err != nil {
			reqLog.Warn("grok_media.mark_balance_dispatched_failed", zap.Error(err))
			h.errorResponse(c, http.StatusServiceUnavailable, "billing_service_error", "Media billing reservation is unavailable")
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
		return h.gatewayService.ForwardGrokMedia(requestCtx, c, account, endpoint, requestID, body, contentType)
	}()

	forwardDurationMs := time.Since(forwardStart).Milliseconds()
	upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
	responseLatencyMs := forwardDurationMs
	if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
		responseLatencyMs = forwardDurationMs - upstreamLatencyMs
	}
	service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

	if err != nil {
		var failoverErr *service.UpstreamFailoverError
		if errors.As(err, &failoverErr) {
			mediaHoldTransferred = mediaHold != nil
			h.reportOpenAIAccountScheduleResult(c, account, requestModel, false, nil)
			if c.Writer.Size() != writerSizeBeforeForward {
				h.handleFailoverExhausted(c, failoverErr, true)
				return
			}
			reqLog.Warn("grok_media.automatic_replay_suppressed",
				zap.Int64("account_id", account.ID),
				zap.Int("upstream_status", failoverErr.StatusCode),
				zap.Bool("media_generation", endpoint.IsGenerationRequest()),
			)
			h.handleFailoverExhausted(c, failoverErr, false)
			return
		}
		h.reportOpenAIAccountScheduleResult(c, account, requestModel, false, nil)
		if c.Writer.Size() == writerSizeBeforeForward {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		}
		reqLog.Warn("grok_media.forward_failed",
			zap.Int64("account_id", account.ID),
			zap.Error(err),
		)
		return
	}

	mediaHoldTransferred = mediaHold != nil
	h.reportOpenAIAccountScheduleResult(c, account, requestModel, true, nil)
	videoGeneration := endpoint.IsVideoGenerationRequest()
	actualMediaCost := mediaBalanceActualCost(mediaPricingSnapshot, result, requestInfo.N, requestInfo.DurationSeconds, videoGeneration)
	if markErr := markMediaBalanceHoldForCapture(h, mediaHold, actualMediaCost); markErr != nil {
		reqLog.Warn("grok_media.mark_balance_capture_pending_failed", zap.Error(markErr))
	}
	if endpoint.IsVideoGenerationRequest() && strings.TrimSpace(result.ResponseID) != "" {
		if err := h.gatewayService.BindGrokMediaVideoRequestAccount(requestCtx, apiKey.GroupID, result.ResponseID, account.ID); err != nil {
			reqLog.Warn("grok_media.bind_video_request_account_failed",
				zap.Int64("account_id", account.ID),
				zap.String("request_id", result.ResponseID),
				zap.Error(err),
			)
		}
	}
	if shouldRecordGrokMediaUsage(endpoint, requestModel) {
		recordGrokMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, result, requestModel, body, requestID, mediaPricingSnapshot, mediaHold)
	}
	reqLog.Debug("grok_media.request_completed", zap.Int64("account_id", account.ID))
}

func shouldRecordGrokMediaUsage(endpoint service.GrokMediaEndpoint, requestModel string) bool {
	return endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) != ""
}

func recordGrokMediaUsage(
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
	requestID string,
	mediaPricingSnapshot *service.MediaGenerationPricingSnapshot,
	mediaHold *service.MediaBalanceHoldCommand,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	payloadForHash := body
	if len(payloadForHash) == 0 && strings.TrimSpace(requestID) != "" {
		payloadForHash = []byte(requestID)
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	channelUsageFields := service.ChannelUsageFields{
		OriginalModel:      requestModel,
		ChannelMappedModel: requestModel,
	}
	if err := recordMediaUsageWithRetry(func(ctx context.Context) error {
		return h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:               result,
			APIKey:               apiKey,
			User:                 apiKey.User,
			Account:              account,
			Subscription:         subscription,
			InboundEndpoint:      inboundEndpoint,
			UpstreamEndpoint:     upstreamEndpoint,
			UserAgent:            userAgent,
			IPAddress:            clientIP,
			RequestPayloadHash:   service.HashUsageRequestPayload(payloadForHash),
			APIKeyService:        h.apiKeyService,
			QuotaPlatform:        quotaPlatform,
			MediaPricingSnapshot: mediaPricingSnapshot,
			MediaBalanceHoldRequestID: func() string {
				if mediaHold == nil {
					return ""
				}
				return mediaHold.RequestID
			}(),
			ChannelUsageFields: channelUsageFields,
		})
	}); err != nil {
		logger.L().With(
			zap.String("component", "handler.openai_gateway.grok_media"),
			zap.Int64("user_id", subject.UserID),
			zap.Int64("api_key_id", apiKey.ID),
			zap.Any("group_id", apiKey.GroupID),
			zap.String("model", requestModel),
			zap.Int64("account_id", account.ID),
		).Error("grok_media.record_usage_failed", zap.Error(err))
		reqLog.Debug("grok_media.record_usage_failed", zap.Error(err))
	}
}
