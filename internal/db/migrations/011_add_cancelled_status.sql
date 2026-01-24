-- Add cancelled status to leads table
-- This allows soft cancellation of leads (no hard deletes)
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE leads ADD CONSTRAINT leads_status_check CHECK (status IN (
    'lead_created', 'test_booked', 'tested', 'offer_sent', 'booking_confirmed',
    'paid_full', 'deposit_paid', 'waiting_for_round', 'schedule_assigned', 'ready_to_start', 'in_classes', 'cancelled'
));

-- Add cancelled_at timestamp for audit trail
ALTER TABLE leads ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMP WITH TIME ZONE;

-- Add index for filtering cancelled leads
CREATE INDEX IF NOT EXISTS idx_leads_status_cancelled ON leads(status) WHERE status = 'cancelled';
