-- Expand Assigned Level from 1–4 to 1–8. Backward compatible: existing Level 1–4 unchanged.
-- "Not tested yet" (NULL) remains valid.
ALTER TABLE placement_tests DROP CONSTRAINT IF EXISTS placement_tests_assigned_level_check;
ALTER TABLE placement_tests ADD CONSTRAINT placement_tests_assigned_level_check CHECK (assigned_level IS NULL OR (assigned_level >= 1 AND assigned_level <= 8));
