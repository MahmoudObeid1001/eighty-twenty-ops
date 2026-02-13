-- Add explicit offer_sent_at timestamp for cold-lead timing.
-- Avoid using leads.updated_at because unrelated edits can reset the timer.

ALTER TABLE leads
  ADD COLUMN IF NOT EXISTS offer_sent_at TIMESTAMP WITH TIME ZONE;

-- Backfill legacy rows: if currently offer_sent and no explicit timestamp,
-- use updated_at as best available approximation.
UPDATE leads
SET offer_sent_at = updated_at
WHERE status = 'offer_sent'
  AND offer_sent_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_leads_offer_sent_at ON leads(offer_sent_at);
