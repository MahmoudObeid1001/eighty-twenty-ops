CREATE TABLE IF NOT EXISTS session_reschedule_notification_reads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reschedule_id UUID NOT NULL REFERENCES class_session_reschedules(id) ON DELETE CASCADE,
    read_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, reschedule_id)
);

CREATE INDEX IF NOT EXISTS idx_session_reschedule_notification_reads_user_id
    ON session_reschedule_notification_reads(user_id);

CREATE INDEX IF NOT EXISTS idx_session_reschedule_notification_reads_reschedule_id
    ON session_reschedule_notification_reads(reschedule_id);
