-- Migration 043: Add late_joiners table and refine attendance statuses

-- 1. Create late_joiners table for audit and undo capability
CREATE TABLE IF NOT EXISTS late_joiners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    joined_at_session_number INT NOT NULL CHECK (joined_at_session_number IN (1, 2)),  -- Session 2 absolute limit
    reason TEXT NOT NULL,
    added_by_user_id UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- For Undo capability
    previous_class_days TEXT,
    previous_class_time TIME,
    previous_class_group_index INT,
    UNIQUE (lead_id)  -- One late join per student
);

CREATE INDEX IF NOT EXISTS idx_late_joiners_lead_id ON late_joiners(lead_id);
CREATE INDEX IF NOT EXISTS idx_late_joiners_class_key ON late_joiners(class_key);

-- 2. Add CHECK constraint to attendance status for data integrity
-- Note: 'N/A' is uppercase to match PRESENT/ABSENT casing
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'attendance_status_check') THEN
        ALTER TABLE attendance ADD CONSTRAINT attendance_status_check
        CHECK (status IN ('PRESENT', 'ABSENT', 'LATE', 'EXCUSED', 'N/A', ''));
    END IF;
END $$;
