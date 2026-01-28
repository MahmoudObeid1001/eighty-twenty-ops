-- Create grades table for tracking student grades at session 8
CREATE TABLE IF NOT EXISTS grades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    session_number INTEGER NOT NULL CHECK (session_number = 8),
    grade TEXT NOT NULL CHECK (grade IN ('A', 'B', 'C', 'F')),
    notes TEXT,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (lead_id, class_key, session_number)
);

CREATE INDEX IF NOT EXISTS idx_grades_lead_id ON grades(lead_id);
CREATE INDEX IF NOT EXISTS idx_grades_class_key ON grades(class_key);
CREATE INDEX IF NOT EXISTS idx_grades_grade ON grades(grade);
