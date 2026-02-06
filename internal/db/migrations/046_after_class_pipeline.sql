-- After Class Pipeline Architecture
-- Adds support for student history tracking, expanded levels, and returning student management

-- 1. Update leads status constraint to include new statuses for after-class workflow
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE leads ADD CONSTRAINT leads_status_check CHECK (status IN (
    'lead_created', 'test_booked', 'tested', 'offer_sent', 'booking_confirmed',
    'paid_full', 'deposit_paid', 'waiting_for_round', 'schedule_assigned', 'ready_to_start',
    'in_classes', 'cancelled', 'paused', 'renewal_pending'
));

-- 2. Update placement_tests assigned_level constraint from 1-8 to 1-10
ALTER TABLE placement_tests DROP CONSTRAINT IF EXISTS placement_tests_assigned_level_check;
ALTER TABLE placement_tests ADD CONSTRAINT placement_tests_assigned_level_check 
    CHECK (assigned_level IS NULL OR (assigned_level >= 1 AND assigned_level <= 10));

-- 3. Add new columns to leads table for returning student management
ALTER TABLE leads ADD COLUMN IF NOT EXISTS remaining_credits INTEGER DEFAULT 0;
ALTER TABLE leads ADD COLUMN IF NOT EXISTS is_returning BOOLEAN DEFAULT false;

-- 4. Create class_enrollments table for historical tracking
CREATE TABLE IF NOT EXISTS class_enrollments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    class_key TEXT NOT NULL REFERENCES class_groups(class_key) ON DELETE CASCADE,
    
    -- Snapshot fields (captured at enrollment)
    level INTEGER NOT NULL,
    class_days TEXT NOT NULL,
    class_time TEXT NOT NULL,
    mentor_name TEXT,
    
    -- Result fields (populated after class completion)
    final_grade TEXT CHECK (final_grade IN ('A', 'B', 'C', 'F')),
    outcome TEXT CHECK (outcome IN ('promoted', 'repeated')),
    
    -- Timestamps
    enrolled_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE,
    
    -- Prevent duplicate enrollments
    UNIQUE (lead_id, class_key)
);

-- 5. Create indexes for class_enrollments table
CREATE INDEX IF NOT EXISTS idx_class_enrollments_lead_id ON class_enrollments(lead_id);
CREATE INDEX IF NOT EXISTS idx_class_enrollments_class_key ON class_enrollments(class_key);
CREATE INDEX IF NOT EXISTS idx_class_enrollments_completed_at ON class_enrollments(completed_at);
