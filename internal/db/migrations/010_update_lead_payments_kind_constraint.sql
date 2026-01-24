-- Update lead_payments kind constraint to allow deposit, full_payment, and top_up
-- First, find and drop any existing check constraint on the kind column
DO $$
DECLARE
    constraint_name TEXT;
BEGIN
    -- Find the constraint name for the kind column check constraint
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'lead_payments'::regclass
      AND conname LIKE '%kind%check%'
      AND contype = 'c';
    
    -- Drop the constraint if found
    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE lead_payments DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

-- Add new constraint that allows: course, deposit, full_payment, top_up
ALTER TABLE lead_payments ADD CONSTRAINT lead_payments_kind_check 
    CHECK (kind IN ('course', 'deposit', 'full_payment', 'top_up'));
