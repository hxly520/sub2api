package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeMediaGenerationStatusProviderVariants(t *testing.T) {
	tests := map[string]string{
		"SUCCESS":           MediaGenerationStatusCompleted,
		"completed":         MediaGenerationStatusCompleted,
		"FAILURE":           MediaGenerationStatusFailed,
		"GENERATION_FAILED": MediaGenerationStatusFailed,
		"PROMPT_BLOCKED":    MediaGenerationStatusFailed,
		"rejected":          MediaGenerationStatusFailed,
		"TIMEOUT":           MediaGenerationStatusExpired,
		"queued":            MediaGenerationStatusPending,
		"IN_QUEUE":          MediaGenerationStatusPending,
		"submitted":         MediaGenerationStatusCreating,
		"scheduled":         MediaGenerationStatusPending,
		"IN_PROGRESS":       MediaGenerationStatusRunning,
		"processing":        MediaGenerationStatusRunning,
	}
	for input, expected := range tests {
		require.Equal(t, expected, NormalizeMediaGenerationStatus(input), input)
	}
}
