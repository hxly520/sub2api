-- Freeze asynchronous media pricing at task creation so later channel price or
-- group multiplier changes do not alter the final bill for an in-flight task.

ALTER TABLE media_generation_tasks
    ADD COLUMN IF NOT EXISTS billing_unit_price NUMERIC(20, 10),
    ADD COLUMN IF NOT EXISTS billing_rate_multiplier NUMERIC(20, 10);
