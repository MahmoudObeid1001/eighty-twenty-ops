-- Add cold_lead status for retargeting pipeline
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_status_check;
ALTER TABLE leads ADD CONSTRAINT leads_status_check CHECK (status IN (
    'lead_created', 'test_booked', 'tested', 'offer_sent', 'booking_confirmed',
    'paid_full', 'deposit_paid', 'waiting_for_round', 'schedule_assigned', 'ready_to_start',
    'in_classes', 'cancelled', 'paused', 'renewal_pending', 'cold_lead'
));
