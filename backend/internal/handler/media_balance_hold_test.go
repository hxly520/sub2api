//go:build unit

package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestShouldRetainMediaBalanceHoldAfterDispatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", want: false},
		{name: "explicit user rejection", err: &service.OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, Message: "invalid size"}},
		{name: "known failover rejection", err: &service.UpstreamFailoverError{StatusCode: http.StatusForbidden, MediaOutcomeKnownFailed: true}},
		{name: "known provider cancellation", err: &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway, MediaOutcomeKnownFailed: true}},
		{name: "generic bad gateway", err: &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway}, want: true},
		{name: "transport failure", err: errors.New("connection reset"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldRetainMediaBalanceHoldAfterDispatch(tt.err))
		})
	}
}
