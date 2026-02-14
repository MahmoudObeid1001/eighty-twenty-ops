-- Ensure a non-partial unique index exists for mentor/class evaluation rows.
-- This prevents ON CONFLICT matching issues on environments that previously created a partial index only.
DROP INDEX IF EXISTS uq_mentor_evaluations_mentor_class;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mentor_evaluations_mentor_class
    ON mentor_evaluations(mentor_id, class_key);
