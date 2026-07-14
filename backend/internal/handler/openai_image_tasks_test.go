package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type openAIImageTaskRepoStub struct {
	service.UsageBillingRepository

	idempotencyTasks []*service.MediaGenerationTask
	lookupCalls      int
	lockCalls        int
	releaseCalls     int
}

func (r *openAIImageTaskRepoStub) GetMediaGenerationTaskByTaskID(context.Context, int64, string) (*service.MediaGenerationTask, error) {
	return nil, sql.ErrNoRows
}

func (r *openAIImageTaskRepoStub) GetMediaGenerationTaskByIdempotency(context.Context, int64, string) (*service.MediaGenerationTask, error) {
	index := r.lookupCalls
	r.lookupCalls++
	if index >= len(r.idempotencyTasks) || r.idempotencyTasks[index] == nil {
		return nil, sql.ErrNoRows
	}
	return r.idempotencyTasks[index], nil
}

func (r *openAIImageTaskRepoStub) AcquireMediaGenerationIdempotencyLock(context.Context, int64, string) (func(), error) {
	r.lockCalls++
	return func() { r.releaseCalls++ }, nil
}

func (r *openAIImageTaskRepoStub) CreateMediaGenerationTask(context.Context, *service.MediaGenerationTask) error {
	return nil
}

func (r *openAIImageTaskRepoStub) UpdateMediaGenerationTaskResponse(context.Context, int64, string, int, string, string, string, string, int) error {
	return nil
}

func (r *openAIImageTaskRepoStub) MarkMediaGenerationTaskTerminal(context.Context, int64, string, string, string) error {
	return nil
}

func (r *openAIImageTaskRepoStub) TryAcquireMediaGenerationFinalization(context.Context, int64, string, string, time.Time) (bool, error) {
	return false, nil
}

func (r *openAIImageTaskRepoStub) CompleteMediaGenerationFinalization(context.Context, int64, string, string) (bool, error) {
	return false, nil
}

func (r *openAIImageTaskRepoStub) ReleaseMediaGenerationFinalization(context.Context, int64, string, string, string) error {
	return nil
}

func newOpenAIImageTaskGatewayService(repo service.UsageBillingRepository) *service.OpenAIGatewayService {
	return service.NewOpenAIGatewayService(
		nil,
		nil,
		repo,
		nil,
		nil,
		nil,
		nil,
		&config.Config{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func newOpenAIImageAsyncCreationTestContext(t *testing.T, idempotencyKey string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Idempotency-Key", idempotencyKey)
	return c, recorder
}

func TestPrepareOpenAIImageAsyncCreationLocksBeforeResumingIntent(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","async":true}`)
	fingerprint := service.HashMediaGenerationRequestFingerprint("/v1/images/generations", body)
	creating := &service.MediaGenerationTask{
		TaskID:             "image-public-1",
		PublicTaskID:       "image-public-1",
		APIKeyID:           7,
		MediaType:          "image",
		RequestFingerprint: fingerprint,
		Status:             service.MediaGenerationStatusCreating,
	}
	repo := &openAIImageTaskRepoStub{idempotencyTasks: []*service.MediaGenerationTask{creating, creating}}
	h := &OpenAIGatewayHandler{gatewayService: newOpenAIImageTaskGatewayService(repo)}
	c, _ := newOpenAIImageAsyncCreationTestContext(t, "image-idempotency-key")

	state, handled := h.prepareOpenAIImageAsyncCreation(
		c,
		zap.NewNop(),
		&service.APIKey{ID: 7},
		body,
		&service.OpenAIImagesRequest{Endpoint: "/v1/images/generations"},
	)

	require.False(t, handled)
	require.NotNil(t, state)
	require.Equal(t, "image-public-1", state.publicTaskID)
	require.Same(t, creating, state.resumedTask)
	require.Equal(t, 2, repo.lookupCalls)
	require.Equal(t, 1, repo.lockCalls)
	require.Zero(t, repo.releaseCalls)
	state.close()
	require.Equal(t, 1, repo.releaseCalls)
}

func TestPrepareOpenAIImageAsyncCreationRechecksIntentAfterLock(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","prompt":"draw a cat","async":true}`)
	fingerprint := service.HashMediaGenerationRequestFingerprint("/v1/images/generations", body)
	creating := &service.MediaGenerationTask{
		TaskID:             "image-public-2",
		PublicTaskID:       "image-public-2",
		APIKeyID:           7,
		MediaType:          "image",
		RequestFingerprint: fingerprint,
		Status:             service.MediaGenerationStatusCreating,
	}
	submitted := &service.MediaGenerationTask{
		TaskID:              "image-public-2",
		PublicTaskID:        "image-public-2",
		UpstreamTaskID:      "provider-task-2",
		APIKeyID:            7,
		MediaType:           "image",
		RequestFingerprint:  fingerprint,
		Status:              service.MediaGenerationStatusPending,
		ResponseStatus:      http.StatusAccepted,
		ResponseContentType: "application/json",
		ResponseBody:        `{"id":"provider-task-2","status":"queued"}`,
	}
	repo := &openAIImageTaskRepoStub{idempotencyTasks: []*service.MediaGenerationTask{creating, submitted}}
	h := &OpenAIGatewayHandler{gatewayService: newOpenAIImageTaskGatewayService(repo)}
	c, recorder := newOpenAIImageAsyncCreationTestContext(t, "image-idempotency-key")

	state, handled := h.prepareOpenAIImageAsyncCreation(
		c,
		zap.NewNop(),
		&service.APIKey{ID: 7},
		body,
		&service.OpenAIImagesRequest{Endpoint: "/v1/images/generations"},
	)

	require.True(t, handled)
	require.Nil(t, state)
	require.Equal(t, 2, repo.lookupCalls)
	require.Equal(t, 1, repo.lockCalls)
	require.Equal(t, 1, repo.releaseCalls)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.JSONEq(t, `{"id":"image-public-2","status":"queued"}`, recorder.Body.String())
}

var _ service.MediaGenerationTaskRepository = (*openAIImageTaskRepoStub)(nil)
