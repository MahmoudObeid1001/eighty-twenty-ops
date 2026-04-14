CREATE TABLE IF NOT EXISTS sleeping_lead_reminders (
    lead_id UUID PRIMARY KEY REFERENCES leads(id) ON DELETE CASCADE,
    follow_up_at TIMESTAMP WITH TIME ZONE NOT NULL,
    note TEXT,
    scheduled_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sleeping_lead_reminders_follow_up_at
    ON sleeping_lead_reminders(follow_up_at);
