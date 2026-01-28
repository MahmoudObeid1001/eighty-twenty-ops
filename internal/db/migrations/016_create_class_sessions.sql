-- Create class_sessions table for tracking 8 sessions per class
CREATE TABLE IF NOT EXISTS class_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    session_number INTEGER NOT NULL CHECK (session_number >= 1 AND session_number <= 8),
    scheduled_date DATE NOT NULL,
    scheduled_time TIME NOT NULL,
    scheduled_end_time TIME, -- Calculated: scheduled_time + 2 hours (or configurable duration)
    actual_date DATE,
    actual_time TIME,
    actual_end_time TIME,
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled')),
    completed_at TIMESTAMP WITH TIME ZONE, -- Timestamp when session was marked completed (for refund rule)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (class_key, session_number)
);

CREATE INDEX IF NOT EXISTS idx_class_sessions_class_key ON class_sessions(class_key);
CREATE INDEX IF NOT EXISTS idx_class_sessions_status ON class_sessions(status);
CREATE INDEX IF NOT EXISTS idx_class_sessions_date_time ON class_sessions(scheduled_date, scheduled_time);
CREATE INDEX IF NOT EXISTS idx_class_sessions_completed_at ON class_sessions(completed_at) WHERE completed_at IS NOT NULL;
