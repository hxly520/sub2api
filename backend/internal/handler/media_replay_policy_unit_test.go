//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaReplayFailingUpstream struct {
	service.HTTPUpstream

	mu   sync.Mutex
	hits []int64
}

func (u *mediaReplayFailingUpstream) DoWithTLS(
	_ *http.Request,
	_ string,
	accountID int64,
	_ int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	u.mu.Lock()
	u.hits = append(u.hits, accountID)
	u.mu.Unlock()

	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"upstream unavailable"}}`)),
	}, nil
}

func (u *mediaReplayFailingUpstream) accountHits() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.hits...)
}

func TestGatewayResponsesMediaCreationDisablesAutomaticReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		body     string
		wantHits int
	}{
		{
			name:     "native image generation tool submits once",
			body:     `{"model":"claude-test","input":"draw a skyline","tools":[{"type":"image_generation"}],"stream":false}`,
			wantHits: 1,
		},
		{
			name:     "ordinary text preserves account failover",
			body:     `{"model":"claude-test","input":"write a summary","stream":false}`,
			wantHits: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, upstream, apiKey, group, cleanup := newMediaReplayGatewayHandler(t)
			defer cleanup()

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, group))
			c.Request = req
			c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
			c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})

			h.Responses(c)

			require.Equal(t, http.StatusInternalServerError, recorder.Code, recorder.Body.String())
			require.Len(t, upstream.accountHits(), tt.wantHits)
		})
	}
}

func TestOpenAIChatMediaCreationDisablesAutomaticReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		body     string
		wantCode int
		wantHits []int64
	}{
		{
			name:     "native image generation tool submits once",
			body:     `{"model":"grok","messages":[{"role":"user","content":"draw a skyline"}],"tools":[{"type":"image_generation"}],"stream":false}`,
			wantCode: http.StatusTooManyRequests,
			wantHits: []int64{801},
		},
		{
			name:     "ordinary text preserves account failover",
			body:     `{"model":"grok","messages":[{"role":"user","content":"write a summary"}],"stream":false}`,
			wantCode: http.StatusOK,
			wantHits: []int64{801, 802},
		},
		{
			name:     "passive image namespace preserves text failover",
			body:     `{"model":"grok","messages":[{"role":"user","content":"write a summary"}],"tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}],"tool_choice":"auto","stream":false}`,
			wantCode: http.StatusOK,
			wantHits: []int64{801, 802},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, upstream, router, cleanup := newGrokCredentialFailoverHandler(t, "first_429")
			defer cleanup()

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(recorder, req)

			require.Equal(t, tt.wantCode, recorder.Code, recorder.Body.String())
			require.Equal(t, tt.wantHits, upstream.accountHits())
		})
	}
}

func newMediaReplayGatewayHandler(t *testing.T) (
	*GatewayHandler,
	*mediaReplayFailingUpstream,
	*service.APIKey,
	*service.Group,
	func(),
) {
	t.Helper()

	groupID := int64(9200)
	group := &service.Group{
		ID:                   groupID,
		Hydrated:             true,
		Platform:             service.PlatformAnthropic,
		Status:               service.StatusActive,
		AllowImageGeneration: true,
	}
	accounts := []*service.Account{
		{
			ID: 9201, Name: "first", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 1,
			Credentials:   map[string]any{"api_key": "first-key"},
			AccountGroups: []service.AccountGroup{{AccountID: 9201, GroupID: groupID}},
		},
		{
			ID: 9202, Name: "second", Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: 2,
			Credentials:   map[string]any{"api_key": "second-key"},
			AccountGroups: []service.AccountGroup{{AccountID: 9202, GroupID: groupID}},
		},
	}
	upstream := &mediaReplayFailingUpstream{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.MaxAccountSwitches = 1
	concurrencyService := service.NewConcurrencyService(&fakeConcurrencyCache{})
	schedulerSnapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: accounts}, nil, nil, nil, nil)
	gatewayService := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, nil, nil, nil, nil, nil, nil, cfg,
		schedulerSnapshot, concurrencyService, nil, nil, nil, nil, upstream, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	h := &GatewayHandler{
		gatewayService:      gatewayService,
		billingCacheService: billingCacheService,
		concurrencyHelper:   NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, 0),
		maxAccountSwitches:  1,
		cfg:                 cfg,
	}
	apiKey := &service.APIKey{
		ID: 9203, UserID: 9204, GroupID: &groupID, Group: group, Status: service.StatusActive,
		User: &service.User{ID: 9204, Status: service.StatusActive, Concurrency: 10, Balance: 100},
	}

	return h, upstream, apiKey, group, billingCacheService.Stop
}
