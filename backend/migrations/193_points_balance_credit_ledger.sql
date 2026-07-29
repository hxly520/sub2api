CREATE TABLE IF NOT EXISTS points_balance_credits (
    transaction_id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount NUMERIC(20, 10) NOT NULL CHECK (amount <> 0),
    kind VARCHAR(32) NOT NULL CHECK (kind IN ('checkin', 'manual_grant', 'reversal')),
    source_reference VARCHAR(128) NOT NULL,
    reason VARCHAR(500) NOT NULL DEFAULT '',
    request_nonce VARCHAR(128) NOT NULL,
    payload_hash CHAR(64) NOT NULL,
    request_id VARCHAR(64) NOT NULL DEFAULT '',
    balance_after NUMERIC(20, 10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_points_balance_credits_user_created
    ON points_balance_credits (user_id, created_at DESC);

COMMENT ON TABLE points_balance_credits IS
    'Idempotent Sub2API balance credits signed by the independent points service.';
