package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestTryAcquireMediaGenerationFinalizationRecoversPersistedSuccess(t *testing.T) {
	sql := strings.ToLower(tryAcquireMediaGenerationFinalizationSQL)
	require.Contains(t, sql, "usage_recorded_at is null")
	require.NotContains(t, sql, "finalized_at is null")
	require.Contains(t, sql, "lower(btrim(status)) in ('complete', 'completed', 'success', 'succeeded', 'done')")
	require.Contains(t, sql, "finalization_lease_until is null or finalization_lease_until <= now()")
}

func TestCreateMediaGenerationTaskPreservesInitialPricingSnapshotOnConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)INSERT INTO media_generation_tasks .*billing_unit_price.*ON CONFLICT .*billing_unit_price = COALESCE\(media_generation_tasks\.billing_unit_price, EXCLUDED\.billing_unit_price\).*billing_rate_multiplier = COALESCE\(media_generation_tasks\.billing_rate_multiplier, EXCLUDED\.billing_rate_multiplier\)`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	unitPrice := 0.55
	rateMultiplier := 0.8
	repo := &usageBillingRepository{db: db}
	err = repo.CreateMediaGenerationTask(context.Background(), &service.MediaGenerationTask{
		TaskID:                "video-public-1",
		PublicTaskID:          "video-public-1",
		APIKeyID:              1,
		UserID:                2,
		AccountID:             3,
		Model:                 "seedance-2.0-fast-720p",
		RequestedModel:        "seedance-2.0-fast-720p",
		UpstreamModel:         "seedance-2.0-fast-720p",
		RequestFingerprint:    "fingerprint",
		Status:                service.MediaGenerationStatusCreating,
		BillingMode:           string(service.BillingModeVideo),
		BillingUnitPrice:      &unitPrice,
		BillingRateMultiplier: &rateMultiplier,
		MediaType:             "video",
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMediaGenerationTaskScansPricingSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	columns := []string{
		"id", "task_id", "public_task_id", "upstream_task_id", "api_key_id", "user_id", "account_id",
		"group_id", "subscription_id", "model", "requested_model", "upstream_model", "endpoint",
		"inbound_endpoint", "upstream_endpoint", "channel_id", "channel_mapped_model",
		"billing_model_source", "model_mapping_chain", "request_fingerprint",
		"request_payload_hash", "idempotency_key_hash", "response_status",
		"response_content_type", "response_body", "upstream_result_url", "status", "duration_seconds",
		"resolution", "size_tier", "billing_mode", "billing_unit_price", "billing_rate_multiplier",
		"media_type", "finalized_at", "finalization_lease_token", "finalization_lease_until",
		"usage_recorded_at", "finalization_error", "created_at", "updated_at",
	}
	rows := sqlmock.NewRows(columns).AddRow(
		1, "video-public-1", "video-public-1", "provider-1", 10, 20, 30,
		40, nil, "seedance-2.0-fast-720p", "seedance-2.0-fast-720p", "seedance-2.0-fast-720p", "/v1/videos",
		"/v1/videos", "/v1/videos", 50, "seedance-2.0-fast-720p",
		service.BillingModelSourceChannelMapped, "[]", "fingerprint",
		"payload", "idempotency", 202,
		"application/json", `{"status":"pending"}`, nil, service.MediaGenerationStatusPending, 6,
		service.VideoBillingResolution720P, "", service.BillingModeVideo, 0.55, 0.8,
		"video", nil, nil, nil,
		nil, nil, now, now,
	)
	mock.ExpectQuery(`(?s)SELECT .*billing_unit_price, billing_rate_multiplier.*WHERE api_key_id = \$1 AND public_task_id = \$2`).
		WithArgs(int64(10), "video-public-1").
		WillReturnRows(rows)

	repo := &usageBillingRepository{db: db}
	task, err := repo.GetMediaGenerationTaskByTaskID(context.Background(), 10, "video-public-1")
	require.NoError(t, err)
	require.NotNil(t, task)
	require.NotNil(t, task.BillingUnitPrice)
	require.NotNil(t, task.BillingRateMultiplier)
	require.InDelta(t, 0.55, *task.BillingUnitPrice, 1e-12)
	require.InDelta(t, 0.8, *task.BillingRateMultiplier, 1e-12)
	require.Equal(t, service.BillingModeVideo, task.PricingSnapshot().Mode)
	require.NoError(t, mock.ExpectationsWereMet())
}
