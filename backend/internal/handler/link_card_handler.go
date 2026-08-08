package handler

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type LinkCardHandler struct{ service *service.LinkCardService }

func NewLinkCardHandler(s *service.LinkCardService) *LinkCardHandler {
	return &LinkCardHandler{service: s}
}

func (h *LinkCardHandler) Access(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	result, err := h.service.Access(c.Request.Context(), userID)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}
func (h *LinkCardHandler) Settings(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	access, err := h.service.Access(c.Request.Context(), userID)
	if response.ErrorFrom(c, err) {
		return
	}
	if !access.Allowed {
		response.ErrorFrom(c, service.ErrLinkCardsDisabled)
		return
	}
	settings, err := h.service.GetSettings(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"enabled": settings.Enabled, "public_portal_url": settings.PublicPortalURL, "api_base_url": settings.APIBaseURL, "default_concurrency": settings.DefaultConcurrency, "max_batch_size": settings.MaxBatchSize, "minimum_deposit": settings.MinimumDeposit})
}
func (h *LinkCardHandler) Groups(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	items, err := h.service.ListGroups(c.Request.Context(), userID, false)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, items)
}
func (h *LinkCardHandler) Cards(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	params := linkCardPaginationParams(c)
	filters := parseLinkCardFilters(c)
	items, page, err := h.service.ListCards(c.Request.Context(), userID, false, params, filters)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Paginated(c, items, page.Total, page.Page, page.PageSize)
}

type createLinkCardsBody struct {
	GroupID        int64           `json:"group_id" binding:"required"`
	Quantity       int             `json:"quantity" binding:"required"`
	Amount         decimal.Decimal `json:"amount"`
	DepositAmount  decimal.Decimal `json:"deposit_amount"`
	IdempotencyKey string          `json:"idempotency_key"`
}

func (h *LinkCardHandler) Create(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	var body createLinkCardsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	amount := body.Amount
	if amount.IsZero() {
		amount = body.DepositAmount
	}
	idem := linkCardIdempotency(c, body.IdempotencyKey)
	result, err := h.service.Create(c.Request.Context(), userID, service.CreateLinkCardsRequest{GroupID: body.GroupID, Quantity: body.Quantity, Amount: amount, IdempotencyKey: idem})
	if response.ErrorFrom(c, err) {
		return
	}
	response.Created(c, result)
}

type rechargeLinkCardBody struct {
	Amount         decimal.Decimal `json:"amount"`
	IdempotencyKey string          `json:"idempotency_key"`
}

func (h *LinkCardHandler) Recharge(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	id, ok := positiveInt64Param(c, "id")
	if !ok {
		return
	}
	var body rechargeLinkCardBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	result, err := h.service.Recharge(c.Request.Context(), userID, id, body.Amount, linkCardIdempotency(c, body.IdempotencyKey), false)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

type refundLinkCardBody struct {
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (h *LinkCardHandler) Refund(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	id, ok := positiveInt64Param(c, "id")
	if !ok {
		return
	}
	var body refundLinkCardBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid request")
		return
	}
	result, err := h.service.Refund(c.Request.Context(), userID, id, body.Reason, linkCardIdempotency(c, body.IdempotencyKey), false)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}
func (h *LinkCardHandler) Usage(c *gin.Context) {
	userID, ok := linkCardUserID(c)
	if !ok {
		return
	}
	params := linkCardPaginationParams(c)
	filters := parseLinkCardUsageFilters(c)
	items, page, err := h.service.ListUsage(c.Request.Context(), userID, false, params, filters)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Paginated(c, items, page.Total, page.Page, page.PageSize)
}

type activateLinkCardBody struct {
	Key string `json:"key" binding:"required"`
}

func (h *LinkCardHandler) Activate(c *gin.Context) {
	var body activateLinkCardBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid key")
		return
	}
	result, err := h.service.Activate(c.Request.Context(), body.Key)
	if errors.Is(err, service.ErrLinkCardNotFound) && middleware2.HandleLinkCardActivationFailure(c) {
		return
	}
	if response.ErrorFrom(c, err) {
		return
	}
	middleware2.ResetLinkCardActivationFailures(c)
	response.Success(c, result)
}
func (h *LinkCardHandler) PublicProfile(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Vary", "X-Link-Card-Session")
	result, err := h.service.PortalCard(c.Request.Context(), strings.TrimSpace(c.GetHeader("X-Link-Card-Session")))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}
func (h *LinkCardHandler) PublicUsage(c *gin.Context) {
	params := linkCardPaginationParams(c)
	filters := parseLinkCardUsageFilters(c)
	items, page, err := h.service.PortalUsage(c.Request.Context(), strings.TrimSpace(c.GetHeader("X-Link-Card-Session")), params, filters)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Paginated(c, items, page.Total, page.Page, page.PageSize)
}

func linkCardUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}
func linkCardPaginationParams(c *gin.Context) pagination.PaginationParams {
	page, size := response.ParsePagination(c)
	if c.Query("page_size") == "" && c.Query("limit") == "" {
		size = 10
	}
	return pagination.PaginationParams{Page: page, PageSize: size, SortBy: c.Query("sort_by"), SortOrder: c.Query("sort_order")}
}
func parseLinkCardFilters(c *gin.Context) service.LinkCardListFilters {
	f := service.LinkCardListFilters{Search: strings.TrimSpace(c.Query("search")), Status: strings.TrimSpace(c.Query("status")), CreatorEmail: strings.TrimSpace(c.Query("creator_email"))}
	if v, ok := queryInt64(c, "group_id"); ok {
		f.GroupID = &v
	}
	if v, ok := queryInt64(c, "creator_user_id"); ok {
		f.CreatorUserID = &v
	}
	return f
}
func parseLinkCardUsageFilters(c *gin.Context) service.LinkCardUsageFilters {
	f := service.LinkCardUsageFilters{RequestID: strings.TrimSpace(c.Query("request_id")), Model: strings.TrimSpace(c.Query("model")), RequestType: strings.TrimSpace(c.Query("request_type")), CreatorEmail: strings.TrimSpace(c.Query("creator_email")), Key: strings.TrimSpace(c.Query("key"))}
	if v, ok := queryInt64(c, "card_id"); ok {
		f.CardID = &v
	}
	if v, ok := queryInt64(c, "group_id"); ok {
		f.GroupID = &v
	}
	if v, ok := queryInt64(c, "creator_user_id"); ok {
		f.CreatorUserID = &v
	}
	if raw := strings.TrimSpace(c.Query("stream")); raw != "" {
		v := raw == "true"
		f.Stream = &v
	}
	if v, err := time.Parse(time.RFC3339, strings.TrimSpace(c.Query("start_date"))); err == nil {
		f.StartAt = &v
	}
	if v, err := time.Parse(time.RFC3339, strings.TrimSpace(c.Query("end_date"))); err == nil {
		f.EndAt = &v
	}
	return f
}
func linkCardIdempotency(c *gin.Context, body string) string {
	if value := strings.TrimSpace(c.GetHeader("Idempotency-Key")); value != "" {
		return value
	}
	return strings.TrimSpace(body)
}
func queryInt64(c *gin.Context, name string) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(c.Query(name)), 10, 64)
	return v, err == nil && v > 0
}
func positiveInt64Param(c *gin.Context, name string) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || v <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return v, true
}
