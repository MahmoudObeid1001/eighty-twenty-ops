CREATE TABLE IF NOT EXISTS offer_sent_follow_ups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    message_number INTEGER NOT NULL CHECK (message_number BETWEEN 1 AND 3),
    sent_by_user_id UUID REFERENCES users(id),
    sent_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_offer_sent_follow_ups_lead_id
    ON offer_sent_follow_ups(lead_id, sent_at DESC);

CREATE TABLE IF NOT EXISTS offer_sent_reminders (
    lead_id UUID PRIMARY KEY REFERENCES leads(id) ON DELETE CASCADE,
    follow_up_at TIMESTAMP WITH TIME ZONE NOT NULL,
    note TEXT,
    scheduled_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_offer_sent_reminders_follow_up_at
    ON offer_sent_reminders(follow_up_at);
