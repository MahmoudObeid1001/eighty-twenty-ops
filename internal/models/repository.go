package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/util"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Workflow stage constants - the 8 official stages
const (
	StageNewLead                  = "NEW_LEAD"
	StageTestBooked               = "TEST_BOOKED"
	StageTested                   = "TESTED"
	StageOfferSent                = "OFFER_SENT"
	StageBookingConfirmedPaidFull = "BOOKING_CONFIRMED_PAID_FULL"
	StageBookingConfirmedDeposit  = "BOOKING_CONFIRMED_DEPOSIT"
	StageScheduleSet              = "SCHEDULE_SET"
	StageReadyToStart             = "READY_TO_START"
	StageRenewalPending           = "RENEWAL_PENDING"
	StageWaitingForRound          = "WAITING_FOR_ROUND"
	StageColdLead                 = "COLD_LEAD"
)

var ErrAttendanceDeadlinePassed = errors.New("attendance deadline passed")
var ErrAttendanceIncomplete = errors.New("attendance incomplete")

// Payment state constants
const (
	PaymentStateUnpaid   = "UNPAID"
	PaymentStateDeposit  = "DEPOSIT"
	PaymentStatePaidFull = "PAID_FULL"
)

// MapOldStatusToStage maps legacy status values to new workflow stages for backward compatibility
// Old statuses that don't map directly are converted to the nearest equivalent stage
func MapOldStatusToStage(oldStatus string) string {
	mapping := map[string]string{
		// Direct mappings
		"lead_created":   StageNewLead,
		"test_booked":    StageTestBooked,
		"tested":         StageTested,
		"offer_sent":     StageOfferSent,
		"ready_to_start": StageReadyToStart,
		// Payment-based statuses need context (handled separately with payment state)
		"booking_confirmed": StageOfferSent, // Default mapping, will be upgraded based on payment
		"paid_full":         StageBookingConfirmedPaidFull,
		"deposit_paid":      StageBookingConfirmedDeposit,
		// Schedule-based statuses
		"waiting_for_round": StageWaitingForRound,
		"schedule_assigned": StageScheduleSet,
		"renewal_pending":   StageRenewalPending,
		"cold_lead":         StageColdLead,
	}

	if mapped, ok := mapping[oldStatus]; ok {
		return mapped
	}
	// Default: treat unknown status as new lead
	return StageNewLead
}

// GetPaymentState computes payment state from amount_paid and final_price
// Returns: UNPAID, DEPOSIT, or PAID_FULL
func GetPaymentState(amountPaid sql.NullInt32, finalPrice sql.NullInt32) string {
	if !amountPaid.Valid || amountPaid.Int32 == 0 {
		return PaymentStateUnpaid
	}

	// If final price is known, compare
	if finalPrice.Valid && finalPrice.Int32 > 0 {
		if amountPaid.Int32 >= finalPrice.Int32 {
			return PaymentStatePaidFull
		}
		return PaymentStateDeposit
	}

	// If final price unknown but amount paid > 0, check remaining balance
	// For now, if amount_paid > 0, consider it at least a deposit
	// In practice, we'd need remaining_balance to determine if it's full or deposit
	// Defaulting to DEPOSIT if we can't determine
	return PaymentStateDeposit
}

// ComputeStageFromFormCompletion computes the appropriate workflow stage based on form completion
// Rules: Compute stage from the furthest completed block, never downgrade
// Returns the new stage and the old status (for DB compatibility)
func ComputeStageFromFormCompletion(detail *LeadDetail, currentStatus string) (newStage string, dbStatus string) {
	// Start with current stage (mapped from old status)
	currentStage := MapOldStatusToStage(currentStatus)

	// Stage progression rules (check furthest completed block)

	// 1. If test date + test time exist -> at least TEST_BOOKED
	if detail.PlacementTest != nil && detail.PlacementTest.TestDate.Valid && detail.PlacementTest.TestTime.Valid {
		if currentStage == StageNewLead {
			currentStage = StageTestBooked
		}
	}

	// 2. If assigned level exists (and/or test notes exist) -> at least TESTED
	if detail.PlacementTest != nil && detail.PlacementTest.AssignedLevel.Valid {
		stagesBeforeTested := map[string]bool{
			StageNewLead:    true,
			StageTestBooked: true,
		}
		if stagesBeforeTested[currentStage] {
			currentStage = StageTested
		}
	}

	// 3. If offer final price exists (or bundle selected + final price) -> at least OFFER_SENT
	if detail.Offer != nil && detail.Offer.FinalPrice.Valid && detail.Offer.FinalPrice.Int32 > 0 {
		stagesBeforeOfferSent := map[string]bool{
			StageNewLead:        true,
			StageTestBooked:     true,
			StageTested:         true,
			StageRenewalPending: true, // Allow renewal_pending → offer_sent when offer is saved
		}
		if stagesBeforeOfferSent[currentStage] {
			currentStage = StageOfferSent
		}
	}

	// 4. If payment amount exists:
	//    - if amountPaid >= finalPrice -> BOOKING_CONFIRMED_PAID_FULL
	//    - else if amountPaid > 0 -> BOOKING_CONFIRMED_DEPOSIT
	if detail.Payment != nil && detail.Payment.AmountPaid.Valid && detail.Payment.AmountPaid.Int32 > 0 {
		var finalPrice int32 = 0
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			finalPrice = detail.Offer.FinalPrice.Int32
		}

		if finalPrice > 0 && detail.Payment.AmountPaid.Int32 >= finalPrice {
			// Paid in full
			stagesBeforePaidFull := map[string]bool{
				StageNewLead:                 true,
				StageTestBooked:              true,
				StageTested:                  true,
				StageOfferSent:               true,
				StageBookingConfirmedDeposit: true,
			}
			if stagesBeforePaidFull[currentStage] {
				currentStage = StageBookingConfirmedPaidFull
			}
		} else if detail.Payment.AmountPaid.Int32 > 0 {
			// Deposit paid
			stagesBeforeDeposit := map[string]bool{
				StageNewLead:    true,
				StageTestBooked: true,
				StageTested:     true,
				StageOfferSent:  true,
			}
			if stagesBeforeDeposit[currentStage] {
				currentStage = StageBookingConfirmedDeposit
			}
		}
	}

	// 5. If schedule (class days + class time) selected -> SCHEDULE_SET, then READY_TO_START
	if detail.Scheduling != nil && detail.Scheduling.ClassDays.Valid && detail.Scheduling.ClassTime.Valid {
		// First upgrade to SCHEDULE_SET if before it
		stagesBeforeSchedule := map[string]bool{
			StageNewLead:                  true,
			StageTestBooked:               true,
			StageTested:                   true,
			StageOfferSent:                true,
			StageBookingConfirmedPaidFull: true,
			StageBookingConfirmedDeposit:  true,
		}
		if stagesBeforeSchedule[currentStage] {
			currentStage = StageScheduleSet
		}

		// Then upgrade to READY_TO_START (schedule fully filled)
		stagesBeforeReady := map[string]bool{
			StageNewLead:                  true,
			StageTestBooked:               true,
			StageTested:                   true,
			StageOfferSent:                true,
			StageBookingConfirmedPaidFull: true,
			StageBookingConfirmedDeposit:  true,
			StageScheduleSet:              true,
		}
		if stagesBeforeReady[currentStage] {
			currentStage = StageReadyToStart
		}
	}

	// Map new stage back to DB status for storage
	stageToStatusMap := map[string]string{
		StageNewLead:                  "lead_created",
		StageTestBooked:               "test_booked",
		StageTested:                   "tested",
		StageOfferSent:                "offer_sent",
		StageBookingConfirmedPaidFull: "paid_full",
		StageBookingConfirmedDeposit:  "deposit_paid",
		StageScheduleSet:              "schedule_assigned",
		StageReadyToStart:             "ready_to_start",
	}

	dbStatus = stageToStatusMap[currentStage]
	if dbStatus == "" {
		dbStatus = "lead_created" // Fallback
	}

	return currentStage, dbStatus
}

func GetNextAction(status string) string {
	// Map to canonical stage first for consistent actions
	stage := MapOldStatusToStage(status)

	actions := map[string]string{
		StageNewLead:                  "Book placement test",
		StageTestBooked:               "Run placement test",
		StageTested:                   "Send offer",
		StageOfferSent:                "Wait for booking",
		StageBookingConfirmedPaidFull: "Assign schedule",
		StageBookingConfirmedDeposit:  "Collect remaining",
		StageScheduleSet:              "Mark ready to start",
		StageReadyToStart:             "Ready for activation",
		StageRenewalPending:           "Collect renewal payment",
		StageWaitingForRound:          "Wait for round start",
		StageColdLead:                 "Retarget lead",
	}
	if action, ok := actions[stage]; ok {
		return action
	}
	return "Review"
}

// ComputeLeadFlags computes hot lead flags based on status and payment.
// Business definition: Hot Lead = (status = TESTED OR OFFER_SENT) AND payment_state = UNPAID.
// All such leads are hot immediately (no 2-day gate): they appear in Hot Leads filter, banner count, and detail callout.
// Days since progress are used only for HotLevel (HOT/WARM/COOL) and suggested next action.
func ComputeLeadFlags(item *LeadListItem) {
	// Map to canonical stage for consistent checking
	stage := MapOldStatusToStage(item.Lead.Status)

	// Hot lead stages: only TESTED and OFFER_SENT qualify
	hotLeadStages := map[string]bool{
		StageTested:    true,
		StageOfferSent: true,
	}

	// Check if lead has qualifying stage
	if !hotLeadStages[stage] {
		item.HotLevel = ""
		item.FollowUpDue = false
		item.DaysSinceLastProgress = 0
		return
	}

	// Compute payment state using final_price if available
	paymentState := GetPaymentState(item.AmountPaid, item.FinalPrice)
	item.PaymentState = paymentState // Store for filtering

	if paymentState != PaymentStateUnpaid {
		// Lead has paid (deposit or full), so not a hot lead
		item.HotLevel = ""
		item.FollowUpDue = false
		item.DaysSinceLastProgress = 0
		return
	}

	// Calculate days since last progress using stage-aware anchors:
	// - TESTED: placement test date (or updated fallback)
	// - OFFER_SENT: offer_sent_at (or updated fallback)
	var progressTime time.Time
	if stage == StageTested && item.TestDate.Valid {
		progressTime = item.TestDate.Time
	} else if stage == StageOfferSent && item.Lead.OfferSentAt.Valid {
		progressTime = item.Lead.OfferSentAt.Time
	} else {
		progressTime = item.Lead.UpdatedAt
		if progressTime.IsZero() {
			progressTime = item.Lead.CreatedAt
		}
	}

	now := time.Now()
	daysSince := int(now.Sub(progressTime).Hours() / 24)
	if daysSince < 0 {
		daysSince = 0
	}
	item.DaysSinceLastProgress = daysSince

	// All TESTED/OFFER_SENT + UNPAID leads are hot: include in filter, banner, and detail callout
	item.FollowUpDue = true

	// HotLevel by days: 0–6 HOT, 7–13 WARM, 14+ COOL (just-tested leads are HOT)
	if daysSince <= 6 {
		item.HotLevel = "HOT"
		item.NextAction = "Follow-up due - Call today"
	} else if daysSince <= 13 {
		item.HotLevel = "WARM"
		item.NextAction = "Follow-up due - Offer discount"
	} else {
		item.HotLevel = "COOL"
		item.NextAction = "Follow-up due - Final check"
	}
}

func GetPaymentStatus(remainingBalance, amountPaid sql.NullInt32) string {
	if remainingBalance.Valid && remainingBalance.Int32 > 0 {
		return "Deposit"
	}
	if amountPaid.Valid && amountPaid.Int32 > 0 && (!remainingBalance.Valid || remainingBalance.Int32 == 0) {
		return "Paid full"
	}
	return "Unpaid"
}

