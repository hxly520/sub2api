package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

var (
	upstreamErrorURLPattern      = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>，。；、)]+`)
	upstreamErrorHostPathPattern = regexp.MustCompile(
		`(?i)\b[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}(?::[0-9]{1,5})?/(?:api|v[0-9]+(?:beta)?|backend-api)(?:/[^\s"'<>，。；、)]*)?`,
	)
	upstreamErrorAPIPathPattern = regexp.MustCompile(
		`(?i)(?:\b(?:GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\s+)?/(?:api|v[0-9]+(?:beta)?|backend-api)(?:/[^\s"'<>，。；、)]*)?`,
	)
	upstreamErrorBearerPattern = regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	upstreamErrorKeyPattern    = regexp.MustCompile(`(?i)\b(?:sk|sk-proj|sk-ant|sess|rk|pk|ak|token|secret)[_-][A-Za-z0-9._~+/=-]{12,}\b`)
	upstreamErrorJWTPattern    = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
)

// sanitizeUpstreamErrorMessage is the internal/ops sanitizer. It preserves
// routing context for administrators while removing credentials and tokens.
func sanitizeUpstreamErrorMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}

	return logredact.RedactText(
		msg,
		"api_key",
		"apikey",
		"authorization",
		"cookie",
		"set_cookie",
		"session_token",
		"token",
		"secret",
		"private_key",
	)
}

// sanitizeClientUpstreamErrorMessage additionally removes upstream routing
// details and stack traces. Use this only at the client response boundary.
func sanitizeClientUpstreamErrorMessage(msg string) string {
	hadMessage := strings.TrimSpace(msg) != ""
	msg = trimUpstreamStackTrace(sanitizeUpstreamErrorMessage(msg))
	if msg == "" {
		if hadMessage {
			return "Upstream request failed"
		}
		return ""
	}

	msg = upstreamErrorBearerPattern.ReplaceAllString(msg, `${1}[redacted]`)
	msg = upstreamErrorKeyPattern.ReplaceAllString(msg, `[redacted]`)
	msg = upstreamErrorJWTPattern.ReplaceAllString(msg, `[redacted]`)
	msg = upstreamErrorURLPattern.ReplaceAllString(msg, `[upstream URL]`)
	msg = upstreamErrorHostPathPattern.ReplaceAllString(msg, `[upstream URL]`)
	msg = upstreamErrorAPIPathPattern.ReplaceAllString(msg, `[upstream path]`)
	msg = strings.TrimSpace(msg)
	if msg == "" && hadMessage {
		return "Upstream request failed"
	}
	return msg
}

// SanitizeClientUpstreamErrorMessage exposes the client-boundary sanitizer to
// handlers that render a final failover error after all accounts are exhausted.
func SanitizeClientUpstreamErrorMessage(msg string) string {
	return sanitizeClientUpstreamErrorMessage(msg)
}

// sanitizeUpstreamErrorResponseBody keeps the upstream JSON schema intact and
// sanitizes every string value. Non-JSON bodies are reduced to sanitized text.
func sanitizeUpstreamErrorResponseBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		sanitized := sanitizeClientUpstreamErrorMessage(string(body))
		if sanitized == "" {
			return []byte("Upstream request failed")
		}
		return []byte(sanitized)
	}

	sanitized, changed := sanitizeUpstreamErrorValue(payload)
	if !changed {
		return body
	}
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return body
	}
	return encoded
}

func sanitizeUpstreamErrorValue(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		sanitized := sanitizeClientUpstreamErrorMessage(typed)
		return sanitized, sanitized != typed
	case []any:
		changed := false
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i], changed = sanitizeUpstreamErrorChild(item, changed)
		}
		return out, changed
	case map[string]any:
		changed := false
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key], changed = sanitizeUpstreamErrorChild(item, changed)
		}
		return out, changed
	default:
		return value, false
	}
}

func sanitizeUpstreamErrorChild(value any, alreadyChanged bool) (any, bool) {
	sanitized, changed := sanitizeUpstreamErrorValue(value)
	return sanitized, alreadyChanged || changed
}

func trimUpstreamStackTrace(msg string) string {
	lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isUpstreamStackTraceLine(trimmed) {
			break
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "\n")
}

func isUpstreamStackTraceLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, "traceback (most recent call last):") ||
		strings.HasPrefix(lower, "stack trace") ||
		strings.HasPrefix(lower, "stacktrace") ||
		strings.HasPrefix(lower, "goroutine ") ||
		strings.HasPrefix(lower, "panic:") ||
		strings.HasPrefix(lower, "at ") ||
		strings.HasPrefix(lower, "file \"")
}
