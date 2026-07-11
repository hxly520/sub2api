package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *usageBillingRepository) GetMediaGenerationTaskByTaskID(ctx context.Context, apiKeyID int64, taskID string) (*service.MediaGenerationTask, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, sql.ErrNoRows
	}
	// Client lookups are public-ID only. Migration 174 backfills every legacy
	// row with a public_task_id, so accepting task_id here would keep historical
	// upstream provider IDs usable as client-facing lookup keys.
	return scanMediaGenerationTask(r.db.QueryRowContext(ctx, mediaGenerationTaskSelectSQL+`
		WHERE api_key_id = $1 AND public_task_id = $2
	`, apiKeyID, taskID))
}

func (r *usageBillingRepository) GetMediaGenerationTaskByIdempotency(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (*service.MediaGenerationTask, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	idempotencyKeyHash = strings.TrimSpace(idempotencyKeyHash)
	if idempotencyKeyHash == "" {
		return nil, sql.ErrNoRows
	}
	return scanMediaGenerationTask(r.db.QueryRowContext(ctx, mediaGenerationTaskSelectSQL+`
		WHERE api_key_id = $1 AND idempotency_key_hash = $2
	`, apiKeyID, idempotencyKeyHash))
}

func (r *usageBillingRepository) AcquireMediaGenerationIdempotencyLock(ctx context.Context, apiKeyID int64, idempotencyKeyHash string) (func(), error) {
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	idempotencyKeyHash = strings.TrimSpace(idempotencyKeyHash)
	if idempotencyKeyHash == "" {
		return func() {}, nil
	}
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	lockKey := "media_generation_tasks:idempotency"
	lockSeed := strings.Join([]string{strconv.FormatInt(apiKeyID, 10), idempotencyKeyHash}, ":")
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext($1), hashtext($2))`, lockKey, lockSeed); err != nil {
		_ = conn.Close()
		return nil, err
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(releaseCtx, `SELECT pg_advisory_unlock(hashtext($1), hashtext($2))`, lockKey, lockSeed)
		_ = conn.Close()
	}, nil
}

func (r *usageBillingRepository) CreateMediaGenerationTask(ctx context.Context, task *service.MediaGenerationTask) error {
	if r == nil || r.db == nil {
		return errors.New("usage billing repository db is nil")
	}
	if task == nil {
		return nil
	}
	publicTaskID := task.ClientTaskID()
	upstreamTaskID := task.ProviderTaskID()
	if publicTaskID == "" {
		return errors.New("media generation public task ID is required")
	}
	legacyTaskID := strings.TrimSpace(task.TaskID)
	if legacyTaskID == "" {
		legacyTaskID = publicTaskID
	}
	status := service.NormalizeMediaGenerationStatus(task.Status)
	if status == "" {
		status = service.MediaGenerationStatusPending
	}
	mediaType := strings.TrimSpace(task.MediaType)
	if mediaType == "" {
		mediaType = "video"
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO media_generation_tasks (
			task_id, public_task_id, upstream_task_id, api_key_id, user_id, account_id,
			group_id, subscription_id, model, requested_model, upstream_model, endpoint,
			inbound_endpoint, upstream_endpoint, channel_id, channel_mapped_model,
			billing_model_source, model_mapping_chain, request_fingerprint,
			request_payload_hash, idempotency_key_hash, response_status,
			response_content_type, response_body, upstream_result_url, status,
			duration_seconds, resolution, size_tier, billing_mode, billing_unit_price,
			billing_rate_multiplier, media_type, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16,
			$17, $18, $19,
			$20, NULLIF($21, ''), $22,
			$23, $24, NULLIF($25, ''), $26, $27,
			$28, $29, $30, $31,
			$32, $33, NOW(), NOW()
		)
		ON CONFLICT (api_key_id, task_id) DO UPDATE SET
			public_task_id = EXCLUDED.public_task_id,
			upstream_task_id = COALESCE(EXCLUDED.upstream_task_id, media_generation_tasks.upstream_task_id),
			account_id = EXCLUDED.account_id,
			group_id = EXCLUDED.group_id,
			subscription_id = EXCLUDED.subscription_id,
			model = EXCLUDED.model,
			requested_model = EXCLUDED.requested_model,
			upstream_model = EXCLUDED.upstream_model,
			endpoint = EXCLUDED.endpoint,
			inbound_endpoint = EXCLUDED.inbound_endpoint,
			upstream_endpoint = EXCLUDED.upstream_endpoint,
			channel_id = EXCLUDED.channel_id,
			channel_mapped_model = EXCLUDED.channel_mapped_model,
			billing_model_source = EXCLUDED.billing_model_source,
			model_mapping_chain = EXCLUDED.model_mapping_chain,
			request_fingerprint = EXCLUDED.request_fingerprint,
			request_payload_hash = EXCLUDED.request_payload_hash,
			idempotency_key_hash = COALESCE(EXCLUDED.idempotency_key_hash, media_generation_tasks.idempotency_key_hash),
			response_status = COALESCE(EXCLUDED.response_status, media_generation_tasks.response_status),
			response_content_type = COALESCE(NULLIF(EXCLUDED.response_content_type, ''), media_generation_tasks.response_content_type),
			response_body = COALESCE(NULLIF(EXCLUDED.response_body, ''), media_generation_tasks.response_body),
			upstream_result_url = COALESCE(EXCLUDED.upstream_result_url, media_generation_tasks.upstream_result_url),
			status = EXCLUDED.status,
			duration_seconds = COALESCE(EXCLUDED.duration_seconds, media_generation_tasks.duration_seconds),
			resolution = COALESCE(NULLIF(EXCLUDED.resolution, ''), media_generation_tasks.resolution),
			size_tier = COALESCE(NULLIF(EXCLUDED.size_tier, ''), media_generation_tasks.size_tier),
			billing_mode = COALESCE(NULLIF(media_generation_tasks.billing_mode, ''), NULLIF(EXCLUDED.billing_mode, '')),
			billing_unit_price = COALESCE(media_generation_tasks.billing_unit_price, EXCLUDED.billing_unit_price),
			billing_rate_multiplier = COALESCE(media_generation_tasks.billing_rate_multiplier, EXCLUDED.billing_rate_multiplier),
			media_type = COALESCE(NULLIF(EXCLUDED.media_type, ''), media_generation_tasks.media_type),
			updated_at = NOW()
	`, legacyTaskID, publicTaskID, nullableString(upstreamTaskID), task.APIKeyID, task.UserID, task.AccountID,
		task.GroupID, task.SubscriptionID, task.Model, task.RequestedModel, task.UpstreamModel, task.Endpoint,
		task.InboundEndpoint, task.UpstreamEndpoint, task.ChannelID, task.ChannelMappedModel,
		task.BillingModelSource, task.ModelMappingChain, task.RequestFingerprint,
		task.RequestPayloadHash, task.IdempotencyKeyHash, nullableInt(task.ResponseStatus),
		task.ResponseContentType, task.ResponseBody, task.UpstreamResultURL, status,
		nullableInt(task.DurationSeconds), task.Resolution, task.SizeTier, task.BillingMode,
		nullableFloat64(task.BillingUnitPrice), nullableFloat64(task.BillingRateMultiplier), mediaType)
	return err
}

