ALTER TABLE placement_tests
    ADD COLUMN IF NOT EXISTS appointment_status TEXT NOT NULL DEFAULT 'scheduled';

ALTER TABLE placement_tests
    DROP CONSTRAINT IF EXISTS placement_tests_appointment_status_check;

ALTER TABLE placement_tests
    ADD CONSTRAINT placement_tests_appointment_status_check CHECK (
        appointment_status IN ('scheduled', 'completed', 'no_show', 'cancelled', 'rescheduled')
    );

UPDATE placement_tests
SET appointment_status = 'completed'
WHERE assigned_level IS NOT NULL
  AND appointment_status = 'scheduled';

ALTER TABLE leads
    DROP CONSTRAINT IF EXISTS leads_ops_queue_reason_check;

ALTER TABLE leads
    ADD CONSTRAINT leads_ops_queue_reason_check CHECK (
        ops_queue_reason IS NULL
        OR ops_queue_reason IN ('refund_review', 'private_track', 'placement_test_no_show')
    );

CREATE INDEX IF NOT EXISTS idx_placement_tests_no_show_admin_queue
    ON placement_tests (appointment_status, test_date, test_time)
    WHERE appointment_status = 'no_show';

DROP INDEX IF EXISTS uniq_active_placement_test_student_success_slot;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_placement_test_student_success_slot
    ON placement_tests (scheduled_student_success_user_id, test_date, test_time)
    WHERE scheduled_student_success_user_id IS NOT NULL
      AND test_date IS NOT NULL
      AND test_time IS NOT NULL
      AND assigned_level IS NULL
      AND appointment_status = 'scheduled';
