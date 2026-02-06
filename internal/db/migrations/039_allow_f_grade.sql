-- Migration 039: Allow 'F' grade (overriding 038)
ALTER TABLE grades DROP CONSTRAINT IF EXISTS grades_grade_check;

ALTER TABLE grades ADD CONSTRAINT grades_grade_check 
  CHECK (grade IN ('A', 'B', 'C', 'F'));
