CREATE TABLE IF NOT EXISTS lead_snoozes (
    lead_id UUID PRIMARY KEY REFERENCES leads(id) ON DELETE CASCADE,
    snoozed_until TIMESTAMP WITH TIME ZONE NOT NULL,
    note TEXT,
    scheduled_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_lead_snoozes_until
    ON lead_snoozes (snoozed_until);
