-- Add is_private column to student_notes
-- Private notes are only visible to Student Success, Mentor Heads, and Admins.
ALTER TABLE student_notes ADD COLUMN is_private BOOLEAN DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_student_notes_is_private ON student_notes(is_private) WHERE is_private = TRUE;
