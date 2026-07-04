package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIFailoverFirstTokenMsUsesSuccessfulAttemptStart(t *testing.T) {
	attemptStartedAt := time.Now().Add(-2 * time.Second)
	firstTokenAt := attemptStartedAt.Add(1200 * time.Millisecond)
	original := 6200
	result := &service.OpenAIForwardResult{
		FirstTokenMs: &original,
		FirstTokenAt: &firstTokenAt,
	}

	normalizeOpenAIFailoverFirstTokenMs(nil, nil, result, 1, attemptStartedAt, "responses")

	require.NotNil(t, result.FirstTokenMs)
	require.Equal(t, 1200, *result.FirstTokenMs)
}

func TestNormalizeOpenAIFailoverFirstTokenMsSkipsRequestsWithoutSwitch(t *testing.T) {
	attemptStartedAt := time.Now().Add(-2 * time.Second)
	firstTokenAt := attemptStartedAt.Add(1200 * time.Millisecond)
	original := 6200
	result := &service.OpenAIForwardResult{
		FirstTokenMs: &original,
		FirstTokenAt: &firstTokenAt,
	}

	normalizeOpenAIFailoverFirstTokenMs(nil, nil, result, 0, attemptStartedAt, "responses")

	require.NotNil(t, result.FirstTokenMs)
	require.Equal(t, 6200, *result.FirstTokenMs)
}
