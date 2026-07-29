package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsDefinitiveMediaGenerationFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{name: "explicit forbidden", statusCode: http.StatusForbidden, body: `{"message":"insufficient balance"}`, want: true},
		{name: "explicit rate limit", statusCode: http.StatusTooManyRequests, want: true},
		{name: "provider queue cancellation", statusCode: http.StatusBadGateway, body: `{"message":"管理员取消：生图池排队任务已清空"}`, want: true},
		{name: "structured failed terminal", statusCode: http.StatusBadGateway, body: `{"task":{"status":"failed"}}`, want: true},
		{name: "generic bad gateway", statusCode: http.StatusBadGateway, body: `{"message":"service temporarily unavailable"}`},
		{name: "gateway timeout", statusCode: http.StatusGatewayTimeout},
		{name: "transport error", statusCode: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsDefinitiveMediaGenerationFailure(tt.statusCode, []byte(tt.body)))
		})
	}
}

func TestMarkedDefinitiveMediaGenerationFailurePreservesCause(t *testing.T) {
	cause := errors.New("upstream rejected media generation")
	marked := MarkDefinitiveMediaGenerationFailure(cause)
	if !IsMarkedDefinitiveMediaGenerationFailure(marked) {
		t.Fatal("marked error was not recognized")
	}
	if !errors.Is(marked, cause) {
		t.Fatal("marked error must preserve its cause")
	}
	if again := MarkDefinitiveMediaGenerationFailure(marked); again != marked {
		t.Fatal("marking must be idempotent")
	}
}

func TestOpenAIImageErrorsRetainOriginalMediaOutcome(t *testing.T) {
	incomplete := openAIImagesIncompleteUpstreamError(gjson.Parse(`{"id":"response-1","incomplete_details":{"reason":"max_output_tokens"}}`))
	if incomplete == nil || !incomplete.MediaOutcomeKnownFailed {
		t.Fatal("terminal response.incomplete must be refundable even when presented as 502")
	}

	rejected := openAIImagesUpstreamErrorFromHTTP(http.StatusBadRequest, nil, []byte(`{"error":{"message":"invalid size"}}`))
	if !rejected.MediaOutcomeKnownFailed {
		t.Fatal("original upstream 4xx must remain a known failure")
	}

	ambiguous := openAIImagesUpstreamErrorFromHTTP(http.StatusBadGateway, nil, []byte(`{"error":{"message":"gateway timeout"}}`))
	if ambiguous.MediaOutcomeKnownFailed {
		t.Fatal("generic upstream 5xx must remain ambiguous")
	}
}
