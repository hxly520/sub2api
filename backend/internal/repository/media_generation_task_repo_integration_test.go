//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestMediaGenerationFinalizationConcurrentPollsAcquireOneLease(t *testing.T) {
	ctx := context.Background()
	repo, apiKeyID, taskID := createIntegrationMediaGenerationTask(t, service.MediaGenerationStatusCompleted)

	const pollers = 16
	type result struct {
		token    string
		acquired bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, pollers)
	var wg sync.WaitGroup
	for i := 0; i < pollers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			token := fmt.Sprintf("poller-%d-%s", index, uuid.NewString())
			acquired, err := repo.TryAcquireMediaGenerationFinalization(ctx, apiKeyID, taskID, token, time.Now().UTC().Add(time.Minute))
			results <- result{token: token, acquired: acquired, err: err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	acquiredCount := 0
	winnerToken := ""
	for item := range results {
		require.NoError(t, item.err)
		if item.acquired {
			acquiredCount++
			winnerToken = item.token
		}
	}
	require.Equal(t, 1, acquiredCount)
	require.NotEmpty(t, winnerToken)

	completed, err := repo.CompleteMediaGenerationFinalization(ctx, apiKeyID, taskID, winnerToken)
	require.NoError(t, err)
	require.True(t, completed)

	completed, err = repo.CompleteMediaGenerationFinalization(ctx, apiKeyID, taskID, winnerToken)
	require.NoError(t, err)
	require.False(t, completed)
}

