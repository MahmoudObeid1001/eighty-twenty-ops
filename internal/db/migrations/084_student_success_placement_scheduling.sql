CREATE TABLE IF NOT EXISTS student_success_availability_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_success_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    available_date DATE NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    note TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CHECK (start_time < end_time)
);

CREATE INDEX IF NOT EXISTS idx_student_success_availability_windows_user_date
    ON student_success_availability_windows (student_success_user_id, available_date);

ALTER TABLE placement_tests
    ADD COLUMN IF NOT EXISTS scheduled_student_success_user_id UUID REFERENCES users(id);

CREATE INDEX IF NOT EXISTS idx_placement_tests_scheduled_student_success
    ON placement_tests (scheduled_student_success_user_id, test_date, test_time)
    WHERE scheduled_student_success_user_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_placement_test_student_success_slot
    ON placement_tests (scheduled_student_success_user_id, test_date, test_time)
    WHERE scheduled_student_success_user_id IS NOT NULL
      AND test_date IS NOT NULL
      AND test_time IS NOT NULL
      AND assigned_level IS NULL;
