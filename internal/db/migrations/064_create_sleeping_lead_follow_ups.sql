CREATE TABLE IF NOT EXISTS sleeping_lead_follow_ups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    message_number INTEGER NOT NULL CHECK (message_number BETWEEN 1 AND 3),
    sent_by_user_id UUID REFERENCES users(id),
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sleeping_lead_follow_ups_lead_id
    ON sleeping_lead_follow_ups(lead_id, sent_at DESC);