func GetAllLeads(statusFilter, searchFilter, paymentFilter, hotFilter string, includeCancelled bool, followUpFilter string, returningFilter string, coldFilter string, repeatFilter string) ([]*LeadListItem, error) {
	query := `
		SELECT 
			l.id, l.full_name, l.phone, l.source, l.notes, l.status, l.sent_to_classes,
			COALESCE(l.is_returning, false),
			GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS remaining_credits_calc,
			l.created_by_user_id, l.offer_sent_at, l.created_at, l.updated_at,
			pt.assigned_level, pt.test_date,
			p.remaining_balance, p.amount_paid,
			o.final_price,
			COALESCE(last_ce.outcome, '') as last_outcome,
			COALESCE(last_ce.final_grade, '') as last_final_grade,
			COALESCE(last_refusal.refused_at, NULL) as refused_renewal_at
		FROM leads l
		LEFT JOIN placement_tests pt ON l.id = pt.lead_id
		LEFT JOIN payments p ON l.id = p.lead_id
		LEFT JOIN offers o ON l.id = o.lead_id
		LEFT JOIN LATERAL (
			SELECT outcome, final_grade
			FROM class_enrollments ce
			WHERE ce.lead_id = l.id
			ORDER BY COALESCE(ce.completed_at, ce.enrolled_at) DESC
			LIMIT 1
		) last_ce ON true
		LEFT JOIN LATERAL (
			SELECT refused_at
			FROM renewal_refusals rr
			WHERE rr.lead_id = l.id
			ORDER BY rr.refused_at DESC
			LIMIT 1
		) last_refusal ON true
		WHERE 1=1
		AND l.status != 'in_classes'
		AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
	`

	args := []interface{}{}
	argIndex := 1

	// Apply follow-up filter (high priority follow-up)
	if followUpFilter == "high_priority" {
		query += " AND (l.high_priority_follow_up = true OR (COALESCE(l.levels_purchased_total, 0) > 0 AND GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) <= 0))"
	}

	// Apply returning filter
	if returningFilter == "1" || returningFilter == "true" {
		query += " AND GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) > 0"
	}

	// Apply cold leads filter:
	// - explicit cold bucket (status = cold_lead), or
	// - cold candidates still in offer_sent for 7+ days with no remaining credits.
	if coldFilter == "1" || coldFilter == "true" {
		query += " AND (l.status = 'cold_lead' OR (l.status = 'offer_sent' AND COALESCE(l.offer_sent_at, l.updated_at) <= NOW() - INTERVAL '7 days' AND COALESCE(l.levels_purchased_total, 0) > 0 AND GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) <= 0))"
	} else if !strings.EqualFold(statusFilter, "cold_lead") && !strings.EqualFold(statusFilter, StageColdLead) {
		// Keep cold backlog isolated from default pipeline feed unless user explicitly enters cold mode.
		query += " AND l.status != 'cold_lead'"
	}

	// Apply repeat filter
	if repeatFilter == "1" || repeatFilter == "true" {
		query += " AND last_ce.outcome = 'repeated'"
	}

	// Exclude cancelled by default. Include if includeCancelled=true OR explicitly filtering by status=cancelled.
	excludeCancelled := !includeCancelled && statusFilter != "cancelled"
	if excludeCancelled {
		query += " AND l.status != 'cancelled'"
	}

	// Apply status filter - map new stage names to old status values for DB query
	if statusFilter != "" {
		// Map new stage constants to old DB status values
		stageToStatusMap := map[string]string{
			StageNewLead:                  "lead_created",
			StageTestBooked:               "test_booked",
			StageTested:                   "tested",
			StageOfferSent:                "offer_sent",
			StageBookingConfirmedPaidFull: "paid_full",
			StageBookingConfirmedDeposit:  "deposit_paid",
			StageScheduleSet:              "schedule_assigned",
			StageReadyToStart:             "ready_to_start",
			StageRenewalPending:           "renewal_pending",
			StageWaitingForRound:          "waiting_for_round",
			StageColdLead:                 "cold_lead",
		}

		// If it's a new stage constant, map it; otherwise use as-is (backward compat)
		dbStatus := statusFilter
		if mapped, ok := stageToStatusMap[statusFilter]; ok {
			dbStatus = mapped
		}

		// trust the status in DB
		query += fmt.Sprintf(" AND l.status = $%d", argIndex)
		args = append(args, dbStatus)
		argIndex++
	}

	// Apply search filter (name or phone)
	if searchFilter != "" {
		query += fmt.Sprintf(" AND (LOWER(l.full_name) LIKE LOWER($%d) OR l.phone LIKE $%d)", argIndex, argIndex)
		searchPattern := "%" + searchFilter + "%"
		args = append(args, searchPattern)
	}

	// Default sorting (unless hot filter is active, then we sort after computing flags in Go)
	if hotFilter != "hot" && hotFilter != "1" {
		if statusFilter == "cancelled" {
			// Cancelled view should show most recently cancelled leads first.
			query += " ORDER BY l.cancelled_at DESC NULLS LAST, l.updated_at DESC"
		} else {
			// Main pre-enrolment feed should rank by latest activity.
			query += " ORDER BY l.updated_at DESC, l.created_at DESC"
		}
	} else {
		// For hot filter, we'll sort in Go after computing flags, but still need an ORDER BY for SQL
		query += " ORDER BY l.updated_at DESC, l.created_at DESC"
	}

	var rows *sql.Rows
	var err error
	if len(args) > 0 {
		rows, err = db.DB.Query(query, args...)
	} else {
		rows, err = db.DB.Query(query)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query leads: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var leads []*LeadListItem
	for rows.Next() {
		lead := &Lead{}
		var assignedLevel sql.NullInt32
		var remainingBalance, amountPaid, finalPrice sql.NullInt32
		var testDate sql.NullTime
		var lastOutcome sql.NullString
		var lastFinalGrade sql.NullString
		var refusedRenewalAt sql.NullTime

		err := rows.Scan(
			&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status, &lead.SentToClasses,
			&lead.IsReturning, &lead.RemainingCredits,
			&lead.CreatedByUserID, &lead.OfferSentAt, &lead.CreatedAt, &lead.UpdatedAt,
			&assignedLevel, &testDate,
			&remainingBalance, &amountPaid,
			&finalPrice,
			&lastOutcome,
			&lastFinalGrade,
			&refusedRenewalAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lead: %w", err)
		}

		// Compute payment state
		paymentState := GetPaymentState(amountPaid, finalPrice)
		// Returning students in waiting flow already have prepaid entitlement (credits).
		// Show them as paid-full for Ops list/payment filter even when current-cycle payment snapshot is empty.
		if lead.IsReturning && lead.RemainingCredits.Valid && lead.RemainingCredits.Int32 > 0 {
			switch lead.Status {
			case "waiting_for_round", "schedule_assigned", "ready_to_start":
				paymentState = PaymentStatePaidFull
			}
		}

		item := &LeadListItem{
			Lead:             lead,
			AssignedLevel:    assignedLevel,
			LastOutcome:      lastOutcome,
			LastFinalGrade:   lastFinalGrade,
			RefusedRenewal:   refusedRenewalAt.Valid,
			RefusedRenewalAt: refusedRenewalAt,
			PaymentStatus:    GetPaymentStatus(remainingBalance, amountPaid),
			PaymentState:     paymentState,
			NextAction:       GetNextAction(lead.Status),
			TestDate:         testDate,
			AmountPaid:       amountPaid,
			FinalPrice:       finalPrice,
			RemainingBalance: remainingBalance,
		}

		// Compute hot lead flags (needs finalPrice for proper payment state)
		ComputeLeadFlags(item)

		leads = append(leads, item)
	}

	// Apply payment filter if requested (after computing payment states)
	if paymentFilter != "" {
		var filteredLeads []*LeadListItem
		for _, lead := range leads {
			if lead.PaymentState == paymentFilter {
				filteredLeads = append(filteredLeads, lead)
			}
		}
		leads = filteredLeads
	}

	// Apply hot filter if requested (after payment filter)
	if hotFilter == "hot" || hotFilter == "1" {
		var filteredLeads []*LeadListItem
		for _, lead := range leads {
			if lead.FollowUpDue {
				filteredLeads = append(filteredLeads, lead)
			}
		}
		leads = filteredLeads
	}

	// Sort by hot level and days if hot filter is active
	if hotFilter == "hot" || hotFilter == "1" {
		sort.Slice(leads, func(i, j int) bool {
			// Sort by hot level priority (HOT > WARM > COOL)
			levelPriority := map[string]int{"HOT": 3, "WARM": 2, "COOL": 1, "": 0}
			if levelPriority[leads[i].HotLevel] != levelPriority[leads[j].HotLevel] {
				return levelPriority[leads[i].HotLevel] > levelPriority[leads[j].HotLevel]
			}
			// Then by days descending (most urgent first)
			return leads[i].DaysSinceLastProgress > leads[j].DaysSinceLastProgress
		})
	}

	return leads, nil
}

func GetLatestClassOutcome(leadID uuid.UUID) (*LastClassOutcome, error) {
	out := &LastClassOutcome{}
	err := db.DB.QueryRow(`
		SELECT outcome, final_grade, completed_at
		FROM class_enrollments
		WHERE lead_id = $1
		ORDER BY COALESCE(completed_at, enrolled_at) DESC
		LIMIT 1
	`, leadID).Scan(&out.Outcome, &out.FinalGrade, &out.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest class outcome: %w", err)
	}
	return out, nil
}

func GetLatestClassSchedule(leadID uuid.UUID) (sql.NullString, sql.NullString, error) {
	var classDays sql.NullString
	var classTime sql.NullString
	err := db.DB.QueryRow(`
		SELECT class_days, class_time
		FROM class_enrollments
		WHERE lead_id = $1
		ORDER BY COALESCE(completed_at, enrolled_at) DESC
		LIMIT 1
	`, leadID).Scan(&classDays, &classTime)
	if err == sql.ErrNoRows {
		return sql.NullString{}, sql.NullString{}, nil
	}
	if err != nil {
		return sql.NullString{}, sql.NullString{}, fmt.Errorf("failed to get latest class schedule: %w", err)
	}
	return classDays, classTime, nil
}

func GetLatestClassEnrollment(leadID uuid.UUID) (*ClassEnrollment, error) {
	out := &ClassEnrollment{}
	err := db.DB.QueryRow(`
		SELECT id, lead_id, class_key, level, class_days, 
               TO_CHAR(class_time, 'HH24:MI') as class_time, 
               mentor_name, final_grade, outcome, enrolled_at, completed_at
		FROM class_enrollments
		WHERE lead_id = $1
		ORDER BY COALESCE(completed_at, enrolled_at) DESC
		LIMIT 1
	`, leadID).Scan(&out.ID, &out.LeadID, &out.ClassKey, &out.Level, &out.ClassDays, &out.ClassTime,
		&out.MentorName, &out.FinalGrade, &out.Outcome, &out.EnrolledAt, &out.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest class enrollment: %w", err)
	}
	return out, nil
}

// GetPlacementTestsForStudentSuccess returns leads with placement tests scheduled.
// Used by Student Success dashboard to record test results.
func GetPlacementTestsForStudentSuccess(showCompleted bool) ([]*PlacementTestQueueItem, error) {
	query := `
		SELECT
			l.id, l.full_name, l.phone, l.status,
			pt.test_date, pt.test_time, pt.test_type, pt.assigned_level, pt.test_notes
		FROM leads l
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		WHERE pt.test_date IS NOT NULL
		  AND pt.test_time IS NOT NULL
		  AND l.status != 'cancelled'
		  AND l.status != 'in_classes'
	`
	if showCompleted {
		query += " AND pt.assigned_level IS NOT NULL"
	} else {
		query += " AND pt.assigned_level IS NULL"
	}
	query += `
		ORDER BY pt.test_date ASC, pt.test_time ASC, l.created_at ASC
	`

	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	items := []*PlacementTestQueueItem{}
	for rows.Next() {
		item := &PlacementTestQueueItem{}
		if err := rows.Scan(
			&item.LeadID,
			&item.FullName,
			&item.Phone,
			&item.Status,
			&item.TestDate,
			&item.TestTime,
			&item.TestType,
			&item.AssignedLevel,
			&item.TestNotes,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func GetLeadByID(id uuid.UUID) (*LeadDetail, error) {
	// Get lead
	lead := &Lead{}
	err := db.DB.QueryRow(`
		SELECT id, full_name, phone, source, notes, status, sent_to_classes,
		       levels_purchased_total, levels_consumed, remaining_credits,
		       is_returning, high_priority_follow_up, created_by_user_id, offer_sent_at, created_at, updated_at
		FROM leads WHERE id = $1
	`, id).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status,
		&lead.SentToClasses, &lead.LevelsPurchasedTotal, &lead.LevelsConsumed, &lead.RemainingCredits,
		&lead.IsReturning, &lead.HighPriorityFollowUp, &lead.CreatedByUserID, &lead.OfferSentAt, &lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get lead: %w", err)
	}

	detail := &LeadDetail{Lead: lead}

	// Get placement test
	pt := &PlacementTest{}
	err = db.DB.QueryRow(`
		SELECT id, lead_id, test_date, test_time, test_type, assigned_level, test_notes, run_by_user_id, placement_test_fee, placement_test_fee_paid, placement_test_payment_date, placement_test_payment_method, updated_at
		FROM placement_tests WHERE lead_id = $1
	`, id).Scan(
		&pt.ID, &pt.LeadID, &pt.TestDate, &pt.TestTime, &pt.TestType, &pt.AssignedLevel,
		&pt.TestNotes, &pt.RunByUserID, &pt.PlacementTestFee, &pt.PlacementTestFeePaid, &pt.PlacementTestPaymentDate, &pt.PlacementTestPaymentMethod, &pt.UpdatedAt,
	)
	if err == nil {
		detail.PlacementTest = pt
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get placement test: %w", err)
	}

	// Get offer
	offer := &Offer{}
	err = db.DB.QueryRow(`
		SELECT id, lead_id, bundle_levels, base_price, discount_value, discount_type, final_price, updated_at
		FROM offers WHERE lead_id = $1
	`, id).Scan(
		&offer.ID, &offer.LeadID, &offer.BundleLevels, &offer.BasePrice, &offer.DiscountValue,
		&offer.DiscountType, &offer.FinalPrice, &offer.UpdatedAt,
	)
	if err == nil {
		detail.Offer = offer
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get offer: %w", err)
	}

	// Get booking
	booking := &Booking{}
	err = db.DB.QueryRow(`
		SELECT id, lead_id, book_format, address, city, delivery_notes, updated_at
		FROM bookings WHERE lead_id = $1
	`, id).Scan(
		&booking.ID, &booking.LeadID, &booking.BookFormat, &booking.Address, &booking.City,
		&booking.DeliveryNotes, &booking.UpdatedAt,
	)
	if err == nil {
		detail.Booking = booking
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get booking: %w", err)
	}

	// Get payment
	payment := &Payment{}
	err = db.DB.QueryRow(`
		SELECT id, lead_id, payment_type, amount_paid, remaining_balance, payment_date, updated_at
		FROM payments WHERE lead_id = $1
	`, id).Scan(
		&payment.ID, &payment.LeadID, &payment.PaymentType, &payment.AmountPaid,
		&payment.RemainingBalance, &payment.PaymentDate, &payment.UpdatedAt,
	)
	if err == nil {
		detail.Payment = payment
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}

	// Get scheduling
	scheduling := &Scheduling{}
	var classTimeRaw sql.NullString
	var startTimeRaw sql.NullString
	err = db.DB.QueryRow(`
		SELECT id, lead_id, expected_round, class_days, 
		       TO_CHAR(class_time, 'HH24:MI') as class_time,
		       start_date, 
		       TO_CHAR(start_time, 'HH24:MI') as start_time,
		       class_group_index, updated_at
		FROM scheduling WHERE lead_id = $1
	`, id).Scan(
		&scheduling.ID, &scheduling.LeadID, &scheduling.ExpectedRound, &scheduling.ClassDays,
		&classTimeRaw, &scheduling.StartDate, &startTimeRaw, &scheduling.ClassGroupIndex, &scheduling.UpdatedAt,
	)
	if err == nil {
		// Normalize time format (ensure HH:MM format, not HH:MM:SS)
		scheduling.ClassTime = classTimeRaw
		scheduling.StartTime = startTimeRaw
		detail.Scheduling = scheduling
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get scheduling: %w", err)
	}

	// Get shipping
	shipping := &Shipping{}
	err = db.DB.QueryRow(`
		SELECT id, lead_id, shipment_status, shipment_date, updated_at
		FROM shipping WHERE lead_id = $1
	`, id).Scan(
		&shipping.ID, &shipping.LeadID, &shipping.ShipmentStatus, &shipping.ShipmentDate, &shipping.UpdatedAt,
	)
	if err == nil {
		detail.Shipping = shipping
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get shipping: %w", err)
	}

	return detail, nil
}

func CreateLead(fullName, phone, source, notes, createdByUserID string) (*Lead, error) {
	leadID := uuid.New()
	now := time.Now()

	var createdByUUID *uuid.UUID
	if createdByUserID != "" {
		u, err := uuid.Parse(createdByUserID)
		if err == nil {
			createdByUUID = &u
		}
	}

	var sourceVal, notesVal sql.NullString
	if source != "" {
		sourceVal = sql.NullString{String: source, Valid: true}
	}
	if notes != "" {
		notesVal = sql.NullString{String: notes, Valid: true}
	}

	var createdByID sql.NullString
	if createdByUUID != nil {
		createdByID = sql.NullString{String: createdByUUID.String(), Valid: true}
	}

	_, err := db.DB.Exec(`
		INSERT INTO leads (id, full_name, phone, source, notes, status, sent_to_classes, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, leadID, fullName, phone, sourceVal, notesVal, "lead_created", false, createdByID, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create lead: %w", err)
	}

	return &Lead{
		ID:              leadID,
		FullName:        fullName,
		Phone:           phone,
		Source:          sourceVal,
		Notes:           notesVal,
		Status:          "lead_created",
		CreatedByUserID: createdByID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func UpdateLeadDetail(detail *LeadDetail) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now()

	// Update lead
	_, err = tx.Exec(`
		UPDATE leads
		SET full_name = $1,
		    phone = $2,
		    source = $3,
		    notes = $4,
		    status = $5,
		    sent_to_classes = $6,
		    offer_sent_at = CASE
		        WHEN $5 = 'offer_sent' AND status <> 'offer_sent' THEN $7
		        ELSE offer_sent_at
		    END,
		    updated_at = $7
		WHERE id = $8
	`, detail.Lead.FullName, detail.Lead.Phone, detail.Lead.Source, detail.Lead.Notes, detail.Lead.Status, detail.Lead.SentToClasses, now, detail.Lead.ID)
	if err != nil {
		return fmt.Errorf("failed to update lead: %w", err)
	}

	// Upsert placement test
	if detail.PlacementTest != nil {
		_, err = tx.Exec(`
			INSERT INTO placement_tests (id, lead_id, test_date, test_time, test_type, assigned_level, test_notes, run_by_user_id, placement_test_fee, placement_test_fee_paid, placement_test_payment_date, placement_test_payment_method, updated_at)
			VALUES (COALESCE((SELECT id FROM placement_tests WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			ON CONFLICT (lead_id) DO UPDATE SET
				test_date = EXCLUDED.test_date,
				test_time = EXCLUDED.test_time,
				test_type = EXCLUDED.test_type,
				assigned_level = COALESCE(EXCLUDED.assigned_level, placement_tests.assigned_level),
				test_notes = COALESCE(EXCLUDED.test_notes, placement_tests.test_notes),
				run_by_user_id = EXCLUDED.run_by_user_id,
				placement_test_fee = EXCLUDED.placement_test_fee,
				placement_test_fee_paid = EXCLUDED.placement_test_fee_paid,
				placement_test_payment_date = EXCLUDED.placement_test_payment_date,
				placement_test_payment_method = EXCLUDED.placement_test_payment_method,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.PlacementTest.TestDate, detail.PlacementTest.TestTime,
			detail.PlacementTest.TestType, detail.PlacementTest.AssignedLevel,
			detail.PlacementTest.TestNotes, detail.PlacementTest.RunByUserID,
			detail.PlacementTest.PlacementTestFee, detail.PlacementTest.PlacementTestFeePaid,
			detail.PlacementTest.PlacementTestPaymentDate, detail.PlacementTest.PlacementTestPaymentMethod,
			now)
		if err != nil {
			return fmt.Errorf("failed to upsert placement test: %w", err)
		}
	}

	// Upsert offer
	if detail.Offer != nil {
		_, err = tx.Exec(`
			INSERT INTO offers (id, lead_id, bundle_levels, base_price, discount_value, discount_type, final_price, updated_at)
			VALUES (COALESCE((SELECT id FROM offers WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (lead_id) DO UPDATE SET
				bundle_levels = EXCLUDED.bundle_levels,
				base_price = EXCLUDED.base_price,
				discount_value = EXCLUDED.discount_value,
				discount_type = EXCLUDED.discount_type,
				final_price = EXCLUDED.final_price,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.Offer.BundleLevels, detail.Offer.BasePrice,
			detail.Offer.DiscountValue, detail.Offer.DiscountType, detail.Offer.FinalPrice, now)
		if err != nil {
			return fmt.Errorf("failed to upsert offer: %w", err)
		}
	}

	// Upsert booking
	if detail.Booking != nil {
		_, err = tx.Exec(`
			INSERT INTO bookings (id, lead_id, book_format, address, city, delivery_notes, updated_at)
			VALUES (COALESCE((SELECT id FROM bookings WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6)
			ON CONFLICT (lead_id) DO UPDATE SET
				book_format = EXCLUDED.book_format,
				address = EXCLUDED.address,
				city = EXCLUDED.city,
				delivery_notes = EXCLUDED.delivery_notes,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.Booking.BookFormat, detail.Booking.Address,
			detail.Booking.City, detail.Booking.DeliveryNotes, now)
		if err != nil {
			return fmt.Errorf("failed to upsert booking: %w", err)
		}
	}

	// Upsert payment
	if detail.Payment != nil {
		_, err = tx.Exec(`
			INSERT INTO payments (id, lead_id, payment_type, amount_paid, remaining_balance, payment_date, updated_at)
			VALUES (COALESCE((SELECT id FROM payments WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6)
			ON CONFLICT (lead_id) DO UPDATE SET
				payment_type = EXCLUDED.payment_type,
				amount_paid = EXCLUDED.amount_paid,
				remaining_balance = EXCLUDED.remaining_balance,
				payment_date = EXCLUDED.payment_date,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.Payment.PaymentType, detail.Payment.AmountPaid,
			detail.Payment.RemainingBalance, detail.Payment.PaymentDate, now)
		if err != nil {
			return fmt.Errorf("failed to upsert payment: %w", err)
		}
	}

	// Upsert scheduling
	if detail.Scheduling != nil {
		// Cast class_time and start_time strings to TIME type for PostgreSQL
		var classTimeVal interface{}
		if detail.Scheduling.ClassTime.Valid {
			classTimeVal = detail.Scheduling.ClassTime.String
		} else {
			classTimeVal = nil
		}

		var startTimeVal interface{}
		if detail.Scheduling.StartTime.Valid {
			startTimeVal = detail.Scheduling.StartTime.String
		} else {
			startTimeVal = nil
		}

		_, err = tx.Exec(`
			INSERT INTO scheduling (id, lead_id, expected_round, class_days, class_time, start_date, start_time, class_group_index, updated_at)
			VALUES (COALESCE((SELECT id FROM scheduling WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4::TIME, $5, $6::TIME, $7, $8)
			ON CONFLICT (lead_id) DO UPDATE SET
				expected_round = EXCLUDED.expected_round,
				class_days = EXCLUDED.class_days,
				class_time = EXCLUDED.class_time,
				start_date = EXCLUDED.start_date,
				start_time = EXCLUDED.start_time,
				class_group_index = EXCLUDED.class_group_index,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.Scheduling.ExpectedRound, detail.Scheduling.ClassDays,
			classTimeVal, detail.Scheduling.StartDate, startTimeVal, detail.Scheduling.ClassGroupIndex, now)
		if err != nil {
			return fmt.Errorf("failed to upsert scheduling: %w", err)
		}
	}

	// Upsert shipping
	if detail.Shipping != nil {
		_, err = tx.Exec(`
			INSERT INTO shipping (id, lead_id, shipment_status, shipment_date, updated_at)
			VALUES (COALESCE((SELECT id FROM shipping WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4)
			ON CONFLICT (lead_id) DO UPDATE SET
				shipment_status = EXCLUDED.shipment_status,
				shipment_date = EXCLUDED.shipment_date,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.Shipping.ShipmentStatus, detail.Shipping.ShipmentDate, now)
		if err != nil {
			return fmt.Errorf("failed to upsert shipping: %w", err)
		}
	}

	return tx.Commit()
}

func UpdateLeadStatus(leadID uuid.UUID, status string) error {
	// Waiting list should return the lead to pre-enrolment feed.
	if status == "waiting_for_round" {
		_, err := db.DB.Exec(`
			UPDATE leads
			SET status = $1,
			    sent_to_classes = false,
			    offer_sent_at = CASE
			        WHEN $1 = 'offer_sent' AND status <> 'offer_sent' THEN CURRENT_TIMESTAMP
			        ELSE offer_sent_at
			    END,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, status, leadID)
		return err
	}

	_, err := db.DB.Exec(`
		UPDATE leads
		SET status = $1,
		    offer_sent_at = CASE
		        WHEN $1 = 'offer_sent' AND status <> 'offer_sent' THEN CURRENT_TIMESTAMP
		        ELSE offer_sent_at
		    END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, status, leadID)
	return err
}

// MarkRenewalRefusedAndSetCold moves a returning lead to cold_lead and writes an
// auditable refusal event for renewal reporting.
func MarkRenewalRefusedAndSetCold(leadID uuid.UUID, refusedByUserID *uuid.UUID, notes string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin refusal tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now()
	_, err = tx.Exec(`
		UPDATE leads
		SET status = 'cold_lead',
		    sent_to_classes = false,
		    updated_at = $2
		WHERE id = $1
	`, leadID, now)
	if err != nil {
		return fmt.Errorf("failed to move lead to cold_lead: %w", err)
	}

	var refusedBy interface{}
	if refusedByUserID != nil {
		refusedBy = *refusedByUserID
	}
	_, err = tx.Exec(`
		INSERT INTO renewal_refusals (id, lead_id, refused_at, refused_by_user_id, reason, notes, created_at)
		VALUES ($1, $2, $3, $4, 'refused_renewal', $5, $3)
	`, uuid.New(), leadID, now, refusedBy, strings.TrimSpace(notes))
	if err != nil {
		return fmt.Errorf("failed to insert renewal refusal: %w", err)
	}

	return tx.Commit()
}

func GetLatestRenewalRefusal(leadID uuid.UUID) (*RenewalRefusal, error) {
	item := &RenewalRefusal{}
	err := db.DB.QueryRow(`
		SELECT id, lead_id, refused_at, refused_by_user_id::text, reason, notes, created_at
		FROM renewal_refusals
		WHERE lead_id = $1
		ORDER BY refused_at DESC
		LIMIT 1
	`, leadID).Scan(
		&item.ID,
		&item.LeadID,
		&item.RefusedAt,
		&item.RefusedByUserID,
		&item.Reason,
		&item.Notes,
		&item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query latest renewal refusal: %w", err)
	}
	return item, nil
}

// UpdateLeadPurchasedLevels updates levels_purchased_total for a lead.
// For returning students, levels_consumed is cumulative across history, so we store
// a cumulative purchase target (levels_consumed + newly purchased levels) to preserve
// the invariant: remaining_credits = levels_purchased_total - levels_consumed.
func UpdateLeadPurchasedLevels(leadID uuid.UUID, levels int32) error {
	var levelsConsumed int32
	var isReturning bool
	err := db.DB.QueryRow(`
		SELECT COALESCE(levels_consumed, 0), COALESCE(is_returning, false)
		FROM leads
		WHERE id = $1
	`, leadID).Scan(&levelsConsumed, &isReturning)
	if err != nil {
		return fmt.Errorf("failed to read lead consumption state: %w", err)
	}

	targetPurchased := levels
	if isReturning {
		targetPurchased = levelsConsumed + levels
	}

	_, err = db.DB.Exec(`
		UPDATE leads
		SET levels_purchased_total = $1,
		    remaining_credits = GREATEST($1 - COALESCE(levels_consumed, 0), 0),
		    updated_at = NOW()
		WHERE id = $2
	`, targetPurchased, leadID)

	if err != nil {
		return fmt.Errorf("failed to update levels_purchased_total: %w", err)
	}

	return nil
}

// UpsertSchedulingClassDaysTime updates only class_days and class_time for a lead.
// Used when marking ready to start; preserves expected_round, start_date, start_time.
func UpsertSchedulingClassDaysTime(leadID uuid.UUID, classDays, classTime string) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO scheduling (id, lead_id, class_days, class_time, updated_at)
		VALUES (COALESCE((SELECT id FROM scheduling WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4)
		ON CONFLICT (lead_id) DO UPDATE SET
			class_days = EXCLUDED.class_days,
			class_time = EXCLUDED.class_time,
			updated_at = EXCLUDED.updated_at
	`, leadID, classDays, classTime, now)
	return err
}

// UpdateLeadBasicInfo updates only lead basic info (name, phone, source, notes) - for moderators
func UpdateLeadBasicInfo(lead *Lead) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		UPDATE leads SET full_name = $1, phone = $2, source = $3, notes = $4, updated_at = $5
		WHERE id = $6
	`, lead.FullName, lead.Phone, lead.Source, lead.Notes, now, lead.ID)
	return err
}

// UpdatePlacementTest updates only placement test fields
func UpdatePlacementTest(pt *PlacementTest) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO placement_tests (id, lead_id, assigned_level, test_notes, updated_at)
		VALUES (COALESCE((SELECT id FROM placement_tests WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4)
		ON CONFLICT (lead_id) DO UPDATE SET
			assigned_level = EXCLUDED.assigned_level,
			test_notes = EXCLUDED.test_notes,
			updated_at = EXCLUDED.updated_at
	`, pt.LeadID, pt.AssignedLevel, pt.TestNotes, now)
	return err
}

// UpdateOffer updates only offer fields
func UpdateOffer(offer *Offer) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO offers (id, lead_id, bundle_levels, base_price, discount_value, discount_type, final_price, updated_at)
		VALUES (COALESCE((SELECT id FROM offers WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (lead_id) DO UPDATE SET
			bundle_levels = EXCLUDED.bundle_levels,
			base_price = EXCLUDED.base_price,
			discount_value = EXCLUDED.discount_value,
			discount_type = EXCLUDED.discount_type,
			final_price = EXCLUDED.final_price,
			updated_at = EXCLUDED.updated_at
	`, offer.LeadID, offer.BundleLevels, offer.BasePrice,
		offer.DiscountValue, offer.DiscountType, offer.FinalPrice, now)
	return err
}

// UpsertBookingAndShipping updates booking and shipping independently from full lead save.
func UpsertBookingAndShipping(booking *Booking, shipping *Shipping) error {
	now := time.Now()

	if booking != nil {
		_, err := db.DB.Exec(`
			INSERT INTO bookings (id, lead_id, book_format, address, city, delivery_notes, updated_at)
			VALUES (COALESCE((SELECT id FROM bookings WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6)
			ON CONFLICT (lead_id) DO UPDATE SET
				book_format = EXCLUDED.book_format,
				address = EXCLUDED.address,
				city = EXCLUDED.city,
				delivery_notes = EXCLUDED.delivery_notes,
				updated_at = EXCLUDED.updated_at
		`, booking.LeadID, booking.BookFormat, booking.Address, booking.City, booking.DeliveryNotes, now)
		if err != nil {
			return fmt.Errorf("failed to upsert booking: %w", err)
		}
	}

	if shipping != nil {
		_, err := db.DB.Exec(`
			INSERT INTO shipping (id, lead_id, shipment_status, shipment_date, updated_at)
			VALUES (COALESCE((SELECT id FROM shipping WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4)
			ON CONFLICT (lead_id) DO UPDATE SET
				shipment_status = EXCLUDED.shipment_status,
				shipment_date = EXCLUDED.shipment_date,
				updated_at = EXCLUDED.updated_at
		`, shipping.LeadID, shipping.ShipmentStatus, shipping.ShipmentDate, now)
		if err != nil {
			return fmt.Errorf("failed to upsert shipping: %w", err)
		}
	}

	return nil
}

// BookPlacementTest updates placement test fields and sets status to "test_booked"
// This is a lightweight update that doesn't require offer/pricing fields
func BookPlacementTest(leadID uuid.UUID, testDate sql.NullTime, testTime sql.NullString, testType sql.NullString, testNotes sql.NullString) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now()

	// Update or insert placement test with default fee of 100 if not exists
	_, err = tx.Exec(`
		INSERT INTO placement_tests (id, lead_id, test_date, test_time, test_type, test_notes, placement_test_fee, placement_test_fee_paid, updated_at)
		VALUES (COALESCE((SELECT id FROM placement_tests WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, 100, 0, $6)
		ON CONFLICT (lead_id) DO UPDATE SET
			test_date = EXCLUDED.test_date,
			test_time = EXCLUDED.test_time,
			test_type = EXCLUDED.test_type,
			test_notes = EXCLUDED.test_notes,
			placement_test_fee = COALESCE(placement_tests.placement_test_fee, 100),
			updated_at = EXCLUDED.updated_at
	`, leadID, testDate, testTime, testType, testNotes, now)
	if err != nil {
		return fmt.Errorf("failed to upsert placement test: %w", err)
	}

	// Update lead status to test_booked
	_, err = tx.Exec(`UPDATE leads SET status = $1, updated_at = $2 WHERE id = $3`, "test_booked", now, leadID)
	if err != nil {
		return fmt.Errorf("failed to update lead status: %w", err)
	}

	return tx.Commit()
}

func GetUserByEmail(email string) (*User, error) {
	user := &User{}
	// Case-insensitive lookup so login works regardless of email case (e.g. HR stores normalized, seed may not).
	err := db.DB.QueryRow(`
		SELECT id, email, COALESCE(full_name, ''), COALESCE(phone, ''), password_hash, role, COALESCE(is_active, true), COALESCE(must_change_password, false), created_at
		FROM users WHERE LOWER(TRIM(email)) = LOWER(TRIM($1))
	`, email).Scan(&user.ID, &user.Email, &user.FullName, &user.Phone, &user.PasswordHash, &user.Role, &user.IsActive, &user.MustChangePassword, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func CreateUser(email, passwordHash, role, fullName, phone string) (*User, error) {
	return CreateUserWithMustChange(email, passwordHash, role, fullName, phone, false)
}

func CreateUserWithMustChange(email, passwordHash, role, fullName, phone string, mustChangePassword bool) (*User, error) {
	userID := uuid.New()
	fullName = strings.TrimSpace(fullName)
	phone = strings.TrimSpace(phone)
	var fullNameVal sql.NullString
	var phoneVal sql.NullString
	if fullName != "" {
		fullNameVal = sql.NullString{String: fullName, Valid: true}
	}
	if phone != "" {
		phoneVal = sql.NullString{String: phone, Valid: true}
	}
	_, err := db.DB.Exec(`
		INSERT INTO users (id, email, full_name, phone, password_hash, role, must_change_password, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
	`, userID, email, fullNameVal, phoneVal, passwordHash, role, mustChangePassword)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:                 userID,
		Email:              email,
		FullName:           fullNameVal,
		Phone:              phoneVal,
		PasswordHash:       passwordHash,
		Role:               role,
		MustChangePassword: mustChangePassword,
	}, nil
}

// DeleteLead deletes a lead and all associated data (cascade delete)
func DeleteLead(leadID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Delete related records first (in reverse order of dependencies)
	// Note: If foreign keys have CASCADE DELETE, some of these may be automatic
	// but we'll be explicit for safety
	_, err = tx.Exec(`DELETE FROM shipping WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete shipping: %w", err)
	}

	_, err = tx.Exec(`DELETE FROM scheduling WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete scheduling: %w", err)
	}

	_, err = tx.Exec(`DELETE FROM payments WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete payments: %w", err)
	}

	_, err = tx.Exec(`DELETE FROM bookings WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete bookings: %w", err)
	}

	_, err = tx.Exec(`DELETE FROM offers WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete offers: %w", err)
	}

	_, err = tx.Exec(`DELETE FROM placement_tests WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete placement_tests: %w", err)
	}

	// Finally delete the lead
	_, err = tx.Exec(`DELETE FROM leads WHERE id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete lead: %w", err)
	}

	return tx.Commit()
}

// GetCurrentRound returns the current round number (defaults to 1)
func GetCurrentRound() (int, error) {
	var roundStr string
	err := db.DB.QueryRow(`SELECT value FROM settings WHERE key = 'current_round'`).Scan(&roundStr)
	if err == sql.ErrNoRows {
		// Initialize to 1 if not exists
		_, err = db.DB.Exec(`INSERT INTO settings (key, value) VALUES ('current_round', '1')`)
		if err != nil {
			return 1, err
		}
		return 1, nil
	}
	if err != nil {
		return 1, err
	}
	round, err := strconv.Atoi(roundStr)
	if err != nil {
		return 1, err
	}
	return round, nil
}

// IncrementCurrentRound increments the current round by 1
func IncrementCurrentRound() error {
	_, err := db.DB.Exec(`
		INSERT INTO settings (key, value) VALUES ('current_round', '1')
		ON CONFLICT (key) DO UPDATE SET value = (CAST(settings.value AS INTEGER) + 1)::TEXT, updated_at = CURRENT_TIMESTAMP
	`)
	return err
}

// GetEligibleStudentsForClasses returns students eligible for classes board
// Eligibility: status=ready_to_start, assigned_level set, class_days set, class_time set
func GetEligibleStudentsForClasses() ([]*ClassStudent, error) {
	query := `
		SELECT l.id, l.full_name, l.phone, s.class_group_index
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.status = 'ready_to_start'
		AND l.sent_to_classes = true
		AND pt.assigned_level IS NOT NULL
		AND s.class_days IS NOT NULL
		AND s.class_time IS NOT NULL
		ORDER BY pt.assigned_level, s.class_days, s.class_time, s.class_group_index, l.full_name
	`
	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query eligible students: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var students []*ClassStudent
	for rows.Next() {
		s := &ClassStudent{}
		err := rows.Scan(&s.LeadID, &s.FullName, &s.Phone, &s.GroupIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to scan student: %w", err)
		}
		students = append(students, s)
	}
	return students, rows.Err()
}

// GetClassGroups groups eligible students by (level, days, time, group_index) and computes readiness
func GetClassGroups() ([]*ClassGroup, error) {
	// Get all eligible students with their level, days, time
	// Include both ready_to_start and in_classes students so late-joined students
	// remain visible on Ops board even before Mentor Head starts the round.
	query := `
		SELECT l.id, l.full_name, l.phone, pt.assigned_level, s.class_days, s.class_time, s.class_group_index,
		       COALESCE(cg.round_status, 'not_started'),
		       COALESCE(l.is_returning, false),
		       GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS remaining_credits_calc
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		LEFT JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE (l.status = 'ready_to_start' OR l.status = 'in_classes')
		AND l.sent_to_classes = true
		AND pt.assigned_level IS NOT NULL
		AND s.class_days IS NOT NULL
		AND s.class_time IS NOT NULL
		AND COALESCE(cg.round_status, 'not_started') != 'closed'
		ORDER BY pt.assigned_level, s.class_days, s.class_time, COALESCE(s.class_group_index, 1), l.full_name
	`
	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query class groups: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	// Group by (level, days, time, group_index)
	groupsMap := make(map[string]*ClassGroup)
	for rows.Next() {
		var leadID uuid.UUID
		var fullName, phone string
		var assignedLevel sql.NullInt32
		var classDays, classTime sql.NullString
		var groupIndex sql.NullInt32
		var roundStatus string
		var isReturning bool
		var remainingCredits int32

		err := rows.Scan(&leadID, &fullName, &phone, &assignedLevel, &classDays, &classTime, &groupIndex, &roundStatus, &isReturning, &remainingCredits)
		if err != nil {
			return nil, fmt.Errorf("failed to scan student: %w", err)
		}

		if !assignedLevel.Valid || !classDays.Valid || !classTime.Valid {
			continue
		}

		// Default group_index to 1 if null (temporary grouping for display)
		// Unassigned students will be auto-assigned by the handler
		idx := int32(1)
		if groupIndex.Valid {
			idx = groupIndex.Int32
		}

		// Create key: level-days-time-index
		key := fmt.Sprintf("%d-%s-%s-%d", assignedLevel.Int32, classDays.String, classTime.String, idx)

		group, exists := groupsMap[key]
		if !exists {
			group = &ClassGroup{
				Level:        assignedLevel.Int32,
				ClassDays:    classDays.String,
				ClassTime:    classTime.String,
				GroupIndex:   idx,
				RoundStatus:  roundStatus,
				StudentCount: 0,
				Students:     []*ClassStudent{},
			}
			groupsMap[key] = group
		}

		group.Students = append(group.Students, &ClassStudent{
			LeadID:           leadID,
			FullName:         fullName,
			Phone:            phone,
			IsReturning:      isReturning,
			RemainingCredits: remainingCredits,
			GroupIndex:       groupIndex,
		})
		group.StudentCount++
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Convert map to slice and compute readiness, generate class_key
	var groups []*ClassGroup
	var classKeys []string
	for _, group := range groupsMap {
		// Compute readiness: STARTED if active, else 6=LOCKED, 4-5=READY, <4=NOT READY
		if group.RoundStatus == "active" {
			group.Readiness = "STARTED"
		} else if group.StudentCount >= 6 {
			group.Readiness = "LOCKED"
		} else if group.StudentCount >= 4 {
			group.Readiness = "READY"
		} else {
			group.Readiness = "NOT READY"
		}
		// Generate class key
		group.ClassKey = GenerateClassKey(group.Level, group.ClassDays, group.ClassTime, group.GroupIndex)
		classKeys = append(classKeys, group.ClassKey)
		groups = append(groups, group)
	}

	// Load workflow state for all groups
	if len(classKeys) > 0 {
		workflows, err := GetClassGroupWorkflowsBatch(classKeys)
		if err == nil {
			var visible []*ClassGroup
			for _, group := range groups {
				if wf, ok := workflows[group.ClassKey]; ok {
					if wf.HiddenInOps {
						continue
					}
					group.SentToMentor = wf.SentToMentor
					group.SentAt = wf.SentAt
					group.ReturnedAt = wf.ReturnedAt
					visible = append(visible, group)
				} else {
					visible = append(visible, group)
				}
			}
			groups = visible
		}
	}

	// Load current session numbers for active rounds
	if len(groups) > 0 {
		var activeKeys []string
		for _, group := range groups {
			if group.RoundStatus == "active" {
				activeKeys = append(activeKeys, group.ClassKey)
			}
		}

		if len(activeKeys) > 0 {
			rows, err := db.DB.Query(`
				SELECT cg.class_key,
				       (SELECT COUNT(*) FROM class_sessions WHERE class_key = cg.class_key AND status = 'completed') + 1 AS current_session
				FROM class_groups cg
				WHERE cg.class_key = ANY($1)
				  AND COALESCE(cg.round_status, '') = 'active'
			`, pq.Array(activeKeys))
			if err == nil {
				sessionMap := make(map[string]int32)
				for rows.Next() {
					var classKey string
					var currentSession int32
					if err := rows.Scan(&classKey, &currentSession); err != nil {
						continue
					}
					sessionMap[classKey] = currentSession
				}
				_ = rows.Close()

				for _, group := range groups {
					if currentSession, ok := sessionMap[group.ClassKey]; ok {
						group.CurrentSession = sql.NullInt32{Int32: currentSession, Valid: true}
					}
				}
			}
		}
	}

	// Sort by level, then days, then time, then group index
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Level != groups[j].Level {
			return groups[i].Level < groups[j].Level
		}
		if groups[i].ClassDays != groups[j].ClassDays {
			return groups[i].ClassDays < groups[j].ClassDays
		}
		if groups[i].ClassTime != groups[j].ClassTime {
			return groups[i].ClassTime < groups[j].ClassTime
		}
		return groups[i].GroupIndex < groups[j].GroupIndex
	})

	return groups, nil
}

// AssignClassGroup assigns a student to a class group, auto-creating if needed
// Returns the group_index assigned
// Note: Student must already have sent_to_classes=true (this is checked by GetClassGroups)
func AssignClassGroup(leadID uuid.UUID) (int32, error) {
	tx, err := db.DB.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Get student's level, days, time
	// Note: We don't check sent_to_classes here because GetClassGroups already filters for it
	var assignedLevel sql.NullInt32
	var classDays, classTime sql.NullString
	err = tx.QueryRow(`
		SELECT pt.assigned_level, s.class_days, s.class_time
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.id = $1
		AND l.status = 'ready_to_start'
		AND l.sent_to_classes = true
		AND pt.assigned_level IS NOT NULL
		AND s.class_days IS NOT NULL
		AND s.class_time IS NOT NULL
	`, leadID).Scan(&assignedLevel, &classDays, &classTime)
	if err != nil {
		return 0, fmt.Errorf("student not eligible for classes: %w", err)
	}

	// Track group indices that are not eligible due to active/closed/sent_to_mentor
	activeGroups := map[int32]bool{}
	blockedGroups := map[int32]bool{}
	groupRows, err := tx.Query(`
		SELECT COALESCE(class_number, 1), COALESCE(round_status, 'not_started'), COALESCE(sent_to_mentor, false)
		FROM class_groups
		WHERE level = $1
		  AND class_days = $2
		  AND class_time = $3
	`, assignedLevel.Int32, classDays.String, classTime.String)
	if err != nil {
		return 0, fmt.Errorf("failed to query class groups: %w", err)
	}
	for groupRows.Next() {
		var idx int32
		var roundStatus string
		var sentToMentor bool
		if err := groupRows.Scan(&idx, &roundStatus, &sentToMentor); err == nil {
			if roundStatus == "active" {
				activeGroups[idx] = true
			}
			if roundStatus == "closed" || sentToMentor {
				blockedGroups[idx] = true
			}
		}
	}
	_ = groupRows.Close()

	for groupIndex := int32(1); ; groupIndex++ {
		if activeGroups[groupIndex] || blockedGroups[groupIndex] {
			continue
		}

		// Advisory lock per (level, days, time, groupIndex) to prevent race conditions
		lockKey := fmt.Sprintf("%d|%s|%s|%d", assignedLevel.Int32, classDays.String, classTime.String, groupIndex)
		if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
			return 0, fmt.Errorf("failed to lock class group: %w", err)
		}

		// Re-check group state after lock
		var roundStatus sql.NullString
		var sentToMentor sql.NullBool
		err := tx.QueryRow(`
			SELECT COALESCE(round_status, 'not_started'), COALESCE(sent_to_mentor, false)
			FROM class_groups
			WHERE level = $1
			  AND class_days = $2
			  AND class_time = $3
			  AND COALESCE(class_number, 1) = $4
		`, assignedLevel.Int32, classDays.String, classTime.String, groupIndex).Scan(&roundStatus, &sentToMentor)
		if err == nil {
			if roundStatus.String == "active" || roundStatus.String == "closed" || sentToMentor.Bool {
				continue
			}
		} else if err != sql.ErrNoRows {
			return 0, fmt.Errorf("failed to check class group state: %w", err)
		}

		var count int
		err = tx.QueryRow(`
			SELECT COUNT(*)
			FROM leads l
			INNER JOIN placement_tests pt ON l.id = pt.lead_id
			INNER JOIN scheduling s ON l.id = s.lead_id
			WHERE l.status = 'ready_to_start'
			AND l.sent_to_classes = true
			AND pt.assigned_level = $1
			AND s.class_days = $2
			AND s.class_time = $3
			AND COALESCE(s.class_group_index, 0) = $4
		`, assignedLevel.Int32, classDays.String, classTime.String, groupIndex).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("failed to check group capacity: %w", err)
		}

		// If this group has < 6 students, assign here
		if count < 6 {
			_, err = tx.Exec(`
				UPDATE scheduling SET class_group_index = $1, updated_at = CURRENT_TIMESTAMP
				WHERE lead_id = $2
			`, groupIndex, leadID)
			if err != nil {
				return 0, fmt.Errorf("failed to assign class group: %w", err)
			}
			if err := tx.Commit(); err != nil {
				return 0, fmt.Errorf("failed to commit class group assignment: %w", err)
			}
			return groupIndex, nil
		}
		// Otherwise, continue to next group index
	}
}

// MoveStudentBetweenGroups moves a student to a different group (or creates new)
func MoveStudentBetweenGroups(leadID uuid.UUID, targetGroupIndex int32) error {
	// Get student's level, days, time
	var assignedLevel sql.NullInt32
	var classDays, classTime sql.NullString
	err := db.DB.QueryRow(`
		SELECT pt.assigned_level, s.class_days, s.class_time
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.id = $1
	`, leadID).Scan(&assignedLevel, &classDays, &classTime)
	if err != nil {
		return fmt.Errorf("failed to get student details: %w", err)
	}

	// Disallow moving into active/closed or sent-to-mentor class groups
	var roundStatus sql.NullString
	var sentToMentor sql.NullBool
	err = db.DB.QueryRow(`
		SELECT COALESCE(round_status, 'not_started'), COALESCE(sent_to_mentor, false)
		FROM class_groups
		WHERE level = $1
		  AND class_days = $2
		  AND class_time = $3
		  AND COALESCE(class_number, 1) = $4
	`, assignedLevel.Int32, classDays.String, classTime.String, targetGroupIndex).Scan(&roundStatus, &sentToMentor)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check class group status: %w", err)
	}
	if err == nil {
		if roundStatus.String == "active" {
			return fmt.Errorf("target group is started (active round)")
		}
		if roundStatus.String == "closed" {
			return fmt.Errorf("target group is closed")
		}
		if sentToMentor.Valid && sentToMentor.Bool {
			return fmt.Errorf("target group is sent to mentor head")
		}
	}

	// Check if target group exists and is not locked
	var count int
	err = db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.status = 'ready_to_start'
		AND l.sent_to_classes = true
		AND pt.assigned_level = $1
		AND s.class_days = $2
		AND s.class_time = $3
		AND COALESCE(s.class_group_index, 0) = $4
	`, assignedLevel.Int32, classDays.String, classTime.String, targetGroupIndex).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check target group: %w", err)
	}

	// If target group is locked (6 students), reject
	if count >= 6 {
		return fmt.Errorf("target group is locked (6 students)")
	}

	// Move student
	_, err = db.DB.Exec(`
		UPDATE scheduling SET class_group_index = $1, updated_at = CURRENT_TIMESTAMP
		WHERE lead_id = $2
	`, targetGroupIndex, leadID)
	if err != nil {
		return fmt.Errorf("failed to move student: %w", err)
	}

	return nil
}

// GetAvailableGroupsForMove returns available groups (not locked) for a student's key (level+days+time)
func GetAvailableGroupsForMove(leadID uuid.UUID) ([]int32, error) {
	// Get student's level, days, time
	var assignedLevel sql.NullInt32
	var classDays, classTime sql.NullString
	err := db.DB.QueryRow(`
		SELECT pt.assigned_level, s.class_days, s.class_time
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.id = $1
	`, leadID).Scan(&assignedLevel, &classDays, &classTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get student details: %w", err)
	}

	// Find all groups for this key and their counts
	rows, err := db.DB.Query(`
		SELECT COALESCE(s.class_group_index, 1), COUNT(*)
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		LEFT JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE l.status = 'ready_to_start'
		AND l.sent_to_classes = true
		AND pt.assigned_level = $1
		AND s.class_days = $2
		AND s.class_time = $3
		AND COALESCE(cg.round_status, 'not_started') NOT IN ('active', 'closed')
		AND COALESCE(cg.sent_to_mentor, false) = false
		GROUP BY COALESCE(s.class_group_index, 1)
		ORDER BY COALESCE(s.class_group_index, 1)
	`, assignedLevel.Int32, classDays.String, classTime.String)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var availableGroups []int32
	for rows.Next() {
		var groupIndex int32
		var count int
		err := rows.Scan(&groupIndex, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		// Only include groups that are not locked (< 6)
		if count < 6 {
			availableGroups = append(availableGroups, groupIndex)
		}
	}

	return availableGroups, rows.Err()
}

// GetMoveOptionsForLead returns available class options across the same level.
// Includes a "new_same" option for opening a new class with the same days/time.
func GetMoveOptionsForLead(leadID uuid.UUID) ([]MoveClassOption, error) {
	// Get student's level, days, time, and current group
	var assignedLevel sql.NullInt32
	var classDays, classTime sql.NullString
	var currentGroup sql.NullInt32
	err := db.DB.QueryRow(`
		SELECT pt.assigned_level, s.class_days, s.class_time, s.class_group_index
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.id = $1
	`, leadID).Scan(&assignedLevel, &classDays, &classTime, &currentGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get student details: %w", err)
	}
	if !assignedLevel.Valid || !classDays.Valid || !classTime.Valid {
		return nil, nil
	}

	currentClassNumber := int32(1)
	if currentGroup.Valid {
		currentClassNumber = currentGroup.Int32
	}

	// Base option: create new class with same days/time
	options := []MoveClassOption{{
		Value: "new_same",
		Label: "Create New Class (same days/time)",
	}}

	// Available classes for this level (not sent/active/closed) with capacity < 6
	rows, err := db.DB.Query(`
		WITH counts AS (
			SELECT pt.assigned_level AS level,
			       s.class_days AS class_days,
			       TO_CHAR(s.class_time, 'HH24:MI') AS class_time,
			       COALESCE(s.class_group_index, 1) AS class_number,
			       COUNT(*) AS student_count
			FROM leads l
			INNER JOIN placement_tests pt ON l.id = pt.lead_id
			INNER JOIN scheduling s ON l.id = s.lead_id
			WHERE l.status = 'ready_to_start'
			  AND l.sent_to_classes = true
			GROUP BY pt.assigned_level, s.class_days, TO_CHAR(s.class_time, 'HH24:MI'), COALESCE(s.class_group_index, 1)
		)
		SELECT cg.class_key, cg.class_days, cg.class_time, cg.class_number, COALESCE(c.student_count, 0)
		FROM class_groups cg
		LEFT JOIN counts c ON c.level = cg.level
		  AND c.class_days = cg.class_days
		  AND c.class_time = LEFT(cg.class_time, 5)
		  AND c.class_number = cg.class_number
		WHERE cg.level = $1
		  AND COALESCE(cg.sent_to_mentor, false) = false
		  AND COALESCE(cg.round_status, 'not_started') NOT IN ('active', 'closed')
		ORDER BY cg.class_days, cg.class_time, cg.class_number
	`, assignedLevel.Int32)
	if err != nil {
		return nil, fmt.Errorf("failed to query available classes: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var classKey, days, timeStr string
		var classNumber int32
		var count int
		if err := rows.Scan(&classKey, &days, &timeStr, &classNumber, &count); err != nil {
			return nil, fmt.Errorf("failed to scan class option: %w", err)
		}
		if count >= 6 {
			continue
		}
		// Skip current class
		if days == classDays.String && timeStr == classTime.String && classNumber == currentClassNumber {
			continue
		}
		options = append(options, MoveClassOption{
			Value: fmt.Sprintf("class_key:%s", classKey),
			Label: fmt.Sprintf("%s @ %s (Class #%d)", days, timeStr, classNumber),
		})
	}

	return options, rows.Err()
}

// MoveStudentToClassKey moves a student to a specific class group (possibly different days/time).
func MoveStudentToClassKey(leadID uuid.UUID, classKey string) error {
	// Get target class group details
	var level int32
	var classDays, classTime string
	var classNumber int32
	var sentToMentor bool
	var roundStatus sql.NullString
	err := db.DB.QueryRow(`
		SELECT level, class_days, class_time, class_number, sent_to_mentor, round_status
		FROM class_groups
		WHERE class_key = $1
	`, classKey).Scan(&level, &classDays, &classTime, &classNumber, &sentToMentor, &roundStatus)
	if err == sql.ErrNoRows {
		parsedLevel, parsedDays, parsedTime, parsedNumber, parseErr := parseClassKey(classKey)
		if parseErr != nil {
			return fmt.Errorf("target class not found")
		}
		now := time.Now()
		_, insertErr := db.DB.Exec(`
			INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, updated_at)
			VALUES ($1, $2, $3, $4, $5, false, $6)
			ON CONFLICT (class_key) DO NOTHING
		`, classKey, parsedLevel, parsedDays, parsedTime, parsedNumber, now)
		if insertErr != nil {
			return fmt.Errorf("failed to create target class: %w", insertErr)
		}
		level = parsedLevel
		classDays = parsedDays
		classTime = parsedTime
		classNumber = parsedNumber
		sentToMentor = false
		roundStatus = sql.NullString{String: "not_started", Valid: true}
		err = nil
	}
	if err != nil {
		return fmt.Errorf("failed to load target class: %w", err)
	}
	if sentToMentor || (roundStatus.Valid && (roundStatus.String == "active" || roundStatus.String == "closed")) {
		return fmt.Errorf("target class not available")
	}

	// Capacity check
	var count int
	err = db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.status = 'ready_to_start'
		  AND l.sent_to_classes = true
		  AND pt.assigned_level = $1
		  AND s.class_days = $2
		  AND s.class_time = $3
		  AND COALESCE(s.class_group_index, 1) = $4
	`, level, classDays, classTime, classNumber).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check target class capacity: %w", err)
	}
	if count >= 6 {
		return fmt.Errorf("target class is locked (6 students)")
	}

	_, err = db.DB.Exec(`
		UPDATE scheduling
		SET class_days = $1,
		    class_time = $2,
		    class_group_index = $3,
		    updated_at = CURRENT_TIMESTAMP
		WHERE lead_id = $4
	`, classDays, classTime, classNumber, leadID)
	if err != nil {
		return fmt.Errorf("failed to move student to class: %w", err)
	}

	return nil
}

func parseClassKey(classKey string) (int32, string, string, int32, error) {
	parts := strings.Split(classKey, "|")
	if len(parts) != 4 {
		return 0, "", "", 0, fmt.Errorf("invalid class key")
	}
	levelStr := strings.TrimPrefix(parts[0], "L")
	levelInt, err := strconv.Atoi(levelStr)
	if err != nil {
		return 0, "", "", 0, fmt.Errorf("invalid class level")
	}
	numberInt, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, "", "", 0, fmt.Errorf("invalid class number")
	}
	return int32(levelInt), parts[1], parts[2], int32(numberInt), nil
}

// SendLeadToClasses marks a lead as sent to classes board
func SendLeadToClasses(leadID uuid.UUID) error {
	_, err := db.DB.Exec(`
		UPDATE leads 
		SET sent_to_classes = true, 
		    high_priority = false, 
		    high_priority_reason = '', 
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, leadID)
	if err != nil {
		return err
	}

	// Ensure the lead is assigned to a non-started class group (or a new group).
	// This prevents "Send to Classes" from placing a lead into an active round.
	_, err = AssignClassGroup(leadID)
	if err != nil {
		return fmt.Errorf("failed to assign class group: %w", err)
	}
	return nil
}

// GenerateClassKey creates a stable class key from level, days, time, and group index
func GenerateClassKey(level int32, classDays, classTime string, groupIndex int32) string {
	return fmt.Sprintf("L%d|%s|%s|%d", level, classDays, classTime, groupIndex)
}

// GetClassGroupWorkflow gets workflow state for a class group by class_key
func GetClassGroupWorkflow(classKey string) (*ClassGroupWorkflow, error) {
	wf := &ClassGroupWorkflow{}
	var sentAt, returnedAt, hiddenAt, roundStartedAt, roundClosedAt sql.NullTime
	var hiddenBy, roundStartedBy, roundClosedBy sql.NullString
	var roundStatus sql.NullString
	err := db.DB.QueryRow(`
		SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, returned_at, updated_at,
		       hidden_in_ops, hidden_at, hidden_by::text,
		       COALESCE(round_status, 'not_started'), round_started_at, round_started_by::text, round_closed_at, round_closed_by::text
		FROM class_groups WHERE class_key = $1
	`, classKey).Scan(
		&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
		&wf.SentToMentor, &sentAt, &returnedAt, &wf.UpdatedAt,
		&wf.HiddenInOps, &hiddenAt, &hiddenBy,
		&roundStatus, &roundStartedAt, &roundStartedBy, &roundClosedAt, &roundClosedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Not found is OK - means not sent yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get class group workflow: %w", err)
	}
	wf.SentAt, wf.ReturnedAt = sentAt, returnedAt
	wf.HiddenAt, wf.HiddenBy = hiddenAt, hiddenBy
	wf.RoundStartedAt, wf.RoundClosedAt = roundStartedAt, roundClosedAt
	wf.RoundStartedBy, wf.RoundClosedBy = roundStartedBy, roundClosedBy
	if roundStatus.Valid {
		wf.RoundStatus = roundStatus.String
	} else {
		wf.RoundStatus = "not_started"
	}
	return wf, nil
}

// SendClassGroupToMentor marks a class group as sent to mentor head
func SendClassGroupToMentor(classKey string, level int32, classDays, classTime string, classNumber int32) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, $6, $6)
		ON CONFLICT (class_key) DO UPDATE SET
			sent_to_mentor = true,
			sent_at = $6,
			returned_at = NULL,
			hidden_in_ops = false,
			hidden_at = NULL,
			hidden_by = NULL,
			updated_at = $6
	`, classKey, level, classDays, classTime, classNumber, now)
	return err
}

// ReturnClassGroupFromMentor clears the sent_to_mentor flag and removes mentor assignment.
// Dashboard uses GetClassGroupsSentToMentor() which selects WHERE sent_to_mentor = true;
// this UPDATE sets sent_to_mentor = false so the class no longer matches and disappears from the list.
func ReturnClassGroupFromMentor(classKey string) error {
	// Validate current state for clearer error messages.
	var sentToMentor bool
	var roundStatus string
	err := db.DB.QueryRow(`
		SELECT sent_to_mentor, COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
	`, classKey).Scan(&sentToMentor, &roundStatus)
	if err == sql.ErrNoRows {
		return ErrClassNotFound
	}
	if err != nil {
		return fmt.Errorf("return class_groups select: %w", err)
	}
	if roundStatus == "active" {
		return ErrClassRoundActive
	}
	if !sentToMentor {
		return ErrClassAlreadyReturned
	}

	now := time.Now()
	res, err := db.DB.Exec(`
		UPDATE class_groups
		SET sent_to_mentor = false,
			round_status = 'not_started',
			round_started_at = NULL,
			round_started_by = NULL,
			hidden_in_ops = false,
			hidden_at = NULL,
			hidden_by = NULL,
			returned_at = $2,
			updated_at = $2
		WHERE class_key = $1
		  AND COALESCE(round_status, '') != 'active'
		  AND sent_to_mentor = true
	`, classKey, now)
	if err != nil {
		return fmt.Errorf("return class_groups update: %w", err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		// Re-check state in case it changed after the initial guard.
		var recheckSent bool
		var recheckStatus string
		err := db.DB.QueryRow(`
			SELECT sent_to_mentor, COALESCE(round_status, 'not_started')
			FROM class_groups
			WHERE class_key = $1
		`, classKey).Scan(&recheckSent, &recheckStatus)
		if err == sql.ErrNoRows {
			return ErrClassNotFound
		}
		if err != nil {
			return fmt.Errorf("return class_groups recheck: %w", err)
		}
		if recheckStatus == "active" {
			return ErrClassRoundActive
		}
		if !recheckSent {
			return ErrClassAlreadyReturned
		}
		return fmt.Errorf("class cannot be returned")
	}
	_, err = db.DB.Exec(`DELETE FROM mentor_assignments WHERE class_key = $1`, classKey)
	if err != nil {
		return fmt.Errorf("return mentor_assignments delete: %w", err)
	}
	// Move students back to ready_to_start so class appears as NOT STARTED in Ops.
	_, err = db.DB.Exec(`
		UPDATE leads l
		SET status = 'ready_to_start', updated_at = NOW()
		FROM scheduling s
		INNER JOIN placement_tests pt ON pt.lead_id = s.lead_id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE l.id = s.lead_id
		  AND cg.class_key = $1
		  AND l.status = 'in_classes'
	`, classKey)
	if err != nil {
		return fmt.Errorf("return lead status update: %w", err)
	}
	return nil
}

// ArchiveClassInOps hides a class from the Ops Classes board.
// Allowed only when class is sent_to_mentor and round_status is active.
func ArchiveClassInOps(classKey string, userID uuid.UUID) error {
	now := time.Now()
	res, err := db.DB.Exec(`
		UPDATE class_groups
		SET hidden_in_ops = true,
		    hidden_at = $2,
		    hidden_by = $3,
		    updated_at = $2
		WHERE class_key = $1
		  AND sent_to_mentor = true
		  AND COALESCE(round_status, '') = 'active'
	`, classKey, now, userID)
	if err != nil {
		return fmt.Errorf("archive class update: %w", err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("class not eligible for archive")
	}
	return nil
}

// UnarchiveClassInOps restores a class to the Ops Classes board.
func UnarchiveClassInOps(classKey string) error {
	_, err := db.DB.Exec(`
		UPDATE class_groups
		SET hidden_in_ops = false,
		    hidden_at = NULL,
		    hidden_by = NULL,
		    updated_at = NOW()
		WHERE class_key = $1
	`, classKey)
	if err != nil {
		return fmt.Errorf("unarchive class update: %w", err)
	}
	return nil
}

// ArchivedOpsClass represents an archived class entry for Ops view.
type ArchivedOpsClass struct {
	ClassKey     string
	Level        int32
	ClassDays    string
	ClassTime    string
	ClassNumber  int32
	SentAt       sql.NullTime
	RoundStarted sql.NullTime
	HiddenAt     sql.NullTime
	HiddenBy     sql.NullString
	RoundStatus  string
	SentToMentor bool
}

// GetArchivedOpsClasses returns classes hidden from Ops with optional filters.
func GetArchivedOpsClasses(classKeyLike string, fromDate, toDate *time.Time) ([]*ArchivedOpsClass, error) {
	query := `
		SELECT class_key, level, class_days, class_time, class_number,
		       sent_at, round_started_at, hidden_at, hidden_by::text,
		       COALESCE(round_status, 'not_started'), sent_to_mentor
		FROM class_groups
		WHERE hidden_in_ops = true
	`
	var args []interface{}
	argN := 1
	if classKeyLike != "" {
		query += fmt.Sprintf(" AND class_key ILIKE $%d", argN)
		args = append(args, "%"+classKeyLike+"%")
		argN++
	}
	if fromDate != nil {
		query += fmt.Sprintf(" AND hidden_at >= $%d", argN)
		args = append(args, *fromDate)
		argN++
	}
	if toDate != nil {
		query += fmt.Sprintf(" AND hidden_at <= $%d", argN)
		args = append(args, *toDate)
	}
	query += " ORDER BY hidden_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query archived classes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*ArchivedOpsClass
	for rows.Next() {
		c := &ArchivedOpsClass{}
		var roundStatus sql.NullString
		err := rows.Scan(
			&c.ClassKey, &c.Level, &c.ClassDays, &c.ClassTime, &c.ClassNumber,
			&c.SentAt, &c.RoundStarted, &c.HiddenAt, &c.HiddenBy,
			&roundStatus, &c.SentToMentor,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan archived class: %w", err)
		}
		if roundStatus.Valid {
			c.RoundStatus = roundStatus.String
		} else {
			c.RoundStatus = "not_started"
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetClassGroupWorkflowsBatch gets workflow state for multiple class keys
func GetClassGroupWorkflowsBatch(classKeys []string) (map[string]*ClassGroupWorkflow, error) {
	if len(classKeys) == 0 {
		return make(map[string]*ClassGroupWorkflow), nil
	}

	query := `SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, returned_at, updated_at,
		hidden_in_ops, hidden_at, hidden_by::text,
		COALESCE(round_status, 'not_started'), round_started_at, round_started_by::text, round_closed_at, round_closed_by::text
		FROM class_groups WHERE class_key = ANY($1)`
	rows, err := db.DB.Query(query, classKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to query class group workflows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]*ClassGroupWorkflow)
	for rows.Next() {
		wf := &ClassGroupWorkflow{}
		var sentAt, returnedAt, hiddenAt, roundStartedAt, roundClosedAt sql.NullTime
		var hiddenBy, roundStartedBy, roundClosedBy, roundStatus sql.NullString
		err := rows.Scan(
			&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
			&wf.SentToMentor, &sentAt, &returnedAt, &wf.UpdatedAt,
			&wf.HiddenInOps, &hiddenAt, &hiddenBy,
			&roundStatus, &roundStartedAt, &roundStartedBy, &roundClosedAt, &roundClosedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class group workflow: %w", err)
		}
		wf.SentAt, wf.ReturnedAt = sentAt, returnedAt
		wf.HiddenAt, wf.HiddenBy = hiddenAt, hiddenBy
		wf.RoundStartedAt, wf.RoundClosedAt = roundStartedAt, roundClosedAt
		wf.RoundStartedBy, wf.RoundClosedBy = roundStartedBy, roundClosedBy
		if roundStatus.Valid {
			wf.RoundStatus = roundStatus.String
		} else {
			wf.RoundStatus = "not_started"
		}
		result[wf.ClassKey] = wf
	}
	return result, rows.Err()
}

// UpdateLeadStatusFromPayment updates lead status based on payment state.
// When total_course_paid >= offer_final_price: set status to paid_full.
// When paid_full but total now < final: revert to offer_sent.
// Does nothing if lead is cancelled.
func UpdateLeadStatusFromPayment(leadID uuid.UUID) error {
	var currentStatus string
	err := db.DB.QueryRow(`SELECT status FROM leads WHERE id = $1`, leadID).Scan(&currentStatus)
	if err != nil {
		return fmt.Errorf("failed to get lead status: %w", err)
	}
	// Don't override statuses that are already beyond payment gating.
	if currentStatus == "cancelled" || currentStatus == "ready_to_start" || currentStatus == "in_classes" {
		return nil
	}
	var finalPrice sql.NullInt32
	err = db.DB.QueryRow(`SELECT final_price FROM offers WHERE lead_id = $1`, leadID).Scan(&finalPrice)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get offer: %w", err)
	}
	if err == sql.ErrNoRows || !finalPrice.Valid || finalPrice.Int32 <= 0 {
		return nil
	}
	totalCoursePaid, err := GetTotalCoursePaidCurrentCycle(leadID)
	if err != nil {
		return fmt.Errorf("failed to get total course paid: %w", err)
	}
	var newStatus string
	if totalCoursePaid >= finalPrice.Int32 {
		newStatus = "paid_full"
	} else if currentStatus == "paid_full" {
		newStatus = "offer_sent"
	} else {
		return nil
	}
	if newStatus != currentStatus {
		_, err = db.DB.Exec(`UPDATE leads SET status = $1, updated_at = $2 WHERE id = $3`, newStatus, time.Now(), leadID)
		if err != nil {
			return fmt.Errorf("failed to update lead status: %w", err)
		}
	}
	return nil
}

// GetTotalCoursePaid returns the net course payments for a lead (sum of payments - sum of refunds)
func GetTotalCoursePaid(leadID uuid.UUID) (int32, error) {
	// Sum all course payments from lead_payments table
	var totalPayments sql.NullInt32
	err := db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM lead_payments
		WHERE lead_id = $1
	`, leadID).Scan(&totalPayments)
	if err != nil {
		return 0, fmt.Errorf("failed to get total course payments: %w", err)
	}

	paymentsTotal := int32(0)
	if totalPayments.Valid {
		paymentsTotal = totalPayments.Int32
	}

	// Sum all refunds for this lead (OUT transactions with category='refund')
	var totalRefunds sql.NullInt32
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM transactions
		WHERE lead_id = $1
		AND transaction_type = 'OUT'
		AND category = 'refund'
	`, leadID).Scan(&totalRefunds)
	if err != nil {
		return 0, fmt.Errorf("failed to get total refunds: %w", err)
	}

	refundsTotal := int32(0)
	if totalRefunds.Valid {
		refundsTotal = totalRefunds.Int32
	}

	// Net = payments - refunds
	netTotal := paymentsTotal - refundsTotal
	if netTotal < 0 {
		netTotal = 0 // Don't return negative (shouldn't happen, but safety check)
	}

	return netTotal, nil
}

type UnusedCreditsRefundBreakdown struct {
	UnusedCreditsValue int32
	RemainingCredits   int32
	ConsumedLevels     int32
	ConsumedValue      int32
	OriginalPaidValue  int32
	BundleLevels       int32
}

type PaymentCycle struct {
	ID               uuid.UUID
	LeadID           uuid.UUID
	StartedAt        time.Time
	BundleLevels     int32
	FinalPrice       int32
	ConsumedBaseline int32
	Status           string
}

func inferBundleLevelsFromPaidAmount(amount int32) int32 {
	switch {
	case amount <= 0:
		return 0
	case amount <= 1300:
		return 1
	case amount <= 2400:
		return 2
	case amount <= 3300:
		return 3
	default:
		return 4
	}
}

func GetActivePaymentCycle(leadID uuid.UUID) (*PaymentCycle, error) {
	cycle := &PaymentCycle{}
	err := db.DB.QueryRow(`
		SELECT id, lead_id, started_at, bundle_levels, final_price, consumed_baseline, status
		FROM payment_cycles
		WHERE lead_id = $1 AND status = 'active'
		ORDER BY started_at DESC
		LIMIT 1
	`, leadID).Scan(
		&cycle.ID, &cycle.LeadID, &cycle.StartedAt, &cycle.BundleLevels,
		&cycle.FinalPrice, &cycle.ConsumedBaseline, &cycle.Status,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query active payment cycle: %w", err)
	}
	return cycle, nil
}

func UpsertActivePaymentCycle(leadID uuid.UUID, bundleLevels int32, finalPrice int32) error {
	if bundleLevels < 1 || bundleLevels > 4 {
		return fmt.Errorf("invalid bundle levels for cycle")
	}
	if finalPrice <= 0 {
		return fmt.Errorf("invalid final price for cycle")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin payment cycle tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var levelsConsumed int32
	err = tx.QueryRow(`SELECT COALESCE(levels_consumed, 0) FROM leads WHERE id = $1`, leadID).Scan(&levelsConsumed)
	if err != nil {
		return fmt.Errorf("failed to read lead consumption for cycle: %w", err)
	}

	var cycleID uuid.UUID
	var existingBundle int32
	var existingFinal int32
	var consumedBaseline int32
	err = tx.QueryRow(`
		SELECT id, bundle_levels, final_price, consumed_baseline
		FROM payment_cycles
		WHERE lead_id = $1 AND status = 'active'
		ORDER BY started_at DESC
		LIMIT 1
	`, leadID).Scan(&cycleID, &existingBundle, &existingFinal, &consumedBaseline)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to load active payment cycle: %w", err)
	}

	now := time.Now()
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`
			INSERT INTO payment_cycles (lead_id, started_at, bundle_levels, final_price, consumed_baseline, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'active', $2, $2)
		`, leadID, now, bundleLevels, finalPrice, levelsConsumed)
		if err != nil {
			return fmt.Errorf("failed to create active payment cycle: %w", err)
		}
		return tx.Commit()
	}

	consumedInCycle := levelsConsumed - consumedBaseline
	if consumedInCycle < 0 {
		consumedInCycle = 0
	}
	remainingInCycle := existingBundle - consumedInCycle

	if remainingInCycle <= 0 {
		_, err = tx.Exec(`
			UPDATE payment_cycles
			SET status = 'closed', closed_at = $2, updated_at = $2
			WHERE id = $1
		`, cycleID, now)
		if err != nil {
			return fmt.Errorf("failed to close exhausted payment cycle: %w", err)
		}
		_, err = tx.Exec(`
			INSERT INTO payment_cycles (lead_id, started_at, bundle_levels, final_price, consumed_baseline, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'active', $2, $2)
		`, leadID, now, bundleLevels, finalPrice, levelsConsumed)
		if err != nil {
			return fmt.Errorf("failed to create replacement payment cycle: %w", err)
		}
		return tx.Commit()
	}

	// Once consumption begins, cycle deal terms are immutable for safety.
	if consumedInCycle > 0 && (existingBundle != bundleLevels || existingFinal != finalPrice) {
		return fmt.Errorf("cannot modify active payment cycle after consumption has started")
	}

	_, err = tx.Exec(`
		UPDATE payment_cycles
		SET bundle_levels = $2,
		    final_price = $3,
		    updated_at = $4
		WHERE id = $1
	`, cycleID, bundleLevels, finalPrice, now)
	if err != nil {
		return fmt.Errorf("failed to update active payment cycle: %w", err)
	}

	return tx.Commit()
}

// GetUnusedCreditsRefundBreakdown calculates unused-credit refund from carryover entitlement.
// It is cycle-scoped: for returning students, only the latest pre-cycle payment (before current cycle start)
// is used for unused-credit valuation to avoid mixing with current-cycle cash flow.
func GetUnusedCreditsRefundBreakdown(leadID uuid.UUID) (*UnusedCreditsRefundBreakdown, error) {
	const singleLevelPriceEGP int32 = 1300

	breakdown := &UnusedCreditsRefundBreakdown{}

	var remainingCredits int32
	var isReturning bool
	var levelsConsumed int32
	err := db.DB.QueryRow(`
		SELECT
			GREATEST(COALESCE(levels_purchased_total, 0) - COALESCE(levels_consumed, 0), 0) AS remaining_credits,
			COALESCE(is_returning, false) AS is_returning,
			COALESCE(levels_consumed, 0) AS levels_consumed
		FROM leads
		WHERE id = $1
	`, leadID).Scan(&remainingCredits, &isReturning, &levelsConsumed)
	if err != nil {
		return nil, fmt.Errorf("failed to load lead credits: %w", err)
	}

	breakdown.RemainingCredits = remainingCredits
	if remainingCredits <= 0 {
		return breakdown, nil
	}

	// Unused-credit valuation is only for returning carryover credits.
	if !isReturning {
		return breakdown, nil
	}

	cycle, err := GetActivePaymentCycle(leadID)
	if err != nil {
		return nil, err
	}
	if cycle == nil {
		// Legacy fallback only when cycle record is absent.
		var cycleStart sql.NullTime
		err = db.DB.QueryRow(`
			SELECT MAX(completed_at)
			FROM class_enrollments
			WHERE lead_id = $1
		`, leadID).Scan(&cycleStart)
		if err != nil {
			return nil, fmt.Errorf("failed to get current cycle start: %w", err)
		}
		if !cycleStart.Valid {
			return nil, fmt.Errorf("cannot value unused credits without a prior completed class")
		}

		var paidAmount int32
		err = db.DB.QueryRow(`
			SELECT amount
			FROM lead_payments
			WHERE lead_id = $1
			  AND created_at < $2
			ORDER BY created_at DESC
			LIMIT 1
		`, leadID, cycleStart.Time).Scan(&paidAmount)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("cannot value unused credits: no pre-cycle payment found")
			}
			return nil, fmt.Errorf("failed to get pre-cycle payment: %w", err)
		}
		if paidAmount <= 0 {
			return nil, fmt.Errorf("cannot value unused credits: invalid pre-cycle payment amount")
		}

		bundleLevels := inferBundleLevelsFromPaidAmount(paidAmount)
		if bundleLevels < remainingCredits {
			bundleLevels = remainingCredits
		}
		consumedLevels := bundleLevels - remainingCredits
		if consumedLevels < 0 {
			consumedLevels = 0
		}
		consumedValue := consumedLevels * singleLevelPriceEGP
		unusedCreditsValue := paidAmount - consumedValue
		if unusedCreditsValue < 0 {
			unusedCreditsValue = 0
		}
		maxUnusedByStandard := remainingCredits * singleLevelPriceEGP
		if unusedCreditsValue > maxUnusedByStandard {
			unusedCreditsValue = maxUnusedByStandard
		}
		breakdown.BundleLevels = bundleLevels
		breakdown.ConsumedLevels = consumedLevels
		breakdown.ConsumedValue = consumedValue
		breakdown.OriginalPaidValue = paidAmount
		breakdown.UnusedCreditsValue = unusedCreditsValue
		log.Printf("💰 Carryover refund (legacy fallback): lead=%s paid=%d bundle_levels=%d remaining=%d consumed=%d unused=%d",
			leadID, paidAmount, bundleLevels, remainingCredits, consumedLevels, unusedCreditsValue)
		return breakdown, nil
	}

	consumedInCycle := levelsConsumed - cycle.ConsumedBaseline
	if consumedInCycle < 0 {
		consumedInCycle = 0
	}
	remainingInCycle := cycle.BundleLevels - consumedInCycle
	if remainingInCycle < 0 {
		remainingInCycle = 0
	}

	refundableLevels := remainingCredits
	if remainingInCycle < refundableLevels {
		refundableLevels = remainingInCycle
	}
	consumedLevels := cycle.BundleLevels - refundableLevels
	if consumedLevels < 0 {
		consumedLevels = 0
	}

	consumedValue := consumedLevels * singleLevelPriceEGP
	unusedCreditsValue := cycle.FinalPrice - consumedValue
	if unusedCreditsValue < 0 {
		unusedCreditsValue = 0
	}

	// Safety ceiling: unused-credit value should not exceed standard value of remaining levels.
	maxUnusedByStandard := refundableLevels * singleLevelPriceEGP
	if unusedCreditsValue > maxUnusedByStandard {
		unusedCreditsValue = maxUnusedByStandard
	}

	breakdown.BundleLevels = cycle.BundleLevels
	breakdown.ConsumedLevels = consumedLevels
	breakdown.ConsumedValue = consumedValue
	breakdown.OriginalPaidValue = cycle.FinalPrice
	breakdown.UnusedCreditsValue = unusedCreditsValue

	log.Printf("💰 Carryover refund (cycle): lead=%s paid=%d bundle_levels=%d remaining=%d consumed=%d unused=%d",
		leadID, cycle.FinalPrice, cycle.BundleLevels, refundableLevels, consumedLevels, unusedCreditsValue)

	return breakdown, nil
}

// CalculateUnusedCreditsRefund returns only the unused-credits value.
func CalculateUnusedCreditsRefund(leadID uuid.UUID) (int32, error) {
	breakdown, err := GetUnusedCreditsRefundBreakdown(leadID)
	if err != nil {
		return 0, err
	}
	return breakdown.UnusedCreditsValue, nil
}

// GetTotalCoursePaidCurrentCycle returns the net course payments in the active cycle.
// It uses payment_cycles.started_at when available, and falls back to last class completion for legacy rows.
func GetTotalCoursePaidCurrentCycle(leadID uuid.UUID) (int32, error) {
	cycle, err := GetActivePaymentCycle(leadID)
	if err != nil {
		return 0, err
	}
	var cycleStart time.Time
	if cycle != nil {
		cycleStart = cycle.StartedAt
	} else {
		var legacyStart sql.NullTime
		err = db.DB.QueryRow(`
			SELECT MAX(completed_at)
			FROM class_enrollments
			WHERE lead_id = $1
		`, leadID).Scan(&legacyStart)
		if err != nil {
			return 0, fmt.Errorf("failed to get cycle start: %w", err)
		}
		if !legacyStart.Valid {
			return GetTotalCoursePaid(leadID)
		}
		cycleStart = legacyStart.Time
	}

	// Sum all course payments from lead_payments table since cycle start.
	// Safety fallback on payment_date avoids missing same-cycle payments when legacy
	// rows were inserted a few milliseconds before cycle_started_at.
	var totalPayments sql.NullInt32
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM lead_payments
		WHERE lead_id = $1
		  AND (
		    created_at >= $2
		    OR payment_date >= $3::date
		  )
	`, leadID, cycleStart, cycleStart).Scan(&totalPayments)
	if err != nil {
		return 0, fmt.Errorf("failed to get current-cycle payments: %w", err)
	}

	paymentsTotal := int32(0)
	if totalPayments.Valid {
		paymentsTotal = totalPayments.Int32
	}

	// Sum all refunds for this lead since cycle start.
	var totalRefunds sql.NullInt32
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM transactions
		WHERE lead_id = $1
		  AND transaction_type = 'OUT'
		  AND category = 'refund'
		  AND transaction_date >= $2::date
	`, leadID, cycleStart).Scan(&totalRefunds)
	if err != nil {
		return 0, fmt.Errorf("failed to get current-cycle refunds: %w", err)
	}

	refundsTotal := int32(0)
	if totalRefunds.Valid {
		refundsTotal = totalRefunds.Int32
	}

	netTotal := paymentsTotal - refundsTotal
	if netTotal < 0 {
		netTotal = 0
	}

	return netTotal, nil
}

// GetLeadPayments returns all course payments for a lead
func GetLeadPayments(leadID uuid.UUID) ([]*LeadPayment, error) {
	rows, err := db.DB.Query(`
		SELECT id, lead_id, kind, amount, payment_method, payment_date, notes, created_at, updated_at
		FROM lead_payments
		WHERE lead_id = $1
		ORDER BY payment_date DESC, created_at DESC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query lead payments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var payments []*LeadPayment
	for rows.Next() {
		p := &LeadPayment{}
		var notes sql.NullString
		err := rows.Scan(
			&p.ID, &p.LeadID, &p.Kind, &p.Amount, &p.PaymentMethod,
			&p.PaymentDate, &notes, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lead payment: %w", err)
		}
		p.Notes = notes
		payments = append(payments, p)
	}

	return payments, rows.Err()
}

func GetLeadPaymentsSince(leadID uuid.UUID, since time.Time) ([]*LeadPayment, error) {
	rows, err := db.DB.Query(`
		SELECT id, lead_id, kind, amount, payment_method, payment_date, notes, created_at, updated_at
		FROM lead_payments
		WHERE lead_id = $1
		  AND created_at >= $2
		ORDER BY payment_date DESC, created_at DESC
	`, leadID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to query lead payments since: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var payments []*LeadPayment
	for rows.Next() {
		p := &LeadPayment{}
		var notes sql.NullString
		err := rows.Scan(
			&p.ID, &p.LeadID, &p.Kind, &p.Amount, &p.PaymentMethod,
			&p.PaymentDate, &notes, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lead payment: %w", err)
		}
		p.Notes = notes
		payments = append(payments, p)
	}

	return payments, rows.Err()
}

func GetLeadPaymentsBefore(leadID uuid.UUID, before time.Time) ([]*LeadPayment, error) {
	rows, err := db.DB.Query(`
		SELECT id, lead_id, kind, amount, payment_method, payment_date, notes, created_at, updated_at
		FROM lead_payments
		WHERE lead_id = $1
		  AND created_at < $2
		ORDER BY payment_date DESC, created_at DESC
	`, leadID, before)
	if err != nil {
		return nil, fmt.Errorf("failed to query lead payments before: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var payments []*LeadPayment
	for rows.Next() {
		p := &LeadPayment{}
		var notes sql.NullString
		err := rows.Scan(
			&p.ID, &p.LeadID, &p.Kind, &p.Amount, &p.PaymentMethod,
			&p.PaymentDate, &notes, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lead payment: %w", err)
		}
		p.Notes = notes
		payments = append(payments, p)
	}

	return payments, rows.Err()
}

// CreateLeadPayment creates a course payment record and corresponding finance transaction
func CreateLeadPayment(leadID uuid.UUID, kind string, amount int32, paymentMethod string, paymentDate time.Time, notes string) (*LeadPayment, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	// Validate payment date is not in the future
	if err := util.ValidateNotFutureDate(paymentDate); err != nil {
		return nil, err
	}

	// Validate kind is one of allowed values
	allowedKinds := map[string]bool{
		"course":       true,
		"deposit":      true,
		"full_payment": true,
		"top_up":       true,
	}
	if !allowedKinds[kind] {
		return nil, fmt.Errorf("invalid payment kind: %s", kind)
	}

	// Validate payment method
	allowedMethods := map[string]bool{
		"vodafone_cash": true,
		"bank_transfer": true,
		"paypal":        true,
		"other":         true,
	}
	if !allowedMethods[paymentMethod] {
		return nil, fmt.Errorf("invalid payment method: %s", paymentMethod)
	}

	payment := &LeadPayment{
		ID:            uuid.New(),
		LeadID:        leadID,
		Kind:          kind,
		Amount:        amount,
		PaymentMethod: paymentMethod,
		PaymentDate:   paymentDate,
		Notes:         sql.NullString{String: notes, Valid: notes != ""},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Insert payment record
	_, err := db.DB.Exec(`
		INSERT INTO lead_payments (id, lead_id, kind, amount, payment_method, payment_date, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
	`, payment.ID, payment.LeadID, payment.Kind, payment.Amount, payment.PaymentMethod, payment.PaymentDate, payment.Notes, payment.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create lead payment: %w", err)
	}

	// Create corresponding finance transaction (IN)
	refKey := fmt.Sprintf("lead:%s:course_payment:%s", leadID.String(), payment.ID.String())
	refIDStr := leadID.String()
	paymentDateValue := paymentDate.Format("2006-01-02")
	now := payment.CreatedAt

	_, err = db.DB.Exec(`
		INSERT INTO transactions (id, transaction_date, transaction_type, category, amount, payment_method, lead_id, ref_type, ref_id, ref_sub_type, ref_key, notes, created_at, updated_at)
		VALUES ($1, $2::date, $3::text, $4::text, $5::integer, $6::text, $7::uuid, $8::text, $9::text, $10::text, $11::text, $12, $13::timestamp with time zone, $13::timestamp with time zone)
	`, uuid.New(), paymentDateValue, "IN", "course_payment", amount, paymentMethod, leadID, "lead", refIDStr, "course_payment", refKey, payment.Notes, now)
	if err != nil {
		// Rollback payment if transaction creation fails
		if _, deleteErr := db.DB.Exec(`DELETE FROM lead_payments WHERE id = $1`, payment.ID); deleteErr != nil {
			log.Printf("WARNING: failed to rollback lead payment %s: %v", payment.ID, deleteErr)
		}
		return nil, fmt.Errorf("failed to create finance transaction: %w", err)
	}

	if err := UpdateLeadStatusFromPayment(leadID); err != nil {
		// Log but don't fail
		log.Printf("WARNING: failed to auto-update lead status after payment: %v", err)
	}

	return payment, nil
}

// CreateRefund creates a refund transaction (OUT) for a lead
func CreateRefund(leadID uuid.UUID, amount int32, paymentMethod string, transactionDate time.Time, notes string) (*Transaction, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	// Validate transaction date is not in the future
	if err := util.ValidateNotFutureDate(transactionDate); err != nil {
		return nil, err
	}

	// Validate payment method
	allowedMethods := map[string]bool{
		"vodafone_cash": true,
		"bank_transfer": true,
		"paypal":        true,
		"other":         true,
	}
	if !allowedMethods[paymentMethod] {
		return nil, fmt.Errorf("invalid payment method: %s", paymentMethod)
	}

	// Validate refund doesn't exceed refundable amount (session-based rule)
	refundableAmount, err := GetRefundableAmount(leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate refund amount: %w", err)
	}
	if amount > refundableAmount {
		return nil, fmt.Errorf("refund amount (%d) cannot exceed refundable amount (%d)", amount, refundableAmount)
	}

	// Create ref_key for traceability
	refKey := fmt.Sprintf("lead:%s:refund:%s", leadID.String(), uuid.New().String())
	refIDStr := leadID.String()
	now := time.Now()
	transactionDateValue := transactionDate.Format("2006-01-02")

	tx := &Transaction{
		ID:              uuid.New(),
		TransactionDate: transactionDate,
		TransactionType: "OUT",
		Category:        "refund",
		Amount:          amount,
		PaymentMethod:   sql.NullString{String: paymentMethod, Valid: true},
		LeadID:          sql.NullString{String: leadID.String(), Valid: true},
		RefType:         sql.NullString{String: "lead", Valid: true},
		RefID:           sql.NullString{String: refIDStr, Valid: true},
		RefSubType:      sql.NullString{String: "refund", Valid: true},
		RefKey:          sql.NullString{String: refKey, Valid: true},
		Notes:           sql.NullString{String: notes, Valid: notes != ""},
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	_, err = db.DB.Exec(`
		INSERT INTO transactions (id, transaction_date, transaction_type, category, amount, payment_method, lead_id, ref_type, ref_id, ref_sub_type, ref_key, notes, created_at, updated_at)
		VALUES ($1, $2::date, $3::text, $4::text, $5::integer, $6::text, $7::uuid, $8::text, $9::text, $10::text, $11::text, $12, $13::timestamp with time zone, $13::timestamp with time zone)
	`, tx.ID, transactionDateValue, tx.TransactionType, tx.Category, tx.Amount, tx.PaymentMethod, leadID, "lead", refIDStr, "refund", refKey, tx.Notes, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create refund transaction: %w", err)
	}

	if err := UpdateLeadStatusFromPayment(leadID); err != nil {
		log.Printf("WARNING: failed to auto-update lead status after refund: %v", err)
	}

	return tx, nil
}

// CreateCancelRefundIdempotent creates a refund (OUT) for cancel flow with deterministic ref_key.
// Retries do not double-create. Uses ref_key = "cancel_refund:<leadID>:<date>:<amount>".
func CreateCancelRefundIdempotent(leadID uuid.UUID, amount int32, paymentMethod string, transactionDate time.Time, notes string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if err := util.ValidateNotFutureDate(transactionDate); err != nil {
		return err
	}
	allowedMethods := map[string]bool{
		"vodafone_cash": true, "bank_transfer": true, "paypal": true, "other": true,
	}
	if !allowedMethods[paymentMethod] {
		return fmt.Errorf("invalid payment method: %s", paymentMethod)
	}
	refundableAmount, err := GetCancelRefundableAmount(leadID)
	if err != nil {
		return fmt.Errorf("failed to validate refund amount: %w", err)
	}
	if amount > refundableAmount {
		return fmt.Errorf("refund amount (%d) cannot exceed refundable amount (%d)", amount, refundableAmount)
	}

	refKey := fmt.Sprintf("cancel_refund:%s:%s:%d", leadID.String(), transactionDate.Format("2006-01-02"), amount)
	refIDStr := leadID.String()
	now := time.Now()
	transactionDateValue := transactionDate.Format("2006-01-02")

	var id uuid.UUID
	err = db.DB.QueryRow(`
		INSERT INTO transactions (id, transaction_date, transaction_type, category, amount, payment_method, lead_id, ref_type, ref_id, ref_sub_type, ref_key, notes, created_at, updated_at)
		VALUES (gen_random_uuid(), $1::date, 'OUT', 'refund', $2::integer, $3::text, $4::uuid, 'lead', $5::text, 'refund', $6::text, $7, $8::timestamp with time zone, $8::timestamp with time zone)
		ON CONFLICT (ref_key) DO NOTHING
		RETURNING id
	`, transactionDateValue, amount, paymentMethod, leadID, refIDStr, refKey, notes, now).Scan(&id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to create cancel refund transaction: %w", err)
	}
	if err := UpdateLeadStatusFromPayment(leadID); err != nil {
		log.Printf("WARNING: failed to auto-update lead status after cancel refund: %v", err)
	}
	return nil
}

// GetCancelRefundableAmount returns the maximum refundable amount for the cancel-lead flow.
// It must stay aligned with pre-enrolment cancel modal calculation:
// use unused-credits valuation when present; otherwise fallback to course paid.
func GetCancelRefundableAmount(leadID uuid.UUID) (int32, error) {
	lead, err := GetLeadByID(leadID)
	if err != nil {
		return 0, fmt.Errorf("failed to load lead detail: %w", err)
	}
	if lead == nil || lead.Lead == nil {
		return 0, fmt.Errorf("lead detail not found")
	}

	hasRemainingCredits := false
	if lead.Lead.LevelsPurchasedTotal.Valid {
		remaining := lead.Lead.LevelsPurchasedTotal.Int32
		if lead.Lead.LevelsConsumed.Valid {
			remaining -= lead.Lead.LevelsConsumed.Int32
		}
		hasRemainingCredits = remaining > 0
	} else if lead.Lead.RemainingCredits.Valid {
		hasRemainingCredits = lead.Lead.RemainingCredits.Int32 > 0
	}

	var totalCoursePaid int32
	if hasRemainingCredits || lead.Lead.IsReturning {
		totalCoursePaid, err = GetTotalCoursePaidCurrentCycle(leadID)
	} else {
		totalCoursePaid, err = GetTotalCoursePaid(leadID)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get course paid total: %w", err)
	}

	var unusedCreditsValue int32
	if hasRemainingCredits {
		unusedCreditsValue, err = CalculateUnusedCreditsRefund(leadID)
		if err != nil {
			return 0, fmt.Errorf("failed to calculate unused credits refund: %w", err)
		}
	}

	if unusedCreditsValue > 0 {
		return unusedCreditsValue, nil
	}
	return totalCoursePaid, nil
}

// CancelLead soft-cancels a lead (sets status to cancelled, does not delete)
func CancelLead(leadID uuid.UUID) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		UPDATE leads 
		SET status = 'cancelled', cancelled_at = $1, updated_at = $1
		WHERE id = $2
	`, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to cancel lead: %w", err)
	}
	return nil
}

// ReopenLead reopens a cancelled lead (sets status back to a valid active status)
func ReopenLead(leadID uuid.UUID) error {
	// Set status to lead_created as default, admin can update later
	_, err := db.DB.Exec(`
		UPDATE leads 
		SET status = 'lead_created', cancelled_at = NULL, updated_at = $1
		WHERE id = $2 AND status = 'cancelled'
	`, time.Now(), leadID)
	if err != nil {
		return fmt.Errorf("failed to reopen lead: %w", err)
	}
	return nil
}

// CreateExpense creates an OUT transaction for an expense
func CreateExpense(category string, amount int32, paymentMethod string, transactionDate time.Time, notes string) (*Transaction, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	// Validate transaction date is not in the future
	if err := util.ValidateNotFutureDate(transactionDate); err != nil {
		return nil, err
	}

	// Validate payment method
	allowedMethods := map[string]bool{
		"vodafone_cash": true,
		"bank_transfer": true,
		"paypal":        true,
		"other":         true,
	}
	if !allowedMethods[paymentMethod] {
		return nil, fmt.Errorf("invalid payment method: %s", paymentMethod)
	}

	tx := &Transaction{
		ID:              uuid.New(),
		TransactionDate: transactionDate,
		TransactionType: "OUT",
		Category:        category,
		Amount:          amount,
		PaymentMethod:   sql.NullString{String: paymentMethod, Valid: true},
		Notes:           sql.NullString{String: notes, Valid: notes != ""},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	transactionDateValue := transactionDate.Format("2006-01-02")
	_, err := db.DB.Exec(`
		INSERT INTO transactions (id, transaction_date, transaction_type, category, amount, payment_method, notes, created_at, updated_at)
		VALUES ($1, $2::date, $3::text, $4::text, $5::integer, $6::text, $7, $8::timestamp with time zone, $8::timestamp with time zone)
	`, tx.ID, transactionDateValue, tx.TransactionType, tx.Category, tx.Amount, tx.PaymentMethod, tx.Notes, tx.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create expense: %w", err)
	}

	return tx, nil
}

// UpsertPlacementTestIncome creates or updates a finance transaction for placement test payment
func UpsertPlacementTestIncome(leadID uuid.UUID, amountPaid int32, paymentDate sql.NullTime, paymentMethod sql.NullString) error {
	if amountPaid <= 0 {
		// No payment, nothing to sync
		return nil
	}

	if !paymentDate.Valid {
		return fmt.Errorf("payment date is required")
	}

	// Validate payment date is not in the future
	if err := util.ValidateNotFutureDate(paymentDate.Time); err != nil {
		return err
	}

	if !paymentMethod.Valid {
		return fmt.Errorf("payment method is required")
	}

	// Create unique ref_key for idempotency
	refKey := fmt.Sprintf("lead:%s:placement_test", leadID.String())
	refIDStr := leadID.String()
	paymentDateValue := paymentDate.Time.Format("2006-01-02")
	now := time.Now()

	// Use ON CONFLICT to update if exists, insert if not
	_, err := db.DB.Exec(`
		INSERT INTO transactions (id, transaction_date, transaction_type, category, amount, payment_method, lead_id, ref_type, ref_id, ref_sub_type, ref_key, created_at, updated_at)
		VALUES (gen_random_uuid(), $1::date, $2::text, $3::text, $4::integer, $5::text, $6::uuid, $7::text, $8::text, $9::text, $10::text, $11::timestamp with time zone, $11::timestamp with time zone)
		ON CONFLICT (ref_key) DO UPDATE SET
			transaction_date = EXCLUDED.transaction_date,
			amount = EXCLUDED.amount,
			payment_method = EXCLUDED.payment_method,
			updated_at = EXCLUDED.updated_at
	`, paymentDateValue, "IN", "placement_test", amountPaid, paymentMethod.String, leadID, "lead", refIDStr, "placement_test", refKey, now)

	if err != nil {
		return fmt.Errorf("failed to upsert placement test income: %w", err)
	}

	return nil
}

// CalculateLevelsPurchased calculates levels purchased and bundle type from total paid amount
// Bundle prices: 1 level = 1300, 2 levels = 2400, 3 levels = 3300, 4 levels = 4000
func CalculateLevelsPurchased(bundleLevels sql.NullInt32, totalPaid int32) (levelsPurchased sql.NullInt32, bundleType sql.NullString) {
	if !bundleLevels.Valid || bundleLevels.Int32 <= 0 {
		return sql.NullInt32{Valid: false}, sql.NullString{String: "none", Valid: true}
	}

	// If bundle levels is specified, use it
	levelsPurchased = bundleLevels
	bundleType = sql.NullString{String: fmt.Sprintf("bundle%d", bundleLevels.Int32), Valid: true}
	if bundleLevels.Int32 == 1 {
		bundleType = sql.NullString{String: "single", Valid: true}
	}

	return levelsPurchased, bundleType
}

// UpdateLeadCreditsFromPayments updates lead's levels_purchased_total and bundle_type based on payments
func UpdateLeadCreditsFromPayments(leadID uuid.UUID, bundleLevels sql.NullInt32) error {
	payments, err := GetLeadPayments(leadID)
	if err != nil {
		return err
	}

	var totalPaid int32 = 0
	for _, p := range payments {
		totalPaid += p.Amount
	}

	levelsPurchased, bundleType := CalculateLevelsPurchased(bundleLevels, totalPaid)

	if levelsPurchased.Valid {
		if err := UpdateLeadPurchasedLevels(leadID, levelsPurchased.Int32); err != nil {
			return err
		}
	}

	_, err = db.DB.Exec(`
		UPDATE leads SET 
			bundle_type = $1,
			updated_at = $2
		WHERE id = $3
	`, bundleType, time.Now(), leadID)

	return err
}

// EnsureFinanceLedgerSync backfills missing finance transactions for legacy payment records.
// Idempotent via ref_key uniqueness.
func EnsureFinanceLedgerSync() error {
	now := time.Now()

	// Backfill placement test payments
	_, err := db.DB.Exec(`
		INSERT INTO transactions (
			id, transaction_date, transaction_type, category, amount, payment_method, lead_id,
			ref_type, ref_id, ref_sub_type, ref_key, created_at, updated_at
		)
		SELECT
			gen_random_uuid(),
			pt.placement_test_payment_date,
			'IN',
			'placement_test',
			pt.placement_test_fee_paid,
			pt.placement_test_payment_method,
			pt.lead_id,
			'lead',
			pt.lead_id::text,
			'placement_test',
			'lead:' || pt.lead_id::text || ':placement_test',
			$1,
			$1
		FROM placement_tests pt
		WHERE pt.placement_test_fee_paid IS NOT NULL
		  AND pt.placement_test_fee_paid > 0
		  AND pt.placement_test_payment_date IS NOT NULL
		  AND pt.placement_test_payment_method IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM transactions t
			WHERE t.ref_key = 'lead:' || pt.lead_id::text || ':placement_test'
		  )
	`, now)
	if err != nil {
		return fmt.Errorf("failed to backfill placement test transactions: %w", err)
	}

	// Backfill course payments
	_, err = db.DB.Exec(`
		INSERT INTO transactions (
			id, transaction_date, transaction_type, category, amount, payment_method, lead_id,
			ref_type, ref_id, ref_sub_type, ref_key, notes, created_at, updated_at
		)
		SELECT
			gen_random_uuid(),
			lp.payment_date,
			'IN',
			'course_payment',
			lp.amount,
			lp.payment_method,
			lp.lead_id,
			'lead',
			lp.lead_id::text,
			'course_payment',
			'lead:' || lp.lead_id::text || ':course_payment:' || lp.id::text,
			lp.notes,
			$1,
			$1
		FROM lead_payments lp
		WHERE NOT EXISTS (
			SELECT 1 FROM transactions t
			WHERE t.ref_key = 'lead:' || lp.lead_id::text || ':course_payment:' || lp.id::text
		)
	`, now)
	if err != nil {
		return fmt.Errorf("failed to backfill course payment transactions: %w", err)
	}

	return nil
}

// GetFinanceSummary returns aggregated finance data for today and date range
func GetFinanceSummary(dateFrom, dateTo sql.NullTime) (*FinanceSummary, error) {
	today := time.Now().Format("2006-01-02")

	summary := &FinanceSummary{
		INByCategory:  make(map[string]int32),
		OUTByCategory: make(map[string]int32),
		CreditsBreakdown: map[string]int{
			"0":  0,
			"1":  0,
			"2":  0,
			"3+": 0,
		},
	}

	// Today's totals
	var todayIN, todayOUT sql.NullInt32
	err := db.DB.QueryRow(`
		SELECT 
			COALESCE(SUM(CASE WHEN transaction_type = 'IN' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN transaction_type = 'OUT' THEN amount ELSE 0 END), 0)
		FROM transactions
		WHERE transaction_date = $1::date
	`, today).Scan(&todayIN, &todayOUT)
	if err != nil {
		return nil, fmt.Errorf("failed to get today's totals: %w", err)
	}
	if todayIN.Valid {
		summary.TodayIN = todayIN.Int32
	}
	if todayOUT.Valid {
		summary.TodayOUT = todayOUT.Int32
	}
	summary.TodayNet = summary.TodayIN - summary.TodayOUT

	// Date range totals
	rangeQuery := "SELECT COALESCE(SUM(CASE WHEN transaction_type = 'IN' THEN amount ELSE 0 END), 0), COALESCE(SUM(CASE WHEN transaction_type = 'OUT' THEN amount ELSE 0 END), 0) FROM transactions WHERE 1=1"
	rangeArgs := []interface{}{}
	argIndex := 1

	if dateFrom.Valid {
		rangeQuery += fmt.Sprintf(" AND transaction_date >= $%d::date", argIndex)
		rangeArgs = append(rangeArgs, dateFrom.Time.Format("2006-01-02"))
		argIndex++
	}
	if dateTo.Valid {
		rangeQuery += fmt.Sprintf(" AND transaction_date <= $%d::date", argIndex)
		rangeArgs = append(rangeArgs, dateTo.Time.Format("2006-01-02"))
	}

	var rangeIN, rangeOUT sql.NullInt32
	if len(rangeArgs) > 0 {
		err = db.DB.QueryRow(rangeQuery, rangeArgs...).Scan(&rangeIN, &rangeOUT)
	} else {
		err = db.DB.QueryRow(rangeQuery).Scan(&rangeIN, &rangeOUT)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get range totals: %w", err)
	}
	if rangeIN.Valid {
		summary.RangeIN = rangeIN.Int32
	}
	if rangeOUT.Valid {
		summary.RangeOUT = rangeOUT.Int32
	}
	summary.RangeNet = summary.RangeIN - summary.RangeOUT

	// Category breakdowns for date range
	categoryQuery := "SELECT category, transaction_type, COALESCE(SUM(amount), 0) FROM transactions WHERE 1=1"
	if dateFrom.Valid {
		categoryQuery += fmt.Sprintf(" AND transaction_date >= $%d::date", len(rangeArgs)-1)
	}
	if dateTo.Valid {
		categoryQuery += fmt.Sprintf(" AND transaction_date <= $%d::date", len(rangeArgs))
	}
	categoryQuery += " GROUP BY category, transaction_type"

	var categoryRows *sql.Rows
	if len(rangeArgs) > 0 {
		categoryRows, err = db.DB.Query(categoryQuery, rangeArgs...)
	} else {
		categoryRows, err = db.DB.Query(categoryQuery)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category breakdown: %w", err)
	}
	defer func() {
		_ = categoryRows.Close()
	}()

	for categoryRows.Next() {
		var category, txType string
		var amount int32
		err := categoryRows.Scan(&category, &txType, &amount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category: %w", err)
		}
		if txType == "IN" {
			summary.INByCategory[category] = amount
		} else {
			summary.OUTByCategory[category] = amount
		}
	}

	// Credits summary aligned with BI scope:
	// - non-cancelled leads only
	// - remaining credits computed as GREATEST(purchased - consumed, 0)
	var totalRemaining sql.NullInt32
	var studentsWithCredits sql.NullInt32
	var creditsTracked sql.NullInt32
	err = db.DB.QueryRow(`
		WITH credits AS (
			SELECT
				status,
				GREATEST(COALESCE(levels_purchased_total, 0) - COALESCE(levels_consumed, 0), 0) AS remaining_credits
			FROM leads
		)
		SELECT
			COALESCE(SUM(remaining_credits), 0)::int,
			COUNT(*) FILTER (WHERE remaining_credits > 0)::int,
			COUNT(*)::int
		FROM credits
		WHERE status <> 'cancelled'
	`).Scan(&totalRemaining, &studentsWithCredits, &creditsTracked)
	if err == nil && totalRemaining.Valid {
		summary.TotalRemainingLevels = totalRemaining.Int32
	}
	if err == nil && studentsWithCredits.Valid {
		summary.StudentsWithCredits = int(studentsWithCredits.Int32)
	}
	if err == nil && creditsTracked.Valid {
		summary.CreditsTracked = int(creditsTracked.Int32)
	}

	// Credits breakdown by count
	creditsRows, err := db.DB.Query(`
		WITH credits AS (
			SELECT
				status,
				GREATEST(COALESCE(levels_purchased_total, 0) - COALESCE(levels_consumed, 0), 0) AS remaining_credits
			FROM leads
		)
		SELECT
			CASE 
				WHEN remaining_credits = 0 THEN '0'
				WHEN remaining_credits = 1 THEN '1'
				WHEN remaining_credits = 2 THEN '2'
				ELSE '3+'
			END as bucket,
			COUNT(*)
		FROM credits
		WHERE status <> 'cancelled'
		GROUP BY bucket
	`)
	if err == nil {
		defer func() {
			_ = creditsRows.Close()
		}()
		for creditsRows.Next() {
			var bucket string
			var count int
			if err := creditsRows.Scan(&bucket, &count); err == nil {
				summary.CreditsBreakdown[bucket] = count
			}
		}
	}

	return summary, nil
}

// GetCurrentCashBalance returns SUM(IN) - SUM(OUT) over full history (no date filter).
func GetCurrentCashBalance() (int32, error) {
	var totalIN, totalOUT sql.NullInt32
	err := db.DB.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN transaction_type = 'IN' THEN amount ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN transaction_type = 'OUT' THEN amount ELSE 0 END), 0)
		FROM transactions
	`).Scan(&totalIN, &totalOUT)
	if err != nil {
		return 0, fmt.Errorf("failed to get current cash balance: %w", err)
	}
	in := int32(0)
	if totalIN.Valid {
		in = totalIN.Int32
	}
	out := int32(0)
	if totalOUT.Valid {
		out = totalOUT.Int32
	}
	return in - out, nil
}

// GetCurrentCashBalanceByPaymentMethod returns IN/OUT/Net grouped as Cash (vodafone_cash, other) vs Bank (bank_transfer, paypal).
func GetCurrentCashBalanceByPaymentMethod() ([]PaymentMethodBalance, error) {
	rows, err := db.DB.Query(`
		SELECT bucket,
			COALESCE(SUM(in_amt), 0)::integer AS in_total,
			COALESCE(SUM(out_amt), 0)::integer AS out_total
		FROM (
			SELECT
				CASE WHEN payment_method IN ('vodafone_cash', 'other') OR payment_method IS NULL THEN 'Cash' ELSE 'Bank' END AS bucket,
				CASE WHEN transaction_type = 'IN' THEN amount ELSE 0 END AS in_amt,
				CASE WHEN transaction_type = 'OUT' THEN amount ELSE 0 END AS out_amt
			FROM transactions
		) t
		GROUP BY bucket
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance by payment method: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []PaymentMethodBalance
	for rows.Next() {
		var b PaymentMethodBalance
		var inT, outT int32
		if err := rows.Scan(&b.Label, &inT, &outT); err != nil {
			return nil, fmt.Errorf("scan balance by method: %w", err)
		}
		b.In = inT
		b.Out = outT
		b.Net = inT - outT
		result = append(result, b)
	}
	return result, nil
}

// GetTransactions returns paginated transactions with optional filters
func GetTransactions(dateFrom, dateTo sql.NullTime, transactionTypeFilter, categoryFilter, paymentMethodFilter string, limit, offset int) ([]*Transaction, error) {
	query := `
		SELECT id, transaction_date, transaction_type, category, amount, payment_method, lead_id, notes, 
		       ref_type, ref_id, ref_sub_type, ref_key, created_at, updated_at
		FROM transactions
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	if dateFrom.Valid {
		query += fmt.Sprintf(" AND transaction_date >= $%d::date", argIndex)
		args = append(args, dateFrom.Time.Format("2006-01-02"))
		argIndex++
	}
	if dateTo.Valid {
		query += fmt.Sprintf(" AND transaction_date <= $%d::date", argIndex)
		args = append(args, dateTo.Time.Format("2006-01-02"))
		argIndex++
	}
	if transactionTypeFilter != "" {
		query += fmt.Sprintf(" AND transaction_type = $%d", argIndex)
		args = append(args, transactionTypeFilter)
		argIndex++
	}
	if categoryFilter != "" {
		query += fmt.Sprintf(" AND category = $%d", argIndex)
		args = append(args, categoryFilter)
		argIndex++
	}
	if paymentMethodFilter != "" {
		query += fmt.Sprintf(" AND payment_method = $%d", argIndex)
		args = append(args, paymentMethodFilter)
		argIndex++
	}

	query += " ORDER BY transaction_date DESC, created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, limit, offset)

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var transactions []*Transaction
	for rows.Next() {
		tx := &Transaction{}
		var paymentMethod, leadID, notes, refType, refID, refSubType, refKey sql.NullString
		var transactionDate time.Time

		err := rows.Scan(
			&tx.ID, &transactionDate, &tx.TransactionType, &tx.Category, &tx.Amount,
			&paymentMethod, &leadID, &notes, &refType, &refID, &refSubType, &refKey,
			&tx.CreatedAt, &tx.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}

		tx.TransactionDate = transactionDate
		tx.PaymentMethod = paymentMethod
		tx.LeadID = leadID
		tx.Notes = notes
		tx.RefType = refType
		tx.RefID = refID
		tx.RefSubType = refSubType
		tx.RefKey = refKey

		transactions = append(transactions, tx)
	}

	return transactions, rows.Err()
}

// GroupTransactionsByDay groups transactions by date and calculates daily totals
// Transactions should be ordered by date DESC (newest first)
func GroupTransactionsByDay(transactions []*Transaction) []*LedgerDayGroup {
	if len(transactions) == 0 {
		return []*LedgerDayGroup{}
	}

	// Map to store groups by date string (YYYY-MM-DD)
	groupsMap := make(map[string]*LedgerDayGroup)
	// Slice to preserve order (newest first)
	orderedDates := []string{}

	for _, tx := range transactions {
		// Get date key (YYYY-MM-DD)
		dateKey := tx.TransactionDate.Format("2006-01-02")

		// Get or create group for this date
		group, exists := groupsMap[dateKey]
		if !exists {
			// Create new group
			// Normalize date to start of day for consistent Date field
			date := time.Date(tx.TransactionDate.Year(), tx.TransactionDate.Month(), tx.TransactionDate.Day(), 0, 0, 0, 0, tx.TransactionDate.Location())
			group = &LedgerDayGroup{
				Date:         date,
				DateLabel:    dateKey,
				InTotal:      0,
				OutTotal:     0,
				NetTotal:     0,
				Transactions: []*Transaction{},
			}
			groupsMap[dateKey] = group
			orderedDates = append(orderedDates, dateKey)
		}

		// Add transaction to group
		group.Transactions = append(group.Transactions, tx)

		// Update totals based on transaction type
		switch tx.TransactionType {
		case "IN":
			group.InTotal += tx.Amount
		case "OUT":
			// OUT transactions are already positive amounts in the DB, but we display them as negative
			// For totals, we sum the absolute value
			group.OutTotal += tx.Amount
		}
	}

	// Calculate net totals and build ordered result
	result := make([]*LedgerDayGroup, 0, len(orderedDates))
	for _, dateKey := range orderedDates {
		group := groupsMap[dateKey]
		group.NetTotal = group.InTotal - group.OutTotal
		result = append(result, group)
	}

	return result
}

// GetCancelledLeadsSummary returns financial summary for all cancelled leads
func GetCancelledLeadsSummary() ([]*CancelledLeadSummary, error) {
	query := `
		SELECT 
			l.id,
			l.full_name,
			l.phone,
			l.cancelled_at,
			COALESCE(pt.placement_test_fee_paid, 0) as placement_test_paid,
			COALESCE((SELECT SUM(amount) FROM lead_payments WHERE lead_id = l.id), 0) as course_paid,
			COALESCE((SELECT SUM(amount) FROM transactions WHERE lead_id = l.id AND category = 'refund' AND transaction_type = 'OUT'), 0) as refunded
		FROM leads l
		LEFT JOIN placement_tests pt ON pt.lead_id = l.id
		WHERE l.status = 'cancelled'
		ORDER BY l.cancelled_at DESC NULLS LAST, l.updated_at DESC
	`

	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query cancelled leads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []*CancelledLeadSummary
	for rows.Next() {
		s := &CancelledLeadSummary{}
		var cancelledAt sql.NullTime

		err := rows.Scan(
			&s.LeadID, &s.FullName, &s.Phone, &cancelledAt,
			&s.PlacementTestPaid, &s.CoursePaid, &s.Refunded,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cancelled lead: %w", err)
		}

		s.CancelledAt = cancelledAt
		s.NetMoney = s.CoursePaid - s.Refunded

		summaries = append(summaries, s)
	}

	return summaries, rows.Err()
}

// GetCancelledLeadsTotals returns aggregate totals for all cancelled leads
func GetCancelledLeadsTotals() (totalPlacementTest, totalCoursePaid, totalRefunded, netOutstanding int32, err error) {
	query := `
		SELECT 
			COALESCE(SUM(DISTINCT pt.placement_test_fee_paid), 0) as total_placement_test,
			COALESCE((SELECT SUM(amount) FROM lead_payments WHERE lead_id IN (SELECT id FROM leads WHERE status = 'cancelled')), 0) as total_course_paid,
			COALESCE((SELECT SUM(amount) FROM transactions WHERE lead_id IN (SELECT id FROM leads WHERE status = 'cancelled') AND category = 'refund' AND transaction_type = 'OUT'), 0) as total_refunded
		FROM leads l
		LEFT JOIN placement_tests pt ON pt.lead_id = l.id
		WHERE l.status = 'cancelled'
	`

	err = db.DB.QueryRow(query).Scan(&totalPlacementTest, &totalCoursePaid, &totalRefunded)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to get cancelled leads totals: %w", err)
	}

	netOutstanding = totalCoursePaid - totalRefunded
	return totalPlacementTest, totalCoursePaid, totalRefunded, netOutstanding, nil
}

// ============================================================================
// Milestone 2: Active Classes Repository Functions
// ============================================================================

// CreateClassSessions creates 8 sessions for a class when round starts
// Sessions are scheduled weekly (every 7 days) starting from startDate
func CreateClassSessions(classKey string, startDate time.Time, startTime string) error {
	// Parse start time to calculate end time (default 2 hours duration)
	// Try multiple formats to handle HH:MM and HH:MM:SS
	startTimeParsed, err := time.Parse("15:04", startTime)
	if err != nil {
		startTimeParsed, err = time.Parse("15:04:05", startTime)
		if err != nil {
			return fmt.Errorf("invalid start time format: %w", err)
		}
	}
	endTimeParsed := startTimeParsed.Add(2 * time.Hour)
	endTime := endTimeParsed.Format("15:04")

	now := time.Now()
	for i := 1; i <= 8; i++ {
		sessionDate := startDate.AddDate(0, 0, (i-1)*7) // Weekly sessions
		_, err := db.DB.Exec(`
			INSERT INTO class_sessions (id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time, status, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'scheduled', $6, $6)
			ON CONFLICT (class_key, session_number) DO NOTHING
		`, classKey, i, sessionDate, startTime, endTime, now)
		if err != nil {
			return fmt.Errorf("failed to create session %d: %w", i, err)
		}
	}
	return nil
}

// SetRoundStarted sets round_status='active', round_started_at=NOW(), round_started_by=userID for a class.
func SetRoundStarted(classKey string, startedByUserID uuid.UUID) error {
	_, err := db.DB.Exec(`
		UPDATE class_groups
		SET round_status = 'active', round_started_at = NOW(), round_started_by = $2, updated_at = NOW()
		WHERE class_key = $1
	`, classKey, startedByUserID)
	return err
}

// GetClassSessions returns all sessions for a class, ordered by session_number
func GetClassSessions(classKey string) ([]*ClassSession, error) {
	rows, err := db.DB.Query(`
		SELECT id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time,
		       actual_date, actual_time, actual_end_time, status, completed_at, created_at, updated_at
		FROM class_sessions
		WHERE class_key = $1
		ORDER BY session_number
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query class sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sessions []*ClassSession
	for rows.Next() {
		s := &ClassSession{}
		var scheduledTime, scheduledEndTime, actualTime, actualEndTime sql.NullString
		var actualDate, completedAt sql.NullTime

		err := rows.Scan(
			&s.ID, &s.ClassKey, &s.SessionNumber, &s.ScheduledDate,
			&scheduledTime, &scheduledEndTime, &actualDate, &actualTime, &actualEndTime,
			&s.Status, &completedAt, &s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		s.ScheduledTime = scheduledTime
		s.ScheduledEndTime = scheduledEndTime
		s.ActualDate = actualDate
		s.ActualTime = actualTime
		s.ActualEndTime = actualEndTime
		s.CompletedAt = completedAt

		sessions = append(sessions, s)
	}

	return sessions, rows.Err()
}

// CompleteSession marks a session as completed and sets completed_at timestamp
// If session_number = 1, also increments levels_consumed for all students in the class
func CompleteSession(sessionID uuid.UUID, actualDate time.Time, actualTime string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Get session info
	var classKey string
	var sessionNumber int32
	err = tx.QueryRow(`
		SELECT class_key, session_number FROM class_sessions WHERE id = $1
	`, sessionID).Scan(&classKey, &sessionNumber)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Require attendance for all applicable students before completing the session.
	var missingCount int
	err = tx.QueryRow(`
		SELECT COUNT(*)
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE cg.class_key = $1
		  AND l.status = 'in_classes'
		  AND NOT EXISTS (
			  SELECT 1 FROM attendance a
			  WHERE a.session_id = $2 AND a.lead_id = l.id
		  )
		  AND NOT EXISTS (
			  SELECT 1 FROM late_joiners lj
			  WHERE lj.lead_id = l.id
				AND lj.class_key = cg.class_key
				AND $3 < lj.joined_at_session_number
		  )
	`, classKey, sessionID, sessionNumber).Scan(&missingCount)
	if err != nil {
		return fmt.Errorf("failed to validate attendance completion: %w", err)
	}
	if missingCount > 0 {
		return ErrAttendanceIncomplete
	}

	now := time.Now()
	// Update session status
	_, err = tx.Exec(`
		UPDATE class_sessions
		SET status = 'completed', actual_date = $1, actual_time = $2, completed_at = $3, updated_at = $3
		WHERE id = $4
	`, actualDate, actualTime, now, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	// If session 1, increment levels_consumed for all students in class
	if sessionNumber == 1 {
		_, err = tx.Exec(`
			UPDATE leads
			SET levels_consumed = COALESCE(levels_consumed, 0) + 1, updated_at = $1
			WHERE id IN (
				SELECT s.lead_id
				FROM scheduling s
				INNER JOIN class_groups cg ON (
					cg.level = (SELECT pt.assigned_level FROM placement_tests pt WHERE pt.lead_id = s.lead_id)
					AND cg.class_days = s.class_days
					AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
					AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
				)
				WHERE cg.class_key = $2
			)
		`, now, classKey)
		if err != nil {
			return fmt.Errorf("failed to increment levels_consumed: %w", err)
		}
	}

	return tx.Commit()
}

// CancelAndRescheduleSession cancels a session and reschedules it to a new date/time (same session_number)
func CancelAndRescheduleSession(sessionID uuid.UUID, newDate time.Time, newTime string) error {
	// Parse new time to calculate end time
	startTimeParsed, err := time.Parse("15:04", newTime)
	if err != nil {
		return fmt.Errorf("invalid time format: %w", err)
	}
	endTimeParsed := startTimeParsed.Add(2 * time.Hour)
	endTime := endTimeParsed.Format("15:04")

	now := time.Now()
	_, err = db.DB.Exec(`
		UPDATE class_sessions
		SET scheduled_date = $1, scheduled_time = $2, scheduled_end_time = $3,
		    actual_date = NULL, actual_time = NULL, actual_end_time = NULL, completed_at = NULL,
		    status = 'scheduled', updated_at = $4
		WHERE id = $5
	`, newDate, newTime, endTime, now, sessionID)
	if err != nil {
		return fmt.Errorf("failed to reschedule session: %w", err)
	}
	return nil
}

// ShiftClassRoundStart changes the first session date for a class and moves the full
// 8-session schedule with it. It is only allowed before any session is completed.
func ShiftClassRoundStart(classKey string, newStartDate time.Time, changedByUserID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var classDays, classTime, roundStatus string
	err = tx.QueryRow(`
		SELECT class_days, class_time, COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
	`, classKey).Scan(&classDays, &classTime, &roundStatus)
	if err == sql.ErrNoRows {
		return fmt.Errorf("class not found")
	}
	if err != nil {
		return fmt.Errorf("failed to load class details: %w", err)
	}

	if roundStatus == "closed" {
		return fmt.Errorf("cannot change start date for a closed class")
	}

	allowedWeekdays, ok := allowedRoundStartWeekdays(classDays)
	if !ok {
		return fmt.Errorf("unsupported class_days value %q", classDays)
	}
	if !containsWeekday(allowedWeekdays, newStartDate.Weekday()) {
		return fmt.Errorf("start date must be %s for %s classes", weekdayListLabel(allowedWeekdays), classDays)
	}

	var sessionCount, completedCount int
	err = tx.QueryRow(`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE status = 'completed')
		FROM class_sessions
		WHERE class_key = $1
	`, classKey).Scan(&sessionCount, &completedCount)
	if err != nil {
		return fmt.Errorf("failed to inspect class sessions: %w", err)
	}
	if sessionCount == 0 {
		return fmt.Errorf("cannot change start date before the round is started")
	}
	if completedCount > 0 {
		return fmt.Errorf("cannot change start date after session completion has begun")
	}

	sessionDates, err := buildClassSessionDates(classDays, newStartDate, 8)
	if err != nil {
		return err
	}

	now := time.Now()
	for i := 1; i <= 8; i++ {
		sessionDate := sessionDates[i-1]
		_, err = tx.Exec(`
			UPDATE class_sessions
			SET scheduled_date = $1,
			    updated_at = $2
			WHERE class_key = $3
			  AND session_number = $4
		`, sessionDate, now, classKey, i)
		if err != nil {
			return fmt.Errorf("failed to update session %d: %w", i, err)
		}
	}

	parsedTime, err := parseSessionClock(classTime)
	if err != nil {
		return fmt.Errorf("failed to parse class time %q: %w", classTime, err)
	}
	roundStartedAt := time.Date(
		newStartDate.Year(), newStartDate.Month(), newStartDate.Day(),
		parsedTime.Hour(), parsedTime.Minute(), 0, 0, time.UTC,
	)

	_, err = tx.Exec(`
		UPDATE class_groups
		SET round_started_at = $1,
		    round_started_by = $2,
		    updated_at = $3
		WHERE class_key = $4
	`, roundStartedAt, changedByUserID, now, classKey)
	if err != nil {
		return fmt.Errorf("failed to update class group: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE scheduling s
		SET start_date = $1,
		    start_time = COALESCE(s.start_time, s.class_time),
		    updated_at = $2
		FROM placement_tests pt,
		     class_groups cg
		WHERE pt.lead_id = s.lead_id
		  AND cg.level = pt.assigned_level
		  AND cg.class_days = s.class_days
		  AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
		  AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		  AND cg.class_key = $3
	`, newStartDate, now, classKey)
	if err != nil {
		return fmt.Errorf("failed to update scheduling start dates: %w", err)
	}

	return tx.Commit()
}

func buildClassSessionDates(classDays string, startDate time.Time, count int) ([]time.Time, error) {
	if count <= 0 {
		return nil, nil
	}

	allowedWeekdays, ok := allowedRoundStartWeekdays(classDays)
	if !ok {
		return nil, fmt.Errorf("unsupported class_days value %q", classDays)
	}

	current := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	if !containsWeekday(allowedWeekdays, current.Weekday()) {
		return nil, fmt.Errorf("start date must be %s for %s classes", weekdayListLabel(allowedWeekdays), classDays)
	}

	sessionDates := make([]time.Time, 0, count)
	sessionDates = append(sessionDates, current)
	for len(sessionDates) < count {
		current = nextScheduledClassDate(current, allowedWeekdays)
		sessionDates = append(sessionDates, current)
	}

	return sessionDates, nil
}

func nextScheduledClassDate(from time.Time, allowedWeekdays []time.Weekday) time.Time {
	current := from.AddDate(0, 0, 1)
	for {
		if containsWeekday(allowedWeekdays, current.Weekday()) {
			return time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
		}
		current = current.AddDate(0, 0, 1)
	}
}

func allowedRoundStartWeekdays(classDays string) ([]time.Weekday, bool) {
	switch strings.TrimSpace(classDays) {
	case "Sat/Tues":
		return []time.Weekday{time.Saturday, time.Tuesday}, true
	case "Sun/Wed":
		return []time.Weekday{time.Sunday, time.Wednesday}, true
	case "Mon/Thu":
		return []time.Weekday{time.Monday, time.Thursday}, true
	default:
		return nil, false
	}
}

func containsWeekday(days []time.Weekday, target time.Weekday) bool {
	for _, day := range days {
		if day == target {
			return true
		}
	}
	return false
}

func weekdayListLabel(days []time.Weekday) string {
	labels := make([]string, 0, len(days))
	for _, day := range days {
		labels = append(labels, weekdayLabel(day))
	}
	return strings.Join(labels, " or ")
}

func weekdayLabel(day time.Weekday) string {
	switch day {
	case time.Saturday:
		return "Saturday"
	case time.Sunday:
		return "Sunday"
	case time.Monday:
		return "Monday"
	case time.Tuesday:
		return "Tuesday"
	case time.Wednesday:
		return "Wednesday"
	case time.Thursday:
		return "Thursday"
	case time.Friday:
		return "Friday"
	default:
		return day.String()
	}
}

func parseSessionClock(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("missing time value")
	}
	layouts := []string{"15:04:05", "15:04"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time format: %s", value)
}

// ComputeSessionEndTime builds the scheduled session end datetime from a ClassSession.
func ComputeSessionEndTime(session *ClassSession) (time.Time, error) {
	if session == nil {
		return time.Time{}, fmt.Errorf("session not found")
	}
	loc, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		loc = time.Local
	}
	year, month, day := session.ScheduledDate.Date()
	var baseTime string
	useEndTime := false
	if session.ScheduledEndTime.Valid {
		baseTime = session.ScheduledEndTime.String
		useEndTime = true
	} else if session.ScheduledTime.Valid {
		baseTime = session.ScheduledTime.String
	}
	if baseTime == "" {
		return time.Time{}, fmt.Errorf("session time is missing")
	}
	parsed, err := parseSessionClock(baseTime)
	if err != nil {
		return time.Time{}, err
	}
	end := time.Date(year, month, day, parsed.Hour(), parsed.Minute(), parsed.Second(), 0, loc)
	if !useEndTime {
		end = end.Add(2 * time.Hour)
	}
	return end, nil
}

// MarkAttendance upserts attendance record for a student in a session.
// enforceDeadline blocks updates after 24 hours past the scheduled session end time.
func MarkAttendance(sessionID, leadID uuid.UUID, status string, notes string, markedByUserID uuid.UUID, enforceDeadline bool) error {
	if enforceDeadline {
		session, err := GetSessionByID(sessionID)
		if err != nil {
			return err
		}
		endTime, err := ComputeSessionEndTime(session)
		if err != nil {
			return err
		}
		deadline := endTime.Add(24 * time.Hour)
		if time.Now().In(deadline.Location()).After(deadline) {
			return ErrAttendanceDeadlinePassed
		}
	}

	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO attendance (id, session_id, lead_id, status, notes, marked_by_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $6)
		ON CONFLICT (session_id, lead_id) DO UPDATE SET
			status = EXCLUDED.status,
			notes = EXCLUDED.notes,
			marked_by_user_id = EXCLUDED.marked_by_user_id,
			updated_at = EXCLUDED.updated_at
	`, sessionID, leadID, status, notes, markedByUserID, now)
	return err
}

// GetAttendanceForSession returns all attendance records for a session
func GetAttendanceForSession(sessionID uuid.UUID) ([]*Attendance, error) {
	rows, err := db.DB.Query(`
		SELECT id, session_id, lead_id, status, notes, marked_by_user_id, created_at, updated_at
		FROM attendance
		WHERE session_id = $1
		ORDER BY lead_id
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query attendance: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []*Attendance
	for rows.Next() {
		a := &Attendance{}
		var notes, markedByUserID sql.NullString

		err := rows.Scan(
			&a.ID, &a.SessionID, &a.LeadID, &a.Status,
			&notes, &markedByUserID, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan attendance: %w", err)
		}

		a.Notes = notes
		a.MarkedByUserID = markedByUserID
		records = append(records, a)
	}

	return records, rows.Err()
}

// GetAttendanceByClassKey loads attendance rows grouped by session then lead.
func GetAttendanceByClassKey(classKey string) (map[uuid.UUID]map[uuid.UUID]*Attendance, error) {
	rows, err := db.DB.Query(`
		SELECT a.id, a.session_id, a.lead_id, a.status, a.notes, a.marked_by_user_id, a.created_at, a.updated_at
		FROM attendance a
		INNER JOIN class_sessions cs ON cs.id = a.session_id
		WHERE cs.class_key = $1
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query attendance by class: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[uuid.UUID]map[uuid.UUID]*Attendance)
	for rows.Next() {
		rec := &Attendance{}
		var notes, markedByUserID sql.NullString
		if err := rows.Scan(
			&rec.ID, &rec.SessionID, &rec.LeadID, &rec.Status,
			&notes, &markedByUserID, &rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan attendance by class: %w", err)
		}
		rec.Notes = notes
		rec.MarkedByUserID = markedByUserID
		if _, ok := out[rec.SessionID]; !ok {
			out[rec.SessionID] = make(map[uuid.UUID]*Attendance)
		}
		out[rec.SessionID][rec.LeadID] = rec
	}
	return out, rows.Err()
}

// UpsertSessionPerformance stores task/participation inputs for one student-session pair.
func UpsertSessionPerformance(classSessionID, leadID uuid.UUID, taskCompleted bool, participationScore int) error {
	if participationScore < 1 || participationScore > 5 {
		return fmt.Errorf("participation score must be between 1 and 5")
	}
	_, err := db.DB.Exec(`
		INSERT INTO session_performance (id, class_session_id, lead_id, task_completed, participation_score, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (class_session_id, lead_id) DO UPDATE SET
			task_completed = EXCLUDED.task_completed,
			participation_score = EXCLUDED.participation_score,
			updated_at = CURRENT_TIMESTAMP
	`, classSessionID, leadID, taskCompleted, participationScore)
	if err != nil {
		return fmt.Errorf("failed to upsert session performance: %w", err)
	}
	return nil
}

// GetSessionPerformanceByClassKey loads performance rows grouped by session then lead.
func GetSessionPerformanceByClassKey(classKey string) (map[uuid.UUID]map[uuid.UUID]*SessionPerformance, error) {
	rows, err := db.DB.Query(`
		SELECT sp.id, sp.class_session_id, sp.lead_id, sp.task_completed, sp.participation_score, sp.created_at, sp.updated_at
		FROM session_performance sp
		INNER JOIN class_sessions cs ON cs.id = sp.class_session_id
		WHERE cs.class_key = $1
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query session performance by class: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[uuid.UUID]map[uuid.UUID]*SessionPerformance)
	for rows.Next() {
		rec := &SessionPerformance{}
		if err := rows.Scan(
			&rec.ID, &rec.ClassSessionID, &rec.LeadID,
			&rec.TaskCompleted, &rec.ParticipationScore,
			&rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan session performance by class: %w", err)
		}
		if _, ok := out[rec.ClassSessionID]; !ok {
			out[rec.ClassSessionID] = make(map[uuid.UUID]*SessionPerformance)
		}
		out[rec.ClassSessionID][rec.LeadID] = rec
	}
	return out, rows.Err()
}

func gradeFromScore(total float64) string {
	switch {
	case total >= 85:
		return "A"
	case total >= 70:
		return "B"
	case total >= 50:
		return "C"
	default:
		return "F"
	}
}

func isAbsentStatus(status string) bool {
	s := strings.ToUpper(strings.TrimSpace(status))
	return s == "ABSENT"
}

func isAttendedStatus(status string) bool {
	s := strings.ToUpper(strings.TrimSpace(status))
	return s == "PRESENT" || s == "LATE"
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// GetGradePreviewsByClass computes deterministic grading breakdown for each student in a class.
// Legacy-safe defaults:
// - missing task rows count as not completed for sessions 2..8
// - missing participation score defaults to 3/5 for attended sessions
func GetGradePreviewsByClass(classKey string) (map[uuid.UUID]GradePreview, error) {
	students, err := GetStudentsInClassGroup(classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load class students: %w", err)
	}
	sessions, err := GetClassSessions(classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load class sessions: %w", err)
	}
	attendanceBySession, err := GetAttendanceByClassKey(classKey)
	if err != nil {
		return nil, err
	}
	perfBySession, err := GetSessionPerformanceByClassKey(classKey)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]GradePreview, len(students))
	for _, student := range students {
		if student == nil {
			continue
		}
		var absences int
		var completedTasks int
		var attendedSessions int
		var participationTotal float64
		usedLegacyTaskFallback := false

		for _, session := range sessions {
			if session == nil {
				continue
			}
			var attendance *Attendance
			if byLead, ok := attendanceBySession[session.ID]; ok {
				attendance = byLead[student.LeadID]
			}
			status := ""
			if attendance != nil {
				status = attendance.Status
			}
			if isAbsentStatus(status) {
				absences++
			}

			var perf *SessionPerformance
			if byLead, ok := perfBySession[session.ID]; ok {
				perf = byLead[student.LeadID]
			}

			if session.SessionNumber >= 2 && session.SessionNumber <= 8 {
				taskCompleted := false
				if perf != nil {
					taskCompleted = perf.TaskCompleted
				} else {
					usedLegacyTaskFallback = true
				}
				if taskCompleted {
					completedTasks++
				}
			}

			if isAttendedStatus(status) {
				score := float64(3)
				if perf != nil && perf.ParticipationScore >= 1 && perf.ParticipationScore <= 5 {
					score = float64(perf.ParticipationScore)
				}
				participationTotal += score
				attendedSessions++
			}
		}

		attendanceScore := 0.0
		if absences > 2 {
			attendanceScore = 0
		} else {
			attendanceScore = (float64(attendedSessions) / 8.0) * 50.0
		}

		taskScore := float64(0)
		if completedTasks > 1 {
			taskScore = (float64(completedTasks) / 7.0) * 40.0
		}

		avgParticipation := float64(3)
		if attendedSessions > 0 {
			avgParticipation = participationTotal / float64(attendedSessions)
		}
		participationScore := (avgParticipation / 5.0) * 10.0

		total := attendanceScore + taskScore + participationScore
		out[student.LeadID] = GradePreview{
			LeadID:                 student.LeadID,
			Absences:               absences,
			CompletedTasks:         completedTasks,
			AttendedSessions:       attendedSessions,
			AverageParticipation:   round2(avgParticipation),
			AttendanceScore:        round2(attendanceScore),
			TaskScore:              round2(taskScore),
			ParticipationScore:     round2(participationScore),
			TotalScore:             round2(total),
			CalculatedGrade:        gradeFromScore(total),
			UsedLegacyTaskFallback: usedLegacyTaskFallback,
		}
	}

	return out, nil
}

// EnterGrade inserts or updates a grade for a student at session 8
func EnterGrade(leadID uuid.UUID, classKey string, grade string, notes string, createdByUserID uuid.UUID) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO grades (id, lead_id, class_key, session_number, grade, notes, created_by_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, 8, $3, $4, $5, $6, $6)
		ON CONFLICT (lead_id, class_key, session_number) DO UPDATE SET
			grade = EXCLUDED.grade,
			notes = EXCLUDED.notes,
			updated_at = EXCLUDED.updated_at
	`, leadID, classKey, grade, notes, createdByUserID, now)
	return err
}

// GetGrade returns the grade for a student in a class (session 8)
func GetGrade(leadID uuid.UUID, classKey string) (*Grade, error) {
	g := &Grade{}
	var notes, createdByUserID sql.NullString

	err := db.DB.QueryRow(`
		SELECT id, lead_id, class_key, session_number, grade, notes, created_by_user_id, created_at, updated_at
		FROM grades
		WHERE lead_id = $1 AND class_key = $2 AND session_number = 8
	`, leadID, classKey).Scan(
		&g.ID, &g.LeadID, &g.ClassKey, &g.SessionNumber,
		&g.Grade, &notes, &createdByUserID, &g.CreatedAt, &g.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get grade: %w", err)
	}

	g.Notes = notes
	g.CreatedByUserID = createdByUserID
	return g, nil
}

// AddStudentNote adds a note for a student
func AddStudentNote(leadID uuid.UUID, classKey string, sessionNumber sql.NullInt32, noteText string, isPrivate bool, createdByUserID uuid.UUID) error {
	now := time.Now()
	var classKeyNull sql.NullString
	if classKey != "" {
		classKeyNull = sql.NullString{String: classKey, Valid: true}
	}

	var noteID uuid.UUID
	err := db.DB.QueryRow(`
		INSERT INTO student_notes (id, lead_id, class_key, session_number, note_text, is_private, created_by_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id
	`, leadID, classKeyNull, sessionNumber, noteText, isPrivate, createdByUserID, now).Scan(&noteID)
	if err != nil {
		return fmt.Errorf("database insert failed: %w", err)
	}

	// Verify the note was actually inserted by querying it back
	var verifyID uuid.UUID
	var verifyText string
	verifyErr := db.DB.QueryRow(`
		SELECT id, note_text FROM student_notes WHERE id = $1
	`, noteID).Scan(&verifyID, &verifyText)
	if verifyErr != nil {
		return fmt.Errorf("note inserted but verification query failed: %w (inserted id: %s)", verifyErr, noteID)
	}
	if verifyText != noteText {
		return fmt.Errorf("note text mismatch: inserted %q but verified %q", noteText, verifyText)
	}

	return nil
}

// GetStudentNotes returns all notes for a student, ordered by created_at DESC (newest first)
// Includes creator email via LEFT JOIN with users table
// Notes are NOT filtered by sessions/round - they return regardless of session count
func GetStudentNotes(leadID uuid.UUID) ([]*StudentNote, error) {
	rows, err := db.DB.Query(`
		SELECT sn.id, sn.lead_id, sn.class_key, sn.session_number, sn.note_text, sn.is_private,
		       sn.created_by_user_id, u.email as created_by_email, sn.created_at, sn.updated_at
		FROM student_notes sn
		LEFT JOIN users u ON u.id = sn.created_by_user_id
		WHERE sn.lead_id::uuid = $1
		ORDER BY sn.created_at DESC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query student notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notes []*StudentNote
	for rows.Next() {
		n := &StudentNote{}
		var classKey sql.NullString
		var sessionNumberInt sql.NullInt32
		var createdByUserID sql.NullString
		var createdByEmail sql.NullString

		err := rows.Scan(
			&n.ID, &n.LeadID, &classKey, &sessionNumberInt,
			&n.NoteText, &n.IsPrivate, &createdByUserID, &createdByEmail, &n.CreatedAt, &n.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}

		n.ClassKey = classKey
		n.SessionNumber = sessionNumberInt
		n.CreatedByUserID = createdByUserID
		n.CreatedByEmail = createdByEmail
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return notes, nil
}

// GetStudentNoteByID returns a single note by ID (to check creator)
func GetStudentNoteByID(noteID uuid.UUID) (*StudentNote, error) {
	n := &StudentNote{}
	var classKey sql.NullString
	var sessionNumber sql.NullInt32
	var createdByUserID sql.NullString
	var createdByEmail sql.NullString

	err := db.DB.QueryRow(`
		SELECT sn.id, sn.lead_id, sn.class_key, sn.session_number, sn.note_text,
		       sn.created_by_user_id, u.email as created_by_email, sn.created_at, sn.updated_at
		FROM student_notes sn
		LEFT JOIN users u ON u.id = sn.created_by_user_id
		WHERE sn.id = $1
	`, noteID).Scan(
		&n.ID, &n.LeadID, &classKey, &sessionNumber, &n.NoteText,
		&createdByUserID, &createdByEmail, &n.CreatedAt, &n.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get student note: %w", err)
	}

	if classKey.Valid {
		n.ClassKey = sql.NullString{String: classKey.String, Valid: true}
	}
	if sessionNumber.Valid {
		n.SessionNumber = sql.NullInt32{Int32: sessionNumber.Int32, Valid: true}
	}
	n.CreatedByUserID = createdByUserID
	n.CreatedByEmail = createdByEmail
	return n, nil
}

// DeleteStudentNote deletes a note by ID
func DeleteStudentNote(noteID uuid.UUID) error {
	result, err := db.DB.Exec(`
		DELETE FROM student_notes WHERE id = $1
	`, noteID)
	if err != nil {
		return fmt.Errorf("failed to delete student note: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("note not found")
	}
	return nil
}

// GetRefundableAmount calculates refundable amount based on session completion markers (not wall-clock time)
// Rules:
// - If session 2 has completed_at IS NOT NULL: refundable = 0
// - If session 1 has completed_at IS NOT NULL AND session 2 has completed_at IS NULL: refundable = 50% of course paid
// - Otherwise: refundable = 100% of course paid
func GetRefundableAmount(leadID uuid.UUID) (int32, error) {
	totalCoursePaid, err := GetTotalCoursePaid(leadID)
	if err != nil {
		return 0, fmt.Errorf("failed to get total course paid: %w", err)
	}

	// Get student's class_key
	var classKey sql.NullString
	err = db.DB.QueryRow(`
		SELECT cg.class_key
		FROM scheduling s
		INNER JOIN placement_tests pt ON pt.lead_id = s.lead_id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE s.lead_id = $1
		LIMIT 1
	`, leadID).Scan(&classKey)
	if err == sql.ErrNoRows || !classKey.Valid {
		// No active class, full refund available
		return totalCoursePaid, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get class key: %w", err)
	}

	// Check session completion markers
	var session1Completed, session2Completed bool
	err = db.DB.QueryRow(`
		SELECT 
			EXISTS(SELECT 1 FROM class_sessions WHERE class_key = $1 AND session_number = 1 AND completed_at IS NOT NULL),
			EXISTS(SELECT 1 FROM class_sessions WHERE class_key = $1 AND session_number = 2 AND completed_at IS NOT NULL)
	`, classKey.String).Scan(&session1Completed, &session2Completed)
	if err != nil {
		return 0, fmt.Errorf("failed to check session completion: %w", err)
	}

	if session2Completed {
		return 0, nil // No refund after session 2 completed
	}
	if session1Completed {
		return totalCoursePaid / 2, nil // 50% refund after session 1 completed
	}

	return totalCoursePaid, nil // Full refund before session 1 completed
}

// AssignMentorToClass assigns a mentor (user with role='mentor') to a class
func AssignMentorToClass(classKey string, mentorUserID uuid.UUID, createdByUserID uuid.UUID) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO mentor_assignments (id, mentor_user_id, class_key, assigned_at, created_by_user_id)
		VALUES (gen_random_uuid(), $1, $2, $3, $4)
		ON CONFLICT (class_key) DO UPDATE SET
			mentor_user_id = EXCLUDED.mentor_user_id,
			assigned_at = EXCLUDED.assigned_at,
			created_by_user_id = EXCLUDED.created_by_user_id
	`, mentorUserID, classKey, now, createdByUserID)
	return err
}

// CheckMentorDoubleBookByDaysTime returns true if mentor is already assigned to another class
// (different class_key) with the same class_days and class_time. Also returns days and time for error message.
func CheckMentorDoubleBookByDaysTime(mentorUserID uuid.UUID, excludeClassKey, classDays, classTime string) (hasConflict bool, days, timeStr string, err error) {
	var conflictDays, conflictTime string
	rowErr := db.DB.QueryRow(`
		SELECT cg.class_days, cg.class_time
		FROM mentor_assignments ma
		INNER JOIN class_groups cg ON cg.class_key = ma.class_key
		WHERE ma.mentor_user_id = $1
		  AND ma.class_key != $2
		  AND cg.class_days = $3
		  AND cg.class_time = $4
		LIMIT 1
	`, mentorUserID, excludeClassKey, classDays, classTime).Scan(&conflictDays, &conflictTime)
	if rowErr == sql.ErrNoRows {
		return false, "", "", nil
	}
	if rowErr != nil {
		return false, "", "", fmt.Errorf("failed to check double-book: %w", rowErr)
	}
	return true, conflictDays, conflictTime, nil
}

// UnassignMentorFromClass removes the mentor assignment for a class. Caller must ensure no sessions exist.
func UnassignMentorFromClass(classKey string) error {
	res, err := db.DB.Exec(`DELETE FROM mentor_assignments WHERE class_key = $1`, classKey)
	if err != nil {
		return fmt.Errorf("failed to unassign mentor: %w", err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return nil // No assignment existed; idempotent
	}
	return nil
}

// CheckMentorScheduleConflict checks if assigning a mentor to a class would create overlapping sessions
func CheckMentorScheduleConflict(mentorUserID uuid.UUID, date time.Time, startTime, endTime string) (bool, error) {
	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM class_sessions cs
		INNER JOIN mentor_assignments ma ON cs.class_key = ma.class_key
		WHERE ma.mentor_user_id = $1
		AND cs.scheduled_date = $2
		AND cs.status != 'cancelled'
		AND (
			(cs.scheduled_time <= $3 AND cs.scheduled_end_time > $3) OR
			(cs.scheduled_time < $4 AND cs.scheduled_end_time >= $4) OR
			(cs.scheduled_time >= $3 AND cs.scheduled_end_time <= $4)
		)
	`, mentorUserID, date, startTime, endTime).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check conflict: %w", err)
	}
	return count > 0, nil
}

// GetMentorAssignment returns the mentor assignment for a class
func GetMentorAssignment(classKey string) (*MentorAssignment, error) {
	ma := &MentorAssignment{}
	var createdByUserID sql.NullString

	err := db.DB.QueryRow(`
		SELECT id, mentor_user_id, class_key, assigned_at, created_by_user_id
		FROM mentor_assignments
		WHERE class_key = $1
	`, classKey).Scan(
		&ma.ID, &ma.MentorUserID, &ma.ClassKey, &ma.AssignedAt, &createdByUserID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get mentor assignment: %w", err)
	}

	ma.CreatedByUserID = createdByUserID
	return ma, nil
}

// GetMentorClasses returns all classes assigned to a mentor
func GetMentorClasses(mentorUserID uuid.UUID) ([]*ClassGroupWorkflow, error) {
	rows, err := db.DB.Query(`
		SELECT cg.class_key, cg.level, cg.class_days, cg.class_time, cg.class_number,
		       cg.sent_to_mentor, cg.sent_at, cg.returned_at, cg.updated_at
		FROM class_groups cg
		INNER JOIN mentor_assignments ma ON cg.class_key = ma.class_key
		WHERE ma.mentor_user_id = $1 AND cg.sent_to_mentor = true AND COALESCE(cg.round_status, '') != 'closed'
		ORDER BY cg.level, cg.class_days, cg.class_time
	`, mentorUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor classes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var classes []*ClassGroupWorkflow
	for rows.Next() {
		c := &ClassGroupWorkflow{}
		var sentAt, returnedAt sql.NullTime

		err := rows.Scan(
			&c.ClassKey, &c.Level, &c.ClassDays, &c.ClassTime, &c.ClassNumber,
			&c.SentToMentor, &sentAt, &returnedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class: %w", err)
		}

		c.SentAt = sentAt
		c.ReturnedAt = returnedAt
		classes = append(classes, c)
	}

	return classes, rows.Err()
}

// GetMentorReminders returns active reminders for a mentor
func GetMentorReminders(mentorUserID uuid.UUID) ([]*MentorReminder, error) {
	var reminders []*MentorReminder

	// 1. Check for completed sessions with incomplete attendance
	attendanceRows, err := db.DB.Query(`
		SELECT DISTINCT
			cg.class_key,
			cg.level,
			cg.class_days,
			cg.class_time,
			cg.class_number,
			cs.session_number
		FROM class_groups cg
		INNER JOIN mentor_assignments ma ON cg.class_key = ma.class_key
		INNER JOIN class_sessions cs ON cs.class_key = cg.class_key
		WHERE ma.mentor_user_id = $1
		  AND cg.sent_to_mentor = true
		  AND COALESCE(cg.round_status, '') = 'active'
		  AND cs.status = 'completed'
		  AND cs.completed_at IS NOT NULL
		  AND EXISTS (
			  -- Check if there are students without attendance for this session
			  SELECT 1
			  FROM leads l
			  INNER JOIN scheduling s ON s.lead_id = l.id
			  INNER JOIN placement_tests pt ON pt.lead_id = l.id
			  WHERE l.status = 'in_classes'
				AND pt.assigned_level = cg.level
				AND s.class_days = cg.class_days
				AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
				AND COALESCE(s.class_group_index, 1) = COALESCE(cg.class_number, 1)
				AND NOT EXISTS (
					SELECT 1 FROM attendance a
					WHERE a.session_id = cs.id AND a.lead_id = l.id
				)
				-- Exclude late joiners for sessions before they joined
				AND NOT EXISTS (
					SELECT 1 FROM late_joiners lj
					WHERE lj.lead_id = l.id
					  AND lj.class_key = cg.class_key
					  AND cs.session_number < lj.joined_at_session_number
				)
		  )
		ORDER BY cs.session_number DESC
		LIMIT 5
	`, mentorUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to query attendance reminders: %w", err)
	}
	defer func() {
		_ = attendanceRows.Close()
	}()

	for attendanceRows.Next() {
		var classKey, classDays, classTime string
		var level, classNumber, sessionNumber int32

		err := attendanceRows.Scan(&classKey, &level, &classDays, &classTime, &classNumber, &sessionNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan attendance reminder: %w", err)
		}

		reminders = append(reminders, &MentorReminder{
			Type:          "attendance",
			ClassKey:      classKey,
			Level:         level,
			ClassDays:     classDays,
			ClassTime:     classTime,
			ClassNumber:   classNumber,
			SessionNumber: sessionNumber,
			Message:       fmt.Sprintf("Session %d attendance is incomplete", sessionNumber),
		})
	}

	// 2. Check for classes with session 8 completed but missing grades
	gradingRows, err := db.DB.Query(`
		SELECT
			cg.class_key,
			cg.level,
			cg.class_days,
			cg.class_time,
			cg.class_number
		FROM class_groups cg
		INNER JOIN mentor_assignments ma ON cg.class_key = ma.class_key
		WHERE ma.mentor_user_id = $1
		  AND cg.sent_to_mentor = true
		  AND COALESCE(cg.round_status, '') != 'closed'
		  AND EXISTS (
			  -- Session 8 is completed
			  SELECT 1
			  FROM class_sessions cs
			  WHERE cs.class_key = cg.class_key
				AND cs.session_number = 8
				AND cs.status = 'completed'
		  )
		  AND EXISTS (
			  -- Check if there are students in this class missing grades
			  SELECT 1
			  FROM leads l
			  INNER JOIN scheduling s ON s.lead_id = l.id
			  INNER JOIN placement_tests pt ON pt.lead_id = l.id
			  WHERE l.status = 'in_classes'
				AND pt.assigned_level = cg.level
				AND s.class_days = cg.class_days
				AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
				AND COALESCE(s.class_group_index, 1) = COALESCE(cg.class_number, 1)
				AND NOT EXISTS (
					SELECT 1 FROM grades g
					WHERE g.lead_id = l.id
					  AND g.class_key = cg.class_key
					  AND g.session_number = 8
				)
		  )
		ORDER BY cg.level, cg.class_days, cg.class_time
		LIMIT 5
	`, mentorUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to query grading reminders: %w", err)
	}
	defer func() {
		_ = gradingRows.Close()
	}()

	for gradingRows.Next() {
		var classKey, classDays, classTime string
		var level, classNumber int32

		err := gradingRows.Scan(&classKey, &level, &classDays, &classTime, &classNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan grading reminder: %w", err)
		}

		reminders = append(reminders, &MentorReminder{
			Type:          "grading",
			ClassKey:      classKey,
			Level:         level,
			ClassDays:     classDays,
			ClassTime:     classTime,
			ClassNumber:   classNumber,
			SessionNumber: 8,
			Message:       "All sessions complete - final grading required",
		})
	}

	return reminders, nil
}

// PromoteStudent handles the promotion logic for a single student when a round ends.
// This includes: snapshotting history, checking credits, promoting level, and detaching schedule.
func PromoteStudent(tx *sql.Tx, leadID uuid.UUID, classKey string, now time.Time) error {
	// 1. Get current class details (level, days, time, mentor name)
	var level int32
	var classDays, classTime string
	var mentorName sql.NullString

	err := tx.QueryRow(`
		SELECT cg.level, cg.class_days, cg.class_time, u.email
		FROM class_groups cg
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id::uuid
		WHERE cg.class_key = $1
	`, classKey).Scan(&level, &classDays, &classTime, &mentorName)
	if err != nil {
		return fmt.Errorf("failed to get class details: %w", err)
	}

	// 2. Get current assigned level from placement_tests
	var currentLevel sql.NullInt32
	err = tx.QueryRow(`
		SELECT assigned_level FROM placement_tests WHERE lead_id = $1
	`, leadID).Scan(&currentLevel)
	if err != nil {
		return fmt.Errorf("failed to get assigned level: %w", err)
	}

	// 3. Get grade from grades table (session 8)
	var finalGrade sql.NullString
	err = tx.QueryRow(`
		SELECT grade FROM grades WHERE lead_id = $1 AND class_key = $2 AND session_number = 8
	`, leadID, classKey).Scan(&finalGrade)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get grade: %w", err)
	}

	// 4. Count absences to determine outcome
	var absences int
	err = tx.QueryRow(`
		SELECT COUNT(*)
		FROM attendance a
		INNER JOIN class_sessions cs ON a.session_id = cs.id
		WHERE a.lead_id = $1 AND cs.class_key = $2 AND a.status IN ('ABSENT', 'LATE')
	`, leadID, classKey).Scan(&absences)
	if err != nil {
		return fmt.Errorf("failed to count absences: %w", err)
	}

	// Determine outcome: repeat if absences > 2 OR grade = 'F'
	shouldRepeat := absences > 2 || (finalGrade.Valid && finalGrade.String == "F")
	outcome := "promoted"
	if shouldRepeat {
		outcome = "repeated"
	}

	// 5. Snapshot to class_enrollments
	_, err = tx.Exec(`
		INSERT INTO class_enrollments (
			lead_id, class_key, level, class_days, class_time, mentor_name,
			final_grade, outcome, enrolled_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (lead_id, class_key) DO UPDATE SET
			final_grade = EXCLUDED.final_grade,
			outcome = EXCLUDED.outcome,
			completed_at = EXCLUDED.completed_at
	`, leadID, classKey, level, classDays, classTime, mentorName, finalGrade, outcome, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert class enrollment: %w", err)
	}

	// 6. Credit check and status update (compute remaining credits from purchased - consumed)
	var purchased, consumed sql.NullInt32
	err = tx.QueryRow(`
		SELECT COALESCE(levels_purchased_total, 0), COALESCE(levels_consumed, 0)
		FROM leads WHERE id = $1
	`, leadID).Scan(&purchased, &consumed)
	if err != nil {
		return fmt.Errorf("failed to get credits: %w", err)
	}

	creditsRemaining := int32(0)
	if purchased.Valid {
		creditsRemaining = purchased.Int32
	}
	if consumed.Valid {
		creditsRemaining -= consumed.Int32
	}
	if creditsRemaining < 0 {
		creditsRemaining = 0
	}

	newCredits := creditsRemaining
	if outcome == "promoted" && newCredits > 0 {
		newCredits -= 1
	}

	// Status is based on credits BEFORE the promotion deduction.
	// If the student had any credits when they finished, they should wait for a round.
	newStatus := "renewal_pending"
	if creditsRemaining > 0 {
		newStatus = "waiting_for_round"
	}

	highPriorityFollowUp := newStatus == "renewal_pending"

	// 7. Set returning flag and remaining credits snapshot
	_, err = tx.Exec(`
		UPDATE leads 
		SET remaining_credits = $1,
		    status = $2,
		    is_returning = true,
		    high_priority_follow_up = $5,
		    high_priority = false,
		    high_priority_reason = '',
		    updated_at = $3
		WHERE id = $4
	`, newCredits, newStatus, now, leadID, highPriorityFollowUp)
	if err != nil {
		return fmt.Errorf("failed to update lead status: %w", err)
	}

	// 7b. Clear current offer/payment snapshots for returning cycle.
	// History remains in lead_payments/transactions.
	_, err = tx.Exec(`DELETE FROM payments WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to clear payment snapshot: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM offers WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to clear offer snapshot: %w", err)
	}

	// 8. Promote level (only if outcome = 'promoted')
	if outcome == "promoted" && currentLevel.Valid {
		nextLevel := currentLevel.Int32 + 1
		_, err = tx.Exec(`
			UPDATE placement_tests SET assigned_level = $1, updated_at = $2 WHERE lead_id = $3
		`, nextLevel, now, leadID)
		if err != nil {
			return fmt.Errorf("failed to promote level: %w", err)
		}
	}

	// 9. Detach schedule (CRITICAL: prevents ghost-joining)
	// We keep class_days and class_time as preferences for the next round,
	// but clear class_group_index to remove them from any specific class.
	_, err = tx.Exec(`
		UPDATE scheduling 
		SET class_group_index = NULL, updated_at = $1
		WHERE lead_id = $2
	`, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to clear scheduling: %w", err)
	}

	// Set sent_to_classes = false
	_, err = tx.Exec(`
		UPDATE leads SET sent_to_classes = false, updated_at = $1 WHERE id = $2
	`, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to update sent_to_classes: %w", err)
	}

	return nil
}

// CloseRound computes outcomes for all students in a class and sets high_priority_follow_up flag.

// Returns to Operations by setting sent_to_mentor = false and round_status = 'closed'.
func CloseRound(classKey string, closedByUserID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	now := time.Now()

	// Get current mentor assigned to the class
	var mentorUserID sql.NullString
	err = tx.QueryRow(`SELECT mentor_user_id FROM mentor_assignments WHERE class_key = $1`, classKey).Scan(&mentorUserID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get current mentor assignment: %w", err)
	}

	// Get all active class students in the class.
	// IMPORTANT: Keep this roster rule aligned with GetStudentsInClassGroup(),
	// otherwise close-round validation can include non-class students.
	rows, err := tx.Query(`
		SELECT s.lead_id
		FROM scheduling s
		INNER JOIN leads l ON l.id = s.lead_id
		INNER JOIN placement_tests pt ON pt.lead_id = s.lead_id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE cg.class_key = $1
		  AND l.status = 'in_classes'
	`, classKey)
	if err != nil {
		return fmt.Errorf("failed to query students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var leadIDs []uuid.UUID
	for rows.Next() {
		var leadID uuid.UUID
		if err := rows.Scan(&leadID); err != nil {
			return fmt.Errorf("failed to scan lead ID: %w", err)
		}
		leadIDs = append(leadIDs, leadID)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close student rows: %w", err)
	}

	// VALIDATION: Ensure all students have grades before closing the round
	var studentsWithoutGrades int
	err = tx.QueryRow(`
		SELECT COUNT(*)
		FROM unnest($1::uuid[]) AS students(lead_id)
		WHERE NOT EXISTS (
			SELECT 1 FROM grades g
			WHERE g.lead_id = students.lead_id
			  AND g.class_key = $2
			  AND g.session_number = 8
		)
	`, pq.Array(leadIDs), classKey).Scan(&studentsWithoutGrades)
	if err != nil {
		return fmt.Errorf("failed to validate grades: %w", err)
	}
	if studentsWithoutGrades > 0 {
		return fmt.Errorf("cannot close round: %d student(s) are missing final grades", studentsWithoutGrades)
	}

	// For each student, promote them to the next level
	for _, leadID := range leadIDs {
		if err := PromoteStudent(tx, leadID, classKey, now); err != nil {
			return fmt.Errorf("failed to promote student %s: %w", leadID, err)
		}
	}

	// Return class to Operations and mark round closed
	// Prepare closed_mentor_user_id as proper nullable value
	var closedMentorID interface{}
	if mentorUserID.Valid {
		closedMentorID = mentorUserID.String
	} else {
		closedMentorID = nil
	}

	_, err = tx.Exec(`
		UPDATE class_groups
		SET sent_to_mentor = false, returned_at = $1, updated_at = $1,
		    round_status = 'closed', round_closed_at = $1, round_closed_by = $3,
		    closed_mentor_user_id = $4,
		    hidden_in_ops = true, hidden_at = $1, hidden_by = $3
		WHERE class_key = $2
	`, now, classKey, closedByUserID, closedMentorID)
	if err != nil {
		return fmt.Errorf("failed to return class: %w", err)
	}

	// Clean up mentor assignment
	_, err = tx.Exec(`DELETE FROM mentor_assignments WHERE class_key = $1`, classKey)
	if err != nil {
		return fmt.Errorf("failed to delete mentor assignment: %w", err)
	}

	return tx.Commit()
}

// ReopenClosedRound reopens a closed class if fewer than 8 sessions are completed.
// Reopen will set sent_to_mentor = true and round_status = 'active'.
func ReopenClosedRound(classKey string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now()

	var roundStatus string
	err = tx.QueryRow(`SELECT COALESCE(round_status, '') FROM class_groups WHERE class_key = $1`, classKey).Scan(&roundStatus)
	if err != nil {
		return fmt.Errorf("failed to load class: %w", err)
	}
	if roundStatus != "closed" {
		return fmt.Errorf("class is not closed")
	}

	var completedCount int32
	err = tx.QueryRow(`
		SELECT COALESCE(COUNT(*), 0)
		FROM class_sessions
		WHERE class_key = $1 AND status = 'completed'
	`, classKey).Scan(&completedCount)
	if err != nil {
		return fmt.Errorf("failed to count completed sessions: %w", err)
	}
	if completedCount >= 8 {
		return fmt.Errorf("class already completed")
	}

	_, err = tx.Exec(`
		UPDATE class_groups
		SET sent_to_mentor = true,
		    returned_at = NULL,
		    updated_at = $1,
		    round_status = 'active',
		    round_closed_at = NULL,
		    round_closed_by = NULL,
		    closed_mentor_user_id = NULL
		WHERE class_key = $2
	`, now, classKey)
	if err != nil {
		return fmt.Errorf("failed to reopen class: %w", err)
	}

	return tx.Commit()
}

// GetArchivedClassGroups returns all class groups where round_status = 'closed'.
// sort: 'oldest' (ASC) or 'newest' (DESC) by round_closed_at.
// from/to filter by round_closed_at date (inclusive) when provided.
func GetArchivedClassGroups(sort string, fromDate, toDate *time.Time) ([]*ClassGroupWorkflow, error) {
	order := "ASC"
	if sort == "newest" {
		order = "DESC"
	}

	query := `
		SELECT class_key, level, class_days, class_time, class_number,
		       sent_to_mentor, sent_at, returned_at, updated_at,
		       round_status, round_started_at, round_started_by,
		       round_closed_at, round_closed_by, closed_mentor_user_id,
		       COALESCE((SELECT COUNT(*) FROM class_sessions WHERE class_key = class_groups.class_key AND status = 'completed'), 0) AS completed_sessions
		FROM class_groups
		WHERE round_status = 'closed'
	`
	args := []interface{}{}
	argIndex := 1

	if fromDate != nil {
		query += fmt.Sprintf(" AND round_closed_at::date >= $%d", argIndex)
		args = append(args, fromDate.Format("2006-01-02"))
		argIndex++
	}
	if toDate != nil {
		query += fmt.Sprintf(" AND round_closed_at::date <= $%d", argIndex)
		args = append(args, toDate.Format("2006-01-02"))
	}

	query += fmt.Sprintf(" ORDER BY round_closed_at %s", order)

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query archived class groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []*ClassGroupWorkflow
	for rows.Next() {
		g := &ClassGroupWorkflow{}
		var sentAt, returnedAt, startedAt, closedAt sql.NullTime
		var startedBy, closedBy, closedMentorID sql.NullString

		err := rows.Scan(
			&g.ClassKey, &g.Level, &g.ClassDays, &g.ClassTime, &g.ClassNumber,
			&g.SentToMentor, &sentAt, &returnedAt, &g.UpdatedAt,
			&g.RoundStatus, &startedAt, &startedBy,
			&closedAt, &closedBy, &closedMentorID, &g.CompletedSessions,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan archived class group: %w", err)
		}

		g.SentAt = sentAt
		g.ReturnedAt = returnedAt
		g.RoundStartedAt = startedAt
		g.RoundStartedBy = startedBy
		g.RoundClosedAt = closedAt
		g.RoundClosedBy = closedBy
		g.ClosedMentorUserID = closedMentorID
		groups = append(groups, g)
	}

	return groups, rows.Err()
}

// CountClassEnrollments returns the historical roster size for a class.
func CountClassEnrollments(classKey string) (int, error) {
	var count int
	err := db.DB.QueryRow(`SELECT COUNT(*) FROM class_enrollments WHERE class_key = $1`, classKey).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// SubmitFeedback submits student success feedback for a student at session 4 or 8
func SubmitFeedback(leadID uuid.UUID, classKey string, sessionNumber int32, feedbackText string, followUpRequired bool, createdByUserID uuid.UUID) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO student_success_feedback (id, lead_id, class_key, session_number, feedback_text, follow_up_required, created_by_user_id, created_at, updated_at, status)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $7, 'received')
		ON CONFLICT (lead_id, class_key, session_number) DO UPDATE SET
			feedback_text = EXCLUDED.feedback_text,
			follow_up_required = EXCLUDED.follow_up_required,
			status = 'received',
			updated_at = EXCLUDED.updated_at
	`, leadID, classKey, sessionNumber, feedbackText, followUpRequired, createdByUserID, now)
	return err
}

// GetClassFeedbackRecords returns all feedback records for a given class.
func GetClassFeedbackRecords(classKey string) ([]*StudentSuccessFeedback, error) {
	rows, err := db.DB.Query(`
		SELECT id, lead_id, class_key, session_number, feedback_text, follow_up_required, COALESCE(status, 'sent'), created_by_user_id, created_at, updated_at
		FROM student_success_feedback
		WHERE class_key = $1
		ORDER BY session_number, created_at DESC
	`, classKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []*StudentSuccessFeedback
	for rows.Next() {
		f := &StudentSuccessFeedback{}
		if err := rows.Scan(&f.ID, &f.LeadID, &f.ClassKey, &f.SessionNumber, &f.FeedbackText, &f.FollowUpRequired, &f.Status, &f.CreatedByUserID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, f)
	}
	return results, rows.Err()
}

// CreateFeedbackCollectedUpload stores a feedback file uploaded by Student Success or Mentor Head.
func CreateFeedbackCollectedUpload(leadID uuid.UUID, classKey string, sessionNumber *int32, fileName, fileURL, mimeType string, sizeBytes int64, note string, uploadedBy uuid.UUID) (*FeedbackCollectedUpload, error) {
	var sn sql.NullInt32
	if sessionNumber != nil {
		sn = sql.NullInt32{Int32: *sessionNumber, Valid: true}
	}
	var mt sql.NullString
	if mimeType != "" {
		mt = sql.NullString{String: mimeType, Valid: true}
	}
	var sz sql.NullInt32
	if sizeBytes > 0 {
		sz = sql.NullInt32{Int32: int32(sizeBytes), Valid: true}
	}
	var noteVal sql.NullString
	if note != "" {
		noteVal = sql.NullString{String: note, Valid: true}
	}

	var out FeedbackCollectedUpload
	err := db.DB.QueryRow(`
		INSERT INTO feedback_collected_uploads (
			lead_id, class_key, session_number, file_name, file_url, mime_type, size_bytes, note, uploaded_by_user_id, uploaded_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
		RETURNING id, lead_id, class_key, session_number, file_name, file_url, mime_type, size_bytes, note, uploaded_by_user_id, uploaded_at
	`, leadID, classKey, sn, fileName, fileURL, mt, sz, noteVal, uploadedBy).Scan(
		&out.ID, &out.LeadID, &out.ClassKey, &out.SessionNumber, &out.FileName, &out.FileURL,
		&out.MimeType, &out.SizeBytes, &out.Note, &out.UploadedByUser, &out.UploadedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert feedback upload: %w", err)
	}
	return &out, nil
}

// GetFeedbackCollectedUploadsByClass returns feedback uploads for a class, newest first.
func GetFeedbackCollectedUploadsByClass(classKey string) ([]*FeedbackCollectedUpload, error) {
	rows, err := db.DB.Query(`
		SELECT f.id, f.lead_id, f.class_key, f.session_number, f.file_name, f.file_url,
		       f.mime_type, f.size_bytes, f.note, u.email, f.uploaded_at
		FROM feedback_collected_uploads f
		LEFT JOIN users u ON f.uploaded_by_user_id = u.id
		WHERE f.class_key = $1
		ORDER BY f.uploaded_at DESC
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query feedback uploads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*FeedbackCollectedUpload
	for rows.Next() {
		item := &FeedbackCollectedUpload{}
		if err := rows.Scan(
			&item.ID, &item.LeadID, &item.ClassKey, &item.SessionNumber, &item.FileName, &item.FileURL,
			&item.MimeType, &item.SizeBytes, &item.Note, &item.UploadedByUser, &item.UploadedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan feedback upload: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetFeedbackCollectedUploadByID returns a single feedback upload record.
func GetFeedbackCollectedUploadByID(id uuid.UUID) (*FeedbackCollectedUpload, error) {
	item := &FeedbackCollectedUpload{}
	err := db.DB.QueryRow(`
		SELECT id, lead_id, class_key, session_number, file_name, file_url,
		       mime_type, size_bytes, note, uploaded_by_user_id, uploaded_at
		FROM feedback_collected_uploads
		WHERE id = $1
	`, id).Scan(
		&item.ID, &item.LeadID, &item.ClassKey, &item.SessionNumber, &item.FileName, &item.FileURL,
		&item.MimeType, &item.SizeBytes, &item.Note, &item.UploadedByUser, &item.UploadedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

// DeleteFeedbackCollectedUpload removes a feedback upload record.
func DeleteFeedbackCollectedUpload(id uuid.UUID) error {
	_, err := db.DB.Exec(`DELETE FROM feedback_collected_uploads WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete feedback upload: %w", err)
	}
	return nil
}

// GetPendingFeedback returns students who need feedback at session 4 or 8
func GetPendingFeedback(sessionNumber int32) ([]struct {
	LeadID   uuid.UUID
	FullName string
	Phone    string
	ClassKey string
}, error) {
	rows, err := db.DB.Query(`
		SELECT DISTINCT l.id, l.full_name, l.phone, cs.class_key
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		INNER JOIN class_sessions cs ON cs.class_key = cg.class_key AND cs.session_number = $1
		WHERE cs.status = 'completed'
		AND NOT EXISTS (
			SELECT 1 FROM student_success_feedback cof
			WHERE cof.lead_id = l.id AND cof.class_key = cs.class_key AND cof.session_number = $1
		)
		ORDER BY l.full_name
	`, sessionNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending feedback: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []struct {
		LeadID   uuid.UUID
		FullName string
		Phone    string
		ClassKey string
	}
	for rows.Next() {
		var r struct {
			LeadID   uuid.UUID
			FullName string
			Phone    string
			ClassKey string
		}
		if err := rows.Scan(&r.LeadID, &r.FullName, &r.Phone, &r.ClassKey); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		results = append(results, r)
	}

	return results, rows.Err()
}

// LogAbsenceFollowUp logs a follow-up action for an absence
func LogAbsenceFollowUp(leadID uuid.UUID, sessionID uuid.UUID, messageSent bool, reason, studentReply, actionTaken, notes string, createdByUserID uuid.UUID) error {
	now := time.Now()
	var sessionIDNull sql.NullString
	if sessionID != uuid.Nil {
		sessionIDNull = sql.NullString{String: sessionID.String(), Valid: true}
	}

	_, err := db.DB.Exec(`
		INSERT INTO absence_follow_up_logs (id, lead_id, session_id, message_sent, reason, student_reply, action_taken, notes, created_by_user_id, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, leadID, sessionIDNull, messageSent, reason, studentReply, actionTaken, notes, createdByUserID, now)
	return err
}

// GetAbsenceFollowUpLogs returns all follow-up logs for a student
func GetAbsenceFollowUpLogs(leadID uuid.UUID) ([]*AbsenceFollowUpLog, error) {
	rows, err := db.DB.Query(`
		SELECT id, lead_id, session_id, message_sent, reason, student_reply, action_taken, notes, created_by_user_id, created_at
		FROM absence_follow_up_logs
		WHERE lead_id = $1
		ORDER BY created_at DESC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query follow-up logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var logs []*AbsenceFollowUpLog
	for rows.Next() {
		l := &AbsenceFollowUpLog{}
		var sessionID, reason, studentReply, actionTaken, notes, createdByUserID sql.NullString

		err := rows.Scan(
			&l.ID, &l.LeadID, &sessionID, &l.MessageSent,
			&reason, &studentReply, &actionTaken, &notes, &createdByUserID, &l.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log: %w", err)
		}

		l.SessionID = sessionID
		l.Reason = reason
		l.StudentReply = studentReply
		l.ActionTaken = actionTaken
		l.Notes = notes
		l.CreatedByUserID = createdByUserID
		logs = append(logs, l)
	}

	return logs, rows.Err()
}

// GetUsersByRole returns all users with a specific role
func GetUsersByRole(role string) ([]*User, error) {
	rows, err := db.DB.Query(`
		SELECT id, email, COALESCE(full_name, ''), COALESCE(phone, ''), password_hash, role, COALESCE(is_active, true), COALESCE(must_change_password, false), created_at
		FROM users
		WHERE role = $1
		  AND COALESCE(is_active, true) = true
		ORDER BY email
	`, role)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.PasswordHash, &u.Role, &u.IsActive, &u.MustChangePassword, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u)
	}

	return users, rows.Err()
}

func normalizeTrelloChecks(raw []bool) []bool {
	out := make([]bool, 8)
	copy(out, raw)
	return out
}

func parseTrelloChecksJSON(raw sql.NullString) []bool {
	if !raw.Valid {
		return make([]bool, 8)
	}
	var parsed []bool
	if err := json.Unmarshal([]byte(raw.String), &parsed); err != nil {
		return make([]bool, 8)
	}
	return normalizeTrelloChecks(parsed)
}

type classComplianceMetrics struct {
	Statuses       []string
	PunctualityPct int
	WhatsAppPct    int
	ChecksCount    int
	RemindersSent  int
	AbsentCount    int
	DelayedCount   int
}

func getClassComplianceMetrics(classKey string) (*classComplianceMetrics, error) {
	sessions, err := GetComplianceByClassKey(classKey)
	if err != nil {
		return nil, err
	}

	statuses := make([]string, 8)
	for i := range statuses {
		statuses[i] = "unknown"
	}

	checkedCount := 0
	remindersSent := 0
	absentCount := 0
	delayedCount := 0

	for _, s := range sessions {
		if s.SessionNumber < 1 || s.SessionNumber > 8 {
			continue
		}
		idx := int(s.SessionNumber - 1)
		if s.Check == nil {
			continue
		}
		checkedCount++
		if s.Check.Reminder1D {
			remindersSent++
		}
		if s.Check.Reminder1H {
			remindersSent++
		}
		if s.Check.ReminderTasks {
			remindersSent++
		}
		if s.Check.IsAbsent {
			absentCount++
			statuses[idx] = "absent"
			continue
		}
		if s.Check.DelayMinutes > 0 {
			delayedCount++
			statuses[idx] = "late"
			continue
		}
		statuses[idx] = "on-time"
	}

	whatsappPercent := 0
	if checkedCount > 0 {
		whatsappPercent = int((float64(remindersSent) / float64(checkedCount*3)) * 100)
		if whatsappPercent > 100 {
			whatsappPercent = 100
		}
	}

	equivalentAbsences := absentCount + (delayedCount / 2)
	onTimeEquivalent := 8 - equivalentAbsences
	if onTimeEquivalent < 0 {
		onTimeEquivalent = 0
	}
	punctualityPercent := int((float64(onTimeEquivalent) / 8.0) * 100.0)

	return &classComplianceMetrics{
		Statuses:       statuses,
		PunctualityPct: punctualityPercent,
		WhatsAppPct:    whatsappPercent,
		ChecksCount:    checkedCount,
		RemindersSent:  remindersSent,
		AbsentCount:    absentCount,
		DelayedCount:   delayedCount,
	}, nil
}

func computeAttendanceFromCompliance(classKey string) (statuses []string, punctualityPercent int, whatsappPercent int, err error) {
	metrics, err := getClassComplianceMetrics(classKey)
	if err != nil {
		return nil, 0, 0, err
	}
	return metrics.Statuses, metrics.PunctualityPct, metrics.WhatsAppPct, nil
}

func computeCollectiveClassScore(sessionQuality10 int, feedback10 int, trelloPercent int, punctualityPercent int, whatsappPercent int) int {
	sessionQualityPct := sessionQuality10 * 10
	feedbackPct := feedback10 * 10
	weighted := float64(punctualityPercent)*0.25 +
		float64(sessionQualityPct)*0.25 +
		float64(feedbackPct)*0.20 +
		float64(whatsappPercent)*0.10 +
		float64(trelloPercent)*0.20
	return int(math.Round(weighted))
}

func durationLabel(start, end sql.NullTime) string {
	if !end.Valid {
		return "Ongoing"
	}
	if !start.Valid {
		return "Closed"
	}
	d := end.Time.Sub(start.Time)
	if d < 0 {
		d = 0
	}
	days := int(d.Hours() / 24)
	if days < 30 {
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	months := days / 30
	if months < 12 {
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}
	years := months / 12
	if years == 1 {
		return "1 year"
	}
	return fmt.Sprintf("%d years", years)
}

// GetMentorEvaluationsByRoundStatus returns mentor evaluations grouped by mentor, scoped to one round status.
func GetMentorEvaluationsByRoundStatus(roundStatus string, mentorQuery string, fromDate, toDate *time.Time) ([]*MentorEvaluationMentorItem, error) {
	if roundStatus != "active" && roundStatus != "closed" {
		return nil, fmt.Errorf("invalid round status scope")
	}

	rows, err := db.DB.Query(`
		WITH scoped_classes AS (
			SELECT ma.class_key AS class_key, ma.mentor_user_id AS mentor_user_id
			FROM mentor_assignments ma
			INNER JOIN class_groups cg ON cg.class_key = ma.class_key
			WHERE $1 = 'active' AND cg.round_status = 'active'
			UNION ALL
			SELECT cg.class_key AS class_key, cg.closed_mentor_user_id AS mentor_user_id
			FROM class_groups cg
			WHERE $1 = 'closed'
			  AND cg.round_status = 'closed'
			  AND cg.closed_mentor_user_id IS NOT NULL
		)
		SELECT
			u.id, u.email, COALESCE(u.full_name, ''), COALESCE(u.phone, ''), u.password_hash, u.role, u.created_at,
			cg.class_key, cg.level, cg.class_days, cg.class_time, cg.class_number, cg.round_status,
			COALESCE(me.kpi_session_quality, 0) AS kpi_session_quality,
			COALESCE(me.kpi_students_feedback, 0) AS kpi_students_feedback,
			me.trello_session_checks
		FROM scoped_classes sc
		INNER JOIN class_groups cg ON cg.class_key = sc.class_key
		INNER JOIN users u ON u.id = sc.mentor_user_id
		LEFT JOIN mentor_evaluations me ON me.mentor_id = u.id AND me.class_key = cg.class_key
		WHERE u.role = 'mentor'
		  AND (
		    $2 = ''
		    OR LOWER(BTRIM(COALESCE(u.full_name, ''))) = LOWER(BTRIM($2))
		    OR LOWER(BTRIM(u.email)) = LOWER(BTRIM($2))
		    OR (
		      regexp_replace(BTRIM($2), '[0-9+() -]', '', 'g') = ''
		      AND (
		        BTRIM(COALESCE(u.phone, '')) = BTRIM($2)
		        OR (
		          regexp_replace(BTRIM($2), '\D', '', 'g') <> ''
		          AND regexp_replace(COALESCE(u.phone, ''), '\D', '', 'g') LIKE ('%' || regexp_replace(BTRIM($2), '\D', '', 'g') || '%')
		        )
		      )
		    )
		    OR (
		      POSITION(' ' IN BTRIM($2)) > 0
		      AND (
		        COALESCE(u.full_name, '') ILIKE ('%' || $2 || '%')
		        OR u.email ILIKE ('%' || $2 || '%')
		      )
		    )
		  )
		  AND (
		    $1 <> 'closed'
		    OR $3::date IS NULL
		    OR cg.round_closed_at::date >= $3::date
		  )
		  AND (
		    $1 <> 'closed'
		    OR $4::date IS NULL
		    OR cg.round_closed_at::date <= $4::date
		  )
		ORDER BY u.email ASC, cg.level ASC, cg.class_days ASC, cg.class_time ASC, cg.class_number ASC
	`, roundStatus, strings.TrimSpace(mentorQuery), fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor evaluations by class: %w", err)
	}
	defer func() { _ = rows.Close() }()

	mentorMap := map[uuid.UUID]*MentorEvaluationMentorItem{}
	order := make([]uuid.UUID, 0)

	for rows.Next() {
		user := &User{}
		classItem := MentorEvaluationClassItem{}
		var trelloJSON sql.NullString

		if err := rows.Scan(
			&user.ID, &user.Email, &user.FullName, &user.Phone, &user.PasswordHash, &user.Role, &user.CreatedAt,
			&classItem.ClassKey, &classItem.Level, &classItem.ClassDays, &classItem.ClassTime, &classItem.ClassNumber, &classItem.RoundStatus,
			&classItem.KPISessionQuality, &classItem.KPIStudentsFeedback, &trelloJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor evaluation class row: %w", err)
		}

		statuses, punctuality, whatsapp, err := computeAttendanceFromCompliance(classItem.ClassKey)
		if err != nil {
			return nil, fmt.Errorf("failed to compute compliance metrics for class %s: %w", classItem.ClassKey, err)
		}

		classItem.TrelloSessionChecks = parseTrelloChecksJSON(trelloJSON)
		classItem.AttendanceStatuses = statuses
		classItem.AttendancePercent = punctuality
		classItem.AutoWhatsAppPercent = whatsapp

		entry, ok := mentorMap[user.ID]
		if !ok {
			entry = &MentorEvaluationMentorItem{
				User:          user,
				ActiveClasses: make([]MentorEvaluationClassItem, 0, 2),
			}
			mentorMap[user.ID] = entry
			order = append(order, user.ID)
		}
		entry.ActiveClasses = append(entry.ActiveClasses, classItem)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate mentor evaluation class rows: %w", err)
	}

	out := make([]*MentorEvaluationMentorItem, 0, len(order))
	for _, id := range order {
		out = append(out, mentorMap[id])
	}
	return out, nil
}

// GetMentorEvaluationsActiveByClass returns mentor evaluations grouped by mentor, scoped to active classes only.
func GetMentorEvaluationsActiveByClass() ([]*MentorEvaluationMentorItem, error) {
	return GetMentorEvaluationsByRoundStatus("active", "", nil, nil)
}

func GetMentorDirectory() ([]*MentorDirectoryItem, error) {
	rows, err := db.DB.Query(`
		WITH taught_classes AS (
			SELECT ma.mentor_user_id AS mentor_id, ma.class_key
			FROM mentor_assignments ma
			UNION
			SELECT cg.closed_mentor_user_id AS mentor_id, cg.class_key
			FROM class_groups cg
			WHERE cg.closed_mentor_user_id IS NOT NULL
		),
		taught_counts AS (
			SELECT mentor_id, COUNT(DISTINCT class_key) AS total_classes_taught
			FROM taught_classes
			GROUP BY mentor_id
		),
		active_counts AS (
			SELECT ma.mentor_user_id AS mentor_id, COUNT(DISTINCT ma.class_key) AS active_classes
			FROM mentor_assignments ma
			JOIN class_groups cg ON cg.class_key = ma.class_key
			WHERE COALESCE(cg.round_status, '') = 'active'
			GROUP BY ma.mentor_user_id
		)
		SELECT
			u.id,
			COALESCE(NULLIF(BTRIM(u.full_name), ''), u.email) AS display_name,
			u.email,
			COALESCE(NULLIF(BTRIM(u.phone), ''), '-') AS phone,
			COALESCE(tc.total_classes_taught, 0) AS total_classes_taught,
			COALESCE(ac.active_classes, 0) AS active_classes
		FROM users u
		LEFT JOIN taught_counts tc ON tc.mentor_id = u.id
		LEFT JOIN active_counts ac ON ac.mentor_id = u.id
		WHERE u.role = 'mentor'
		ORDER BY u.email ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor directory: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]*MentorDirectoryItem, 0)
	for rows.Next() {
		item := &MentorDirectoryItem{}
		var activeClasses int
		if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Phone, &item.TotalClassesTaught, &activeClasses); err != nil {
			return nil, fmt.Errorf("failed to scan mentor directory row: %w", err)
		}
		if activeClasses > 0 {
			item.Status = "active"
		} else {
			item.Status = "inactive"
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate mentor directory rows: %w", err)
	}
	return out, nil
}

func GetMentorProfile(mentorID uuid.UUID) (*MentorProfile, error) {
	profile := &MentorProfile{}

	var role string
	var fullName sql.NullString
	var phone sql.NullString
	err := db.DB.QueryRow(`
		SELECT id, email, role, full_name, phone
		FROM users
		WHERE id = $1
	`, mentorID).Scan(&profile.MentorDetails.ID, &profile.MentorDetails.Email, &role, &fullName, &phone)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load mentor details: %w", err)
	}
	if role != "mentor" {
		return nil, nil
	}
	if fullName.Valid && strings.TrimSpace(fullName.String) != "" {
		profile.MentorDetails.Name = strings.TrimSpace(fullName.String)
	} else {
		profile.MentorDetails.Name = profile.MentorDetails.Email
	}
	if phone.Valid && strings.TrimSpace(phone.String) != "" {
		profile.MentorDetails.Phone = strings.TrimSpace(phone.String)
	} else {
		profile.MentorDetails.Phone = "-"
	}

	var activeClasses int
	err = db.DB.QueryRow(`
		SELECT COUNT(DISTINCT ma.class_key) AS active_classes
		FROM mentor_assignments ma
		JOIN class_groups cg ON cg.class_key = ma.class_key
		WHERE ma.mentor_user_id = $1
		  AND COALESCE(cg.round_status, '') = 'active'
	`, mentorID).Scan(&activeClasses)
	if err != nil {
		return nil, fmt.Errorf("failed to compute mentor profile stats: %w", err)
	}
	if activeClasses > 0 {
		profile.MentorDetails.Status = "active"
	} else {
		profile.MentorDetails.Status = "inactive"
	}
	profile.MentorDetails.TotalClassesTaught = profile.Stats.TotalClasses

	rows, err := db.DB.Query(`
		WITH class_keys AS (
			SELECT ma.class_key
			FROM mentor_assignments ma
			WHERE ma.mentor_user_id = $1
			UNION
			SELECT cg.class_key
			FROM class_groups cg
			WHERE cg.closed_mentor_user_id = $1
		)
		SELECT
			ck.class_key,
			cg.level,
			cg.class_days,
			cg.class_time,
			COALESCE(cg.round_started_at, cg.sent_at, ma.assigned_at, cg.round_closed_at) AS start_date,
			cg.round_closed_at,
			COALESCE(me.kpi_session_quality, 0) AS kpi_session_quality,
			COALESCE(me.kpi_students_feedback, 0) AS kpi_students_feedback,
			me.trello_session_checks
		FROM class_keys ck
		INNER JOIN class_groups cg ON cg.class_key = ck.class_key
		LEFT JOIN mentor_assignments ma ON ma.class_key = ck.class_key AND ma.mentor_user_id = $1
		LEFT JOIN mentor_evaluations me ON me.mentor_id = $1 AND me.class_key = ck.class_key
		ORDER BY COALESCE(cg.round_started_at, cg.sent_at, ma.assigned_at, cg.round_closed_at) DESC, ma.assigned_at DESC NULLS LAST
	`, mentorID)
	if err != nil {
		return nil, fmt.Errorf("failed to load mentor class history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	history := make([]MentorClassHistoryItem, 0)
	totalEval := 0
	classCountForKPI := 0
	totalFeedback := 0
	feedbackCount := 0
	totalReminderSlots := 0
	totalRemindersSent := 0

	for rows.Next() {
		item := MentorClassHistoryItem{}
		var sessionQuality, feedback int
		var trelloJSON sql.NullString
		if err := rows.Scan(
			&item.ClassKey, &item.Level, &item.Days, &item.Time,
			&item.StartDate, &item.EndDate,
			&sessionQuality, &feedback, &trelloJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor class history row: %w", err)
		}

		metrics, err := getClassComplianceMetrics(item.ClassKey)
		if err != nil {
			return nil, fmt.Errorf("failed to compute compliance for class %s: %w", item.ClassKey, err)
		}

		trelloChecks := parseTrelloChecksJSON(trelloJSON)
		checked := 0
		for _, ok := range trelloChecks {
			if ok {
				checked++
			}
		}
		trelloPercent := (checked * 100) / 8

		item.ComplianceScore = metrics.WhatsAppPct
		item.EvaluationScore = computeCollectiveClassScore(sessionQuality, feedback, trelloPercent, metrics.PunctualityPct, metrics.WhatsAppPct)
		item.Duration = durationLabel(item.StartDate, item.EndDate)
		if feedback > 0 {
			totalFeedback += feedback * 10
			feedbackCount++
		}

		totalEval += item.EvaluationScore
		classCountForKPI++
		totalReminderSlots += metrics.ChecksCount * 3
		totalRemindersSent += metrics.RemindersSent

		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate mentor class history rows: %w", err)
	}

	profile.ClassHistory = history
	profile.Stats.TotalClasses = len(history)
	for _, item := range history {
		if item.StartDate.Valid {
			if !profile.Stats.FirstClassDate.Valid || item.StartDate.Time.Before(profile.Stats.FirstClassDate.Time) {
				profile.Stats.FirstClassDate = item.StartDate
			}
			if !profile.Stats.LastClassDate.Valid || item.StartDate.Time.After(profile.Stats.LastClassDate.Time) {
				profile.Stats.LastClassDate = item.StartDate
			}
		}
	}
	profile.MentorDetails.TotalClassesTaught = profile.Stats.TotalClasses
	if classCountForKPI > 0 {
		profile.Stats.AvgRating = int(math.Round(float64(totalEval) / float64(classCountForKPI)))
	}
	if feedbackCount > 0 {
		profile.Stats.FeedbackMeter = int(math.Round(float64(totalFeedback) / float64(feedbackCount)))
	}
	if totalReminderSlots > 0 {
		profile.Stats.ComplianceScore = int(math.Round((float64(totalRemindersSent) / float64(totalReminderSlots)) * 100))
	}

	testimonials, err := GetMentorTestimonials(mentorID)
	if err != nil {
		return nil, fmt.Errorf("failed to load mentor testimonials: %w", err)
	}
	profile.Testimonials = testimonials
	return profile, nil
}

func GetMentorTestimonials(mentorID uuid.UUID) ([]MentorTestimonial, error) {
	rows, err := db.DB.Query(`
		SELECT
			mt.id,
			mt.mentor_id,
			mt.class_key,
			mt.testimonial_text,
			mt.created_by_user_id,
			COALESCE(u.email, '') AS created_by_email,
			mt.created_at
		FROM mentor_testimonials mt
		LEFT JOIN users u ON u.id = mt.created_by_user_id
		WHERE mt.mentor_id = $1
		ORDER BY mt.created_at DESC
	`, mentorID)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor testimonials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]MentorTestimonial, 0)
	for rows.Next() {
		var item MentorTestimonial
		if err := rows.Scan(
			&item.ID,
			&item.MentorID,
			&item.ClassKey,
			&item.TestimonialText,
			&item.CreatedByUserID,
			&item.CreatedByEmail,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor testimonial: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate mentor testimonials: %w", err)
	}
	return out, nil
}

func CreateMentorTestimonial(mentorID uuid.UUID, classKey string, testimonialText string, createdByUserID uuid.UUID) (*MentorTestimonial, error) {
	classKey = strings.TrimSpace(classKey)
	testimonialText = strings.TrimSpace(testimonialText)
	if classKey == "" {
		return nil, fmt.Errorf("class_key is required")
	}
	if testimonialText == "" {
		return nil, fmt.Errorf("testimonial_text is required")
	}

	// Ensure mentor exists and role is mentor.
	var mentorExists bool
	if err := db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM users WHERE id = $1 AND role = 'mentor'
		)
	`, mentorID).Scan(&mentorExists); err != nil {
		return nil, fmt.Errorf("failed to validate mentor: %w", err)
	}
	if !mentorExists {
		return nil, fmt.Errorf("mentor not found")
	}

	// Ensure class exists.
	var classExists bool
	if err := db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM class_groups WHERE class_key = $1
		)
	`, classKey).Scan(&classExists); err != nil {
		return nil, fmt.Errorf("failed to validate class_key: %w", err)
	}
	if !classExists {
		return nil, fmt.Errorf("class_key not found")
	}

	item := &MentorTestimonial{}
	err := db.DB.QueryRow(`
		INSERT INTO mentor_testimonials (
			id, mentor_id, class_key, testimonial_text, created_by_user_id, created_at, updated_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW(), NOW())
		RETURNING id, mentor_id, class_key, testimonial_text, created_by_user_id, created_at
	`, mentorID, classKey, testimonialText, createdByUserID).Scan(
		&item.ID,
		&item.MentorID,
		&item.ClassKey,
		&item.TestimonialText,
		&item.CreatedByUserID,
		&item.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mentor testimonial: %w", err)
	}

	creator, err := GetUserByID(createdByUserID.String())
	if err == nil && creator != nil {
		item.CreatedByEmail = creator.Email
	}
	return item, nil
}

// UpsertMentorEvaluationByClass creates or updates class-scoped manual evaluation fields.
func UpsertMentorEvaluationByClass(mentorID uuid.UUID, classKey string, evaluatorID uuid.UUID, sessionQuality int, studentsFeedback int, trelloSessionChecks []bool) error {
	if classKey == "" {
		return fmt.Errorf("class_key is required")
	}
	if sessionQuality < 1 || sessionQuality > 10 {
		return fmt.Errorf("session_quality must be between 1 and 10")
	}
	if studentsFeedback < 1 || studentsFeedback > 10 {
		return fmt.Errorf("students_feedback must be between 1 and 10")
	}
	trelloSessionChecks = normalizeTrelloChecks(trelloSessionChecks)
	trelloJSON, err := json.Marshal(trelloSessionChecks)
	if err != nil {
		return fmt.Errorf("failed to encode trello checks: %w", err)
	}

	var exists bool
	err = db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM class_groups cg
			INNER JOIN mentor_assignments ma ON ma.class_key = cg.class_key
			WHERE cg.class_key = $1
			  AND cg.round_status = 'active'
			  AND ma.mentor_user_id = $2
		)
	`, classKey, mentorID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to validate active class ownership: %w", err)
	}
	if !exists {
		return fmt.Errorf("class is not an active class assigned to this mentor")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin mentor evaluation upsert tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	updateRes, err := tx.Exec(`
		UPDATE mentor_evaluations
		SET kpi_session_quality = $3,
		    kpi_students_feedback = $4,
		    trello_session_checks = $5::jsonb,
		    evaluator_id = $6,
		    updated_at = NOW()
		WHERE mentor_id = $1
		  AND class_key = $2
	`, mentorID, classKey, sessionQuality, studentsFeedback, string(trelloJSON), evaluatorID)
	if err != nil {
		return fmt.Errorf("failed to update mentor evaluation by class: %w", err)
	}

	updatedRows, err := updateRes.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read mentor evaluation update rows affected: %w", err)
	}
	if updatedRows == 0 {
		_, err = tx.Exec(`
			INSERT INTO mentor_evaluations (
				mentor_id, class_key, kpi_session_quality, kpi_students_feedback, trello_session_checks, evaluator_id, updated_at
			)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6, NOW())
		`, mentorID, classKey, sessionQuality, studentsFeedback, string(trelloJSON), evaluatorID)
		if err != nil {
			// If another request inserted concurrently, retry as update in same tx.
			if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
				_, err = tx.Exec(`
					UPDATE mentor_evaluations
					SET kpi_session_quality = $3,
					    kpi_students_feedback = $4,
					    trello_session_checks = $5::jsonb,
					    evaluator_id = $6,
					    updated_at = NOW()
					WHERE mentor_id = $1
					  AND class_key = $2
				`, mentorID, classKey, sessionQuality, studentsFeedback, string(trelloJSON), evaluatorID)
			}
			if err != nil {
				return fmt.Errorf("failed to upsert mentor evaluation by class: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit mentor evaluation upsert tx: %w", err)
	}
	return nil
}

// GetUserByID returns a user by ID
func GetUserByID(userID string) (*User, error) {
	u := &User{}
	err := db.DB.QueryRow(`
		SELECT id, email, COALESCE(full_name, ''), COALESCE(phone, ''), password_hash, role, COALESCE(is_active, true), COALESCE(must_change_password, false), created_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.PasswordHash, &u.Role, &u.IsActive, &u.MustChangePassword, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return u, nil
}

func GetAllUsers() ([]*User, error) {
	rows, err := db.DB.Query(`
		SELECT id, email, COALESCE(full_name, ''), COALESCE(phone, ''), password_hash, role, COALESCE(is_active, true), COALESCE(must_change_password, false), created_at
		FROM users
		ORDER BY created_at DESC, email ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]*User, 0, 32)
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Phone, &u.PasswordHash, &u.Role, &u.IsActive, &u.MustChangePassword, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func ForceChangeUserPassword(userID string, passwordHash string) error {
	_, err := db.DB.Exec(`
		UPDATE users
		SET password_hash = $2,
		    must_change_password = false
		WHERE id = $1
	`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("failed to update user password: %w", err)
	}
	return nil
}

func DeleteUserByID(userID string) error {
	result, err := db.DB.Exec(`DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify deleted user: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func DeactivateUserByID(userID string) error {
	result, err := db.DB.Exec(`UPDATE users SET is_active = false WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("failed to deactivate user: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify deactivated user: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetClassGroupByKey returns a class group by class_key
func GetClassGroupByKey(classKey string) (*ClassGroupWorkflow, error) {
	return GetClassGroupWorkflow(classKey)
}

// GetSessionByID returns a session by ID
func GetSessionByID(sessionID uuid.UUID) (*ClassSession, error) {
	s := &ClassSession{}
	var scheduledTime, scheduledEndTime, actualTime, actualEndTime sql.NullString
	var actualDate, completedAt sql.NullTime

	err := db.DB.QueryRow(`
		SELECT id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time,
		       actual_date, actual_time, actual_end_time, status, completed_at, created_at, updated_at
		FROM class_sessions
		WHERE id = $1
	`, sessionID).Scan(
		&s.ID, &s.ClassKey, &s.SessionNumber, &s.ScheduledDate,
		&scheduledTime, &scheduledEndTime, &actualDate, &actualTime, &actualEndTime,
		&s.Status, &completedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	s.ScheduledTime = scheduledTime
	s.ScheduledEndTime = scheduledEndTime
	s.ActualDate = actualDate
	s.ActualTime = actualTime
	s.ActualEndTime = actualEndTime
	s.CompletedAt = completedAt
	return s, nil
}

// GetClassGroupsSentToMentor returns all class groups where sent_to_mentor = true (excluding closed rounds)
func GetClassGroupsSentToMentor() ([]*ClassGroupWorkflow, error) {
	rows, err := db.DB.Query(`
		SELECT class_key, level, class_days, class_time, class_number,
		       sent_to_mentor, sent_at, returned_at, updated_at,
		       hidden_in_ops, hidden_at, hidden_by::text
		FROM class_groups
		WHERE sent_to_mentor = true AND COALESCE(round_status, '') != 'closed'
		ORDER BY level, class_days, class_time
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query class groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []*ClassGroupWorkflow
	for rows.Next() {
		g := &ClassGroupWorkflow{}
		var sentAt, returnedAt, hiddenAt sql.NullTime
		var hiddenBy sql.NullString

		err := rows.Scan(
			&g.ClassKey, &g.Level, &g.ClassDays, &g.ClassTime, &g.ClassNumber,
			&g.SentToMentor, &sentAt, &returnedAt, &g.UpdatedAt,
			&g.HiddenInOps, &hiddenAt, &hiddenBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class group: %w", err)
		}

		g.SentAt = sentAt
		g.ReturnedAt = returnedAt
		g.HiddenAt = hiddenAt
		g.HiddenBy = hiddenBy
		groups = append(groups, g)
	}

	return groups, rows.Err()
}

// StudentSuccessClassRow represents a class in the Student Success dashboard
type StudentSuccessClassRow struct {
	ClassKey           string `json:"class_key"`
	Level              int    `json:"level"`
	ClassDays          string `json:"class_days"`
	ClassTime          string `json:"class_time"`
	ClassNumber        int    `json:"class_number"`
	MentorEmail        string `json:"mentor_email"`
	MentorName         string `json:"mentor_name"`
	MentorUserID       string `json:"mentor_user_id"`
	StudentCount       int    `json:"student_count"`
	HasHighPriority    bool   `json:"has_high_priority"`
	HighPriorityReason string `json:"high_priority_reason"`
}

// GetActiveClassesForStudentSuccess returns all classes where round_status = 'IN_PROGRESS'.
// Includes mentor email/name (if assigned), schedule, level, class_number, student_count, and at-risk flag.
func GetActiveClassesForStudentSuccess() ([]StudentSuccessClassRow, error) {
	rows, err := db.DB.Query(`
		SELECT cg.class_key, cg.level, cg.class_days, cg.class_time, cg.class_number,
		       COALESCE(u.email, ''), COALESCE(u.email, ''), COALESCE(ma.mentor_user_id::text, ''),
		       EXISTS (
			       SELECT 1 FROM leads l
				   INNER JOIN scheduling s ON s.lead_id = l.id
				   INNER JOIN placement_tests pt ON pt.lead_id = l.id
				   WHERE pt.assigned_level = cg.level
				     AND s.class_days = cg.class_days
				     AND TO_CHAR(s.class_time, 'HH24:MI') = LEFT(cg.class_time, 5)
				     AND COALESCE(s.class_group_index, 1) = COALESCE(cg.class_number, 1)
				     AND (
					     -- Manual flag (not the automated absence one)
					     (l.high_priority = TRUE AND l.high_priority_reason NOT LIKE '%3+ sessions%')
					     OR
					     -- Actual absences in THIS class
					     (
						     SELECT COUNT(*) 
						     FROM attendance a 
						     JOIN class_sessions cs ON a.session_id = cs.id 
						     WHERE a.lead_id = l.id 
						       AND cs.class_key = cg.class_key 
						       AND a.status = 'ABSENT'
						 ) >= 3
					 )
			   ) as has_high_priority,
			   (
			       SELECT COALESCE(STRING_AGG(DISTINCT 
				       CASE 
					       WHEN l.high_priority = TRUE AND l.high_priority_reason NOT LIKE '%3+ sessions%' THEN l.high_priority_reason
					       ELSE l.full_name || ': 3+ absences'
				       END, '; '), '')
			       FROM leads l
				   INNER JOIN scheduling s ON s.lead_id = l.id
				   INNER JOIN placement_tests pt ON pt.lead_id = l.id
				   WHERE pt.assigned_level = cg.level
				     AND s.class_days = cg.class_days
				     AND TO_CHAR(s.class_time, 'HH24:MI') = LEFT(cg.class_time, 5)
				     AND COALESCE(s.class_group_index, 1) = COALESCE(cg.class_number, 1)
				     AND (
					     (l.high_priority = TRUE AND l.high_priority_reason NOT LIKE '%3+ sessions%')
					     OR
					     (
						     SELECT COUNT(*) 
						     FROM attendance a 
						     JOIN class_sessions cs ON a.session_id = cs.id 
						     WHERE a.lead_id = l.id 
						       AND cs.class_key = cg.class_key 
						       AND a.status = 'ABSENT'
						 ) >= 3
					 )
			   ) as high_priority_reason
		FROM class_groups cg
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		WHERE cg.round_status = 'active'
		  AND cg.sent_to_mentor = true
		ORDER BY cg.level, cg.class_days, cg.class_time
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query active classes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []StudentSuccessClassRow
	for rows.Next() {
		var r StudentSuccessClassRow
		if err := rows.Scan(&r.ClassKey, &r.Level, &r.ClassDays, &r.ClassTime, &r.ClassNumber,
			&r.MentorEmail, &r.MentorName, &r.MentorUserID, &r.HasHighPriority, &r.HighPriorityReason); err != nil {
			return nil, fmt.Errorf("failed to scan: %w", err)
		}
		students, _ := GetStudentsInClassGroup(r.ClassKey)
		r.StudentCount = len(students)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetAttendanceMissedSessions returns map of lead_id -> slice of missed session numbers for a class.
func GetAttendanceMissedSessions(classKey string) (map[uuid.UUID][]int32, error) {
	rows, err := db.DB.Query(`
		SELECT a.lead_id, cs.session_number
		FROM attendance a
		INNER JOIN class_sessions cs ON cs.id = a.session_id
		WHERE cs.class_key = $1 AND a.status = 'ABSENT'
		ORDER BY cs.session_number
	`, classKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	m := make(map[uuid.UUID][]int32)
	for rows.Next() {
		var id uuid.UUID
		var n int32
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		m[id] = append(m[id], n)
	}
	return m, rows.Err()
}

// GetStudentSuccessClassDetail returns class + students (with missed_sessions) + sessions + feedback.
// When allowClosed=true, closed classes are allowed and students are sourced from class_enrollments.
func GetStudentSuccessClassDetail(classKey string, allowClosed bool) (classGroup *ClassGroupWorkflow, students []*ClassStudent, sessions []*ClassSession, missedSessions map[uuid.UUID][]int32, feedbackRecords []*StudentSuccessFeedback, completedCount int, err error) {
	classGroup, err = GetClassGroupByKey(classKey)
	if err != nil || classGroup == nil {
		return nil, nil, nil, nil, nil, 0, err
	}
	if classGroup.RoundStatus != "active" {
		if !allowClosed || classGroup.RoundStatus != "closed" {
			return nil, nil, nil, nil, nil, 0, fmt.Errorf("class is not active")
		}
	}
	if !classGroup.SentToMentor {
		return nil, nil, nil, nil, nil, 0, fmt.Errorf("class is not sent to mentor")
	}
	if classGroup.RoundStatus == "closed" {
		students, err = GetStudentsForMentorHeadClass(classKey)
		if err != nil {
			return nil, nil, nil, nil, nil, 0, err
		}
	} else {
		students, err = GetStudentsInClassGroup(classKey)
		if err != nil {
			return nil, nil, nil, nil, nil, 0, err
		}
	}
	sessions, err = GetClassSessions(classKey)
	if err != nil {
		sessions = nil
	}
	missedSessions, _ = GetAttendanceMissedSessions(classKey)
	if missedSessions == nil {
		missedSessions = make(map[uuid.UUID][]int32)
	}

	feedbackRecords, _ = GetClassFeedbackRecords(classKey)
	if feedbackRecords == nil {
		feedbackRecords = []*StudentSuccessFeedback{}
	}

	for _, s := range sessions {
		if s.Status == "completed" {
			completedCount++
		}
	}

	return classGroup, students, sessions, missedSessions, feedbackRecords, completedCount, nil
}

// GetStudentsInClassGroup returns all students in a class group
func GetStudentsInClassGroup(classKey string) ([]*ClassStudent, error) {
	rows, err := db.DB.Query(`
		SELECT l.id, l.full_name, l.phone, l.is_returning, s.class_group_index, lj.joined_at_session_number
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		LEFT JOIN late_joiners lj ON lj.lead_id = l.id AND lj.class_key = cg.class_key
		WHERE cg.class_key = $1
		  AND l.status = 'in_classes'
		ORDER BY l.full_name
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var students []*ClassStudent
	for rows.Next() {
		s := &ClassStudent{}
		var groupIndex sql.NullInt32

		err := rows.Scan(&s.LeadID, &s.FullName, &s.Phone, &s.IsReturning, &groupIndex, &s.JoinedAtSessionNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan student: %w", err)
		}

		s.GroupIndex = groupIndex
		students = append(students, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return students, nil
}

// GetStudentsForMentorHeadClass returns students for mentor head views.
// Includes ready_to_start students when class is sent_to_mentor and round hasn't started yet.
// Keeps the original roster rule for other roles (in_classes only).
func GetStudentsForMentorHeadClass(classKey string) ([]*ClassStudent, error) {
	// If class is closed, return historical roster from class_enrollments.
	var roundStatus sql.NullString
	err := db.DB.QueryRow(`SELECT COALESCE(round_status, 'not_started') FROM class_groups WHERE class_key = $1`, classKey).Scan(&roundStatus)
	if err == nil && roundStatus.Valid && roundStatus.String == "closed" {
		rows, err := db.DB.Query(`
			SELECT l.id, l.full_name, l.phone, l.is_returning, lj.joined_at_session_number
			FROM class_enrollments ce
			INNER JOIN leads l ON l.id = ce.lead_id
			LEFT JOIN late_joiners lj ON lj.lead_id = l.id AND lj.class_key = ce.class_key
			WHERE ce.class_key = $1
			ORDER BY l.full_name
		`, classKey)
		if err != nil {
			return nil, fmt.Errorf("failed to query closed class students: %w", err)
		}
		defer func() { _ = rows.Close() }()

		var students []*ClassStudent
		for rows.Next() {
			s := &ClassStudent{}
			if err := rows.Scan(&s.LeadID, &s.FullName, &s.Phone, &s.IsReturning, &s.JoinedAtSessionNumber); err != nil {
				return nil, fmt.Errorf("failed to scan closed class student: %w", err)
			}
			students = append(students, s)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return students, nil
	}

	rows, err := db.DB.Query(`
		SELECT l.id, l.full_name, l.phone, s.class_group_index, lj.joined_at_session_number
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		LEFT JOIN late_joiners lj ON lj.lead_id = l.id AND lj.class_key = cg.class_key
		WHERE cg.class_key = $1
		  AND (
			l.status = 'in_classes'
			OR (
				cg.sent_to_mentor = true
				AND COALESCE(cg.round_status, 'not_started') = 'not_started'
				AND l.status = 'ready_to_start'
			)
			OR COALESCE(cg.round_status, '') = 'closed'
		  )
		ORDER BY l.full_name
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var students []*ClassStudent
	for rows.Next() {
		s := &ClassStudent{}
		var groupIndex sql.NullInt32

		err := rows.Scan(&s.LeadID, &s.FullName, &s.Phone, &groupIndex, &s.JoinedAtSessionNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan student: %w", err)
		}

		s.GroupIndex = groupIndex
		students = append(students, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return students, nil
}

// GetEligibleClassesForLateJoin returns classes a student is eligible to join as a late joiner.
// Rule: Class must be same level, session <= 2, and current enrollment 4-5.
// Eligibility includes:
// - active classes, and
// - sent_to_mentor + not_started classes (pre-start exception).
func GetEligibleClassesForLateJoin(leadID uuid.UUID) ([]*EligibleClass, error) {
	// Lead must be ready to start before late-join assignment.
	var leadStatus string
	err := db.DB.QueryRow(`SELECT status FROM leads WHERE id = $1`, leadID).Scan(&leadStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("lead not found")
		}
		return nil, fmt.Errorf("failed to get lead status: %w", err)
	}
	if leadStatus != "ready_to_start" {
		return nil, fmt.Errorf("late join is only available for ready-to-start students")
	}

	// 1. Get student's assigned level
	var assignedLevel sql.NullInt32
	err = db.DB.QueryRow(`
		SELECT assigned_level FROM placement_tests WHERE lead_id = $1
	`, leadID).Scan(&assignedLevel)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("student has no placement test")
		}
		return nil, err
	}
	if !assignedLevel.Valid {
		return nil, fmt.Errorf("student has no assigned level")
	}

	// 2. Query classes matching level and eligible class state.
	rows, err := db.DB.Query(`
		SELECT cg.class_key, cg.level, cg.class_days, cg.class_time,
		       (SELECT COUNT(*) FROM class_sessions WHERE class_key = cg.class_key AND status = 'completed') + 1 as current_session,
		       COALESCE(cg.round_status, 'not_started') as round_status
		FROM class_groups cg
		WHERE cg.level = $1
		  AND (
		    COALESCE(cg.round_status, 'not_started') = 'active'
		    OR (
		      COALESCE(cg.round_status, 'not_started') = 'not_started'
		      AND COALESCE(cg.sent_to_mentor, false) = true
		    )
		  )
	`, assignedLevel.Int32)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var eligible []*EligibleClass
	for rows.Next() {
		ec := &EligibleClass{}
		var roundStatus string
		err := rows.Scan(&ec.ClassKey, &ec.Level, &ec.ClassDays, &ec.ClassTime, &ec.CurrentSession, &roundStatus)
		if err != nil {
			return nil, err
		}

		// Skip if session > 2
		if ec.CurrentSession > 2 {
			continue
		}

		// Count current enrollment:
		// - active classes: in_classes students
		// - sent/not_started classes: ready_to_start + in_classes roster
		err = db.DB.QueryRow(`
			SELECT COUNT(DISTINCT l.id)
			FROM leads l
			INNER JOIN scheduling s ON s.lead_id = l.id
			INNER JOIN placement_tests pt ON pt.lead_id = l.id
			INNER JOIN class_groups cg ON (
				cg.level = pt.assigned_level
				AND cg.class_days = s.class_days
				AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
				AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
			)
			WHERE cg.class_key = $1
			  AND (
			    l.status = 'in_classes'
			    OR (
			      $2 = 'not_started'
			      AND l.status = 'ready_to_start'
			    )
			  )
		`, ec.ClassKey, roundStatus).Scan(&ec.CurrentEnrollment)
		if err != nil {
			return nil, fmt.Errorf("failed to count eligible class students: %w", err)
		}

		// Filter by capacity (4-5 students)
		if ec.CurrentEnrollment >= 4 && ec.CurrentEnrollment <= 5 {
			eligible = append(eligible, ec)
		}
	}

	return eligible, nil
}

// AddLateJoiner adds a student to an active class group after the round has started.
// It updates scheduling, lead status, creates an audit record, and backfills N/A attendance.
func AddLateJoiner(leadID uuid.UUID, classKey string, reason string, userID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Validate current session <= 2
	var currentSession int32
	err = tx.QueryRow(`
		SELECT COUNT(*) + 1 
		FROM class_sessions 
		WHERE class_key = $1 AND status = 'completed'
	`, classKey).Scan(&currentSession)
	if err != nil {
		return fmt.Errorf("failed to get current session: %w", err)
	}
	if currentSession > 2 {
		return fmt.Errorf("cannot join class: too late (current session: %d)", currentSession)
	}

	// 1.5 Validate class state and load class details.
	// Allowed:
	// - active class
	// - sent_to_mentor + not_started class (pre-start exception)
	var roundStatus string
	var sentToMentor bool
	var level, classNumber int32
	var classDays, classTime string
	err = tx.QueryRow(`
		SELECT COALESCE(round_status, 'not_started'),
		       COALESCE(sent_to_mentor, false),
		       level, class_days, class_time, COALESCE(class_number, 1)
		FROM class_groups
		WHERE class_key = $1
	`, classKey).Scan(&roundStatus, &sentToMentor, &level, &classDays, &classTime, &classNumber)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("class not found")
		}
		return fmt.Errorf("failed to get class details: %w", err)
	}
	isActiveClass := roundStatus == "active"
	isSentNotStartedClass := roundStatus == "not_started" && sentToMentor
	if !isActiveClass && !isSentNotStartedClass {
		return fmt.Errorf("cannot join class: class must be active or sent to mentor head (not started)")
	}

	// 2. Validate capacity (4-5 students currently)
	// For not-started sent classes, ready_to_start students are part of current roster.
	var studentCount int
	err = tx.QueryRow(`
		SELECT COUNT(DISTINCT l.id)
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE cg.class_key = $1
		  AND (
		    l.status = 'in_classes'
		    OR (
		      $2 = 'not_started'
		      AND l.status = 'ready_to_start'
		    )
		  )
	`, classKey, roundStatus).Scan(&studentCount)
	if err != nil {
		return fmt.Errorf("failed to count students: %w", err)
	}
	if studentCount < 4 || studentCount > 5 {
		return fmt.Errorf("cannot join class: invalid capacity (current students: %d, required: 4-5)", studentCount)
	}

	// 3. Validate lead status eligibility.
	var status string
	err = tx.QueryRow(`SELECT status FROM leads WHERE id = $1`, leadID).Scan(&status)
	if err != nil {
		return fmt.Errorf("failed to get lead status: %w", err)
	}
	if status != "ready_to_start" {
		return fmt.Errorf("late join is only available for ready-to-start students")
	}
	if status == "in_classes" {
		return fmt.Errorf("student is already enrolled in a class")
	}

	// 4. Store current scheduling snapshot for undo functionality
	var prevDays, prevTime sql.NullString
	var prevIndex sql.NullInt32
	err = tx.QueryRow(`
		SELECT class_days, class_time, class_group_index 
		FROM scheduling 
		WHERE lead_id = $1
	`, leadID).Scan(&prevDays, &prevTime, &prevIndex)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get previous scheduling: %w", err)
	}

	// 5. Create late_joiners audit record
	_, err = tx.Exec(`
		INSERT INTO late_joiners (
			id, lead_id, class_key, joined_at_session_number, reason, added_by_user_id,
			previous_class_days, previous_class_time, previous_class_group_index, created_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`, leadID, classKey, currentSession, reason, userID, prevDays, prevTime, prevIndex)
	if err != nil {
		return fmt.Errorf("failed to create late joiner record: %w", err)
	}

	// 6. Update scheduling to match target class
	_, err = tx.Exec(`
		UPDATE scheduling
		SET class_days = $1,
		    class_time = $2,
		    class_group_index = $3,
		    updated_at = NOW()
		WHERE lead_id = $4
	`, classDays, classTime, classNumber, leadID)
	if err != nil {
		return fmt.Errorf("failed to update scheduling: %w", err)
	}

	// 7. Update lead status to 'in_classes' and mark as sent to classes
	_, err = tx.Exec(`
		UPDATE leads
		SET status = 'in_classes', sent_to_classes = true, updated_at = NOW()
		WHERE id = $1
	`, leadID)
	if err != nil {
		return fmt.Errorf("failed to update lead status: %w", err)
	}

	// 8. Backfill 'N/A' attendance for past sessions
	_, err = tx.Exec(`
		INSERT INTO attendance (id, session_id, lead_id, status, created_at, updated_at)
		SELECT gen_random_uuid(), cs.id, $1, 'N/A', NOW(), NOW()
		FROM class_sessions cs
		WHERE cs.class_key = $2
		  AND cs.session_number < $3
	`, leadID, classKey, currentSession)
	if err != nil {
		return fmt.Errorf("failed to backfill attendance: %w", err)
	}

	// 9. Insert notifications.
	// If class is sent but not started, notify Mentor Head only.
	// Otherwise (active), keep existing recipients (mentor, mentor heads, student success).
	notificationQuery := `
		INSERT INTO late_joiner_notifications (lead_id, class_key, user_id, joined_at_session_number)
		SELECT $1, $2, u.id, $3
		FROM users u
		LEFT JOIN mentor_assignments ma ON ma.mentor_user_id = u.id AND ma.class_key = $2
		WHERE (
			$4 = 'not_started' AND $5 = true AND u.role = 'mentor_head'
		) OR (
			$4 = 'active' AND (u.role IN ('mentor_head', 'student_success') OR ma.id IS NOT NULL)
		)
		ON CONFLICT (lead_id, class_key, user_id) DO NOTHING
	`
	_, err = tx.Exec(notificationQuery, leadID, classKey, currentSession, roundStatus, sentToMentor)
	if err != nil {
		// Log but don't fail transaction for notifications
		log.Printf("WARNING: Failed to insert late joiner notifications: %v", err)
	}

	return tx.Commit()
}

// GetLateJoinerByLeadID returns a late joiner record if it exists for a lead.
func GetLateJoinerByLeadID(leadID uuid.UUID) (*LateJoiner, error) {
	query := `
		SELECT lead_id, class_key, joined_at_session_number,
		       previous_class_days, previous_class_time, previous_class_group_index,
		       reason, added_by_user_id, created_at
		FROM late_joiners
		WHERE lead_id = $1
	`
	var lj LateJoiner
	err := db.DB.QueryRow(query, leadID).Scan(
		&lj.LeadID, &lj.ClassKey, &lj.JoinedAtSessionNumber,
		&lj.PreviousClassDays, &lj.PreviousClassTime, &lj.PreviousClassGroupIndex,
		&lj.Reason, &lj.AddedByUserID, &lj.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lj, nil
}

// GetActiveClassCurrentSession returns the current session number (completed + 1) for a class.
func GetActiveClassCurrentSession(classKey string) (int32, error) {
	var currentSession int32
	err := db.DB.QueryRow(`
		SELECT COUNT(*) + 1 
		FROM class_sessions 
		WHERE class_key = $1 AND status = 'completed'
	`, classKey).Scan(&currentSession)
	if err != nil {
		return 0, err
	}
	return currentSession, nil
}

// UndoLateJoiner reverts a late join action.
// Allowed only if the class current session is still <= 2.
func UndoLateJoiner(leadID uuid.UUID, userID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Get late_joiners record for lead
	var lj LateJoiner
	err = tx.QueryRow(`
		SELECT class_key, joined_at_session_number, 
		       previous_class_days, previous_class_time, previous_class_group_index
		FROM late_joiners
		WHERE lead_id = $1
	`, leadID).Scan(&lj.ClassKey, &lj.JoinedAtSessionNumber, &lj.PreviousClassDays, &lj.PreviousClassTime, &lj.PreviousClassGroupIndex)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no late joiner record found for this student")
		}
		return fmt.Errorf("failed to get late joiner record: %w", err)
	}

	// 2. Validate class current session still <= 2
	var currentSession int32
	err = tx.QueryRow(`
		SELECT COUNT(*) + 1 
		FROM class_sessions 
		WHERE class_key = $1 AND status = 'completed'
	`, lj.ClassKey).Scan(&currentSession)
	if err != nil {
		return fmt.Errorf("failed to get current session: %w", err)
	}
	if currentSession > 2 {
		return fmt.Errorf("cannot undo late join: class has progressed beyond session 2 (current session: %d)", currentSession)
	}

	// 3. Delete N/A attendance records for that class/lead
	_, err = tx.Exec(`
		DELETE FROM attendance
		WHERE lead_id = $1
		  AND session_id IN (
			  SELECT id FROM class_sessions
			  WHERE class_key = $2
				AND session_number < $3
		  )
		  AND status = 'N/A'
	`, leadID, lj.ClassKey, lj.JoinedAtSessionNumber)
	if err != nil {
		return fmt.Errorf("failed to delete N/A attendance: %w", err)
	}

	// 4. Revert scheduling to previous snapshot
	_, err = tx.Exec(`
		UPDATE scheduling
		SET class_days = $1,
		    class_time = $2,
		    class_group_index = $3,
		    updated_at = NOW()
		WHERE lead_id = $4
	`, lj.PreviousClassDays, lj.PreviousClassTime, lj.PreviousClassGroupIndex, leadID)
	if err != nil {
		return fmt.Errorf("failed to revert scheduling: %w", err)
	}

	// 5. Update leads.status to 'ready_to_start' and reset sent_to_classes
	_, err = tx.Exec(`
		UPDATE leads
		SET status = 'ready_to_start', sent_to_classes = false, updated_at = NOW()
		WHERE id = $1
	`, leadID)
	if err != nil {
		return fmt.Errorf("failed to revert lead status: %w", err)
	}

	// 6. Delete late_joiner_notifications for this lead/class
	_, err = tx.Exec(`DELETE FROM late_joiner_notifications WHERE lead_id = $1 AND class_key = $2`, leadID, lj.ClassKey)
	if err != nil {
		log.Printf("WARNING: Failed to delete late joiner notifications on undo: %v", err)
	}

	// 7. Delete late_joiners record
	_, err = tx.Exec(`DELETE FROM late_joiners WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete audit record: %w", err)
	}

	return tx.Commit()
}

// StartClassRound starts the round for a class group: sets status to 'active' and creates 8 sessions
func StartClassRound(classKey string, startedByUserID uuid.UUID, startDate time.Time, startTime string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()

	assignment, err := GetMentorAssignment(classKey)
	if err != nil {
		return fmt.Errorf("failed to check mentor assignment: %w", err)
	}
	if assignment == nil {
		return fmt.Errorf("cannot start round: mentor not assigned")
	}

	// 1. Update class_groups round status
	res, err := tx.Exec(`
		UPDATE class_groups 
		SET round_status = 'active', 
			round_started_at = $1, 
			round_started_by = $2,
			updated_at = $3
		WHERE class_key = $4
	`, now, startedByUserID, now, classKey)
	if err != nil {
		return fmt.Errorf("failed to update class group status: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("class group not found: %s", classKey)
	}

	// 2. Create 8 sessions
	// Parse time ensuring HH:MM format
	parsedTime, err := time.Parse("15:04", startTime)
	if err != nil {
		// Try other formats if needed or just fail
		parsedTime, err = time.Parse("15:04:05", startTime)
		if err != nil {
			return fmt.Errorf("invalid time format %q: %v", startTime, err)
		}
	}
	formattedTime := parsedTime.Format("15:04")

	for i := 1; i <= 8; i++ {
		sessionDate := startDate.AddDate(0, 0, (i-1)*7)
		sessionID := uuid.New()

		_, err := tx.Exec(`
			INSERT INTO class_sessions (id, class_key, session_number, scheduled_date, scheduled_time, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::TIME, $6, $7, $8)
		`, sessionID, classKey, i, sessionDate, formattedTime, "scheduled", now, now)

		if err != nil {
			return fmt.Errorf("failed to create session %d: %w", i, err)
		}
	}

	// 3. Update lead status to in_classes for students in this class
	_, err = tx.Exec(`
		UPDATE leads l
		SET status = 'in_classes', updated_at = NOW()
		FROM scheduling s
		INNER JOIN placement_tests pt ON pt.lead_id = s.lead_id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE l.id = s.lead_id
		  AND cg.class_key = $1
		  AND l.status IN ('ready_to_start', 'schedule_assigned')
	`, classKey)
	if err != nil {
		return fmt.Errorf("failed to update lead status: %w", err)
	}

	return tx.Commit()
}

// GetAbsenceFeed returns attendance events for a class (ABSENT/LATE) along with follow-up info
func GetAbsenceFeed(classKey, filter, search string) ([]*AbsenceFeedItem, error) {
	query := `
		SELECT 
			s.session_number,
			s.scheduled_date,
			s.scheduled_time::TEXT,
			l.id,
			l.full_name,
			l.phone,
			a.status,
			COALESCE(u.email, 'unknown'),
			a.created_at,
			a.notes,
			f.id,
			f.status,
			f.note,
			f.updated_at,
			f.resolved,
			f.resolved_at,
			lj.joined_at_session_number
		FROM class_sessions s
		JOIN attendance a ON s.id = a.session_id
		JOIN leads l ON a.lead_id = l.id
		LEFT JOIN late_joiners lj ON l.id = lj.lead_id
		LEFT JOIN users u ON a.marked_by_user_id = u.id
		LEFT JOIN followups f ON f.class_key = s.class_key AND f.lead_id = l.id AND f.session_number = s.session_number AND f.deleted_at IS NULL
		WHERE s.class_key = $1 
		  AND a.status IN ('ABSENT', 'LATE')
		  AND (f.resolved IS NULL OR f.resolved = false)
		  AND (f.status IS NULL OR f.status != 'NO_RESPONSE')
	`
	args := []interface{}{classKey}
	argIdx := 2

	if filter != "" && filter != "all" {
		switch filter {
		case "unresolved":
			// Item is already filtered by base query, but let's keep it explicit if needed.
			// Actually, the base query now handles "unresolved" by default.
			// We can leave this as a no-op or remove it.
		case "absent":
			query += " AND a.status = 'ABSENT'"
		case "late":
			query += " AND a.status = 'LATE'"
		}
	}

	if search != "" {
		query += fmt.Sprintf(" AND (l.full_name ILIKE $%d OR l.phone ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+search+"%")
	}

	query += " ORDER BY s.session_number DESC, a.created_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query absence feed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []*AbsenceFeedItem{}
	for rows.Next() {
		item := &AbsenceFeedItem{}
		var fID sql.NullString
		var fStatus sql.NullString
		var fNote sql.NullString
		var fUpdatedAt sql.NullTime
		var fResolved sql.NullBool
		var fResolvedAt sql.NullTime
		var mNote sql.NullString
		var sDate time.Time
		var joinedAt sql.NullInt32

		err := rows.Scan(
			&item.SessionNumber,
			&sDate,
			&item.StartTime,
			&item.StudentID,
			&item.StudentName,
			&item.StudentPhone,
			&item.Status,
			&item.MarkedBy,
			&item.MarkedAt,
			&mNote,
			&fID,
			&fStatus,
			&fNote,
			&fUpdatedAt,
			&fResolved,
			&fResolvedAt,
			&joinedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan absence feed item: %w", err)
		}

		item.SessionDate = sDate.Format("2006-01-02")
		item.MentorNote = mNote.String
		if joinedAt.Valid {
			v := joinedAt.Int32
			item.JoinedAtSessionNumber = &v
		}

		if fID.Valid {
			fid, _ := uuid.Parse(fID.String)
			var resolvedAt *time.Time
			if fResolvedAt.Valid {
				t := fResolvedAt.Time
				resolvedAt = &t
			}
			item.FollowUp = &FollowUpInfo{
				ID:         fid,
				Status:     fStatus.String,
				LastNote:   fNote.String,
				UpdatedAt:  fUpdatedAt.Time,
				Resolved:   fResolved.Bool,
				ResolvedAt: resolvedAt,
			}
		}

		results = append(results, item)
	}

	return results, rows.Err()
}

// CreateFollowUp creates or updates a follow-up note
func CreateFollowUp(classKey string, leadID uuid.UUID, sessionNumber int, note string, status string, createdBy uuid.UUID) error {
	standardizedStatus := normalizeFollowUpStatus(status)
	_, err := db.DB.Exec(`
		INSERT INTO followups (class_key, lead_id, session_number, note, status, created_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (class_key, lead_id, session_number)
		DO UPDATE SET note = $4, status = $5, created_by = $6, updated_at = NOW()
		WHERE followups.deleted_at IS NULL
	`, classKey, leadID, sessionNumber, note, standardizedStatus, createdBy)
	return err
}

// ResolveFollowUp marks a follow-up as resolved
func ResolveFollowUp(id uuid.UUID, resolvedBy uuid.UUID) error {
	_, err := db.DB.Exec(`
		UPDATE followups 
		SET resolved = true, resolved_at = NOW(), resolved_by_user_id = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, resolvedBy, id)
	return err
}

// UpdateFollowUpStatus updates the status of a follow-up
func UpdateFollowUpStatus(id uuid.UUID, status string) error {
	standardizedStatus := normalizeFollowUpStatus(status)
	_, err := db.DB.Exec(`
		UPDATE followups SET status = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL
	`, standardizedStatus, id)
	return err
}

// UpdateFollowUp handles generic update of follow-up details
func UpdateFollowUp(id uuid.UUID, status, note string, resolved bool, userID uuid.UUID) error {
	var err error
	standardizedStatus := normalizeFollowUpStatus(status)
	if resolved {
		_, err = db.DB.Exec(`
			UPDATE followups 
			SET status = $1, note = $2, resolved = true, resolved_at = NOW(), resolved_by_user_id = $3, updated_at = NOW()
			WHERE id = $4 AND deleted_at IS NULL
		`, standardizedStatus, note, userID, id)
	} else {
		_, err = db.DB.Exec(`
			UPDATE followups 
			SET status = $1, note = $2, updated_at = NOW()
			WHERE id = $3 AND deleted_at IS NULL
		`, standardizedStatus, note, id)
	}
	return err
}

// normalizeFollowUpStatus maps UI values to DB enum values.
func normalizeFollowUpStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "none", "not_contacted":
		return "NOT_CONTACTED"
	case "contacted":
		return "CONTACTED"
	case "not_replied":
		return "NOT_REPLIED"
	case "no_response":
		return "NO_RESPONSE"
	case "resolved":
		return "RESOLVED"
	default:
		return strings.ToUpper(status)
	}
}

// AutoProgressFollowupStatuses automatically advances follow-up statuses based on the timeline:
// CONTACTED -> NOT_REPLIED (after 24h)
// NOT_REPLIED -> NO_RESPONSE (after 4 days / 96h)
func AutoProgressFollowupStatuses() error {
	now := time.Now()

	// 1. CONTACTED -> NOT_REPLIED (24h)
	_, err := db.DB.Exec(`
		UPDATE followups
		SET status = 'NOT_REPLIED', updated_at = CURRENT_TIMESTAMP
		WHERE status = 'CONTACTED' AND created_at < $1 AND deleted_at IS NULL
	`, now.Add(-24*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to progress CONTACTED to NOT_REPLIED: %w", err)
	}

	// 2. NOT_REPLIED -> NO_RESPONSE (4 days total)
	_, err = db.DB.Exec(`
		UPDATE followups
		SET status = 'NO_RESPONSE', updated_at = CURRENT_TIMESTAMP
		WHERE (status = 'NOT_REPLIED' OR status = 'CONTACTED') AND created_at < $1 AND deleted_at IS NULL
	`, now.Add(-96*time.Hour))
	if err != nil {
		return fmt.Errorf("failed to progress to NO_RESPONSE: %w", err)
	}

	return nil
}

// GetFollowUps returns follow-up records for a class, filtered by resolved status
func GetFollowUps(classKey string, resolved bool) ([]*FollowUpListItem, error) {
	rows, err := db.DB.Query(`
		SELECT 
			f.id, f.lead_id, l.full_name, l.phone, f.session_number, 
			a.status as attendance_status, f.note, f.status, f.created_at, f.resolved, f.resolved_at
		FROM followups f
		JOIN leads l ON f.lead_id = l.id
		LEFT JOIN class_sessions s ON s.class_key = f.class_key AND s.session_number = f.session_number
		LEFT JOIN attendance a ON a.session_id = s.id AND a.lead_id = f.lead_id
		WHERE f.class_key = $1 AND f.resolved = $2 AND f.status = 'NO_RESPONSE' AND f.deleted_at IS NULL
		ORDER BY f.created_at DESC
	`, classKey, resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to query follow-ups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []*FollowUpListItem{}
	for rows.Next() {
		item := &FollowUpListItem{}
		var note sql.NullString
		var resolvedAt sql.NullTime
		var attStatus sql.NullString
		if err := rows.Scan(
			&item.ID, &item.LeadID, &item.StudentName, &item.StudentPhone, &item.SessionNumber,
			&attStatus, &note, &item.Status, &item.CreatedAt, &item.Resolved, &resolvedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan follow-up: %w", err)
		}
		item.Note = note.String
		item.AttendanceStatus = attStatus.String
		if resolvedAt.Valid {
			item.ResolvedAt = &resolvedAt.Time
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

// ResolveAbsence marks an absence as resolved, creating a follow-up record if necessary
func ResolveAbsence(classKey string, leadID uuid.UUID, sessionNumber int, resolvedBy uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO followups (class_key, lead_id, session_number, note, status, created_by, updated_at, resolved, resolved_at, resolved_by_user_id)
		VALUES ($1, $2, $3, '', 'RESOLVED', $4, NOW(), true, NOW(), $4)
		ON CONFLICT (class_key, lead_id, session_number) 
		DO UPDATE SET resolved = true, resolved_at = NOW(), resolved_by_user_id = $4, updated_at = NOW()
		WHERE followups.deleted_at IS NULL
	`, classKey, leadID, sessionNumber, resolvedBy)
	return err
}

// ========== COMPLAINTS WORKFLOW ==========

// CreateComplaint creates a new complaint case (Student Success only)
func CreateComplaint(classKey, studentPhone, category, complaintText, urgency string, createdByUserID uuid.UUID) (*ComplaintCase, error) {
	var complaint ComplaintCase
	err := db.DB.QueryRow(`
		INSERT INTO followups (
			type, class_key, student_phone, category, complaint_text, urgency,
			status, created_by, session_number, note
		)
		VALUES ('complaint', $1, $2, $3, $4, $5, 'NOT_CONTACTED', $6, NULL, $4)
		RETURNING id, type, class_key, student_phone, category, complaint_text,
		          urgency, status, created_by, created_at, updated_at, resolved
	`, classKey, studentPhone, category, complaintText, urgency, createdByUserID).Scan(
		&complaint.ID, &complaint.Type, &complaint.ClassKey, &complaint.StudentPhone,
		&complaint.Category, &complaint.ComplaintText, &complaint.Urgency,
		&complaint.Status, &complaint.CreatedBy, &complaint.CreatedAt, &complaint.UpdatedAt,
		&complaint.Resolved,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create complaint: %w", err)
	}

	// Create initial note with the complaint text
	_ = CreateFollowUpNote(complaint.ID, "Initial Complaint: "+complaintText, "comment", createdByUserID)
	_ = CreateFollowUpNote(complaint.ID, "Complaint created by Student Success", "system", createdByUserID)

	return &complaint, nil
}

// GetFollowUpsWithComplaints returns all follow-ups (absences + complaints) for Student Success
func GetFollowUpsWithComplaints(classKey string, showResolved bool) ([]*FollowUpListItem, error) {
	query := `
		SELECT f.id, COALESCE(f.lead_id::text, ''), COALESCE(l.full_name, 'Unknown'),
		       COALESCE(l.phone, f.student_phone), COALESCE(f.session_number, 0), 
		       f.type, COALESCE(f.category, ''), COALESCE(f.urgency, ''),
		       f.status, COALESCE(f.note, f.complaint_text, '') as note,
		       f.created_at, f.resolved, f.resolved_at
		FROM followups f
		LEFT JOIN leads l ON f.lead_id = l.id
		WHERE f.class_key = $1 AND f.deleted_at IS NULL
	`

	if !showResolved {
		query += " AND f.resolved = false"
	}

	query += " ORDER BY f.created_at DESC"

	rows, err := db.DB.Query(query, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query follow-ups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []*FollowUpListItem{}
	for rows.Next() {
		item := &FollowUpListItem{}
		var leadIDStr sql.NullString
		var sessionNum sql.NullInt32
		var fType, category, urgency sql.NullString
		var note sql.NullString
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&item.ID, &leadIDStr, &item.StudentName, &item.StudentPhone, &sessionNum,
			&fType, &category, &urgency, &item.Status, &note, &item.CreatedAt,
			&item.Resolved, &resolvedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan follow-up: %w", err)
		}

		if leadIDStr.Valid {
			item.LeadID, _ = uuid.Parse(leadIDStr.String)
		}
		if sessionNum.Valid {
			item.SessionNumber = sessionNum.Int32
		}
		item.Note = note.String

		// Add type indicator to note for display
		if fType.Valid && fType.String == "complaint" {
			prefix := fmt.Sprintf("[COMPLAINT - %s/%s] ", category.String, urgency.String)
			item.Note = prefix + item.Note
		}

		if resolvedAt.Valid {
			item.ResolvedAt = &resolvedAt.Time
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

// GetComplaintsForMentorHead returns all complaint cases for Mentor Head
func GetComplaintsForMentorHead(showResolved bool) ([]*ComplaintListItem, error) {
	query := `
		SELECT f.id, f.class_key, COALESCE(l.full_name, 'Unknown') as student_name,
		       f.student_phone, f.category, f.urgency, f.status,
		       COALESCE(f.complaint_text, '') as complaint_text,
		       f.created_at, f.resolved, f.resolved_at
		FROM followups f
		LEFT JOIN leads l ON f.lead_id = l.id
		WHERE f.type = 'complaint' AND f.deleted_at IS NULL
	`

	if !showResolved {
		query += " AND f.resolved = false"
	}

	query += " ORDER BY f.created_at DESC"

	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query complaints: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []*ComplaintListItem{}
	for rows.Next() {
		item := &ComplaintListItem{}
		var resolvedAt sql.NullTime

		err := rows.Scan(
			&item.ID, &item.ClassKey, &item.StudentName, &item.StudentPhone,
			&item.Category, &item.Urgency, &item.Status, &item.ComplaintText,
			&item.CreatedAt, &item.Resolved, &resolvedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan complaint: %w", err)
		}
		item.LastNote = item.ComplaintText

		if resolvedAt.Valid {
			item.ResolvedAt = &resolvedAt.Time
		}

		// Fetch all notes for history
		notes, err := GetFollowUpNotes(item.ID)
		if err == nil {
			item.Notes = notes
			if len(notes) > 0 {
				item.LastNote = notes[0].NoteText
			}
		}

		results = append(results, item)
	}
	return results, rows.Err()
}

// UpdateComplaintStatus updates the status of a complaint and adds a note
func UpdateComplaintStatus(id uuid.UUID, status, note string, userID uuid.UUID) error {
	_, err := db.DB.Exec(`
		UPDATE followups SET status = $1, updated_at = NOW() WHERE id = $2 AND type = 'complaint'
	`, strings.ToUpper(status), id)

	if err != nil {
		return fmt.Errorf("failed to update complaint status: %w", err)
	}

	// Add status change note
	statusNote := "Status changed to: " + status
	if note != "" {
		statusNote += "\nNote: " + note
	}

	return CreateFollowUpNote(id, statusNote, "status_change", userID)
}

// ResolveComplaint resolves a complaint with a required resolution note
func ResolveComplaint(id uuid.UUID, resolutionNote string, userID uuid.UUID) error {
	if resolutionNote == "" {
		return fmt.Errorf("resolution note is required")
	}

	_, err := db.DB.Exec(`
		UPDATE followups
		SET status = 'RESOLVED', resolved = true, resolved_at = NOW(),
		    resolved_by_user_id = $1, updated_at = NOW()
		WHERE id = $2 AND type = 'complaint'
	`, userID, id)

	if err != nil {
		return fmt.Errorf("failed to resolve complaint: %w", err)
	}

	return CreateFollowUpNote(id, resolutionNote, "resolution", userID)
}

// SoftDeleteComplaint marks a complaint as deleted (Manager-only)
func SoftDeleteComplaint(id uuid.UUID, reason string, userID uuid.UUID) error {
	if reason == "" {
		return fmt.Errorf("delete reason is required")
	}

	result, err := db.DB.Exec(`
		UPDATE followups
		SET deleted_at = NOW(), deleted_by_user_id = $1, delete_reason = $2
		WHERE id = $3 AND type = 'complaint'
	`, userID, reason, id)

	if err != nil {
		return fmt.Errorf("failed to delete complaint: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("complaint not found or already deleted")
	}

	return nil
}

// CreateFollowUpNote adds a note to the audit trail for a follow-up case
func CreateFollowUpNote(caseID uuid.UUID, noteText, noteType string, userID uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO followup_case_notes (case_id, note_text, note_type, created_by_user_id)
		VALUES ($1, $2, $3, $4)
	`, caseID, noteText, noteType, userID)

	return err
}

// GetFollowUpNotes returns all notes for a follow-up case
func GetFollowUpNotes(caseID uuid.UUID) ([]*FollowUpCaseNote, error) {
	rows, err := db.DB.Query(`
		SELECT fcn.id, fcn.case_id, fcn.note_text, fcn.note_type, fcn.created_at,
		       fcn.created_by_user_id, COALESCE(u.email, '') as created_by_email
		FROM followup_case_notes fcn
		LEFT JOIN users u ON u.id = fcn.created_by_user_id
		WHERE fcn.case_id = $1
		ORDER BY fcn.created_at DESC
	`, caseID)

	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	notes := []*FollowUpCaseNote{}
	for rows.Next() {
		note := &FollowUpCaseNote{}
		err := rows.Scan(
			&note.ID, &note.CaseID, &note.NoteText, &note.NoteType,
			&note.CreatedAt, &note.CreatedByUserID, &note.CreatedByEmail,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}
		notes = append(notes, note)
	}

	return notes, rows.Err()
}

func gradeMirrorNoteID(gradeID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("grade-note:"+gradeID.String()))
}

func syncGradeNoteMirror(gradeID uuid.UUID, leadID uuid.UUID, classKey string, notes string, createdByUserID uuid.UUID) error {
	noteID := gradeMirrorNoteID(gradeID)
	trimmed := strings.TrimSpace(notes)
	if trimmed == "" {
		_, err := db.DB.Exec(`DELETE FROM student_notes WHERE id = $1`, noteID)
		if err != nil {
			return fmt.Errorf("failed to delete mirrored grade note: %w", err)
		}
		return nil
	}

	_, err := db.DB.Exec(`
		INSERT INTO student_notes (id, lead_id, class_key, session_number, note_text, is_private, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, 8, $4, false, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET
			lead_id = EXCLUDED.lead_id,
			class_key = EXCLUDED.class_key,
			session_number = EXCLUDED.session_number,
			note_text = EXCLUDED.note_text,
			is_private = false,
			created_by_user_id = EXCLUDED.created_by_user_id,
			updated_at = CURRENT_TIMESTAMP
	`, noteID, leadID, classKey, trimmed, createdByUserID)
	if err != nil {
		return fmt.Errorf("failed to upsert mirrored grade note: %w", err)
	}
	return nil
}

// InsertGrade creates a new grade record (session 8 only)
func InsertGrade(leadID uuid.UUID, classKey string, grade string, notes string, createdByUserID uuid.UUID) (uuid.UUID, error) {
	gradeID := uuid.New()
	var actualGradeID uuid.UUID

	err := db.DB.QueryRow(`
		INSERT INTO grades (id, lead_id, class_key, session_number, grade, notes, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, 8, $4, $5, $6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (lead_id, class_key, session_number) DO UPDATE SET
			grade = EXCLUDED.grade,
			notes = EXCLUDED.notes,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, gradeID, leadID, classKey, grade, notes, createdByUserID).Scan(&actualGradeID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert grade: %w", err)
	}

	if err := syncGradeNoteMirror(actualGradeID, leadID, classKey, notes, createdByUserID); err != nil {
		return uuid.Nil, err
	}

	return actualGradeID, nil
}

// DeleteGrade removes a grade record for session 8
func DeleteGrade(leadID uuid.UUID, classKey string) error {
	var deletedGradeID uuid.UUID
	err := db.DB.QueryRow(`
		DELETE FROM grades
		WHERE lead_id = $1 AND class_key = $2 AND session_number = 8
		RETURNING id
	`, leadID, classKey).Scan(&deletedGradeID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete grade: %w", err)
	}

	if _, err := db.DB.Exec(`DELETE FROM student_notes WHERE id = $1`, gradeMirrorNoteID(deletedGradeID)); err != nil {
		return fmt.Errorf("failed to delete mirrored grade note: %w", err)
	}
	return nil
}

// UpdateGrade updates an existing grade record
func UpdateGrade(gradeID uuid.UUID, grade string, notes string, updatedByUserID uuid.UUID) error {
	var leadID uuid.UUID
	var classKey string
	err := db.DB.QueryRow(`
		UPDATE grades
		SET grade = $1, notes = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3
		RETURNING lead_id, class_key
	`, grade, notes, gradeID).Scan(&leadID, &classKey)

	if err != nil {
		return fmt.Errorf("failed to update grade: %w", err)
	}

	if err := syncGradeNoteMirror(gradeID, leadID, classKey, notes, updatedByUserID); err != nil {
		return err
	}

	return nil
}

// GetGradesByClassKey retrieves all grades for a specific class
func GetGradesByClassKey(classKey string) ([]Grade, error) {
	rows, err := db.DB.Query(`
		SELECT g.id, g.lead_id, g.class_key, g.session_number, g.grade, 
		       COALESCE(g.notes, ''), COALESCE(g.created_by_user_id::text, ''), 
		       g.created_at, g.updated_at
		FROM grades g
		WHERE g.class_key = $1
		ORDER BY g.lead_id
	`, classKey)

	if err != nil {
		return nil, fmt.Errorf("failed to query grades: %w", err)
	}
	defer func() { _ = rows.Close() }()

	grades := []Grade{}
	for rows.Next() {
		var g Grade
		var notesStr string
		var createdByStr string

		if err := rows.Scan(&g.ID, &g.LeadID, &g.ClassKey, &g.SessionNumber, &g.Grade,
			&notesStr, &createdByStr, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan grade: %w", err)
		}

		if notesStr != "" {
			g.Notes = sql.NullString{String: notesStr, Valid: true}
		}
		if createdByStr != "" {
			g.CreatedByUserID = sql.NullString{String: createdByStr, Valid: true}
		}

		grades = append(grades, g)
	}

	return grades, rows.Err()
}

// GetGradeByLeadAndClass retrieves a specific grade for a student in a class
func GetGradeByLeadAndClass(leadID uuid.UUID, classKey string) (*Grade, error) {
	var g Grade
	var notesStr string
	var createdByStr string

	err := db.DB.QueryRow(`
		SELECT id, lead_id, class_key, session_number, grade, 
		       COALESCE(notes, ''), COALESCE(created_by_user_id::text, ''),
		       created_at, updated_at
		FROM grades
		WHERE lead_id = $1 AND class_key = $2
	`, leadID, classKey).Scan(&g.ID, &g.LeadID, &g.ClassKey, &g.SessionNumber, &g.Grade,
		&notesStr, &createdByStr, &g.CreatedAt, &g.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get grade: %w", err)
	}

	if notesStr != "" {
		g.Notes = sql.NullString{String: notesStr, Valid: true}
	}
	if createdByStr != "" {
		g.CreatedByUserID = sql.NullString{String: createdByStr, Valid: true}
	}

	return &g, nil
}

// UpdateAbsencePriority checks if a student has 3+ absences in their class and flags them as high priority
func UpdateAbsencePriority(leadID uuid.UUID, classKey string) error {
	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM attendance a
		JOIN class_sessions s ON a.session_id = s.id
		WHERE a.lead_id = $1 AND s.class_key = $2 AND a.status = 'ABSENT'
	`, leadID, classKey).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count absences: %w", err)
	}

	highPriority := count >= 3
	reason := ""
	if highPriority {
		reason = "Student has missed 3+ sessions in current level"
	}

	_, err = db.DB.Exec(`
		UPDATE leads
		SET high_priority = $1, high_priority_reason = $2
		WHERE id = $3
	`, highPriority, reason, leadID)

	return err
}

// LatestReadyDailyReportWindow returns the latest report date whose banner is ready.
// Daily reports become ready at 02:00 Europe/Helsinki on the next day.
func LatestReadyDailyReportWindow(now time.Time) (time.Time, time.Time) {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		loc = time.Local
	}
	hel := now.In(loc)
	today := time.Date(hel.Year(), hel.Month(), hel.Day(), 0, 0, 0, 0, loc)
	reportDate := today.AddDate(0, 0, -1)
	readyAt := time.Date(hel.Year(), hel.Month(), hel.Day(), 2, 0, 0, 0, loc)
	if hel.Before(readyAt) {
		reportDate = reportDate.AddDate(0, 0, -1)
		readyAt = readyAt.AddDate(0, 0, -1)
	}
	return reportDate, readyAt
}

// GetDailyReportPayload builds the Mentor Head/Manager daily report for a date.
func GetDailyReportPayload(inputDate time.Time) (*DailyReportPayload, error) {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		loc = time.Local
	}
	normalizedDate := time.Date(inputDate.Year(), inputDate.Month(), inputDate.Day(), 0, 0, 0, 0, loc)
	readyAt := normalizedDate.AddDate(0, 0, 1).Add(2 * time.Hour)

	rows, err := db.DB.Query(`
		SELECT cs.id, cs.class_key, cs.session_number, cs.scheduled_date,
		       COALESCE(cs.scheduled_time::TEXT, '') AS scheduled_time,
		       cs.actual_date,
		       COALESCE(cs.actual_time::TEXT, '') AS actual_time,
		       cs.status,
		       ma.mentor_user_id::TEXT,
		       COALESCE(u.email, 'Unassigned') AS mentor_email,
		       cg.level, cg.class_days, cg.class_time, cg.class_number
		FROM class_sessions cs
		INNER JOIN class_groups cg ON cg.class_key = cs.class_key
		LEFT JOIN mentor_assignments ma ON ma.class_key = cs.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		WHERE cs.scheduled_date = $1
		  AND COALESCE(cg.round_status, 'not_started') = 'active'
		ORDER BY cs.scheduled_time, cg.level, cg.class_number, cs.class_key
	`, normalizedDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to query daily report sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	report := &DailyReportPayload{
		ReportDate:  normalizedDate.Format("2006-01-02"),
		ReadyAt:     readyAt.Format(time.RFC3339),
		GeneratedAt: time.Now().Format(time.RFC3339),
		ClassRows:   []*DailyReportClassRow{},
	}

	for rows.Next() {
		var scheduledDate time.Time
		var actualDate sql.NullTime
		var mentorIDStr sql.NullString
		var level, classNumber int32
		var classDays, classTime string
		row := &DailyReportClassRow{}

		if err := rows.Scan(
			&row.SessionID,
			&row.ClassKey,
			&row.SessionNumber,
			&scheduledDate,
			&row.ScheduledTime,
			&actualDate,
			&row.ActualTime,
			&row.SessionStatus,
			&mentorIDStr,
			&row.MentorEmail,
			&level,
			&classDays,
			&classTime,
			&classNumber,
		); err != nil {
			return nil, fmt.Errorf("failed to scan daily report session: %w", err)
		}

		row.ScheduledDate = scheduledDate.Format("2006-01-02")
		row.ClassLabel = fmt.Sprintf("Level %d · %s · %s · Class %d", level, classDays, classTime, classNumber)
		if mentorIDStr.Valid && mentorIDStr.String != "" {
			if mentorID, err := uuid.Parse(mentorIDStr.String); err == nil {
				row.MentorID = mentorID
			}
		}

		expected, err := countExpectedStudentsForSession(row.ClassKey, row.SessionNumber)
		if err != nil {
			return nil, err
		}
		absent, err := countAbsentStudentsForSession(row.SessionID)
		if err != nil {
			return nil, err
		}
		row.ExpectedStudents = expected
		row.AbsentStudents = absent

		row.ReportStatus = "missing"
		row.PunctualityStatus = "not_filled"
		if strings.EqualFold(row.SessionStatus, "completed") {
			row.ReportStatus = "filled"
			row.DelayMinutes = computeDailyReportDelayMinutes(row.ScheduledDate, row.ScheduledTime, actualDate, row.ActualTime)
			if row.DelayMinutes > 0 {
				row.PunctualityStatus = "late"
			} else {
				row.PunctualityStatus = "on_time"
			}
			report.ClassesTaught++
		} else {
			report.ClassesMissingReport++
		}

		report.ClassesScheduled++
		report.ExpectedStudents += row.ExpectedStudents
		report.AbsentStudents += row.AbsentStudents
		report.ClassRows = append(report.ClassRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate daily report sessions: %w", err)
	}

	return report, nil
}

func countExpectedStudentsForSession(classKey string, sessionNumber int32) (int, error) {
	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE cg.class_key = $1
		  AND l.status = 'in_classes'
		  AND NOT EXISTS (
			  SELECT 1 FROM late_joiners lj
			  WHERE lj.lead_id = l.id
			    AND lj.class_key = cg.class_key
			    AND $2 < lj.joined_at_session_number
		  )
	`, classKey, sessionNumber).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count expected students for %s S%d: %w", classKey, sessionNumber, err)
	}
	return count, nil
}

func countAbsentStudentsForSession(sessionID uuid.UUID) (int, error) {
	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM attendance
		WHERE session_id = $1
		  AND status = 'ABSENT'
	`, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count absent students for session %s: %w", sessionID, err)
	}
	return count, nil
}

func computeDailyReportDelayMinutes(scheduledDate, scheduledTime string, actualDate sql.NullTime, actualTime string) int {
	if strings.TrimSpace(scheduledDate) == "" || strings.TrimSpace(scheduledTime) == "" || strings.TrimSpace(actualTime) == "" {
		return 0
	}
	loc, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		loc = time.Local
	}
	scheduledDay, err := time.ParseInLocation("2006-01-02", scheduledDate, loc)
	if err != nil {
		return 0
	}
	scheduledClock, err := parseSessionClock(scheduledTime)
	if err != nil {
		return 0
	}
	actualClock, err := parseSessionClock(actualTime)
	if err != nil {
		return 0
	}
	actualDay := scheduledDay
	if actualDate.Valid {
		y, m, d := actualDate.Time.In(loc).Date()
		actualDay = time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	scheduledAt := time.Date(scheduledDay.Year(), scheduledDay.Month(), scheduledDay.Day(), scheduledClock.Hour(), scheduledClock.Minute(), 0, 0, loc)
	actualAt := time.Date(actualDay.Year(), actualDay.Month(), actualDay.Day(), actualClock.Hour(), actualClock.Minute(), 0, 0, loc)
	delay := int(actualAt.Sub(scheduledAt).Minutes())
	if delay < 0 {
		return 0
	}
	return delay
}

// MarkDailyReportRead marks a ready daily report banner as read for a user.
func MarkDailyReportRead(userID uuid.UUID, reportDate time.Time) error {
	_, err := db.DB.Exec(`
		INSERT INTO daily_report_reads (user_id, report_date, read_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id, report_date) DO UPDATE SET read_at = EXCLUDED.read_at
	`, userID, reportDate.Format("2006-01-02"))
	if err != nil {
		return fmt.Errorf("failed to mark daily report read: %w", err)
	}
	return nil
}

// MarkComplaintRead marks a complaint banner as read for a user.
func MarkComplaintRead(userID uuid.UUID, complaintID uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO complaint_reads (user_id, complaint_id, read_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id, complaint_id) DO UPDATE SET read_at = EXCLUDED.read_at
	`, userID, complaintID)
	if err != nil {
		return fmt.Errorf("failed to mark complaint read: %w", err)
	}
	return nil
}

// GetOpsNotificationSummary returns unread daily-report and complaint banners for MH/Manager.
func GetOpsNotificationSummary(userID uuid.UUID, now time.Time) (*OpsNotificationSummary, error) {
	summary := &OpsNotificationSummary{}
	reportDate, readyAt := LatestReadyDailyReportWindow(now)

	var reportRead bool
	if err := db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM daily_report_reads
			WHERE user_id = $1 AND report_date = $2
		)
	`, userID, reportDate.Format("2006-01-02")).Scan(&reportRead); err != nil {
		return nil, fmt.Errorf("failed to check daily report read state: %w", err)
	}

	if !reportRead {
		report, err := GetDailyReportPayload(reportDate)
		if err != nil {
			return nil, err
		}
		if report.ClassesScheduled > 0 {
			summary.DailyReport = &DailyReportNotification{
				ReportDate:           report.ReportDate,
				ReadyAt:              readyAt.Format(time.RFC3339),
				ClassesScheduled:     report.ClassesScheduled,
				ClassesTaught:        report.ClassesTaught,
				ClassesMissingReport: report.ClassesMissingReport,
				AbsentStudents:       report.AbsentStudents,
				ExpectedStudents:     report.ExpectedStudents,
			}
		}
	}

	complaint, err := GetUnreadComplaintNotification(userID)
	if err != nil {
		return nil, err
	}
	summary.Complaint = complaint
	return summary, nil
}

// GetUnreadComplaintNotification returns the newest unread active complaint for a user.
func GetUnreadComplaintNotification(userID uuid.UUID) (*ComplaintNotification, error) {
	var unreadCount int
	if err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM followups f
		LEFT JOIN complaint_reads cr ON cr.complaint_id = f.id AND cr.user_id = $1
		WHERE f.type = 'complaint'
		  AND f.deleted_at IS NULL
		  AND f.resolved = false
		  AND cr.id IS NULL
	`, userID).Scan(&unreadCount); err != nil {
		return nil, fmt.Errorf("failed to count unread complaints: %w", err)
	}
	if unreadCount == 0 {
		return nil, nil
	}

	n := &ComplaintNotification{UnreadCount: unreadCount}
	err := db.DB.QueryRow(`
		SELECT f.id, f.class_key, COALESCE(l.full_name, 'Unknown') AS student_name,
		       COALESCE(f.student_phone, ''), COALESCE(f.urgency, 'medium'), f.created_at
		FROM followups f
		LEFT JOIN leads l ON l.id = f.lead_id
		LEFT JOIN complaint_reads cr ON cr.complaint_id = f.id AND cr.user_id = $1
		WHERE f.type = 'complaint'
		  AND f.deleted_at IS NULL
		  AND f.resolved = false
		  AND cr.id IS NULL
		ORDER BY f.created_at DESC
		LIMIT 1
	`, userID).Scan(&n.ID, &n.ClassKey, &n.StudentName, &n.StudentPhone, &n.Urgency, &n.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to query unread complaint notification: %w", err)
	}
	return n, nil
}

// LateJoinerNotification represents a notification for a late joiner event.
type LateJoinerNotification struct {
	ID                    uuid.UUID  `json:"id"`
	LeadID                uuid.UUID  `json:"lead_id"`
	FullName              string     `json:"full_name"`
	ClassKey              string     `json:"class_key"`
	JoinedAtSessionNumber int32      `json:"joined_at_session_number"`
	CreatedAt             time.Time  `json:"created_at"`
	AcknowledgedAt        *time.Time `json:"acknowledged_at,omitempty"`
}

// GetPendingLateJoinerNotifications returns unacknowledged notifications for a user.
func GetPendingLateJoinerNotifications(userID uuid.UUID) ([]*LateJoinerNotification, error) {
	query := `
		SELECT n.id, n.lead_id, l.full_name, n.class_key, n.joined_at_session_number, n.created_at
		FROM late_joiner_notifications n
		INNER JOIN leads l ON l.id = n.lead_id
		INNER JOIN class_groups cg ON cg.class_key = n.class_key
		INNER JOIN users u ON u.id = n.user_id
		WHERE n.user_id = $1 AND n.acknowledged_at IS NULL
		  AND cg.sent_to_mentor = true
		  AND (
			COALESCE(cg.round_status, 'not_started') = 'active'
			OR (
			  COALESCE(cg.round_status, 'not_started') = 'not_started'
			  AND u.role = 'mentor_head'
			)
		  )
		ORDER BY n.created_at DESC
	`
	rows, err := db.DB.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer func() { _ = rows.Close() }()

	notifications := []*LateJoinerNotification{}
	for rows.Next() {
		n := &LateJoinerNotification{}
		err := rows.Scan(&n.ID, &n.LeadID, &n.FullName, &n.ClassKey, &n.JoinedAtSessionNumber, &n.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		notifications = append(notifications, n)
	}
	return notifications, nil
}

// AcknowledgeLateJoinerNotification marks a notification as acknowledged.
func AcknowledgeLateJoinerNotification(notificationID uuid.UUID, userID uuid.UUID) error {
	_, err := db.DB.Exec(`
		UPDATE late_joiner_notifications 
		SET acknowledged_at = NOW() 
		WHERE id = $1 AND user_id = $2
	`, notificationID, userID)
	if err != nil {
		return fmt.Errorf("failed to acknowledge notification: %w", err)
	}
	return nil
}
