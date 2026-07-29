CREATE TABLE points_balance_grants (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL,
    amount_microusd BIGINT NOT NULL CHECK (amount_microusd > 0 AND amount_microusd % 10000 = 0),
    kind TEXT NOT NULL CHECK (kind IN ('checkin','manual_grant')),
    operation TEXT NOT NULL DEFAULT 'credit' CHECK (operation IN ('credit','debit')),
    status TEXT NOT NULL CHECK (status IN (
        'pending','processing','failed','settled','reversal_pending',
        'reversal_processing','reversed','permanently_failed',
        'reversal_permanently_failed'
    )),
    external_event_id TEXT NOT NULL UNIQUE,
    request_fingerprint CHAR(64) NOT NULL,
    policy_version BIGINT NOT NULL REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ,
    lease_token UUID,
    lease_until TIMESTAMPTZ,
    last_error TEXT,
    reason TEXT NOT NULL DEFAULT '',
    settled_at TIMESTAMPTZ,
    reversed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_points_balance_grants_due ON points_balance_grants(next_attempt_at, created_at)
    WHERE status IN ('pending','failed','reversal_pending');
CREATE INDEX idx_points_balance_grants_user_created ON points_balance_grants(user_id, created_at DESC);

CREATE TABLE points_balance_grant_attempts (
    id BIGSERIAL PRIMARY KEY,
    balance_grant_id UUID NOT NULL REFERENCES points_balance_grants(id) ON DELETE RESTRICT,
    operation TEXT NOT NULL CHECK (operation IN ('credit','debit')),
    attempt_no INTEGER NOT NULL CHECK (attempt_no > 0),
    request_id UUID NOT NULL,
    http_status INTEGER,
    outcome TEXT NOT NULL CHECK (outcome IN ('success','retryable_failure','permanent_failure')),
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(balance_grant_id, operation, attempt_no)
);

CREATE TABLE points_balance_grant_reversals (
    id UUID PRIMARY KEY,
    balance_grant_id UUID NOT NULL REFERENCES points_balance_grants(id) ON DELETE RESTRICT UNIQUE,
    reason TEXT NOT NULL,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE points_checkins
    ADD COLUMN balance_grant_id UUID UNIQUE REFERENCES points_balance_grants(id) ON DELETE RESTRICT,
    ADD CONSTRAINT points_checkins_balance_grant_required CHECK (
        (reward_microusd = 0 AND balance_grant_id IS NULL) OR
        (reward_microusd > 0 AND balance_grant_id IS NOT NULL)
    );

CREATE TRIGGER points_balance_grant_attempts_immutable
    BEFORE UPDATE OR DELETE ON points_balance_grant_attempts
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_balance_grant_reversals_immutable
    BEFORE UPDATE OR DELETE ON points_balance_grant_reversals
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_balance_grants_non_deletable
    BEFORE DELETE ON points_balance_grants
    FOR EACH ROW EXECUTE FUNCTION points_reject_delete();
