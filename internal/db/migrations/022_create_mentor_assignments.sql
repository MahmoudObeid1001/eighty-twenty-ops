-- Create mentor_assignments table for linking mentors (users with role='mentor') to classes
-- Uses mentor_user_id to reference users.id directly (no separate mentors table)
CREATE TABLE IF NOT EXISTS mentor_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mentor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_by_user_id UUID REFERENCES users(id),
    UNIQUE (class_key) -- One mentor per class
);

CREATE INDEX IF NOT EXISTS idx_mentor_assignments_mentor_user_id ON mentor_assignments(mentor_user_id);
CREATE INDEX IF NOT EXISTS idx_mentor_assignments_class_key ON mentor_assignments(class_key);
