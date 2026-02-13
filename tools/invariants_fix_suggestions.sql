-- Read-only fixer suggestions for invariants violations.
-- This script NEVER executes updates; it only prints suggested actions/SQL text.
-- Usage:
--   psql 'postgresql://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable' -f tools/invariants_fix_suggestions.sql

\echo '=== Invariants: Suggested Fixes (READ-ONLY OUTPUT) ==='

WITH lead_ctx AS (
    SELECT
        l.id,
        l.full_name,
        l.phone,
        l.status,
        COALESCE(l.is_returning, false) AS is_returning,
        COALESCE(l.sent_to_classes, false) AS sent_to_classes,
        COALESCE(l.levels_purchased_total, 0) AS levels_purchased_total,
        COALESCE(l.levels_consumed, 0) AS levels_consumed,
        GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS calc_remaining
    FROM leads l
),
violations AS (
    SELECT
        'returning_in_test_stage'::text AS check_name,
        'HIGH'::text AS severity,
        lc.id AS lead_id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Returning-cycle lead should not be in test stages.'::text AS details,
        'Move lead to renewal_pending or waiting_for_round based on remaining credits.'::text AS suggested_action,
        format(
            '/* REVIEW FIRST */ UPDATE leads SET status = ''%%s'' WHERE id = %s; -- choose renewal_pending or waiting_for_round',
            lc.id
        ) AS suggested_sql
    FROM lead_ctx lc
    WHERE lc.is_returning = true
      AND lc.status IN ('lead_created', 'test_booked', 'tested')

    UNION ALL

    SELECT
        'renewal_pending_with_remaining_credits'::text,
        'HIGH'::text,
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Lead has remaining credits but is in renewal_pending.'::text,
        'Set status to waiting_for_round (or another paid-flow status) to match remaining credits.'::text,
        format(
            '/* REVIEW FIRST */ UPDATE leads SET status = ''waiting_for_round'' WHERE id = %s;',
            lc.id
        )
    FROM lead_ctx lc
    WHERE lc.is_returning = true
      AND lc.status = 'renewal_pending'
      AND lc.calc_remaining > 0

    UNION ALL

    SELECT
        'in_classes_not_sent_to_classes'::text,
        'HIGH'::text,
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Lead in in_classes must keep sent_to_classes=true.'::text,
        'Restore sent_to_classes=true for lifecycle consistency.'::text,
        format(
            '/* REVIEW FIRST */ UPDATE leads SET sent_to_classes = true WHERE id = %s;',
            lc.id
        )
    FROM lead_ctx lc
    WHERE lc.status = 'in_classes'
      AND lc.sent_to_classes = false

    UNION ALL

    SELECT
        'late_joiner_state_invariant_broken'::text,
        'HIGH'::text,
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Late joiner exists but lead is not in_classes/sent_to_classes.'::text,
        'Restore in_classes + sent_to_classes, or undo invalid late-join record.'::text,
        format(
            '/* REVIEW FIRST */ UPDATE leads SET status = ''in_classes'', sent_to_classes = true WHERE id = %s;',
            lc.id
        )
    FROM late_joiners lj
    JOIN lead_ctx lc ON lc.id = lj.lead_id
    WHERE lc.status <> 'in_classes'
       OR lc.sent_to_classes = false

    UNION ALL

    SELECT
        'ready_to_start_missing_schedule_or_level'::text,
        'HIGH'::text,
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'ready_to_start requires assigned level + class_days + class_time.'::text,
        'Populate placement level and scheduling fields, or move lead back to a valid earlier state.'::text,
        format(
            '/* REVIEW FIRST */ SELECT * FROM placement_tests WHERE lead_id = %s; SELECT * FROM scheduling WHERE lead_id = %s;',
            lc.id,
            lc.id
        )
    FROM lead_ctx lc
    LEFT JOIN placement_tests pt ON pt.lead_id = lc.id
    LEFT JOIN scheduling s ON s.lead_id = lc.id
    WHERE lc.status = 'ready_to_start'
      AND (
            pt.assigned_level IS NULL
         OR s.class_days IS NULL
         OR s.class_time IS NULL
      )

    UNION ALL

    SELECT
        'in_classes_missing_schedule_or_level'::text,
        'HIGH'::text,
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'in_classes requires assigned level + class_days + class_time.'::text,
        'Backfill schedule/level and verify class_group/class_enrollment linkage for this lead.'::text,
        format(
            '/* REVIEW FIRST */ SELECT * FROM class_enrollments WHERE lead_id = %s;',
            lc.id
        )
    FROM lead_ctx lc
    LEFT JOIN placement_tests pt ON pt.lead_id = lc.id
    LEFT JOIN scheduling s ON s.lead_id = lc.id
    WHERE lc.status = 'in_classes'
      AND (
            pt.assigned_level IS NULL
         OR s.class_days IS NULL
         OR s.class_time IS NULL
      )

    UNION ALL

    SELECT
        'multiple_active_payment_cycles'::text,
        'HIGH'::text,
        l.id,
        l.full_name,
        l.phone,
        l.status,
        'Lead has more than one active payment cycle.'::text,
        'Close extra active cycles; keep only one active cycle per lead.'::text,
        format(
            '/* REVIEW FIRST */ SELECT id, started_at, status FROM payment_cycles WHERE lead_id = %s ORDER BY started_at DESC;',
            l.id
        )
    FROM leads l
    JOIN (
        SELECT lead_id, COUNT(*) AS active_count
        FROM payment_cycles
        WHERE status = 'active'
        GROUP BY lead_id
        HAVING COUNT(*) > 1
    ) pc ON pc.lead_id = l.id

    UNION ALL

    SELECT
        'refunds_exceed_payments'::text,
        'HIGH'::text,
        l.id,
        l.full_name,
        l.phone,
        l.status,
        'Total refunds exceed total course payments.'::text,
        'Audit refunds/payments for this lead and reverse incorrect refund entries if needed.'::text,
        format(
            '/* REVIEW FIRST */ SELECT * FROM transactions WHERE lead_id = %s AND category = ''refund'' ORDER BY created_at DESC; SELECT * FROM lead_payments WHERE lead_id = %s ORDER BY payment_date DESC;',
            l.id,
            l.id
        )
    FROM leads l
    LEFT JOIN (
        SELECT lead_id, COALESCE(SUM(amount), 0) AS paid_total
        FROM lead_payments
        GROUP BY lead_id
    ) p ON p.lead_id = l.id
    LEFT JOIN (
        SELECT lead_id, COALESCE(SUM(amount), 0) AS refund_total
        FROM transactions
        WHERE transaction_type = 'OUT' AND category = 'refund'
        GROUP BY lead_id
    ) r ON r.lead_id = l.id
    WHERE COALESCE(r.refund_total, 0) > COALESCE(p.paid_total, 0)

    UNION ALL

    SELECT
        'returning_levels_underflow'::text,
        'MEDIUM'::text,
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'levels_purchased_total < levels_consumed.'::text,
        'Rebuild purchased/consumed counters from payment cycles + outcomes and then update lead counters.'::text,
        format(
            '/* REVIEW FIRST */ SELECT id, levels_purchased_total, levels_consumed FROM leads WHERE id = %s; SELECT * FROM payment_cycles WHERE lead_id = %s ORDER BY started_at;',
            lc.id,
            lc.id
        )
    FROM lead_ctx lc
    WHERE lc.is_returning = true
      AND lc.levels_purchased_total < lc.levels_consumed
)
SELECT
    check_name,
    severity,
    lead_id,
    full_name,
    phone,
    status,
    details,
    suggested_action,
    suggested_sql
