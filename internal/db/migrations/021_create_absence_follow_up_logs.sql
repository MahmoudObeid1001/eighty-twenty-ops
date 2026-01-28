-- Create absence_follow_up_logs table for tracking absence follow-up actions
CREATE TABLE IF NOT EXISTS absence_follow_up_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    session_id UUID REFERENCES class_sessions(id) ON DELETE SET NULL,
    message_sent BOOLEAN DEFAULT false,
    reason TEXT,
    student_reply TEXT,
    action_taken TEXT,
    notes TEXT,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_absence_logs_lead_id ON absence_follow_up_logs(lead_id);
CREATE INDEX IF NOT EXISTS idx_absence_logs_session_id ON absence_follow_up_logs(session_id) WHERE session_id IS NOT NULL;
