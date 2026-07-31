package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

func TestRawChatClientDisconnectBoundsPermanentlyBlockedUpstreamRead(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestCtx)

	body := newBlockingDrainReadCloser()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       body,
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{
		Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1},
	}}

	done := make(chan error, 1)
	go func() {
		_, err := svc.streamRawChatCompletions(
			requestCtx,
			c,
			resp,
			&Account{ID: 1},
			"model",
			"model",
			"model",
			nil,
			nil,
			time.Now(),
			1,
			2,
		)
		done <- err
	}()

	// Let Scanner enter the permanently blocked Read before disconnecting.
	time.Sleep(20 * time.Millisecond)
	started := time.Now()
	cancelRequest()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked upstream ended without an explicit incomplete-stream error")
		}
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Fatalf("blocked upstream released too slowly after disconnect: %s", elapsed)
		}
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("raw chat handler remained blocked after the disconnect drain bound")
	}

	select {
	case <-body.closed:
	default:
		t.Fatal("disconnect drain guard did not close the upstream body")
	}
}
