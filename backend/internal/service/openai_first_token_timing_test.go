package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type firstTokenTimingUpstream struct {
	mu        sync.Mutex
	calls     int
	responses []*http.Response
	errors    []error
	delay     time.Duration
	observe   func(*http.Request)
}

func (u *firstTokenTimingUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	call := u.calls
	u.calls++
	u.mu.Unlock()

	if u.observe != nil {
		u.observe(req)
	}
	if u.delay > 0 {
		time.Sleep(u.delay)
	}
	var resp *http.Response
	if call < len(u.responses) {
		resp = u.responses[call]
	}
	var err error
	if call < len(u.errors) {
		err = u.errors[call]
	}
	return resp, err
}

func (u *firstTokenTimingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func (u *firstTokenTimingUpstream) callCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls
}

func newFirstTokenTimingGinContext(ctx context.Context) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	return c
}

func TestDoOpenAIUpstreamWithFirstTokenTimingDisabledIsTransparent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	detachedCtx := context.WithoutCancel(ctx)

	upstream := &firstTokenTimingUpstream{
		responses: []*http.Response{{StatusCode: http.StatusNoContent}},
		observe: func(req *http.Request) {
			require.True(t, req.Context() == detachedCtx)
			require.NoError(t, req.Context().Err())
		},
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := newFirstTokenTimingGinContext(ctx)
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", nil).WithContext(detachedCtx)

	resp, err := svc.doOpenAIUpstreamWithFirstTokenTiming(ctx, c, req, "", &Account{ID: 1})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, 1, upstream.callCount())
	require.Nil(t, openAIFirstTokenAccepted(c))
}

func TestDoOpenAIUpstreamWithFirstTokenTimingRecordsSuccessfulHeaderWithoutChangingRequestContext(t *testing.T) {
	markedCtx, cancel := context.WithCancel(WithOpenAIFastFirstTokenTiming(context.Background()))
	cancel()
	detachedCtx := context.WithoutCancel(markedCtx)

	upstream := &firstTokenTimingUpstream{
		responses: []*http.Response{{StatusCode: http.StatusOK}},
		delay:     15 * time.Millisecond,
		observe: func(req *http.Request) {
			require.True(t, req.Context() == detachedCtx)
			require.NoError(t, req.Context().Err())
		},
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := newFirstTokenTimingGinContext(markedCtx)
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", nil).WithContext(detachedCtx)

	resp, err := svc.doOpenAIUpstreamWithFirstTokenTiming(markedCtx, c, req, "", &Account{ID: 1})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 1, upstream.callCount())
	require.NotNil(t, openAIFirstTokenAccepted(c))
	require.GreaterOrEqual(t, *openAIFirstTokenAccepted(c), 10)
}

func TestDoOpenAIUpstreamWithFirstTokenTimingFailedAttemptDoesNotLeakOrReplay(t *testing.T) {
	ctx := WithOpenAIFastFirstTokenTiming(context.Background())
	upstream := &firstTokenTimingUpstream{
		responses: []*http.Response{
			{StatusCode: http.StatusServiceUnavailable},
			{StatusCode: http.StatusOK},
		},
	}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := newFirstTokenTimingGinContext(ctx)
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", nil).WithContext(ctx)
	c.Set(openAIFirstTokenAcceptedContextKey, 999)

	resp, err := svc.doOpenAIUpstreamWithFirstTokenTiming(ctx, c, req, "", &Account{ID: 1})
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.Equal(t, 1, upstream.callCount(), "the timing wrapper must not replay a failed request")
	require.Nil(t, openAIFirstTokenAccepted(c), "a failed attempt must clear an earlier accepted sample")

	resp, err = svc.doOpenAIUpstreamWithFirstTokenTiming(ctx, c, req, "", &Account{ID: 1})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 2, upstream.callCount(), "each explicit attempt must issue exactly one upstream request")
	require.NotNil(t, openAIFirstTokenAccepted(c))
}

func TestDoOpenAIUpstreamWithFirstTokenTimingTransportErrorDoesNotLeaveSample(t *testing.T) {
	ctx := WithOpenAIFastFirstTokenTiming(context.Background())
	upstreamErr := errors.New("dial failed")
	upstream := &firstTokenTimingUpstream{errors: []error{upstreamErr}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	c := newFirstTokenTimingGinContext(ctx)
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/responses", nil).WithContext(ctx)
	c.Set(openAIFirstTokenAcceptedContextKey, 999)

	resp, err := svc.doOpenAIUpstreamWithFirstTokenTiming(ctx, c, req, "", &Account{ID: 1})
	require.Nil(t, resp)
	require.ErrorIs(t, err, upstreamErr)
	require.Equal(t, 1, upstream.callCount())
	require.Nil(t, openAIFirstTokenAccepted(c))
}
