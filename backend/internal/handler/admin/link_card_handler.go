package admin

import (
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
func (h *LinkCardHandler) Settings(c *gin.Context) {
	settings, err := h.service.GetSettings(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, settings)
}
func (h *LinkCardHandler) UpdateSettings(c *gin.Context) {
	var body service.UpdateLinkCardSettingsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid settings")
		return
	}
	settings, err := h.service.UpdateSettings(c.Request.Context(), body)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, settings)
}
func (h *LinkCardHandler) Groups(c *gin.Context) {
	items, err := h.service.ListGroups(c.Request.Context(), 0, true)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, items)
}

type linkCardGroupBody struct {
	GroupID            int64 `json:"group_id" binding:"required"`
	Enabled            *bool `json:"enabled"`
	SortOrder          int   `json:"sort_order"`
	DefaultConcurrency *int  `json:"default_concurrency"`
}

func (h *LinkCardHandler) UpsertGroup(c *gin.Context) {
	actor, ok := adminLinkCardActor(c)
	if !ok {
		return
	}
	var body linkCardGroupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid group")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	item, err := h.service.AuthorizeGroup(c.Request.Context(), actor, body.GroupID, enabled, body.SortOrder)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, item)
}
func (h *LinkCardHandler) RemoveGroup(c *gin.Context) {
	var body linkCardGroupBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid group")
		return
	}
	if response.ErrorFrom(c, h.service.RemoveAuthorizedGroup(c.Request.Context(), body.GroupID)) {
		return
	}
	response.Success(c, gin.H{"message": "removed"})
}
func (h *LinkCardHandler) Cards(c *gin.Context) {
	actor, ok := adminLinkCardActor(c)
	if !ok {
		return
	}
	params := adminLinkCardPagination(c)
	filters := adminLinkCardFilters(c)
	items, page, err := h.service.ListCards(c.Request.Context(), actor, true, params, filters)
	if response.ErrorFrom(c, err) {
		return
	}
	summary, err := h.service.Summary(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"items": items, "total": page.Total, "page": page.Page, "page_size": page.PageSize, "pages": page.Pages, "summary": summary})
}
func (h *LinkCardHandler) Usage(c *gin.Context) {
	actor, ok := adminLinkCardActor(c)
	if !ok {
		return
	}
	params := adminLinkCardPagination(c)
	filters := adminLinkCardUsageFilters(c)
	items, page, err := h.service.ListUsage(c.Request.Context(), actor, true, params, filters)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Paginated(c, items, page.Total, page.Page, page.PageSize)
}

type linkCardActionBody struct {
	Action         string          `json:"action" binding:"required"`
	Amount         decimal.Decimal `json:"amount"`
	Concurrency    *int            `json:"concurrency"`
	RPMLimit       *int            `json:"rpm_limit"`
	Reason         string          `json:"reason"`
	IdempotencyKey string          `json:"idempotency_key"`
}

func (h *LinkCardHandler) Action(c *gin.Context) {
	actor, ok := adminLinkCardActor(c)
	if !ok {
		return
	}
	id, ok := adminPositiveID(c, "id")
	if !ok {
		return
	}
	var body linkCardActionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "invalid action")
		return
	}
	idem := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idem == "" {
		idem = strings.TrimSpace(body.IdempotencyKey)
	}
	result, err := h.service.AdminAction(c.Request.Context(), actor, id, body.Action, body.Amount, body.Concurrency, body.RPMLimit, body.Reason, idem)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}
func adminLinkCardActor(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Admin not authenticated")
		return 0, false
	}
	return subject.UserID, true
}
func adminLinkCardPagination(c *gin.Context) pagination.PaginationParams {
	page, size := response.ParsePagination(c)
	if c.Query("page_size") == "" && c.Query("limit") == "" {
		size = 10
	}
	return pagination.PaginationParams{Page: page, PageSize: size, SortBy: c.Query("sort_by"), SortOrder: c.Query("sort_order")}
}
func adminQueryID(c *gin.Context, name string) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(c.Query(name)), 10, 64)
	return v, err == nil && v > 0
}
func adminPositiveID(c *gin.Context, name string) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || v <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return v, true
}
func adminLinkCardFilters(c *gin.Context) service.LinkCardListFilters {
	f := service.LinkCardListFilters{Search: strings.TrimSpace(c.Query("search")), Status: strings.TrimSpace(c.Query("status")), CreatorEmail: strings.TrimSpace(c.Query("creator_email"))}
	if v, ok := adminQueryID(c, "group_id"); ok {
		f.GroupID = &v
	}
	if v, ok := adminQueryID(c, "creator_user_id"); ok {
		f.CreatorUserID = &v
	}
	return f
}
func adminLinkCardUsageFilters(c *gin.Context) service.LinkCardUsageFilters {
	f := service.LinkCardUsageFilters{RequestID: strings.TrimSpace(c.Query("request_id")), Model: strings.TrimSpace(c.Query("model")), RequestType: strings.TrimSpace(c.Query("request_type")), CreatorEmail: strings.TrimSpace(c.Query("creator_email")), Key: strings.TrimSpace(c.Query("key"))}
	if v, ok := adminQueryID(c, "card_id"); ok {
		f.CardID = &v
	}
	if v, ok := adminQueryID(c, "group_id"); ok {
		f.GroupID = &v
	}
	if v, ok := adminQueryID(c, "creator_user_id"); ok {
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
