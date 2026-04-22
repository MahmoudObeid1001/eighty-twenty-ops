CREATE TABLE IF NOT EXISTS class_session_reschedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_session_id UUID NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    session_number INT NOT NULL CHECK (session_number BETWEEN 1 AND 8),
    old_scheduled_date DATE NOT NULL,
    old_scheduled_time TIME NOT NULL,
    new_scheduled_date DATE NOT NULL,
    new_scheduled_time TIME NOT NULL,
    changed_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_class_session_reschedules_session_id
    ON class_session_reschedules (class_session_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_class_session_reschedules_class_key
    ON class_session_reschedules (class_key, created_at DESC);
