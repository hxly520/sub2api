-- Enable pre-output stream failover for installations that already opted into
-- the advanced OpenAI scheduler. Other installations retain legacy behavior.
INSERT INTO settings (key, value)
SELECT
    'openai_first_response_enabled',
    CASE
        WHEN EXISTS (
            SELECT 1 FROM settings
            WHERE key = 'openai_advanced_scheduler_enabled' AND value = 'true'
        ) THEN 'true'
        ELSE 'false'
    END
ON CONFLICT (key) DO NOTHING;

INSERT INTO settings (key, value)
VALUES ('openai_first_response_timeout_ms', '5000')
ON CONFLICT (key) DO NOTHING;
