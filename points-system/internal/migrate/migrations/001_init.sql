CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE points_policy_versions (
    id BIGSERIAL PRIMARY KEY,
    version_no BIGSERIAL UNIQUE NOT NULL,
    effective_date DATE NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    mode TEXT NOT NULL DEFAULT 'all_users' CHECK (mode IN ('all_users','consumer_only')),
    basis TEXT NOT NULL DEFAULT 'yesterday' CHECK (basis IN ('yesterday','total')),
    checkin_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    checkin_daily_limit INTEGER DEFAULT 1 CHECK (checkin_daily_limit IS NULL OR checkin_daily_limit > 0),
    minimum_checkin_spend_microusd BIGINT NOT NULL DEFAULT 0
        CHECK (minimum_checkin_spend_microusd >= 0 AND minimum_checkin_spend_microusd % 10000 = 0),
    checkin_platform_daily_cap_microusd BIGINT CHECK (
        checkin_platform_daily_cap_microusd IS NULL OR
        (checkin_platform_daily_cap_microusd > 0 AND checkin_platform_daily_cap_microusd % 10000 = 0)
    ),
    checkin_user_daily_cap_microusd BIGINT CHECK (
        checkin_user_daily_cap_microusd IS NULL OR
        (checkin_user_daily_cap_microusd > 0 AND checkin_user_daily_cap_microusd % 10000 = 0)
    ),
    checkin_single_award_cap_microusd BIGINT CHECK (
        checkin_single_award_cap_microusd IS NULL OR
        (checkin_single_award_cap_microusd > 0 AND checkin_single_award_cap_microusd % 10000 = 0)
    ),
    points_per_usd_hundredths BIGINT NOT NULL DEFAULT 1000
        CHECK (points_per_usd_hundredths > 0),
    refresh_minute INTEGER NOT NULL DEFAULT 5 CHECK (refresh_minute >= 0 AND refresh_minute < 1440),
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (mode <> 'consumer_only' OR basis = 'yesterday'),
    CHECK (NOT checkin_enabled OR enabled),
    CHECK (NOT checkin_enabled OR (
        checkin_daily_limit > 0 AND checkin_platform_daily_cap_microusd > 0 AND
        checkin_user_daily_cap_microusd > 0 AND checkin_single_award_cap_microusd > 0
        AND checkin_platform_daily_cap_microusd % 10000 = 0
        AND checkin_user_daily_cap_microusd % 10000 = 0
        AND checkin_single_award_cap_microusd % 10000 = 0
    ))
);
CREATE INDEX idx_points_policy_effective ON points_policy_versions(effective_date DESC, version_no DESC);

CREATE TABLE points_policy_tiers (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES points_policy_versions(id) ON DELETE RESTRICT,
    lower_points_hundredths BIGINT NOT NULL CHECK (lower_points_hundredths >= 0),
    upper_points_hundredths BIGINT,
    reward_mode TEXT NOT NULL CHECK (reward_mode IN ('fixed_range','percentage_range')),
    fixed_reward_min_microusd BIGINT,
    fixed_reward_max_microusd BIGINT,
    reward_percentage_min_ppm BIGINT,
    reward_percentage_max_ppm BIGINT,
    CHECK (upper_points_hundredths IS NULL OR upper_points_hundredths > lower_points_hundredths),
    CHECK (
        (reward_mode='fixed_range' AND fixed_reward_min_microusd >= 0 AND
         fixed_reward_max_microusd > 0 AND fixed_reward_max_microusd >= fixed_reward_min_microusd AND
         fixed_reward_min_microusd % 10000 = 0 AND fixed_reward_max_microusd % 10000 = 0 AND
         reward_percentage_min_ppm IS NULL AND reward_percentage_max_ppm IS NULL)
        OR
        (reward_mode='percentage_range' AND reward_percentage_min_ppm >= 0 AND
         reward_percentage_max_ppm > 0 AND reward_percentage_max_ppm >= reward_percentage_min_ppm AND
         reward_percentage_max_ppm <= 1000000 AND fixed_reward_min_microusd IS NULL AND
         fixed_reward_max_microusd IS NULL)
    ),
    EXCLUDE USING gist (
        policy_id WITH =,
        int8range(lower_points_hundredths, upper_points_hundredths, '[)') WITH &&
    )
);

