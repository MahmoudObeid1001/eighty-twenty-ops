-- Create followups table
CREATE TABLE IF NOT EXISTS followups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_key TEXT NOT NULL,
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    session_number INTEGER NOT NULL,
    note TEXT,
    status TEXT NOT NULL DEFAULT 'NOT_CONTACTED', -- NOT_CONTACTED, CONTACTED, RESOLVED
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (class_key, lead_id, session_number)
);

CREATE INDEX IF NOT EXISTS idx_followups_class_key ON followups(class_key);
CREATE INDEX IF NOT EXISTS idx_followups_lead_id ON followups(lead_id);
