ALTER TABLE points_snapshot_refresh_runs
    DROP CONSTRAINT points_snapshot_refresh_runs_trigger_check;
ALTER TABLE points_snapshot_refresh_runs
    ADD CONSTRAINT points_snapshot_refresh_runs_trigger_check
    CHECK (trigger IN ('startup','scheduled','reconcile','manual','history_backfill'));

CREATE TABLE points_usage_history_backfill_jobs (
    id UUID PRIMARY KEY,
    policy_version BIGINT NOT NULL
        REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    points_per_usd_hundredths BIGINT NOT NULL
        CHECK (points_per_usd_hundredths > 0),
    from_date DATE NOT NULL,
    through_date DATE NOT NULL,
    next_date DATE NOT NULL,
    plan_fingerprint CHAR(64) NOT NULL,
    planned_source_users BIGINT NOT NULL CHECK (planned_source_users >= 0),
    planned_source_user_days BIGINT NOT NULL CHECK (planned_source_user_days >= 0),
    planned_source_business_days BIGINT NOT NULL CHECK (planned_source_business_days >= 0),
    planned_source_rows BIGINT NOT NULL CHECK (planned_source_rows >= 0),
    planned_spend_microusd BIGINT NOT NULL CHECK (planned_spend_microusd >= 0),
    planned_points_hundredths BIGINT NOT NULL CHECK (planned_points_hundredths >= 0),
    planned_source_max_usage_log_id BIGINT NOT NULL
        CHECK (planned_source_max_usage_log_id >= 0),
    completed_days INTEGER NOT NULL DEFAULT 0 CHECK (completed_days >= 0),
    applied_source_user_days BIGINT NOT NULL DEFAULT 0
        CHECK (applied_source_user_days >= 0),
    applied_source_business_days BIGINT NOT NULL DEFAULT 0
        CHECK (applied_source_business_days >= 0),
    applied_source_rows BIGINT NOT NULL DEFAULT 0 CHECK (applied_source_rows >= 0),
    applied_source_max_usage_log_id BIGINT NOT NULL DEFAULT 0
        CHECK (applied_source_max_usage_log_id >= 0),
    changed_users BIGINT NOT NULL DEFAULT 0 CHECK (changed_users >= 0),
    delta_spend_microusd BIGINT NOT NULL DEFAULT 0,
    delta_points_hundredths BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('running','failed','succeeded')),
    error_message TEXT,
    created_by BIGINT NOT NULL CHECK (created_by > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    CHECK (through_date >= from_date),
    CHECK (next_date >= from_date AND next_date <= through_date + 1),
    CHECK ((status='running' AND error_message IS NULL AND completed_at IS NULL) OR
           (status='failed' AND NULLIF(BTRIM(error_message),'') IS NOT NULL AND
            completed_at IS NULL) OR
           (status='succeeded' AND error_message IS NULL AND completed_at IS NOT NULL AND
            next_date = through_date + 1))
);

-- A history import is a one-time accounting baseline. A failed job must be
-- resumed instead of creating an overlapping replacement job.
CREATE UNIQUE INDEX idx_points_usage_history_backfill_one_incomplete
    ON points_usage_history_backfill_jobs ((TRUE))
    WHERE status <> 'succeeded';
CREATE INDEX idx_points_usage_history_backfill_jobs_created
    ON points_usage_history_backfill_jobs(created_at DESC);