func TestMediaGenerationFinalizationRestartRecoveryBillsOnce(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := &usageBillingRepository{db: integrationDB}

	user := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("media-recovery-%s@example.com", uuid.NewString()),
		PasswordHash: "hash",
		Balance:      100,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-media-recovery-" + uuid.NewString(),
		Name:   "media-recovery",
	})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "media-recovery-" + uuid.NewString(),
		Type: service.AccountTypeAPIKey,
	})
	taskID := "video-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM media_generation_tasks WHERE api_key_id = $1", apiKey.ID)
	})
	require.NoError(t, repo.CreateMediaGenerationTask(ctx, &service.MediaGenerationTask{
		TaskID:             taskID,
		PublicTaskID:       taskID,
		UpstreamTaskID:     "provider-" + uuid.NewString(),
		APIKeyID:           apiKey.ID,
		UserID:             user.ID,
		AccountID:          account.ID,
		Model:              "grok-video",
		RequestedModel:     "grok-video",
		UpstreamModel:      "grok-video",
		RequestFingerprint: service.HashMediaGenerationRequestFingerprint("/v1/video/generations", []byte(`{"prompt":"test"}`)),
		RequestPayloadHash: service.HashUsageRequestPayload([]byte(`{"prompt":"test"}`)),
		Status:             service.MediaGenerationStatusCompleted,
		DurationSeconds:    15,
		Resolution:         service.VideoBillingResolution720P,
		MediaType:          "video",
	}))

	// Persisting the terminal response may happen before asynchronous usage
	// billing. finalized_at must not prevent a later process from recovering it.
	require.NoError(t, repo.MarkMediaGenerationTaskTerminal(ctx, apiKey.ID, taskID, service.MediaGenerationStatusCompleted, ""))
	firstToken := "before-restart-" + uuid.NewString()
	acquired, err := repo.TryAcquireMediaGenerationFinalization(ctx, apiKey.ID, taskID, firstToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, acquired)

	billing := &service.UsageBillingCommand{
		RequestID:       taskID,
		APIKeyID:        apiKey.ID,
		UserID:          user.ID,
		AccountID:       account.ID,
		AccountType:     account.Type,
		Model:           "grok-video",
		MediaType:       "video",
		BalanceCost:     2.5,
		APIKeyQuotaCost: 2.5,
	}
	firstBilling, err := repo.Apply(ctx, billing)
	require.NoError(t, err)
	require.True(t, firstBilling.Applied)

	// Simulate a process exit after billing committed but before the task row
	// marked usage_recorded_at. A new process can take the expired lease.
	_, err = integrationDB.ExecContext(ctx, `
		UPDATE media_generation_tasks
		SET finalization_lease_until = NOW() - INTERVAL '1 second'
		WHERE api_key_id = $1 AND public_task_id = $2
	`, apiKey.ID, taskID)
	require.NoError(t, err)

	restartedRepo := &usageBillingRepository{db: integrationDB}
	recoveryToken := "after-restart-" + uuid.NewString()
	acquired, err = restartedRepo.TryAcquireMediaGenerationFinalization(ctx, apiKey.ID, taskID, recoveryToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	require.True(t, acquired)

	retriedBilling, err := restartedRepo.Apply(ctx, billing)
	require.NoError(t, err)
	require.False(t, retriedBilling.Applied)
	completed, err := restartedRepo.CompleteMediaGenerationFinalization(ctx, apiKey.ID, taskID, recoveryToken)
	require.NoError(t, err)
	require.True(t, completed)

	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance FROM users WHERE id = $1", user.ID).Scan(&balance))
	require.InDelta(t, 97.5, balance, 0.000001)
	var usageRecorded bool
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		SELECT usage_recorded_at IS NOT NULL
		FROM media_generation_tasks
		WHERE api_key_id = $1 AND public_task_id = $2
	`, apiKey.ID, taskID).Scan(&usageRecorded))
	require.True(t, usageRecorded)
}

func TestMediaGenerationFailedTaskCannotAcquireFinalizationLease(t *testing.T) {
	ctx := context.Background()
	repo, apiKeyID, taskID := createIntegrationMediaGenerationTask(t, service.MediaGenerationStatusFailed)

	acquired, err := repo.TryAcquireMediaGenerationFinalization(
		ctx,
		apiKeyID,
		taskID,
		"failed-task-"+uuid.NewString(),
		time.Now().UTC().Add(time.Minute),
	)
	require.NoError(t, err)
	require.False(t, acquired)
}

func TestMediaGenerationTaskFirstTerminalStateWinsConcurrentPolling(t *testing.T) {
	ctx := context.Background()

	t.Run("late pending cannot replace success", func(t *testing.T) {
		repo, apiKeyID, taskID := createIntegrationMediaGenerationTask(t, service.MediaGenerationStatusPending)
		require.NoError(t, repo.UpdateMediaGenerationTaskResponse(
			ctx, apiKeyID, taskID, 200, "application/json",
			`{"status":"completed","video_url":"https://media.example/success.mp4"}`,
			"https://media.example/success.mp4", service.MediaGenerationStatusCompleted, 8,
		))
		require.NoError(t, repo.UpdateMediaGenerationTaskResponse(
			ctx, apiKeyID, taskID, 200, "application/json",
			`{"status":"pending"}`, "", service.MediaGenerationStatusPending, 0,
		))

		task, err := repo.GetMediaGenerationTaskByTaskID(ctx, apiKeyID, taskID)
		require.NoError(t, err)
		require.Equal(t, service.MediaGenerationStatusCompleted, task.Status)
		require.Contains(t, task.ResponseBody, "success.mp4")
		require.Equal(t, "https://media.example/success.mp4", task.UpstreamResultURL)
	})

	t.Run("late success cannot replace failure or acquire billing lease", func(t *testing.T) {
		repo, apiKeyID, taskID := createIntegrationMediaGenerationTask(t, service.MediaGenerationStatusPending)
		require.NoError(t, repo.UpdateMediaGenerationTaskResponse(
			ctx, apiKeyID, taskID, 200, "application/json",
			`{"status":"failed"}`, "", service.MediaGenerationStatusFailed, 0,
		))
		require.NoError(t, repo.UpdateMediaGenerationTaskResponse(
			ctx, apiKeyID, taskID, 200, "application/json",
			`{"status":"completed","video_url":"https://media.example/late.mp4"}`,
			"https://media.example/late.mp4", service.MediaGenerationStatusCompleted, 8,
		))

		task, err := repo.GetMediaGenerationTaskByTaskID(ctx, apiKeyID, taskID)
		require.NoError(t, err)
		require.Equal(t, service.MediaGenerationStatusFailed, task.Status)
		require.NotContains(t, task.ResponseBody, "late.mp4")
		require.Empty(t, task.UpstreamResultURL)
		acquired, err := repo.TryAcquireMediaGenerationFinalization(
			ctx, apiKeyID, taskID, "late-success-"+uuid.NewString(), time.Now().UTC().Add(time.Minute),
		)
		require.NoError(t, err)
		require.False(t, acquired)
	})
}

func createIntegrationMediaGenerationTask(t *testing.T, status string) (*usageBillingRepository, int64, string) {
	t.Helper()
	ctx := context.Background()
	repo := &usageBillingRepository{db: integrationDB}
	apiKeyID := time.Now().UnixNano()
	taskID := "video-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM media_generation_tasks WHERE api_key_id = $1", apiKeyID)
	})
	require.NoError(t, repo.CreateMediaGenerationTask(ctx, &service.MediaGenerationTask{
		TaskID:             taskID,
		PublicTaskID:       taskID,
		UpstreamTaskID:     "provider-" + uuid.NewString(),
		APIKeyID:           apiKeyID,
		UserID:             apiKeyID,
		AccountID:          apiKeyID,
		Model:              "seedance-2.0-fast-720p",
		RequestedModel:     "seedance-2.0-fast-720p",
		UpstreamModel:      "seedance-2.0-fast-720p",
		RequestFingerprint: service.HashMediaGenerationRequestFingerprint("/v1/videos", []byte(`{"prompt":"test"}`)),
		Status:             status,
		DurationSeconds:    8,
		Resolution:         service.VideoBillingResolution720P,
		MediaType:          "video",
	}))
	return repo, apiKeyID, taskID
}
