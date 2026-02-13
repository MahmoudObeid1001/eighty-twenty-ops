-- Read-only suggestions report for fixing invariant violations safely.
-- It does NOT execute updates; it only prints proposed SQL statements.
--
-- Usage:
--   psql 'postgresql://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable' -f tools/invariants_suggestions.sql

\echo '=== Invariants Suggestions: Proposed Fixes (Read-only) ==='

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
refund_totals AS (
    SELECT
        l.id AS lead_id,
        COALESCE(p.paid_total, 0) AS paid_total,
        COALESCE(r.refund_total, 0) AS refund_total
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
),
suggestions AS (
    SELECT
        'returning_in_test_stage'::text AS check_name,
        'HIGH'::text AS severity,
        lc.id AS lead_id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Return lead to renewal_pending (or waiting_for_round if prepaid credits exist) and prevent test-stage actions for returning cycle.'::text AS suggested_action,
        format(
            $$-- Review first, then run if correct:
UPDATE leads
SET status = 'renewal_pending', updated_at = NOW()
WHERE id = '%s';$$,
            lc.id::text
        ) AS suggested_sql
    FROM lead_ctx lc
    WHERE lc.is_returning = true
      AND lc.status IN ('lead_created', 'test_booked', 'tested')

    UNION ALL

    SELECT
        'renewal_pending_with_remaining_credits',
        'HIGH',
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Lead has credits but status is renewal_pending; likely should be waiting_for_round (business review required).',
        format(
            $$-- Review first, then run if correct:
UPDATE leads
SET status = 'waiting_for_round', updated_at = NOW()
WHERE id = '%s'
  AND GREATEST(COALESCE(levels_purchased_total, 0) - COALESCE(levels_consumed, 0), 0) > 0;$$,
            lc.id::text
        )
    FROM lead_ctx lc
    WHERE lc.is_returning = true
      AND lc.status = 'renewal_pending'
      AND lc.calc_remaining > 0

    UNION ALL

    SELECT
        'in_classes_not_sent_to_classes',
        'HIGH',
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Restore sent_to_classes=true for in_classes lead.',
        format(
            $$-- Review first, then run if correct:
UPDATE leads
SET sent_to_classes = true, updated_at = NOW()
WHERE id = '%s' AND status = 'in_classes';$$,
            lc.id::text
        )
    FROM lead_ctx lc
    WHERE lc.status = 'in_classes'
      AND lc.sent_to_classes = false

    UNION ALL

    SELECT
        'late_joiner_state_invariant_broken',
        'HIGH',
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Late-joiner exists but lead not in in_classes/sent_to_classes; likely set status=in_classes and sent_to_classes=true.',
        format(
            $$-- Review first, then run if correct:
UPDATE leads
SET status = 'in_classes',
    sent_to_classes = true,
    updated_at = NOW()
WHERE id = '%s';$$,
            lc.id::text
        )
    FROM late_joiners lj
    JOIN lead_ctx lc ON lc.id = lj.lead_id
    WHERE lc.status <> 'in_classes'
       OR lc.sent_to_classes = false

    UNION ALL

    SELECT
        'multiple_active_payment_cycles',
        'HIGH',
        l.id,
        l.full_name,
        l.phone,
        l.status,
        'Close older active cycles and keep only latest one active.',
        format(
            $$-- Review first, then run if correct:
WITH ranked AS (
  SELECT id, lead_id, started_at,
         ROW_NUMBER() OVER (PARTITION BY lead_id ORDER BY started_at DESC, created_at DESC) AS rn
  FROM payment_cycles
  WHERE lead_id = '%s' AND status = 'active'
)
UPDATE payment_cycles pc
SET status = 'closed',
    closed_at = NOW(),
    updated_at = NOW()
FROM ranked r
WHERE pc.id = r.id
  AND r.rn > 1;$$,
            l.id::text
        )
    FROM leads l
    JOIN (
        SELECT lead_id
        FROM payment_cycles
        WHERE status = 'active'
        GROUP BY lead_id
        HAVING COUNT(*) > 1
    ) x ON x.lead_id = l.id

    UNION ALL

    SELECT
        'refunds_exceed_payments',
        'HIGH',
        l.id,
        l.full_name,
        l.phone,
        l.status,
        'Manual finance audit required: refunds exceed payments. Do not auto-fix.',
        format(
            $$-- Investigation query:
SELECT
  t.transaction_date,
  t.transaction_type,
  t.category,
  t.amount,
  t.payment_method,
  t.ref_key,
  t.notes
FROM transactions t
WHERE t.lead_id = '%s'
ORDER BY t.transaction_date, t.created_at;$$,
            l.id::text
        )
    FROM leads l
    JOIN refund_totals rt ON rt.lead_id = l.id
    WHERE rt.refund_total > rt.paid_total

    UNION ALL

    SELECT
        'returning_levels_underflow',
        'MEDIUM',
        lc.id,
        lc.full_name,
        lc.phone,
        lc.status,
        'Set levels_purchased_total to at least levels_consumed to remove underflow.',
        format(
            $$-- Review first, then run if correct:
UPDATE leads
SET levels_purchased_total = COALESCE(levels_consumed, 0),
    remaining_credits = 0,
    updated_at = NOW()
WHERE id = '%s'
  AND COALESCE(levels_purchased_total, 0) < COALESCE(levels_consumed, 0);$$,
            lc.id::text
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
    suggested_action,
    suggested_sql
FROM suggestions
ORDER BY
    CASE severity WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END,
    check_name,
    full_name;

\echo '=== Invariants Suggestions: Summary Counts ==='

WITH suggestions AS (
    SELECT 'returning_in_test_stage'::text AS check_name, 'HIGH'::text AS severity
    FROM leads l
    WHERE COALESCE(l.is_returning, false) = true
      AND l.status IN ('lead_created', 'test_booked', 'tested')

    UNION ALL
    SELECT 'renewal_pending_with_remaining_credits', 'HIGH'
    FROM leads l
    WHERE COALESCE(l.is_returning, false) = true
      AND l.status = 'renewal_pending'
      AND GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) > 0

    UNION ALL
    SELECT 'in_classes_not_sent_to_classes', 'HIGH'
    FROM leads l
    WHERE l.status = 'in_classes' AND COALESCE(l.sent_to_classes, false) = false

    UNION ALL
    SELECT 'late_joiner_state_invariant_broken', 'HIGH'
    FROM late_joiners lj
    JOIN leads l ON l.id = lj.lead_id
    WHERE l.status <> 'in_classes' OR COALESCE(l.sent_to_classes, false) = false

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
    FROM leads l
    WHERE COALESCE(l.is_returning, false) = true
      AND COALESCE(l.levels_purchased_total, 0) < COALESCE(l.levels_consumed, 0)
)
SELECT
    severity,
    check_name,
    COUNT(*) AS suggestion_count
FROM suggestions
GROUP BY severity, check_name
ORDER BY
    CASE severity WHEN 'HIGH' THEN 1 WHEN 'MEDIUM' THEN 2 ELSE 3 END,
    check_name;
