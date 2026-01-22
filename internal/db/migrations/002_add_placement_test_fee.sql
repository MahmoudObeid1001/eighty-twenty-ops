-- Add placement test fee fields to placement_tests table
ALTER TABLE placement_tests 
ADD COLUMN IF NOT EXISTS placement_test_fee INTEGER DEFAULT 100,
ADD COLUMN IF NOT EXISTS placement_test_fee_paid INTEGER DEFAULT 0;
