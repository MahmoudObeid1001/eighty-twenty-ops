-- Create community_officer_feedback table for feedback at sessions 4 and 8
CREATE TABLE IF NOT EXISTS community_officer_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    session_number INTEGER NOT NULL CHECK (session_number IN (4, 8)),
    feedback_text TEXT NOT NULL,
    follow_up_required BOOLEAN DEFAULT false,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (lead_id, class_key, session_number)
);

CREATE INDEX IF NOT EXISTS idx_co_feedback_lead_id ON community_officer_feedback(lead_id);
CREATE INDEX IF NOT EXISTS idx_co_feedback_class_key ON community_officer_feedback(class_key);
CREATE INDEX IF NOT EXISTS idx_co_feedback_follow_up ON community_officer_feedback(follow_up_required) WHERE follow_up_required = true;
