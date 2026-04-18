ALTER TABLE class_transfers
    DROP CONSTRAINT IF EXISTS class_transfers_reason_check;

ALTER TABLE class_transfers
    ADD CONSTRAINT class_transfers_reason_check CHECK (
        reason IN (
            'schedule_change',
            'promotion',
            'demotion',
            'refund_to_admin',
            'private_track_to_admin',
            'other_to_admin',
            'late_join',
            'other'
        )
    );
