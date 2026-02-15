CREATE TABLE IF NOT EXISTS session_performance (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_session_id UUID NOT NULL REFERENCES class_sessions(id) ON DELETE CASCADE,
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    task_completed BOOLEAN NOT NULL DEFAULT false,
    participation_score INT NOT NULL DEFAULT 3 CHECK (participation_score BETWEEN 1 AND 5),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (class_session_id, lead_id)
);

CREATE INDEX IF NOT EXISTS idx_session_performance_session_lead
    ON session_performance(class_session_id, lead_id);

CREATE INDEX IF NOT EXISTS idx_session_performance_lead
    ON session_performance(lead_id);
