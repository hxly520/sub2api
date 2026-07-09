-- Track asynchronous media generation tasks so task creation can be
-- idempotent and billing can be finalized only after a successful result.

CREATE TABLE IF NOT EXISTS media_generation_tasks (
    id BIGSERIAL PRIMARY KEY,
    task_id TEXT NOT NULL,
    api_key_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    group_id BIGINT NULL,
    subscription_id BIGINT NULL,
    model TEXT NOT NULL,
    requested_model TEXT NULL,
    upstream_model TEXT NULL,
    endpoint TEXT NULL,
    inbound_endpoint TEXT NULL,
    upstream_endpoint TEXT NULL,
    channel_id BIGINT NULL,
    channel_mapped_model TEXT NULL,
    billing_model_source TEXT NULL,
    model_mapping_chain TEXT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    request_payload_hash VARCHAR(64) NULL,
    idempotency_key_hash VARCHAR(64) NULL,
    response_status INT NULL,
    response_content_type TEXT NULL,
    response_body TEXT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    duration_seconds INT NULL,
    resolution TEXT NULL,
    size_tier VARCHAR(32) NULL,
    billing_mode VARCHAR(32) NULL,
    media_type VARCHAR(32) NOT NULL DEFAULT 'video',
    finalized_at TIMESTAMPTZ NULL,
    finalization_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (api_key_id, task_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_media_generation_tasks_idempotency
    ON media_generation_tasks (api_key_id, idempotency_key_hash)
    WHERE idempotency_key_hash IS NOT NULL AND idempotency_key_hash <> '';

CREATE INDEX IF NOT EXISTS idx_media_generation_tasks_status_updated
    ON media_generation_tasks (status, updated_at);
