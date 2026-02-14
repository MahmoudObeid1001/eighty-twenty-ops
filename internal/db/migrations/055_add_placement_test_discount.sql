-- Add discount fields to placement_tests table for placement test fee discounts
ALTER TABLE placement_tests
ADD COLUMN IF NOT EXISTS discount_value INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS discount_type VARCHAR(20) DEFAULT 'amount' CHECK (discount_type IN ('amount', 'percent'));

-- Add comments for clarity
COMMENT ON COLUMN placement_tests.discount_value IS 'Discount amount (EGP) or percentage (%)';
COMMENT ON COLUMN placement_tests.discount_type IS 'Type of discount: amount or percent';
