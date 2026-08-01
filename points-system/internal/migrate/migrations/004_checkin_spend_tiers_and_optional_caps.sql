ALTER TABLE points_policy_versions
    ADD COLUMN checkin_tier_basis TEXT NOT NULL DEFAULT 'points'
        CHECK (checkin_tier_basis IN ('points','spend'));

ALTER TABLE points_policy_versions
    ADD CONSTRAINT points_policy_spend_tiers_require_yesterday CHECK (
        checkin_tier_basis <> 'spend' OR basis = 'yesterday'
    );

ALTER TABLE points_policy_tiers
    ALTER COLUMN lower_points_hundredths DROP NOT NULL,
    ADD COLUMN lower_spend_microusd BIGINT,
    ADD COLUMN upper_spend_microusd BIGINT;

-- Replace the original point-only exclusion constraint with one exclusion
-- constraint per threshold family. Existing rows remain point based.
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'points_policy_tiers'::regclass AND contype = 'x'
    LOOP
        EXECUTE format('ALTER TABLE points_policy_tiers DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END;
$$;

ALTER TABLE points_policy_tiers
    ADD CONSTRAINT points_policy_tiers_threshold_family_check CHECK (
        (
            lower_points_hundredths IS NOT NULL AND
            lower_points_hundredths >= 0 AND
            (upper_points_hundredths IS NULL OR upper_points_hundredths > lower_points_hundredths) AND
            lower_spend_microusd IS NULL AND upper_spend_microusd IS NULL
        ) OR (
            lower_points_hundredths IS NULL AND upper_points_hundredths IS NULL AND
            lower_spend_microusd IS NOT NULL AND lower_spend_microusd >= 0 AND
            lower_spend_microusd % 10000 = 0 AND
            (upper_spend_microusd IS NULL OR (
                upper_spend_microusd > lower_spend_microusd AND
                upper_spend_microusd % 10000 = 0
            ))
        )
    ),
    ADD CONSTRAINT points_policy_tiers_points_no_overlap EXCLUDE USING gist (
        policy_id WITH =,
        int8range(lower_points_hundredths, upper_points_hundredths, '[)') WITH &&
    ) WHERE (lower_points_hundredths IS NOT NULL),
    ADD CONSTRAINT points_policy_tiers_spend_no_overlap EXCLUDE USING gist (
        policy_id WITH =,
        int8range(lower_spend_microusd, upper_spend_microusd, '[)') WITH &&
    ) WHERE (lower_spend_microusd IS NOT NULL);

CREATE OR REPLACE FUNCTION points_validate_policy_tier_basis()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    expected_basis TEXT;
BEGIN
    SELECT checkin_tier_basis INTO STRICT expected_basis
    FROM points_policy_versions
    WHERE id = NEW.policy_id;

    IF (expected_basis = 'points' AND NEW.lower_points_hundredths IS NULL) OR
       (expected_basis = 'spend' AND NEW.lower_spend_microusd IS NULL) THEN
        RAISE EXCEPTION 'tier threshold family does not match policy checkin_tier_basis';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER points_policy_tiers_validate_basis
    BEFORE INSERT ON points_policy_tiers
    FOR EACH ROW EXECUTE FUNCTION points_validate_policy_tier_basis();

-- The original schema required all three monetary caps whenever check-in was
-- enabled. NULL now explicitly means unlimited; the daily count limit remains
-- mandatory and positive.
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    FOR constraint_name IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'points_policy_versions'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) LIKE '%NOT checkin_enabled%'
          AND pg_get_constraintdef(oid) LIKE '%checkin_platform_daily_cap_microusd > 0%'
    LOOP
        EXECUTE format('ALTER TABLE points_policy_versions DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END;
$$;

ALTER TABLE points_policy_versions
    ADD CONSTRAINT points_policy_checkin_daily_limit_required CHECK (
        NOT checkin_enabled OR checkin_daily_limit > 0
    );
