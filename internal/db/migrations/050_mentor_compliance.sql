-- Mentor compliance checks per class session (Student Success audit)
CREATE TABLE IF NOT EXISTS mentor_session_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_session_id UUID NOT NULL UNIQUE,
    checked_by_user_id UUID NOT NULL,
    reminder_1d BOOLEAN NOT NULL DEFAULT false,
    reminder_1h BOOLEAN NOT NULL DEFAULT false,
    reminder_tasks BOOLEAN NOT NULL DEFAULT false,
    delay_minutes INTEGER NOT NULL DEFAULT 0 CHECK (delay_minutes >= 0),
    is_absent BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT mentor_session_checks_class_session_fk
        FOREIGN KEY (class_session_id)
        REFERENCES class_sessions(id)
        ON DELETE CASCADE,
    CONSTRAINT mentor_session_checks_checked_by_fk
        FOREIGN KEY (checked_by_user_id)
        REFERENCES users(id)
        ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_mentor_session_checks_checked_by_user_id
    ON mentor_session_checks(checked_by_user_id);

