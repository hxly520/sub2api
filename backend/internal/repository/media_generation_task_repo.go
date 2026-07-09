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
	return scanMediaGenerationTask(r.db.QueryRowContext(ctx, mediaGenerationTaskSelectSQL+`
		WHERE api_key_id = $1 AND task_id = $2
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
	if task == nil || strings.TrimSpace(task.TaskID) == "" {
		return nil
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
			task_id, api_key_id, user_id, account_id, group_id, subscription_id, model,
			requested_model, upstream_model, endpoint, inbound_endpoint, upstream_endpoint,
			channel_id, channel_mapped_model, billing_model_source, model_mapping_chain,
			request_fingerprint, request_payload_hash, idempotency_key_hash, response_status,
			response_content_type, response_body, status, duration_seconds, resolution,
			size_tier, billing_mode, media_type, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16,
			$17, $18, NULLIF($19, ''), $20,
			$21, $22, $23, $24, $25,
			$26, $27, $28, NOW(), NOW()
		)
		ON CONFLICT (api_key_id, task_id) DO UPDATE SET
			response_status = EXCLUDED.response_status,
			response_content_type = EXCLUDED.response_content_type,
			response_body = EXCLUDED.response_body,
			status = EXCLUDED.status,
			updated_at = NOW()
	`, task.TaskID, task.APIKeyID, task.UserID, task.AccountID, task.GroupID, task.SubscriptionID, task.Model,
		task.RequestedModel, task.UpstreamModel, task.Endpoint, task.InboundEndpoint, task.UpstreamEndpoint,
		task.ChannelID, task.ChannelMappedModel, task.BillingModelSource, task.ModelMappingChain,
		task.RequestFingerprint, task.RequestPayloadHash, task.IdempotencyKeyHash, nullableInt(task.ResponseStatus),
		task.ResponseContentType, task.ResponseBody, status, nullableInt(task.DurationSeconds), task.Resolution,
		task.SizeTier, task.BillingMode, mediaType)
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
			finalization_error = NULLIF($5, ''),
			updated_at = NOW()
		WHERE api_key_id = $1 AND task_id = $2
	`, apiKeyID, strings.TrimSpace(taskID), status, finalizedAt, strings.TrimSpace(finalizationError))
	return err
}

const mediaGenerationTaskSelectSQL = `
	SELECT id, task_id, api_key_id, user_id, account_id, group_id, subscription_id, model,
		requested_model, upstream_model, endpoint, inbound_endpoint, upstream_endpoint,
		channel_id, channel_mapped_model, billing_model_source, model_mapping_chain,
		request_fingerprint, request_payload_hash, idempotency_key_hash, response_status,
		response_content_type, response_body, status, duration_seconds, resolution,
		size_tier, billing_mode, media_type, finalized_at, finalization_error, created_at, updated_at
	FROM media_generation_tasks
`

func scanMediaGenerationTask(row *sql.Row) (*service.MediaGenerationTask, error) {
	var task service.MediaGenerationTask
	var groupID, subID, channelID sql.NullInt64
	var requestedModel, upstreamModel, endpoint, inboundEndpoint, upstreamEndpoint sql.NullString
	var channelMappedModel, billingModelSource, mappingChain sql.NullString
	var payloadHash, idempotencyHash, responseContentType, responseBody sql.NullString
	var responseStatus, durationSeconds sql.NullInt64
	var resolution, sizeTier, billingMode, mediaType, finalizationError sql.NullString
	var finalizedAt sql.NullTime
	err := row.Scan(
		&task.ID, &task.TaskID, &task.APIKeyID, &task.UserID, &task.AccountID, &groupID, &subID, &task.Model,
		&requestedModel, &upstreamModel, &endpoint, &inboundEndpoint, &upstreamEndpoint,
		&channelID, &channelMappedModel, &billingModelSource, &mappingChain,
		&task.RequestFingerprint, &payloadHash, &idempotencyHash, &responseStatus,
		&responseContentType, &responseBody, &task.Status, &durationSeconds, &resolution,
		&sizeTier, &billingMode, &mediaType, &finalizedAt, &finalizationError, &task.CreatedAt, &task.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
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
	if durationSeconds.Valid {
		task.DurationSeconds = int(durationSeconds.Int64)
	}
	task.Resolution = resolution.String
	task.SizeTier = sizeTier.String
	task.BillingMode = billingMode.String
	task.MediaType = mediaType.String
	if finalizedAt.Valid {
		task.FinalizedAt = &finalizedAt.Time
	}
	task.FinalizationError = finalizationError.String
	return &task, nil
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
