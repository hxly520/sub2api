-- Separate client-facing media task IDs from upstream task IDs and add
-- a short-lived finalization lease so concurrent polling cannot bill twice.

ALTER TABLE media_generation_tasks
    ADD COLUMN IF NOT EXISTS public_task_id TEXT,
    ADD COLUMN IF NOT EXISTS upstream_task_id TEXT,
    ADD COLUMN IF NOT EXISTS upstream_result_url TEXT,
    ADD COLUMN IF NOT EXISTS finalization_lease_token TEXT,
    ADD COLUMN IF NOT EXISTS finalization_lease_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS usage_recorded_at TIMESTAMPTZ;

UPDATE media_generation_tasks
SET public_task_id = CASE
        WHEN public_task_id IS NULL OR public_task_id = ''
            THEN 'video-' || md5(api_key_id::text || ':' || task_id)
        ELSE public_task_id
    END,
    upstream_task_id = CASE
        -- Only legacy rows that predate public IDs may copy task_id into the
        -- provider field. New creation intents already have a public ID and
        -- intentionally keep upstream_task_id NULL until upstream accepts.
        WHEN (public_task_id IS NULL OR public_task_id = '')
             AND (upstream_task_id IS NULL OR upstream_task_id = '')
            THEN task_id
        ELSE upstream_task_id
    END
WHERE public_task_id IS NULL OR public_task_id = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_media_generation_tasks_public_task_id
    ON media_generation_tasks (api_key_id, public_task_id)
    WHERE public_task_id IS NOT NULL AND public_task_id <> '';

CREATE INDEX IF NOT EXISTS idx_media_generation_tasks_finalization_lease
    ON media_generation_tasks (finalization_lease_until)
    WHERE finalized_at IS NULL AND usage_recorded_at IS NULL;
