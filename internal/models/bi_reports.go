package models

import (
	"database/sql"
	"fmt"
	"time"

	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/util"
)

type BISalesConversion struct {
	TestBookedCount int     `json:"test_booked_count"`
	ConvertedCount  int     `json:"converted_count"`
	ConversionRate  float64 `json:"conversion_rate"`
}

type BIBottleneckLead struct {
	LeadID       string `json:"lead_id"`
	FullName     string `json:"full_name"`
	Phone        string `json:"phone"`
	Status       string `json:"status"`
	DaysInStatus int    `json:"days_in_status"`
}

type BIRenewalRate struct {
	ReturningCount int     `json:"returning_count"`
	RenewedCount   int     `json:"renewed_count"`
	RenewalRate    float64 `json:"renewal_rate"`
}

type BIGhostStudent struct {
	LeadID      string `json:"lead_id"`
	FullName    string `json:"full_name"`
	Phone       string `json:"phone"`
	OfferPrice  int32  `json:"offer_price"`
	TotalPaid   int32  `json:"total_paid"`
	Shortfall   int32  `json:"shortfall"`
	ClassStatus string `json:"class_status"`
}

type BIRefundLiability struct {
	StudentsCount int    `json:"students_count"`
	TotalValue    int32  `json:"total_value"`
	PricingModel  string `json:"pricing_model"`
}

type BIRevenuePulse struct {
	ActiveRoundStart *time.Time `json:"active_round_start,omitempty"`
	TotalCollected   int32      `json:"total_collected"`
}

type BIRetentionStudent struct {
	LeadID           string `json:"lead_id"`
	FullName         string `json:"full_name"`
	Phone            string `json:"phone"`
	Status           string `json:"status"`
	RemainingCredits int32  `json:"remaining_credits"`
	LastLevel        int32  `json:"last_level"`
	LastCompletedAt  string `json:"last_completed_at"`
}

type BIActiveClassesMonth struct {
	Month        string `json:"month"`
	ClassesCount int    `json:"classes_count"`
}

