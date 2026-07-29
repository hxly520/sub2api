-- Support the global expiry scan by expiry time before grouping by user.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_media_balance_holds_active_expiry_user
    ON media_balance_holds (expires_at, user_id)
    WHERE status IN ('reserved', 'dispatched', 'capture_pending');
