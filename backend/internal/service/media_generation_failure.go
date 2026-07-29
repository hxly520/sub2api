package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// DefinitiveMediaGenerationError preserves a known terminal no-output outcome
// across client-facing status remapping and error passthrough rules.
type DefinitiveMediaGenerationError struct {
	cause error
}

func (e *DefinitiveMediaGenerationError) Error() string {
	if e == nil || e.cause == nil {
		return "media generation failed"
	}
	return e.cause.Error()
}

func (e *DefinitiveMediaGenerationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func MarkDefinitiveMediaGenerationFailure(err error) error {
	if err == nil || IsMarkedDefinitiveMediaGenerationFailure(err) {
		return err
	}
	return &DefinitiveMediaGenerationError{cause: err}
}

func IsMarkedDefinitiveMediaGenerationFailure(err error) bool {
	var marked *DefinitiveMediaGenerationError
	return errors.As(err, &marked)
}

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
