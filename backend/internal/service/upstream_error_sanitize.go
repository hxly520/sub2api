package service

import (
	"regexp"
	"strings"
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