type BIReportPayload struct {
	GeneratedAt time.Time `json:"generated_at"`
	Filters     struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"filters"`
	Report1 struct {
		Conversion BISalesConversion  `json:"conversion"`
		Bottleneck []BIBottleneckLead `json:"bottleneck"`
		Renewal    BIRenewalRate      `json:"renewal"`
	} `json:"report1"`
	Report2 struct {
		GhostStudents   []BIGhostStudent  `json:"ghost_students"`
		RefundLiability BIRefundLiability `json:"refund_liability"`
		RevenuePulse    BIRevenuePulse    `json:"revenue_pulse"`
	} `json:"report2"`
	Report3 struct {
		Lost    []BIRetentionStudent `json:"lost"`
		Stalled []BIRetentionStudent `json:"stalled"`
	} `json:"report3"`
	Report4 struct {
		ActiveClassesByMonth []BIActiveClassesMonth `json:"active_classes_by_month"`
		StartedLearners      int                    `json:"started_learners"`
		FinishedLearners     int                    `json:"finished_learners"`
	} `json:"report4"`
}

func GetBIReportPayload(fromDate, toDate time.Time) (*BIReportPayload, error) {
	payload := &BIReportPayload{
		GeneratedAt: util.CairoNow(),
	}
	payload.Filters.From = fromDate.Format("2006-01-02")
	payload.Filters.To = toDate.Format("2006-01-02")

	if err := loadSalesPipelineMetrics(payload); err != nil {
		return nil, err
	}
	if err := loadFinancialMetrics(payload); err != nil {
		return nil, err
	}
	if err := loadRetentionMetrics(payload); err != nil {
		return nil, err
	}
	if err := loadOperationalVolumeMetrics(payload, fromDate, toDate); err != nil {
		return nil, err
	}

	return payload, nil
}

func loadSalesPipelineMetrics(payload *BIReportPayload) error {
	var testBooked, converted int
	err := db.DB.QueryRow(`
		WITH recent_tests AS (
			SELECT DISTINCT pt.lead_id
			FROM placement_tests pt
			WHERE pt.test_date IS NOT NULL
			  AND pt.test_date >= (CURRENT_DATE - INTERVAL '30 days')
		)
		SELECT
			COUNT(*)::int AS test_booked_count,
			COUNT(*) FILTER (
				WHERE EXISTS (
					SELECT 1
					FROM lead_payments lp
					WHERE lp.lead_id = rt.lead_id
					  AND lp.payment_date >= (CURRENT_DATE - INTERVAL '30 days')
					  AND lp.amount > 0
				)
			)::int AS converted_count
		FROM recent_tests rt
	`).Scan(&testBooked, &converted)
	if err != nil {
		return fmt.Errorf("failed to load sales conversion: %w", err)
	}
	payload.Report1.Conversion.TestBookedCount = testBooked
	payload.Report1.Conversion.ConvertedCount = converted
	if testBooked > 0 {
		payload.Report1.Conversion.ConversionRate = (float64(converted) / float64(testBooked)) * 100
	}

	rows, err := db.DB.Query(`
		SELECT id::text, full_name, phone, status, GREATEST((CURRENT_DATE - COALESCE(offer_sent_at::date, updated_at::date))::int, 0) AS days_in_status
		FROM leads
		WHERE status = 'offer_sent'
		  AND COALESCE(offer_sent_at, updated_at) <= NOW() - INTERVAL '3 days'
		ORDER BY COALESCE(offer_sent_at, updated_at) ASC
	`)
	if err != nil {
		return fmt.Errorf("failed to load sales bottleneck: %w", err)
	}
	defer func() { _ = rows.Close() }()

	bottleneck := make([]BIBottleneckLead, 0)
	for rows.Next() {
		var item BIBottleneckLead
		if err := rows.Scan(&item.LeadID, &item.FullName, &item.Phone, &item.Status, &item.DaysInStatus); err != nil {
			return fmt.Errorf("failed to scan bottleneck lead: %w", err)
		}
		bottleneck = append(bottleneck, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate bottleneck leads: %w", err)
	}
	payload.Report1.Bottleneck = bottleneck

	var returningCount, renewedCount int
	err = db.DB.QueryRow(`
		WITH returning_leads AS (
			SELECT l.id
			FROM leads l
			WHERE COALESCE(l.is_returning, false) = true
		)
		SELECT
			COUNT(*)::int AS returning_count,
			COUNT(*) FILTER (
				WHERE EXISTS (
					SELECT 1
					FROM lead_payments lp
					WHERE lp.lead_id = r.id
					  AND lp.amount > 0
					  AND lp.payment_date >= COALESCE(
						(
							SELECT MAX(ce.completed_at)::date
							FROM class_enrollments ce
							WHERE ce.lead_id = r.id
						),
						DATE '1970-01-01'
					  )
				)
			)::int AS renewed_count
		FROM returning_leads r
	`).Scan(&returningCount, &renewedCount)
	if err != nil {
		return fmt.Errorf("failed to load renewal rate: %w", err)
	}
	payload.Report1.Renewal.ReturningCount = returningCount
	payload.Report1.Renewal.RenewedCount = renewedCount
	if returningCount > 0 {
		payload.Report1.Renewal.RenewalRate = (float64(renewedCount) / float64(returningCount)) * 100
	}

	return nil
}

func loadFinancialMetrics(payload *BIReportPayload) error {
	rows, err := db.DB.Query(`
		WITH in_class AS (
			SELECT
				l.id,
				l.full_name,
				l.phone,
				l.status,
				COALESCE(pc.final_price, o.final_price, 0) AS offer_price,
				COALESCE(
					(
						SELECT SUM(lp.amount)
						FROM lead_payments lp
						WHERE lp.lead_id = l.id
						  AND lp.payment_date >= COALESCE(pc.started_at::date, DATE '1970-01-01')
					),
					0
				) AS total_paid
			FROM leads l
			LEFT JOIN payment_cycles pc ON pc.lead_id = l.id AND pc.status = 'active'
			LEFT JOIN offers o ON o.lead_id = l.id
			WHERE l.status = 'in_classes'
		)
		SELECT
			id::text,
			full_name,
			phone,
			offer_price::int,
			total_paid::int,
			GREATEST((offer_price - total_paid), 0)::int AS shortfall,
			status
		FROM in_class
		WHERE offer_price > 0
		  AND total_paid < offer_price
		ORDER BY shortfall DESC, full_name ASC
	`)
	if err != nil {
		return fmt.Errorf("failed to load ghost students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ghosts := make([]BIGhostStudent, 0)
	for rows.Next() {
		var item BIGhostStudent
		if err := rows.Scan(&item.LeadID, &item.FullName, &item.Phone, &item.OfferPrice, &item.TotalPaid, &item.Shortfall, &item.ClassStatus); err != nil {
			return fmt.Errorf("failed to scan ghost student: %w", err)
		}
		ghosts = append(ghosts, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate ghost students: %w", err)
	}
	payload.Report2.GhostStudents = ghosts

	var liabilityTotal int32
	var liabilityCount int
	err = db.DB.QueryRow(`
		WITH credits AS (
			SELECT
				GREATEST(COALESCE(levels_purchased_total, 0) - COALESCE(levels_consumed, 0), 0) AS remaining_credits,
				CASE
					WHEN bundle_type = 'bundle4' THEN 1000
					WHEN bundle_type = 'bundle3' THEN 1100
					WHEN bundle_type = 'bundle2' THEN 1200
					ELSE 1250
				END AS per_credit_price
			FROM leads
			WHERE status <> 'cancelled'
		)
		SELECT
			COALESCE(SUM(remaining_credits * per_credit_price), 0)::int,
			COUNT(*) FILTER (WHERE remaining_credits > 0)::int
		FROM credits
	`).Scan(&liabilityTotal, &liabilityCount)
	if err != nil {
		return fmt.Errorf("failed to load refund liability: %w", err)
	}
	payload.Report2.RefundLiability.StudentsCount = liabilityCount
	payload.Report2.RefundLiability.TotalValue = liabilityTotal
	payload.Report2.RefundLiability.PricingModel = "bundle_weighted"

	var activeRoundStart sql.NullTime
	err = db.DB.QueryRow(`
		SELECT MIN(round_started_at)
		FROM class_groups
		WHERE round_status = 'active'
		  AND round_started_at IS NOT NULL
	`).Scan(&activeRoundStart)
	if err != nil {
		return fmt.Errorf("failed to load active round start: %w", err)
	}

	if activeRoundStart.Valid {
		start := activeRoundStart.Time
		payload.Report2.RevenuePulse.ActiveRoundStart = &start

		var pulseTotal int32
		err = db.DB.QueryRow(`
			SELECT COALESCE(SUM(amount), 0)::int
			FROM lead_payments
			WHERE payment_date >= $1::date
		`, activeRoundStart.Time).Scan(&pulseTotal)
		if err != nil {
			return fmt.Errorf("failed to load revenue pulse: %w", err)
		}
		payload.Report2.RevenuePulse.TotalCollected = pulseTotal
	}

	return nil
}

func loadRetentionMetrics(payload *BIReportPayload) error {
	lostRows, err := db.DB.Query(`
		WITH latest_enrollment AS (
			SELECT DISTINCT ON (ce.lead_id)
				ce.lead_id,
				ce.level,
				ce.completed_at
			FROM class_enrollments ce
			WHERE ce.completed_at IS NOT NULL
			ORDER BY ce.lead_id, ce.completed_at DESC
		)
		SELECT
			l.id::text,
			l.full_name,
			l.phone,
			l.status,
			GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0)::int AS remaining_credits,
			le.level::int,
			le.completed_at::date::text
		FROM leads l
		INNER JOIN latest_enrollment le ON le.lead_id = l.id
		WHERE l.status = 'renewal_pending'
		  AND GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) <= 0
		  AND le.completed_at >= NOW() - INTERVAL '30 days'
		ORDER BY le.completed_at DESC
	`)
	if err != nil {
		return fmt.Errorf("failed to load lost students: %w", err)
	}
	defer func() { _ = lostRows.Close() }()

	lost := make([]BIRetentionStudent, 0)
	for lostRows.Next() {
		var item BIRetentionStudent
		if err := lostRows.Scan(&item.LeadID, &item.FullName, &item.Phone, &item.Status, &item.RemainingCredits, &item.LastLevel, &item.LastCompletedAt); err != nil {
			return fmt.Errorf("failed to scan lost student: %w", err)
		}
		lost = append(lost, item)
	}
	if err := lostRows.Err(); err != nil {
		return fmt.Errorf("failed to iterate lost students: %w", err)
	}
	payload.Report3.Lost = lost

	stalledRows, err := db.DB.Query(`
		WITH latest_enrollment AS (
			SELECT DISTINCT ON (ce.lead_id)
				ce.lead_id,
				ce.level,
				ce.completed_at
			FROM class_enrollments ce
			WHERE ce.completed_at IS NOT NULL
			ORDER BY ce.lead_id, ce.completed_at DESC
		)
		SELECT
			l.id::text,
			l.full_name,
			l.phone,
			l.status,
			GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0)::int AS remaining_credits,
			le.level::int,
			le.completed_at::date::text
		FROM leads l
		INNER JOIN latest_enrollment le ON le.lead_id = l.id
		WHERE l.status = 'waiting_for_round'
		  AND GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) > 0
		ORDER BY le.completed_at DESC
	`)
	if err != nil {
		return fmt.Errorf("failed to load stalled students: %w", err)
	}
	defer func() { _ = stalledRows.Close() }()

	stalled := make([]BIRetentionStudent, 0)
	for stalledRows.Next() {
		var item BIRetentionStudent
		if err := stalledRows.Scan(&item.LeadID, &item.FullName, &item.Phone, &item.Status, &item.RemainingCredits, &item.LastLevel, &item.LastCompletedAt); err != nil {
			return fmt.Errorf("failed to scan stalled student: %w", err)
		}
		stalled = append(stalled, item)
	}
	if err := stalledRows.Err(); err != nil {
		return fmt.Errorf("failed to iterate stalled students: %w", err)
	}
	payload.Report3.Stalled = stalled

	return nil
}

func loadOperationalVolumeMetrics(payload *BIReportPayload, fromDate, toDate time.Time) error {
	rows, err := db.DB.Query(`
		WITH months AS (
			SELECT generate_series(
				date_trunc('month', $1::date)::date,
				date_trunc('month', $2::date)::date,
				interval '1 month'
			)::date AS month_start
		),
		class_periods AS (
			SELECT
				cg.class_key,
				cg.round_started_at::date AS start_date,
				COALESCE(cg.round_closed_at::date, CURRENT_DATE) AS end_date
			FROM class_groups cg
			WHERE cg.round_started_at IS NOT NULL
		)
		SELECT
			to_char(m.month_start, 'YYYY-MM') AS month_key,
			COUNT(DISTINCT cp.class_key)::int AS classes_count
		FROM months m
		LEFT JOIN class_periods cp
			ON cp.start_date < (m.month_start + INTERVAL '1 month')::date
		   AND cp.end_date >= m.month_start
		GROUP BY m.month_start
		ORDER BY m.month_start ASC
	`, fromDate, toDate)
	if err != nil {
		return fmt.Errorf("failed to load active classes trend: %w", err)
	}
	defer func() { _ = rows.Close() }()

	trend := make([]BIActiveClassesMonth, 0)
	for rows.Next() {
		var item BIActiveClassesMonth
		if err := rows.Scan(&item.Month, &item.ClassesCount); err != nil {
			return fmt.Errorf("failed to scan active class trend row: %w", err)
		}
		trend = append(trend, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate active class trend rows: %w", err)
	}
	payload.Report4.ActiveClassesByMonth = trend

	var startedLearners int
	err = db.DB.QueryRow(`
		SELECT COUNT(DISTINCT ce.lead_id)::int
		FROM class_enrollments ce
		WHERE ce.enrolled_at::date BETWEEN $1::date AND $2::date
	`, fromDate, toDate).Scan(&startedLearners)
	if err != nil {
		return fmt.Errorf("failed to load started learners count: %w", err)
	}
	payload.Report4.StartedLearners = startedLearners

	var finishedLearners int
	err = db.DB.QueryRow(`
		SELECT COUNT(DISTINCT ce.lead_id)::int
		FROM class_enrollments ce
		WHERE ce.completed_at::date BETWEEN $1::date AND $2::date
	`, fromDate, toDate).Scan(&finishedLearners)
	if err != nil {
		return fmt.Errorf("failed to load finished learners count: %w", err)
	}
	payload.Report4.FinishedLearners = finishedLearners

	return nil
}
