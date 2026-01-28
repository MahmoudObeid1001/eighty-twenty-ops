-- Create mentor_evaluations table for storing mentor KPI and attendance evaluations
CREATE TABLE IF NOT EXISTS mentor_evaluations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mentor_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    kpi_session_quality INTEGER NOT NULL DEFAULT 0 CHECK (kpi_session_quality >= 0 AND kpi_session_quality <= 100),
    kpi_trello INTEGER NOT NULL DEFAULT 0 CHECK (kpi_trello >= 0 AND kpi_trello <= 100),
    kpi_whatsapp INTEGER NOT NULL DEFAULT 0 CHECK (kpi_whatsapp >= 0 AND kpi_whatsapp <= 100),
    kpi_students_feedback INTEGER NOT NULL DEFAULT 0 CHECK (kpi_students_feedback >= 0 AND kpi_students_feedback <= 100),
    attendance_statuses JSONB NOT NULL DEFAULT '["unknown","unknown","unknown","unknown","unknown","unknown","unknown","unknown"]'::jsonb,
    evaluator_id UUID NOT NULL REFERENCES users(id),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create index on mentor_id for faster lookups
CREATE INDEX IF NOT EXISTS idx_mentor_evaluations_mentor_id ON mentor_evaluations(mentor_id);

-- Create index on evaluator_id
CREATE INDEX IF NOT EXISTS idx_mentor_evaluations_evaluator_id ON mentor_evaluations(evaluator_id);
