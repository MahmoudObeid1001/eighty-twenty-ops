-- Create student_notes table for carry-over notes per student
CREATE TABLE IF NOT EXISTS student_notes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT REFERENCES class_groups(class_key) ON DELETE SET NULL,
    session_number INTEGER,
    note_text TEXT NOT NULL,
    created_by_user_id UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_student_notes_lead_id ON student_notes(lead_id);
CREATE INDEX IF NOT EXISTS idx_student_notes_class_key ON student_notes(class_key) WHERE class_key IS NOT NULL;
