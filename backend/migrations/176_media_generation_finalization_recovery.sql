-- Successful media tasks may be persisted before usage billing finishes. If the
-- process exits in that window, a later poll must be able to acquire a new lease
-- even when legacy code already populated finalized_at.

DROP INDEX IF EXISTS idx_media_generation_tasks_finalization_lease;

CREATE INDEX idx_media_generation_tasks_finalization_lease
    ON media_generation_tasks (finalization_lease_until)
    WHERE usage_recorded_at IS NULL;
