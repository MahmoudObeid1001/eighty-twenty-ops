-- Migration 044: Add late joiner notifications table
CREATE TABLE IF NOT EXISTS late_joiner_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at_session_number INT NOT NULL,
    acknowledged_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- Prevent duplicate notifications for the same late joiner event to the same user
    UNIQUE(lead_id, class_key, user_id)
);

CREATE INDEX IF NOT EXISTS idx_late_joiner_notifications_user_id_acknowledged ON late_joiner_notifications(user_id, acknowledged_at) WHERE acknowledged_at IS NULL;
