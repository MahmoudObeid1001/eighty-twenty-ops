-- Add in_classes status to leads
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE leads ADD CONSTRAINT leads_status_check CHECK (status IN (
    'lead_created', 'test_booked', 'tested', 'offer_sent', 'booking_confirmed',
    'paid_full', 'deposit_paid', 'waiting_for_round', 'schedule_assigned', 'ready_to_start', 'in_classes'
));

-- Add class_group_index to scheduling table (tracks which class group within same level+days+time)
ALTER TABLE scheduling ADD COLUMN IF NOT EXISTS class_group_index INTEGER;

-- Create settings table for current_round tracking
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Initialize current_round to 1 if not exists
INSERT INTO settings (key, value) VALUES ('current_round', '1')
ON CONFLICT (key) DO NOTHING;

-- Index for efficient class group queries (assigned_level is in placement_tests, we'll join)
CREATE INDEX IF NOT EXISTS idx_scheduling_class_group ON scheduling(class_days, class_time, class_group_index) WHERE class_days IS NOT NULL AND class_time IS NOT NULL;