FROM violations
ORDER BY
    CASE severity WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END,
    check_name,
    full_name;

\echo '=== Suggested Fixes: Summary by Check ==='

WITH lead_ctx AS (
    SELECT
        l.id,
        l.status,
        COALESCE(l.is_returning, false) AS is_returning,
        COALESCE(l.sent_to_classes, false) AS sent_to_classes,
        COALESCE(l.levels_purchased_total, 0) AS levels_purchased_total,
        COALESCE(l.levels_consumed, 0) AS levels_consumed,
        GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS calc_remaining
    FROM leads l
),
violations AS (
    SELECT 'returning_in_test_stage'::text AS check_name, 'HIGH'::text AS severity
    FROM lead_ctx lc
    WHERE lc.is_returning = true AND lc.status IN ('lead_created', 'test_booked', 'tested')

    UNION ALL
    SELECT 'renewal_pending_with_remaining_credits', 'HIGH'
    FROM lead_ctx lc
    WHERE lc.is_returning = true AND lc.status = 'renewal_pending' AND lc.calc_remaining > 0

    UNION ALL
    SELECT 'in_classes_not_sent_to_classes', 'HIGH'
    FROM lead_ctx lc
    WHERE lc.status = 'in_classes' AND lc.sent_to_classes = false

    UNION ALL
    SELECT 'late_joiner_state_invariant_broken', 'HIGH'
    FROM late_joiners lj
    JOIN lead_ctx lc ON lc.id = lj.lead_id
    WHERE lc.status <> 'in_classes' OR lc.sent_to_classes = false

    UNION ALL
    SELECT 'ready_to_start_missing_schedule_or_level', 'HIGH'
    FROM lead_ctx lc
    LEFT JOIN placement_tests pt ON pt.lead_id = lc.id
    LEFT JOIN scheduling s ON s.lead_id = lc.id
    WHERE lc.status = 'ready_to_start'
      AND (pt.assigned_level IS NULL OR s.class_days IS NULL OR s.class_time IS NULL)

    UNION ALL
    SELECT 'in_classes_missing_schedule_or_level', 'HIGH'
    FROM lead_ctx lc
    LEFT JOIN placement_tests pt ON pt.lead_id = lc.id
    LEFT JOIN scheduling s ON s.lead_id = lc.id
    WHERE lc.status = 'in_classes'
      AND (pt.assigned_level IS NULL OR s.class_days IS NULL OR s.class_time IS NULL)

    UNION ALL
    SELECT 'multiple_active_payment_cycles', 'HIGH'
    FROM (
        SELECT lead_id
        FROM payment_cycles
        WHERE status = 'active'
        GROUP BY lead_id
        HAVING COUNT(*) > 1
    ) x

    UNION ALL
    SELECT 'refunds_exceed_payments', 'HIGH'
    FROM leads l
    LEFT JOIN (
        SELECT lead_id, COALESCE(SUM(amount), 0) AS paid_total
        FROM lead_payments
        GROUP BY lead_id
    ) p ON p.lead_id = l.id
    LEFT JOIN (
        SELECT lead_id, COALESCE(SUM(amount), 0) AS refund_total
        FROM transactions
        WHERE transaction_type = 'OUT' AND category = 'refund'
        GROUP BY lead_id
    ) r ON r.lead_id = l.id
    WHERE COALESCE(r.refund_total, 0) > COALESCE(p.paid_total, 0)

    UNION ALL
    SELECT 'returning_levels_underflow', 'MEDIUM'
    FROM lead_ctx lc
    WHERE lc.is_returning = true AND lc.levels_purchased_total < lc.levels_consumed
)
SELECT
    severity,
    check_name,
    COUNT(*) AS violation_count
FROM violations
GROUP BY severity, check_name
ORDER BY
    CASE severity WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END,
    check_name;
