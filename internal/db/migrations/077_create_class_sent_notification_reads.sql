CREATE TABLE IF NOT EXISTS class_sent_notification_reads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    read_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, class_key)
);

CREATE INDEX IF NOT EXISTS idx_class_sent_notification_reads_user_id
    ON class_sent_notification_reads(user_id);

CREATE INDEX IF NOT EXISTS idx_class_sent_notification_reads_class_key
    ON class_sent_notification_reads(class_key);
