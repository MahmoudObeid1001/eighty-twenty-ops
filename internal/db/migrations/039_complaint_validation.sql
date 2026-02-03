-- Add CHECK constraints for complaint category and urgency
-- Based on SOURCE OF TRUTH fixed lists

ALTER TABLE followups ADD CONSTRAINT followups_category_check 
  CHECK (category IS NULL OR category IN ('mentor_behavior', 'session_quality', 'scheduling', 'content', 'technical', 'admin_process', 'student_behavior', 'other'));

ALTER TABLE followups ADD CONSTRAINT followups_urgency_check 
  CHECK (urgency IS NULL OR urgency IN ('low', 'medium', 'high', 'critical'));
