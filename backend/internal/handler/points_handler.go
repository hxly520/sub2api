package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const pointsCreditBodyLimit = 64 * 1024

type PointsHandler struct {
	service *service.PointsBridgeService
}

func NewPointsHandler(pointsService *service.PointsBridgeService) *PointsHandler {
	return &PointsHandler{service: pointsService}
}

type pointsLaunchRequest struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
}

func (h *PointsHandler) LaunchUser(c *gin.Context) {
	h.launch(c, "user")
}

func (h *PointsHandler) LaunchAdmin(c *gin.Context) {
	h.launch(c, "admin")
}

func (h *PointsHandler) AccessUser(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not found in context")
		return
	}
	allowed, err := h.service.ResolveUserAccess(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"allowed": allowed})
}

func (h *PointsHandler) StatusAdmin(c *gin.Context) {
	status := h.service.Status()
	response.Success(c, gin.H{
		"enabled":                  status.Enabled,
		"configured":               status.Configured,
		"active":                   status.Active,
		"public_url":               status.PublicURL,
		"menu_label":               status.MenuLabel,
		"launch_key_id":            status.LaunchKeyID,
		"launch_secret_configured": status.LaunchSecretConfigured,
		"credit_key_id":            status.CreditKeyID,
		"credit_secret_configured": status.CreditSecretConfigured,
		"launch_ttl_seconds":       status.LaunchTTLSeconds,
		"clock_skew_seconds":       status.ClockSkewSeconds,
	})
}

func (h *PointsHandler) launch(c *gin.Context, role string) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not found in context")
		return
	}
	var request pointsLaunchRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.BadRequest(c, "Invalid points launch request")
			return
		}
	}
	if role == "user" {
		allowed, err := h.service.ResolveUserAccess(c.Request.Context(), subject.UserID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if !allowed {
			response.ErrorFrom(c, service.ErrPointsSystemUnavailable)
			return
		}
	}
	launchURL, err := h.service.CreateLaunchURL(subject.UserID, role, request.Theme, request.Language)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"launch_url": launchURL})
}

func (h *PointsHandler) Credit(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, pointsCreditBodyLimit)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "Invalid points credit request")
		return
	}
	result, err := h.service.VerifyAndApplyCredit(
		c.Request.Context(),
		c.Request.Method,
		c.FullPath(),
		c.GetHeader("X-Points-Timestamp"),
		c.GetHeader("X-Points-Key-ID"),
		c.GetHeader("X-Points-Nonce"),
		c.GetHeader("X-Points-Signature"),
		body,
		strings.TrimSpace(c.GetString("request_id")),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"transaction_id": result.TransactionID.String(),
		"balance_after":  result.BalanceAfter.StringFixed(2),
		"idempotent":     result.Idempotent,
	})
}
