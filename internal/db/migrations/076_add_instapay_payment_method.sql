ALTER TABLE transactions
    DROP CONSTRAINT IF EXISTS transactions_payment_method_check;

ALTER TABLE transactions
    ADD CONSTRAINT transactions_payment_method_check
    CHECK (payment_method IN ('vodafone_cash', 'instapay', 'bank_transfer', 'paypal', 'other'));

ALTER TABLE lead_payments
    DROP CONSTRAINT IF EXISTS lead_payments_payment_method_check;

ALTER TABLE lead_payments
    ADD CONSTRAINT lead_payments_payment_method_check
    CHECK (payment_method IN ('vodafone_cash', 'instapay', 'bank_transfer', 'paypal', 'other'));

ALTER TABLE placement_tests
    DROP CONSTRAINT IF EXISTS placement_tests_placement_test_payment_method_check;

ALTER TABLE placement_tests
    ADD CONSTRAINT placement_tests_placement_test_payment_method_check
    CHECK (
        placement_test_payment_method IS NULL
        OR placement_test_payment_method IN ('vodafone_cash', 'instapay', 'bank_transfer', 'paypal', 'other')
    );
