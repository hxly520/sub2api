package service

import (
	"encoding/json"
	"net/http"
	"strings"
)

// IsDefinitiveMediaGenerationFailure reports whether an upstream response
// proves that media creation was rejected or reached a failed terminal state.
// It is intentionally separate from automatic replay eligibility: an explicit
// cancellation may be refundable while still being unsafe to replay.
func IsDefinitiveMediaGenerationFailure(statusCode int, responseBody []byte) bool {
	switch statusCode {
	case http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusGone,
		http.StatusUnprocessableEntity,
		http.StatusTooManyRequests:
		return true
	}

	text := strings.ToLower(strings.TrimSpace(string(responseBody)))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"生图池排队任务已清空",
		"排队任务已清空",
		"管理员取消",
		"administratively cancelled",
		"administratively canceled",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}

	var payload any
	if json.Unmarshal(responseBody, &payload) != nil {
		return false
	}
	return containsDefinitiveMediaFailure(payload)
}

func containsDefinitiveMediaFailure(value any) bool {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if containsDefinitiveMediaFailure(item) {
				return true
			}
		}
	case map[string]any:
		for key, item := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if text, ok := item.(string); ok {
				normalizedValue := strings.ToLower(strings.TrimSpace(text))
				switch normalizedKey {
				case "status", "state":
					if normalizedValue == "failed" || normalizedValue == "cancelled" || normalizedValue == "canceled" || normalizedValue == "rejected" {
						return true
					}
				case "code", "type":
					if strings.Contains(normalizedValue, "cancelled") || strings.Contains(normalizedValue, "canceled") || strings.Contains(normalizedValue, "queue_cleared") {
						return true
					}
				}
			}
			if containsDefinitiveMediaFailure(item) {
				return true
			}
		}
	}
	return false
}