CREATE TABLE points_usage_history_backfill_days (
    business_date DATE PRIMARY KEY,
    job_id UUID NOT NULL
        REFERENCES points_usage_history_backfill_jobs(id) ON DELETE RESTRICT,
    refresh_run_id UUID NOT NULL UNIQUE
        REFERENCES points_snapshot_refresh_runs(id) ON DELETE RESTRICT,
    policy_version BIGINT NOT NULL
        REFERENCES points_policy_versions(version_no) ON DELETE RESTRICT,
    points_per_usd_hundredths BIGINT NOT NULL
        CHECK (points_per_usd_hundredths > 0),
    source_users INTEGER NOT NULL CHECK (source_users >= 0),
    source_rows BIGINT NOT NULL CHECK (source_rows >= 0),
    changed_users INTEGER NOT NULL CHECK (changed_users >= 0),
    delta_spend_microusd BIGINT NOT NULL,
    delta_points_hundredths BIGINT NOT NULL,
    source_fingerprint CHAR(64) NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_points_usage_history_backfill_days_job
    ON points_usage_history_backfill_days(job_id, business_date);

CREATE OR REPLACE FUNCTION points_guard_usage_history_backfill_job()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION '% is an audit table', TG_TABLE_NAME;
    END IF;
    IF OLD.status = 'succeeded' OR
       NEW.id IS DISTINCT FROM OLD.id OR
       NEW.policy_version IS DISTINCT FROM OLD.policy_version OR
       NEW.points_per_usd_hundredths IS DISTINCT FROM OLD.points_per_usd_hundredths OR
       NEW.from_date IS DISTINCT FROM OLD.from_date OR
       NEW.through_date IS DISTINCT FROM OLD.through_date OR
       NEW.plan_fingerprint IS DISTINCT FROM OLD.plan_fingerprint OR
       NEW.planned_source_users IS DISTINCT FROM OLD.planned_source_users OR
       NEW.planned_source_user_days IS DISTINCT FROM OLD.planned_source_user_days OR
       NEW.planned_source_business_days IS DISTINCT FROM OLD.planned_source_business_days OR
       NEW.planned_source_rows IS DISTINCT FROM OLD.planned_source_rows OR
       NEW.planned_spend_microusd IS DISTINCT FROM OLD.planned_spend_microusd OR
       NEW.planned_points_hundredths IS DISTINCT FROM OLD.planned_points_hundredths OR
       NEW.planned_source_max_usage_log_id IS DISTINCT FROM OLD.planned_source_max_usage_log_id OR
       NEW.created_by IS DISTINCT FROM OLD.created_by OR
       NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'invalid history backfill job mutation';
    END IF;
    IF OLD.status = 'running' AND NEW.status IN ('running','succeeded') THEN
        IF NEW.next_date IS DISTINCT FROM OLD.next_date + 1 OR
           NEW.completed_days IS DISTINCT FROM OLD.completed_days + 1 OR
           NEW.applied_source_user_days < OLD.applied_source_user_days OR
           NEW.applied_source_business_days < OLD.applied_source_business_days OR
           NEW.applied_source_rows < OLD.applied_source_rows OR
           NEW.applied_source_max_usage_log_id < OLD.applied_source_max_usage_log_id OR
           NEW.changed_users < OLD.changed_users OR
           NEW.error_message IS NOT NULL THEN
            RAISE EXCEPTION 'invalid history backfill progress transition';
        END IF;
    ELSIF OLD.status = 'running' AND NEW.status = 'failed' THEN
        IF NEW.next_date IS DISTINCT FROM OLD.next_date OR
           NEW.completed_days IS DISTINCT FROM OLD.completed_days OR
           NEW.applied_source_user_days IS DISTINCT FROM OLD.applied_source_user_days OR
           NEW.applied_source_business_days IS DISTINCT FROM OLD.applied_source_business_days OR
           NEW.applied_source_rows IS DISTINCT FROM OLD.applied_source_rows OR
           NEW.applied_source_max_usage_log_id IS DISTINCT FROM OLD.applied_source_max_usage_log_id OR
           NEW.changed_users IS DISTINCT FROM OLD.changed_users OR
           NEW.delta_spend_microusd IS DISTINCT FROM OLD.delta_spend_microusd OR
           NEW.delta_points_hundredths IS DISTINCT FROM OLD.delta_points_hundredths THEN
            RAISE EXCEPTION 'invalid history backfill failure transition';
        END IF;
    ELSIF OLD.status = 'failed' AND NEW.status = 'running' THEN
        IF NEW.next_date IS DISTINCT FROM OLD.next_date OR
           NEW.completed_days IS DISTINCT FROM OLD.completed_days OR
           NEW.applied_source_user_days IS DISTINCT FROM OLD.applied_source_user_days OR
           NEW.applied_source_business_days IS DISTINCT FROM OLD.applied_source_business_days OR
           NEW.applied_source_rows IS DISTINCT FROM OLD.applied_source_rows OR
           NEW.applied_source_max_usage_log_id IS DISTINCT FROM OLD.applied_source_max_usage_log_id OR
           NEW.changed_users IS DISTINCT FROM OLD.changed_users OR
           NEW.delta_spend_microusd IS DISTINCT FROM OLD.delta_spend_microusd OR
           NEW.delta_points_hundredths IS DISTINCT FROM OLD.delta_points_hundredths OR
           NEW.error_message IS NOT NULL OR NEW.completed_at IS NOT NULL THEN
            RAISE EXCEPTION 'invalid history backfill resume transition';
        END IF;
    ELSE
        RAISE EXCEPTION 'invalid history backfill status transition';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER points_usage_history_backfill_jobs_guard
    BEFORE UPDATE OR DELETE ON points_usage_history_backfill_jobs
    FOR EACH ROW EXECUTE FUNCTION points_guard_usage_history_backfill_job();

CREATE TRIGGER points_usage_history_backfill_days_immutable
    BEFORE UPDATE OR DELETE ON points_usage_history_backfill_days
    FOR EACH ROW EXECUTE FUNCTION points_reject_mutation();
