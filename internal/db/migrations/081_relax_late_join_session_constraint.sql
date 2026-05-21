ALTER TABLE late_joiners
    DROP CONSTRAINT IF EXISTS late_joiners_joined_at_session_number_check;

ALTER TABLE late_joiners
    ADD CONSTRAINT late_joiners_joined_at_session_number_check
    CHECK (joined_at_session_number BETWEEN 1 AND 8);
