-- Update grades constraint to A/B/C only (per SOURCE OF TRUTH)
-- Remove F grade option

ALTER TABLE grades DROP CONSTRAINT IF EXISTS grades_grade_check;

ALTER TABLE grades ADD CONSTRAINT grades_grade_check 
  CHECK (grade IN ('A', 'B', 'C'));