func (r *usageBillingRepository) UpdateMediaGenerationTaskResponse(ctx context.Context, apiKeyID int64, taskID string, responseStatus int, responseContentType, responseBody, upstreamResultURL, status string, durationSeconds int) error {
	if r == nil || r.db == nil {
		return errors.New("usage billing repository db is nil")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	status = service.NormalizeMediaGenerationStatus(status)
	_, err := r.db.ExecContext(ctx, `
		UPDATE media_generation_tasks
		SET response_status = COALESCE($3, response_status),
			response_content_type = COALESCE(NULLIF($4, ''), response_content_type),
			response_body = COALESCE(NULLIF($5, ''), response_body),
			upstream_result_url = COALESCE(NULLIF($6, ''), upstream_result_url),
			status = COALESCE(NULLIF($7, ''), status),
			duration_seconds = COALESCE($8, duration_seconds),
			updated_at = NOW()
		WHERE api_key_id = $1 AND public_task_id = $2
		  AND (
			LOWER(BTRIM(status)) NOT IN (
				'complete', 'completed', 'success', 'succeeded', 'done',
				'fail', 'failed', 'failure', 'error', 'rejected', 'denied', 'aborted',
				'cancel', 'cancelled', 'canceled',
				'expire', 'expired', 'timeout', 'timed_out'
			)
			OR LOWER(BTRIM(status)) = LOWER(BTRIM($7))
		  )
	`, apiKeyID, taskID, nullableInt(responseStatus), strings.TrimSpace(responseContentType), responseBody,
		strings.TrimSpace(upstreamResultURL), status, nullableInt(durationSeconds))
	return err
}

func (r *usageBillingRepository) MarkMediaGenerationTaskTerminal(ctx context.Context, apiKeyID int64, taskID, status, finalizationError string) error {
	if r == nil || r.db == nil {
		return errors.New("usage billing repository db is nil")
	}
	status = service.NormalizeMediaGenerationStatus(status)
	if status == "" {
		return nil
	}
	var finalizedAt sql.NullTime
	if service.IsMediaGenerationSuccessStatus(status) {
		finalizedAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE media_generation_tasks
		SET status = $3,
			finalized_at = COALESCE($4, finalized_at),
			finalization_lease_token = NULL,
			finalization_lease_until = NULL,
			finalization_error = NULLIF($5, ''),
			updated_at = NOW()
		WHERE api_key_id = $1 AND public_task_id = $2
		  AND (
			LOWER(BTRIM(status)) NOT IN (
				'complete', 'completed', 'success', 'succeeded', 'done',
				'fail', 'failed', 'failure', 'error', 'rejected', 'denied', 'aborted',
				'cancel', 'cancelled', 'canceled',
				'expire', 'expired', 'timeout', 'timed_out'
			)
			OR LOWER(BTRIM(status)) = LOWER(BTRIM($3))
		  )
	`, apiKeyID, strings.TrimSpace(taskID), status, finalizedAt, strings.TrimSpace(finalizationError))
	return err
}

const tryAcquireMediaGenerationFinalizationSQL = `
	UPDATE media_generation_tasks
	SET finalization_lease_token = $3,
		finalization_lease_until = $4,
		finalization_error = NULL,
		updated_at = NOW()
	WHERE api_key_id = $1
	  AND (public_task_id = $2 OR task_id = $2)
	  AND usage_recorded_at IS NULL
	  AND LOWER(BTRIM(status)) IN ('complete', 'completed', 'success', 'succeeded', 'done')
	  AND (finalization_lease_until IS NULL OR finalization_lease_until <= NOW())
`

func (r *usageBillingRepository) TryAcquireMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string, leaseUntil time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("usage billing repository db is nil")
	}
	taskID = strings.TrimSpace(taskID)
	leaseToken = strings.TrimSpace(leaseToken)
	if taskID == "" || leaseToken == "" {
		return false, nil
	}
	result, err := r.db.ExecContext(ctx, tryAcquireMediaGenerationFinalizationSQL, apiKeyID, taskID, leaseToken, leaseUntil.UTC())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *usageBillingRepository) CompleteMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("usage billing repository db is nil")
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE media_generation_tasks
		SET status = $4,
			usage_recorded_at = NOW(),
			finalized_at = NOW(),
			finalization_lease_token = NULL,
			finalization_lease_until = NULL,
			finalization_error = NULL,
			updated_at = NOW()
		WHERE api_key_id = $1
		  AND (public_task_id = $2 OR task_id = $2)
		  AND finalization_lease_token = $3
		  AND usage_recorded_at IS NULL
	`, apiKeyID, strings.TrimSpace(taskID), strings.TrimSpace(leaseToken), service.MediaGenerationStatusCompleted)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (r *usageBillingRepository) ReleaseMediaGenerationFinalization(ctx context.Context, apiKeyID int64, taskID, leaseToken, finalizationError string) error {
	if r == nil || r.db == nil {
		return errors.New("usage billing repository db is nil")
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE media_generation_tasks
		SET finalization_lease_token = NULL,
			finalization_lease_until = NULL,
			finalization_error = NULLIF($4, ''),
			updated_at = NOW()
		WHERE api_key_id = $1
		  AND (public_task_id = $2 OR task_id = $2)
		  AND finalization_lease_token = $3
	`, apiKeyID, strings.TrimSpace(taskID), strings.TrimSpace(leaseToken), strings.TrimSpace(finalizationError))
	return err
}

const mediaGenerationTaskSelectSQL = `
	SELECT id, task_id, public_task_id, upstream_task_id, api_key_id, user_id, account_id,
		group_id, subscription_id, model, requested_model, upstream_model, endpoint,
		inbound_endpoint, upstream_endpoint, channel_id, channel_mapped_model,
		billing_model_source, model_mapping_chain, request_fingerprint,
		request_payload_hash, idempotency_key_hash, response_status,
		response_content_type, response_body, upstream_result_url, status, duration_seconds,
		resolution, size_tier, billing_mode, billing_unit_price, billing_rate_multiplier,
		media_type, finalized_at, finalization_lease_token,
		finalization_lease_until, usage_recorded_at, finalization_error, created_at, updated_at
	FROM media_generation_tasks
`

func scanMediaGenerationTask(row *sql.Row) (*service.MediaGenerationTask, error) {
	var task service.MediaGenerationTask
	var publicTaskID, upstreamTaskID sql.NullString
	var groupID, subID, channelID sql.NullInt64
	var requestedModel, upstreamModel, endpoint, inboundEndpoint, upstreamEndpoint sql.NullString
	var channelMappedModel, billingModelSource, mappingChain sql.NullString
	var payloadHash, idempotencyHash, responseContentType, responseBody, upstreamResultURL sql.NullString
	var responseStatus, durationSeconds sql.NullInt64
	var resolution, sizeTier, billingMode, mediaType, finalizationError sql.NullString
	var billingUnitPrice, billingRateMultiplier sql.NullFloat64
	var finalizedAt, finalizationLeaseUntil, usageRecordedAt sql.NullTime
	var finalizationLeaseToken sql.NullString
	err := row.Scan(
		&task.ID, &task.TaskID, &publicTaskID, &upstreamTaskID, &task.APIKeyID, &task.UserID, &task.AccountID,
		&groupID, &subID, &task.Model, &requestedModel, &upstreamModel, &endpoint,
		&inboundEndpoint, &upstreamEndpoint, &channelID, &channelMappedModel,
		&billingModelSource, &mappingChain, &task.RequestFingerprint,
		&payloadHash, &idempotencyHash, &responseStatus,
		&responseContentType, &responseBody, &upstreamResultURL, &task.Status, &durationSeconds, &resolution,
		&sizeTier, &billingMode, &billingUnitPrice, &billingRateMultiplier,
		&mediaType, &finalizedAt, &finalizationLeaseToken,
		&finalizationLeaseUntil, &usageRecordedAt, &finalizationError, &task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	task.PublicTaskID = publicTaskID.String
	task.UpstreamTaskID = upstreamTaskID.String
	task.GroupID = nullableInt64Ptr(groupID)
	task.SubscriptionID = nullableInt64Ptr(subID)
	task.ChannelID = nullableInt64Ptr(channelID)
	task.RequestedModel = requestedModel.String
	task.UpstreamModel = upstreamModel.String
	task.Endpoint = endpoint.String
	task.InboundEndpoint = inboundEndpoint.String
	task.UpstreamEndpoint = upstreamEndpoint.String
	task.ChannelMappedModel = channelMappedModel.String
	task.BillingModelSource = billingModelSource.String
	task.ModelMappingChain = mappingChain.String
	task.RequestPayloadHash = payloadHash.String
	task.IdempotencyKeyHash = idempotencyHash.String
	if responseStatus.Valid {
		task.ResponseStatus = int(responseStatus.Int64)
	}
	task.ResponseContentType = responseContentType.String
	task.ResponseBody = responseBody.String
	task.UpstreamResultURL = upstreamResultURL.String
	if durationSeconds.Valid {
		task.DurationSeconds = int(durationSeconds.Int64)
	}
	task.Resolution = resolution.String
	task.SizeTier = sizeTier.String
	task.BillingMode = billingMode.String
	task.BillingUnitPrice = nullableFloat64Ptr(billingUnitPrice)
	task.BillingRateMultiplier = nullableFloat64Ptr(billingRateMultiplier)
	task.MediaType = mediaType.String
	if finalizedAt.Valid {
		task.FinalizedAt = &finalizedAt.Time
	}
	task.FinalizationLeaseToken = finalizationLeaseToken.String
	if finalizationLeaseUntil.Valid {
		task.FinalizationLeaseUntil = &finalizationLeaseUntil.Time
	}
	if usageRecordedAt.Valid {
		task.UsageRecordedAt = &usageRecordedAt.Time
	}
	task.FinalizationError = finalizationError.String
	return &task, nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}
