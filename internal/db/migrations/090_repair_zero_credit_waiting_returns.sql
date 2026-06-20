-- Repair returning students left in the waiting list after their final paid
-- credit was consumed. Waiting-for-round should only represent students with
-- a real remaining credit.

UPDATE payment_cycles pc
SET status = 'closed',
    closed_at = COALESCE((
        SELECT MAX(ce.completed_at)
        FROM class_enrollments ce
        WHERE ce.lead_id = pc.lead_id
    ), NOW()),
    updated_at = NOW()
FROM leads l
WHERE l.id = pc.lead_id
  AND pc.status = 'active'
  AND COALESCE(pc.consumed_baseline, 0) + COALESCE(pc.bundle_levels, 0) <= COALESCE(l.levels_consumed, 0);

UPDATE leads
SET status = 'renewal_pending',
    sent_to_classes = false,
    remaining_credits = 0,
    high_priority_follow_up = true,
    updated_at = NOW()
WHERE status = 'waiting_for_round'
  AND COALESCE(is_returning, false) = true
  AND GREATEST(COALESCE(levels_purchased_total, 0) - COALESCE(levels_consumed, 0), 0) = 0;
