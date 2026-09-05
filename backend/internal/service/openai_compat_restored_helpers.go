package service

import "strings"

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
