-- Update placement test default fee to 60 EGP.
ALTER TABLE placement_tests
    ALTER COLUMN placement_test_fee SET DEFAULT 60;

-- Move unpaid placement tests that still sit on the old default to the new fee.
UPDATE placement_tests
SET placement_test_fee = 60,
    updated_at = CURRENT_TIMESTAMP
WHERE COALESCE(placement_test_fee_paid, 0) = 0
  AND COALESCE(placement_test_fee, 100) = 100;
