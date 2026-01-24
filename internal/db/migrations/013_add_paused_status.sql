-- Add paused status for mid-round / on-hold leads (already paid, wants to pause)
-- WAITING = pre-start waiting list (no payments). PAUSED = mid-round pause (payments OK).
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE leads ADD CONSTRAINT leads_status_check CHECK (status IN (
    'lead_created', 'test_booked', 'tested', 'offer_sent', 'booking_confirmed',
    'paid_full', 'deposit_paid', 'waiting_for_round', 'schedule_assigned', 'ready_to_start',
    'in_classes', 'cancelled', 'paused'
));

CREATE INDEX IF NOT EXISTS idx_leads_status_paused ON leads(status) WHERE status = 'paused';
