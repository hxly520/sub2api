-- Atomic balance holds for non-idempotent image/video generation.
-- Reserving moves available balance to frozen_balance before the upstream POST;
-- capture/release moves it back exactly once.

CREATE TABLE IF NOT EXISTS media_balance_holds (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    hold_amount NUMERIC(20, 8) NOT NULL CHECK (hold_amount >= 0),
    capture_amount NUMERIC(20, 8) NULL CHECK (capture_amount >= 0),
    settled_amount NUMERIC(20, 8) NOT NULL DEFAULT 0 CHECK (settled_amount >= 0),
    status VARCHAR(16) NOT NULL DEFAULT 'reserved',
    expires_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (request_id, api_key_id)
);

CREATE INDEX IF NOT EXISTS idx_media_balance_holds_reserved_expiry
    ON media_balance_holds (user_id, expires_at)
    WHERE status IN ('reserved', 'capture_pending');

ALTER TABLE media_generation_tasks
    ADD COLUMN IF NOT EXISTS request_count INT NOT NULL DEFAULT 1;
