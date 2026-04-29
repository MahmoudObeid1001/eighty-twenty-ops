ALTER TABLE class_memberships
    ADD COLUMN IF NOT EXISTS level_consumed_at_session_number INT
    CHECK (
        level_consumed_at_session_number IS NULL
        OR level_consumed_at_session_number BETWEEN 1 AND 8
    );

ALTER TABLE class_enrollments
    ADD COLUMN IF NOT EXISTS next_level_consumed_on_close BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE class_enrollments
    ADD COLUMN IF NOT EXISTS continuation_hold_active BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE class_enrollments
    ADD COLUMN IF NOT EXISTS continuation_hold_reason TEXT;

ALTER TABLE class_enrollments
    ADD COLUMN IF NOT EXISTS continuation_hold_applied_by UUID REFERENCES users(id);

ALTER TABLE class_enrollments
    ADD COLUMN IF NOT EXISTS continuation_hold_applied_at TIMESTAMP WITH TIME ZONE;

ALTER TABLE class_enrollments
    ADD COLUMN IF NOT EXISTS continuation_hold_released_by UUID REFERENCES users(id);

ALTER TABLE class_enrollments
    ADD COLUMN IF NOT EXISTS continuation_hold_released_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX IF NOT EXISTS idx_class_memberships_consumption_pending
    ON class_memberships (class_key, joined_at_session_number)
    WHERE level_consumed_at_session_number IS NULL;

CREATE INDEX IF NOT EXISTS idx_class_enrollments_continuation_hold_active
    ON class_enrollments (lead_id, completed_at DESC)
    WHERE continuation_hold_active = TRUE;