CREATE TABLE points_accounts (
    user_id BIGINT PRIMARY KEY,
    total_points_hundredths BIGINT NOT NULL DEFAULT 0 CHECK (total_points_hundredths >= 0),
    total_spend_microusd BIGINT NOT NULL DEFAULT 0 CHECK (total_spend_microusd >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE points_idempotency (
    scope TEXT NOT NULL,
    external_event_id TEXT NOT NULL,
    request_fingerprint CHAR(64) NOT NULL,
    result_reference TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(scope, external_event_id)
);

CREATE TABLE points_ledger (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    kind TEXT NOT NULL CHECK (kind = 'usage_points'),
    delta_points_hundredths BIGINT NOT NULL CHECK (delta_points_hundredths <> 0),
    total_after_hundredths BIGINT NOT NULL CHECK (total_after_hundredths >= 0),
    source TEXT NOT NULL,
    external_event_id TEXT NOT NULL,
    request_fingerprint CHAR(64) NOT NULL,
    policy_version BIGINT REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    business_date DATE,
    reference_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source, external_event_id)
);
CREATE INDEX idx_points_ledger_user_time ON points_ledger(user_id, created_at DESC, id DESC);

CREATE TABLE points_snapshot_refresh_runs (
    id UUID PRIMARY KEY,
    business_date DATE NOT NULL,
    trigger TEXT NOT NULL CHECK (trigger IN ('startup','scheduled','reconcile','manual')),
    source_window_start TIMESTAMPTZ NOT NULL,
    source_window_end TIMESTAMPTZ NOT NULL,
    source_fingerprint CHAR(64),
    source_users INTEGER CHECK (source_users >= 0),
    source_rows BIGINT CHECK (source_rows >= 0),
    changed_users INTEGER CHECK (changed_users >= 0),
    delta_spend_microusd BIGINT,
    delta_points_hundredths BIGINT,
    status TEXT NOT NULL CHECK (status IN ('running','succeeded','failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    CHECK (source_window_end > source_window_start),
    CHECK ((status='running' AND completed_at IS NULL AND error_message IS NULL) OR
           (status='succeeded' AND completed_at IS NOT NULL AND error_message IS NULL AND
            source_fingerprint IS NOT NULL AND source_users IS NOT NULL AND source_rows IS NOT NULL AND
            changed_users IS NOT NULL AND delta_spend_microusd IS NOT NULL AND
            delta_points_hundredths IS NOT NULL) OR
           (status='failed' AND completed_at IS NOT NULL AND
            NULLIF(BTRIM(error_message),'') IS NOT NULL))
);
CREATE INDEX idx_points_snapshot_runs_date ON points_snapshot_refresh_runs(business_date, created_at DESC);

CREATE TABLE points_daily_snapshots (
    user_id BIGINT NOT NULL,
    business_date DATE NOT NULL,
    actual_cost_microusd BIGINT NOT NULL CHECK (actual_cost_microusd >= 0),
    accounted_spend_microusd BIGINT NOT NULL DEFAULT 0 CHECK (accounted_spend_microusd >= 0),
    policy_version BIGINT REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    points_per_usd_hundredths BIGINT NOT NULL DEFAULT 0 CHECK (points_per_usd_hundredths >= 0),
    target_points_hundredths BIGINT NOT NULL DEFAULT 0 CHECK (target_points_hundredths >= 0),
    awarded_points_hundredths BIGINT NOT NULL DEFAULT 0 CHECK (awarded_points_hundredths >= 0),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    status TEXT NOT NULL CHECK (status IN (
        'disabled','ready','empty','needs_review'
    )),
    source_row_count BIGINT NOT NULL DEFAULT 0 CHECK (source_row_count >= 0),
    source_max_usage_log_id BIGINT NOT NULL DEFAULT 0 CHECK (source_max_usage_log_id >= 0),
    source_fingerprint CHAR(64) NOT NULL,
    last_refresh_run_id UUID NOT NULL REFERENCES points_snapshot_refresh_runs(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(user_id, business_date)
);
CREATE INDEX idx_points_daily_snapshots_date ON points_daily_snapshots(business_date, status);
CREATE INDEX idx_points_daily_snapshots_review_user ON points_daily_snapshots(user_id)
    WHERE status='needs_review';

CREATE TABLE points_daily_snapshot_revisions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    business_date DATE NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    actual_cost_microusd BIGINT NOT NULL CHECK (actual_cost_microusd >= 0),
    refresh_run_id UUID NOT NULL REFERENCES points_snapshot_refresh_runs(id) ON DELETE RESTRICT,
    previous_actual_cost_microusd BIGINT CHECK (previous_actual_cost_microusd >= 0),
    delta_actual_cost_microusd BIGINT NOT NULL,
    previous_accounted_spend_microusd BIGINT CHECK (previous_accounted_spend_microusd >= 0),
    accounted_spend_microusd BIGINT NOT NULL CHECK (accounted_spend_microusd >= 0),
    policy_version BIGINT REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    points_per_usd_hundredths BIGINT NOT NULL CHECK (points_per_usd_hundredths >= 0),
    target_points_hundredths BIGINT NOT NULL CHECK (target_points_hundredths >= 0),
    previous_awarded_points_hundredths BIGINT CHECK (previous_awarded_points_hundredths >= 0),
    awarded_points_hundredths BIGINT NOT NULL CHECK (awarded_points_hundredths >= 0),
    delta_points_hundredths BIGINT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('disabled','ready','empty','needs_review')),
    source_row_count BIGINT NOT NULL CHECK (source_row_count >= 0),
    source_max_usage_log_id BIGINT NOT NULL CHECK (source_max_usage_log_id >= 0),
    source_fingerprint CHAR(64) NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id,business_date,revision)
);

CREATE TABLE points_checkin_daily (
    user_id BIGINT NOT NULL,
    business_date DATE NOT NULL,
    checkin_count INTEGER NOT NULL DEFAULT 0 CHECK (checkin_count >= 0),
    awarded_microusd BIGINT NOT NULL DEFAULT 0 CHECK (awarded_microusd >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(user_id, business_date)
);

CREATE TABLE points_checkin_platform_daily_limits (
    business_date DATE PRIMARY KEY,
    awarded_microusd BIGINT NOT NULL DEFAULT 0 CHECK (awarded_microusd >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE points_checkins (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    business_date DATE NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    reward_microusd BIGINT NOT NULL CHECK (reward_microusd >= 0),
    external_event_id TEXT NOT NULL,
    policy_version BIGINT NOT NULL REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    policy_basis TEXT NOT NULL CHECK (policy_basis IN ('yesterday','total')),
    basis_points_hundredths BIGINT NOT NULL CHECK (basis_points_hundredths >= 0),
    yesterday_spend_microusd BIGINT NOT NULL CHECK (yesterday_spend_microusd >= 0),
    reward_base_microusd BIGINT NOT NULL CHECK (reward_base_microusd >= 0),
    reward_mode TEXT NOT NULL CHECK (reward_mode IN ('fixed_range','percentage_range')),
    fixed_reward_min_microusd BIGINT,
    fixed_reward_max_microusd BIGINT,
    reward_percentage_min_ppm BIGINT,
    reward_percentage_max_ppm BIGINT,
    sampled_percentage_ppm BIGINT,
    calculated_reward_microusd BIGINT NOT NULL CHECK (calculated_reward_microusd >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, business_date, ordinal),
    UNIQUE(external_event_id)
);

CREATE TABLE points_checkin_attempts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    business_date DATE NOT NULL,
    external_event_id TEXT NOT NULL UNIQUE,
    outcome TEXT NOT NULL CHECK (outcome IN ('accepted','rejected')),
    rejection_reason TEXT,
    policy_version BIGINT REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    policy_basis TEXT CHECK (policy_basis IN ('yesterday','total')),
    basis_points_hundredths BIGINT CHECK (basis_points_hundredths >= 0),
    yesterday_spend_microusd BIGINT CHECK (yesterday_spend_microusd >= 0),
    reward_base_microusd BIGINT CHECK (reward_base_microusd >= 0),
    reward_mode TEXT CHECK (reward_mode IN ('fixed_range','percentage_range')),
    fixed_reward_min_microusd BIGINT,
    fixed_reward_max_microusd BIGINT,
    reward_percentage_min_ppm BIGINT,
    reward_percentage_max_ppm BIGINT,
    sampled_percentage_ppm BIGINT,
    calculated_reward_microusd BIGINT CHECK (calculated_reward_microusd >= 0),
    actual_reward_microusd BIGINT CHECK (actual_reward_microusd >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((outcome='accepted' AND rejection_reason IS NULL) OR
           (outcome='rejected' AND rejection_reason IS NOT NULL))
);
CREATE INDEX idx_points_checkin_attempts_user_time
    ON points_checkin_attempts(user_id, created_at DESC, id DESC);

CREATE TABLE points_launch_ticket_nonces (
    jti_hash CHAR(64) PRIMARY KEY,
    subject_user_id BIGINT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user','admin')),
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_points_launch_ticket_expiry ON points_launch_ticket_nonces(expires_at);

CREATE TABLE points_sessions (
    token_hash CHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user','admin')),
    theme TEXT NOT NULL CHECK (theme IN ('light','dark')),
    language VARCHAR(16) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_points_sessions_expiry ON points_sessions(expires_at);

CREATE TABLE points_admin_audit (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id BIGINT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    request_id TEXT,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_points_admin_audit_time ON points_admin_audit(created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION points_reject_mutation() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$;

CREATE OR REPLACE FUNCTION points_guard_snapshot_refresh_run() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION '% is an audit table', TG_TABLE_NAME;
    END IF;
    IF OLD.status <> 'running' THEN
        RAISE EXCEPTION 'terminal snapshot refresh run is immutable';
    END IF;
    IF NEW.status NOT IN ('succeeded','failed') OR
       NEW.id IS DISTINCT FROM OLD.id OR
       NEW.business_date IS DISTINCT FROM OLD.business_date OR
       NEW.trigger IS DISTINCT FROM OLD.trigger OR
       NEW.source_window_start IS DISTINCT FROM OLD.source_window_start OR
       NEW.source_window_end IS DISTINCT FROM OLD.source_window_end OR
       NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'invalid snapshot refresh run transition';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION points_reject_delete() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is non-deletable', TG_TABLE_NAME;
END;
$$;

CREATE TRIGGER points_ledger_immutable BEFORE UPDATE OR DELETE ON points_ledger
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_policy_versions_immutable BEFORE UPDATE OR DELETE ON points_policy_versions
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_policy_tiers_immutable BEFORE UPDATE OR DELETE ON points_policy_tiers
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_checkins_immutable BEFORE UPDATE OR DELETE ON points_checkins
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_checkin_attempts_immutable BEFORE UPDATE OR DELETE ON points_checkin_attempts
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_admin_audit_immutable BEFORE UPDATE OR DELETE ON points_admin_audit
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_snapshot_revisions_immutable BEFORE UPDATE OR DELETE ON points_daily_snapshot_revisions
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
CREATE TRIGGER points_snapshot_refresh_runs_guard BEFORE UPDATE OR DELETE ON points_snapshot_refresh_runs
    FOR EACH ROW EXECUTE FUNCTION points_guard_snapshot_refresh_run();
CREATE TRIGGER points_daily_snapshots_non_deletable BEFORE DELETE ON points_daily_snapshots
    FOR EACH ROW EXECUTE FUNCTION points_reject_delete();
CREATE TRIGGER points_snapshot_runs_non_deletable BEFORE DELETE ON points_snapshot_refresh_runs
    FOR EACH ROW EXECUTE FUNCTION points_reject_delete();

-- The first version is disabled. Enabling always requires an explicit, complete
-- version through the admin API and becomes effective no earlier than tomorrow.
INSERT INTO points_policy_versions (
    effective_date, enabled, mode, basis, checkin_enabled, created_by
) VALUES (CURRENT_DATE, FALSE, 'consumer_only', 'yesterday', FALSE, 0);
