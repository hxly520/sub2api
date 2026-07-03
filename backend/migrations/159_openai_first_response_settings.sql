INSERT INTO settings (key, value)
VALUES
    ('openai_first_response_enabled', 'false'),
    ('openai_first_response_timeout_ms', '5000')
ON CONFLICT (key) DO NOTHING;
