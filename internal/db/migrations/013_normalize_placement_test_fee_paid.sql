-- Normalize placement_test_fee_paid to match placement_test_fee (default 100) when invalid.
UPDATE placement_tests
SET
    placement_test_fee = COALESCE(placement_test_fee, 100),
    placement_test_fee_paid = COALESCE(placement_test_fee, 100)
WHERE placement_test_fee_paid IS NOT NULL
  AND placement_test_fee_paid > 0
  AND placement_test_fee_paid <> COALESCE(placement_test_fee, 100);
