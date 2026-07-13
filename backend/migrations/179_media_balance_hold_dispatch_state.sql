-- Include dispatched media holds in lazy expiry reconciliation without changing
-- the checksum of the already published 178 migration.

DROP INDEX IF EXISTS idx_media_balance_holds_reserved_expiry;

CREATE INDEX idx_media_balance_holds_reserved_expiry
    ON media_balance_holds (user_id, expires_at)
    WHERE status IN ('reserved', 'dispatched', 'capture_pending');
