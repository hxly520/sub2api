-- Prepaid link-card keys. Standard API keys remain the default and all
-- existing rows are backfilled as standard keys.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS key_type VARCHAR(16) NOT NULL DEFAULT 'standard',
    ADD COLUMN IF NOT EXISTS link_state VARCHAR(32),
    ADD COLUMN IF NOT EXISTS link_rate_multiplier NUMERIC(10, 4),
    ADD COLUMN IF NOT EXISTS link_original_debit NUMERIC(20, 8),
    ADD COLUMN IF NOT EXISTS link_total_funded NUMERIC(20, 8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS link_total_refunded NUMERIC(20, 8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS link_reserved_amount NUMERIC(20, 8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS link_concurrency INTEGER,
    ADD COLUMN IF NOT EXISTS link_rpm_limit INTEGER,
    ADD COLUMN IF NOT EXISTS link_activated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS link_revoked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS link_frozen_reason VARCHAR(500);

UPDATE api_keys SET key_type = 'standard' WHERE key_type IS NULL OR key_type = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'api_keys_key_type_check'
    ) THEN
        ALTER TABLE api_keys ADD CONSTRAINT api_keys_key_type_check
            CHECK (key_type IN ('standard', 'link'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'api_keys_link_state_check'
    ) THEN
        ALTER TABLE api_keys ADD CONSTRAINT api_keys_link_state_check
            CHECK (link_state IS NULL OR link_state IN (
                'pending_activation', 'active', 'frozen', 'depleted',
                'refunded', 'revoked'
            ));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'api_keys_link_amounts_check'
    ) THEN
        ALTER TABLE api_keys ADD CONSTRAINT api_keys_link_amounts_check
            CHECK (
                link_total_funded >= 0 AND link_total_refunded >= 0
                AND link_reserved_amount >= 0
                AND (link_rate_multiplier IS NULL OR link_rate_multiplier > 0)
                AND (link_original_debit IS NULL OR link_original_debit >= 0)
                AND (link_concurrency IS NULL OR link_concurrency > 0)
                AND (link_rpm_limit IS NULL OR link_rpm_limit >= 0)
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_api_keys_key_type_user_created
    ON api_keys (key_type, user_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_link_state_created
    ON api_keys (link_state, created_at DESC)
    WHERE key_type = 'link' AND deleted_at IS NULL;

-- Media holds use the same table for standard user-balance freezes and
-- prepaid link-card reservations.  The source is immutable after creation.
ALTER TABLE media_balance_holds
    ADD COLUMN IF NOT EXISTS funding_source VARCHAR(16) NOT NULL DEFAULT 'user_balance';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'media_balance_holds_funding_source_check'
    ) THEN
        ALTER TABLE media_balance_holds ADD CONSTRAINT media_balance_holds_funding_source_check
            CHECK (funding_source IN ('user_balance', 'link_card'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_media_balance_holds_link_active
    ON media_balance_holds (api_key_id, expires_at)
    WHERE funding_source = 'link_card' AND status IN ('reserved', 'dispatched', 'capture_pending');

-- The administrator explicitly authorizes which existing Sub2API groups can
-- be selected when a prepaid link key is issued.
CREATE TABLE IF NOT EXISTS link_card_group_authorizations (
    group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE RESTRICT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_link_card_groups_enabled_sort
    ON link_card_group_authorizations (enabled, sort_order, group_id);

-- Durable idempotency for every state-changing card operation. These rows do
-- not expire because replaying an old financial request must never apply it a
-- second time.
CREATE TABLE IF NOT EXISTS link_card_operations (
    id BIGSERIAL PRIMARY KEY,
    scope VARCHAR(64) NOT NULL,
    actor_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    creator_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    api_key_id BIGINT REFERENCES api_keys(id) ON DELETE RESTRICT,
    idempotency_key_hash CHAR(64) NOT NULL,
    request_fingerprint CHAR(64) NOT NULL,
    response_body JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (scope, actor_user_id, idempotency_key_hash)
);

CREATE INDEX IF NOT EXISTS idx_link_card_operations_card_created
    ON link_card_operations (api_key_id, created_at DESC);

-- Append-only money and reserve ledger. reserve_delta is expressed in actual
-- prepaid USD; creator_balance_delta is the matching Sub2API user-balance
-- movement. Usage rows preserve the gateway request id for exact reconciliation.
CREATE TABLE IF NOT EXISTS link_card_ledger (
    id BIGSERIAL PRIMARY KEY,
    operation_id BIGINT REFERENCES link_card_operations(id) ON DELETE RESTRICT,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
    creator_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    entry_type VARCHAR(32) NOT NULL CHECK (entry_type IN (
        'issue', 'recharge', 'usage', 'refund', 'admin_adjustment'
    )),
    reserve_delta NUMERIC(20, 10) NOT NULL,
    creator_balance_delta NUMERIC(20, 10) NOT NULL DEFAULT 0,
    quota_before NUMERIC(20, 10) NOT NULL,
    quota_after NUMERIC(20, 10) NOT NULL,
    quota_used_before NUMERIC(20, 10) NOT NULL,
    quota_used_after NUMERIC(20, 10) NOT NULL,
    request_id VARCHAR(128),
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reason VARCHAR(500) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_link_card_ledger_usage_request
    ON link_card_ledger (api_key_id, request_id, entry_type)
    WHERE request_id IS NOT NULL AND entry_type = 'usage';
CREATE UNIQUE INDEX IF NOT EXISTS uq_link_card_ledger_operation_card_entry
    ON link_card_ledger (operation_id, api_key_id, entry_type)
    WHERE operation_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_link_card_ledger_creator_created
    ON link_card_ledger (creator_user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_link_card_ledger_card_created
    ON link_card_ledger (api_key_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION reject_link_card_ledger_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'link_card_ledger is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_link_card_ledger_immutable ON link_card_ledger;
CREATE TRIGGER trg_link_card_ledger_immutable
    BEFORE UPDATE OR DELETE ON link_card_ledger
    FOR EACH ROW EXECUTE FUNCTION reject_link_card_ledger_mutation();

INSERT INTO settings (key, value, updated_at) VALUES
    ('link_cards_enabled', 'false', NOW()),
    ('link_cards_rollout_user_id', '1', NOW()),
    ('link_cards_development_mode', 'true', NOW()),
    ('link_cards_development_user_ids', '[1]', NOW()),
    ('link_cards_default_concurrency', '5', NOW()),
    ('link_cards_default_rpm_limit', '0', NOW()),
    ('link_cards_max_batch_size', '100', NOW()),
    ('link_cards_minimum_deposit', '', NOW()),
    ('link_cards_public_portal_url', 'https://key.52token.org', NOW()),
    ('link_cards_api_base_url', 'https://api.52token.org/v1', NOW()),
    ('link_cards_public_session_ttl_seconds', '3600', NOW())
ON CONFLICT (key) DO NOTHING;
