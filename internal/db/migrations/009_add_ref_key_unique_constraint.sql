-- Add UNIQUE constraint on ref_key for ON CONFLICT to work
-- First, drop the existing unique index (if it exists)
DROP INDEX IF EXISTS idx_transactions_ref_key_unique;

-- Clean up any duplicate ref_key values before adding the constraint
-- Use ROW_NUMBER() window function since MIN(uuid) doesn't exist
-- Keep the oldest transaction (by created_at, then by id) for each ref_key
DELETE FROM transactions
WHERE id IN (
    SELECT id
    FROM (
        SELECT 
            id,
            ROW_NUMBER() OVER (PARTITION BY ref_key ORDER BY created_at ASC NULLS LAST, id ASC) as rn
        FROM transactions
        WHERE ref_key IS NOT NULL
    ) ranked
    WHERE rn > 1
);

-- Create a UNIQUE constraint on ref_key
-- PostgreSQL allows multiple NULLs in a UNIQUE constraint, which is fine
-- (expenses don't have ref_key, but placement test and course payments do)
-- Use IF NOT EXISTS pattern by dropping constraint first if it exists
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_ref_key_unique;
ALTER TABLE transactions ADD CONSTRAINT transactions_ref_key_unique UNIQUE (ref_key);
