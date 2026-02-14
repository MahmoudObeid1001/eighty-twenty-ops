-- Make mentor evaluations class-scoped (mentor_id + class_key) and store Trello checks per session.
ALTER TABLE mentor_evaluations
    ADD COLUMN IF NOT EXISTS class_key TEXT REFERENCES class_groups(class_key) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS trello_session_checks JSONB NOT NULL DEFAULT '[false,false,false,false,false,false,false,false]'::jsonb;

-- Backfill existing rows to active classes for the same mentor so legacy data remains visible.
UPDATE mentor_evaluations me
SET class_key = ma.class_key
FROM mentor_assignments ma
JOIN class_groups cg ON cg.class_key = ma.class_key
WHERE me.class_key IS NULL
  AND ma.mentor_user_id = me.mentor_id
  AND cg.round_status = 'active'
  AND ma.class_key = (
      SELECT ma2.class_key
      FROM mentor_assignments ma2
      JOIN class_groups cg2 ON cg2.class_key = ma2.class_key
      WHERE ma2.mentor_user_id = me.mentor_id
        AND cg2.round_status = 'active'
      ORDER BY ma2.assigned_at DESC
      LIMIT 1
  );

-- Remove old mentor-only uniqueness; enforce class-scoped uniqueness.
ALTER TABLE mentor_evaluations
    DROP CONSTRAINT IF EXISTS mentor_evaluations_mentor_id_key;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mentor_evaluations_mentor_class
    ON mentor_evaluations(mentor_id, class_key)
    WHERE class_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_mentor_evaluations_class_key
    ON mentor_evaluations(class_key);
