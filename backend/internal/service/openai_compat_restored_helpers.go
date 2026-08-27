package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// writeChatStreamUpstreamFailure terminates an already-started Chat
// Completions SSE response with an explicit error frame.
func writeChatStreamUpstreamFailure(c *gin.Context, message string) error {
	if c == nil || c.Writer == nil {
		return errors.New("chat stream writer is unavailable")
	}
	if !c.Writer.Written() {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("X-Accel-Buffering", "no")
	}
	payload, err := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": sanitizeClientUpstreamErrorMessage(message),
		},
	})
	if err != nil {
		return fmt.Errorf("marshal chat stream error: %w", err)
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// IsOpenAIStreamIncompleteAfterClientDisconnect keeps the legacy predicate
// available to compatibility callers while the core stream handler follows
// the official terminal-event contract.
func IsOpenAIStreamIncompleteAfterClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.HasPrefix(message, "stream usage incomplete") && strings.Contains(message, "client disconnect")
}

// isolateOpenAIAPIKeyRequestSessionHeaders keeps API-key compatibility
// requests tenant-scoped while preserving explicit session/conversation
// headers. Stateless requests use the derived prompt cache key as session.
func isolateOpenAIAPIKeyRequestSessionHeaders(req *http.Request, apiKeyID int64, promptCacheKey string) {
	if req == nil {
		return
	}
	clientSessionID := strings.TrimSpace(req.Header.Get("session_id"))
	clientConversationID := strings.TrimSpace(req.Header.Get("conversation_id"))
	req.Header.Del("session_id")
	req.Header.Del("conversation_id")
	if clientSessionID == "" {
		clientSessionID = strings.TrimSpace(promptCacheKey)
	}
	if clientSessionID != "" {
		req.Header.Set("session_id", generateSessionUUID(isolateOpenAISessionID(apiKeyID, clientSessionID)))
	}
	if clientConversationID != "" {
		req.Header.Set("conversation_id", generateSessionUUID(isolateOpenAISessionID(apiKeyID, clientConversationID)))
	}
}
