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

func ValidateSuggestedStartDateNotPast(d time.Time) error {
	if util.CairoStartOfDay(d).Before(util.CairoStartOfDay(util.CairoNow())) {
		return fmt.Errorf("suggested start date cannot be in the past")
	}
	return nil
}

func ValidateSuggestedStartDateForClassDays(classDays string, d time.Time) error {
	allowedWeekdays, ok := allowedRoundStartWeekdays(classDays)
	if !ok {
		return nil
	}
	if !containsWeekday(allowedWeekdays, util.CairoStartOfDay(d).Weekday()) {
		return fmt.Errorf("suggested start date must be %s for %s classes", weekdayListLabel(allowedWeekdays), classDays)
	}
	return nil
}

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
		"paused":            StageWaitingForRound,
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

	// 3. If offer final price exists (including 0 for a fully discounted offer) -> at least OFFER_SENT
	if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
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
		StageWaitingForRound:          "Batch students into class",
		StageColdLead:                 "Retarget lead",
	}
	if action, ok := actions[stage]; ok {
		return action
	}
	return "Review"
}

func computeOfferSentFollowUp(daysSince int) (int, string, bool) {
	switch {
	case daysSince < 1:
		return 0, "Await reply", false
	case daysSince < 3:
		return 1, "Send Message 1", true
	case daysSince < 5:
		return 2, "Send Message 2", true
	case daysSince < 7:
		return 3, "Send Message 3", true
	default:
		return 0, "Review for cold lead", true
	}
}

func computeOfferSentFollowUpState(anchor time.Time, lastMessageNumber int32, lastMessageSentAt sql.NullTime, reminderAt sql.NullTime, now time.Time) (int, time.Time, bool, string, bool, bool) {
	if reminderAt.Valid {
		reminderDue := !now.Before(reminderAt.Time)
		if reminderDue {
			return 0, reminderAt.Time, true, "Offer reminder due", true, true
		}
		return 0, reminderAt.Time, false, "Offer reminder set", false, true
	}

	switch lastMessageNumber {
	case 0:
		message1At := anchor.Add(24 * time.Hour)
		message2At := anchor.Add(72 * time.Hour)
		message3At := anchor.Add(120 * time.Hour)
		coldReviewAt := anchor.Add(168 * time.Hour)
		switch {
		case now.Before(message1At):
			return 0, message1At, false, "Await reply", false, false
		case now.Before(message2At):
			return 1, message1At, true, "Send Message 1", true, false
		case now.Before(message3At):
			return 2, message2At, true, "Send Message 2", true, false
		case now.Before(coldReviewAt):
			return 3, message3At, true, "Send Message 3", true, false
		default:
			return 0, coldReviewAt, true, "Review for cold lead", true, false
		}
	case 1:
		if !lastMessageSentAt.Valid {
			return 0, time.Time{}, false, "Await reply", false, false
		}
		dueAt := lastMessageSentAt.Time.Add(48 * time.Hour)
		if now.Before(dueAt) {
			return 2, dueAt, false, "Await reply", false, false
		}
		return 2, dueAt, true, "Send Message 2", true, false
	case 2:
		if !lastMessageSentAt.Valid {
			return 0, time.Time{}, false, "Await reply", false, false
		}
		dueAt := lastMessageSentAt.Time.Add(48 * time.Hour)
		if now.Before(dueAt) {
			return 3, dueAt, false, "Await reply", false, false
		}
		return 3, dueAt, true, "Send Message 3", true, false
	default:
		if !lastMessageSentAt.Valid {
			return 0, time.Time{}, true, "Review for cold lead", true, false
		}
		dueAt := lastMessageSentAt.Time.Add(48 * time.Hour)
		return 0, dueAt, !now.Before(dueAt), "Review for cold lead", !now.Before(dueAt), false
	}
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

	// HotLevel by days: 0–6 HOT, 7–13 WARM, 14+ COOL (just-tested leads are HOT)
	if daysSince <= 6 {
		item.HotLevel = "HOT"
	} else if daysSince <= 13 {
		item.HotLevel = "WARM"
	} else {
		item.HotLevel = "COOL"
	}

	// TESTED and OFFER_SENT follow different follow-up playbooks.
	if stage == StageOfferSent {
		if item.Lead.IsReturning {
			item.OfferFollowUpStep = 0
			item.OfferFollowUpDueNow = false
			item.OfferFollowUpDueAt = sql.NullTime{}
			if item.OfferReminderAt.Valid {
				reminderDue := !now.Before(item.OfferReminderAt.Time)
				item.OfferReminderDue = reminderDue
				item.FollowUpDue = reminderDue
				if reminderDue {
					item.NextAction = "Offer reminder due"
				} else {
					item.NextAction = "Offer reminder set"
				}
			} else {
				item.FollowUpDue = false
				item.NextAction = "Renewal follow-up needed"
			}
			return
		}
		anchor := progressTime
		nextStep, dueAt, dueNow, nextAction, actionable, reminderMode := computeOfferSentFollowUpState(anchor, int32(item.OfferFollowUpLastStep), item.OfferFollowUpLastSent, item.OfferReminderAt, now)
		item.OfferFollowUpStep = nextStep
		item.OfferFollowUpDueNow = dueNow
		item.OfferReminderDue = reminderMode && dueNow
		item.NextAction = nextAction
		item.FollowUpDue = actionable
		if !dueAt.IsZero() {
			item.OfferFollowUpDueAt = sql.NullTime{Time: dueAt, Valid: true}
		} else {
			item.OfferFollowUpDueAt = sql.NullTime{}
		}
		return
	}

	// All unpaid TESTED leads still require active follow-up.
	item.FollowUpDue = true
	if daysSince <= 6 {
		item.NextAction = "Follow-up due - Call today"
	} else if daysSince <= 13 {
		item.NextAction = "Follow-up due - Offer discount"
	} else {
		item.NextAction = "Follow-up due - Final check"
	}
}

func applyLeadSnoozeState(item *LeadListItem, now time.Time) {
	if item == nil || item.Lead == nil || !item.SnoozedUntil.Valid {
		return
	}
	if now.Before(item.SnoozedUntil.Time) {
		item.SnoozeDue = false
		return
	}
	item.SnoozeDue = true
	item.NextAction = "التذكير مستحق دلوقتي"
	item.FollowUpDue = true
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

func GetAllLeads(statusFilter, searchFilter, paymentFilter, hotFilter string, includeCancelled bool, followUpFilter string, returningFilter string, coldFilter string, repeatFilter string, opsQueueReasonFilter string) ([]*LeadListItem, error) {
	query := `
		SELECT 
			l.id, l.full_name, l.phone, l.source, l.notes, l.status, l.ops_queue_reason, l.mentor_head_return_reason, l.sent_to_classes,
			COALESCE(l.is_returning, false),
			GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS remaining_credits_calc,
			l.created_by_user_id, l.offer_sent_at, l.created_at, l.updated_at,
			pt.assigned_level, pt.test_date,
			s.class_days,
			TO_CHAR(s.class_time, 'HH24:MI') AS class_time,
			p.remaining_balance, p.amount_paid,
			o.final_price,
			ls.snoozed_until,
			ls.note,
			osr.follow_up_at,
			osr.note,
			COALESCE(last_offer_msg.message_number, 0) AS last_offer_message_number,
			last_offer_msg.sent_at,
			COALESCE(last_ce.level, 0) as last_completed_level,
			COALESCE(last_ce_attendance.attended_sessions, 0) as last_attended_sessions,
			COALESCE(last_ce.outcome, '') as last_outcome,
			COALESCE(last_ce.final_grade, '') as last_final_grade,
			COALESCE(last_refusal.refused_at, NULL) as refused_renewal_at,
			COALESCE(last_refusal.reason, '') as refused_renewal_reason,
			COALESCE(last_refusal.notes, '') as refused_renewal_notes,
			COALESCE(last_refused_msg.message_number, 0) as last_refused_message_number,
			last_refused_msg.sent_at
		FROM leads l
		LEFT JOIN placement_tests pt ON l.id = pt.lead_id
		LEFT JOIN scheduling s ON s.lead_id = l.id
		LEFT JOIN payments p ON l.id = p.lead_id
		LEFT JOIN offers o ON l.id = o.lead_id
		LEFT JOIN lead_snoozes ls ON ls.lead_id = l.id
		LEFT JOIN offer_sent_reminders osr ON osr.lead_id = l.id
		LEFT JOIN LATERAL (
			SELECT osf.message_number, osf.sent_at
			FROM offer_sent_follow_ups osf
			WHERE osf.lead_id = l.id
			ORDER BY osf.message_number DESC, osf.sent_at DESC
			LIMIT 1
		) last_offer_msg ON true
		LEFT JOIN LATERAL (
			SELECT class_key, level, outcome, final_grade
			FROM class_enrollments ce
			WHERE ce.lead_id = l.id
			ORDER BY COALESCE(ce.completed_at, ce.enrolled_at) DESC
			LIMIT 1
		) last_ce ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*) FILTER (WHERE a.status = 'PRESENT')::int AS attended_sessions
			FROM class_sessions cs
			LEFT JOIN attendance a ON a.session_id = cs.id AND a.lead_id = l.id
			WHERE cs.class_key = last_ce.class_key
		) last_ce_attendance ON true
		LEFT JOIN LATERAL (
			SELECT refused_at, reason, notes
			FROM renewal_refusals rr
			WHERE rr.lead_id = l.id
			ORDER BY rr.refused_at DESC
			LIMIT 1
		) last_refusal ON true
		LEFT JOIN LATERAL (
			SELECT rrf.message_number, rrf.sent_at
			FROM refused_renewal_follow_ups rrf
			WHERE rrf.lead_id = l.id
			ORDER BY rrf.message_number DESC, rrf.sent_at DESC
			LIMIT 1
		) last_refused_msg ON true
		WHERE 1=1
		AND l.status != 'in_classes'
		AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
		AND (ls.snoozed_until IS NULL OR ls.snoozed_until <= NOW())
		AND NOT (
			l.status = 'lead_created'
			AND COALESCE(l.ops_queue_reason, '') = ''
			AND COALESCE(pt.test_date::text, '') = ''
			AND COALESCE(pt.test_time::text, '') = ''
			AND l.created_at <= NOW() - INTERVAL '48 hours'
		)
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
		query += " AND (l.status = 'cold_lead' OR (l.status = 'offer_sent' AND osr.follow_up_at IS NULL AND COALESCE(l.offer_sent_at, l.updated_at) <= NOW() - INTERVAL '7 days' AND COALESCE(l.levels_purchased_total, 0) > 0 AND GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) <= 0))"
	} else if !strings.EqualFold(statusFilter, "cold_lead") && !strings.EqualFold(statusFilter, StageColdLead) {
		// Keep cold backlog isolated from default pipeline feed unless user explicitly enters cold mode.
		query += " AND l.status != 'cold_lead'"
	}

	// Apply repeat filter
	if repeatFilter == "1" || repeatFilter == "true" {
		query += " AND last_ce.outcome = 'repeated'"
	}

	if opsQueueReasonFilter != "" {
		query += fmt.Sprintf(" AND l.ops_queue_reason = $%d", argIndex)
		args = append(args, opsQueueReasonFilter)
		argIndex++
	} else {
		query += " AND COALESCE(l.ops_queue_reason, '') != 'private_track'"
	}

	// Waiting-list leads should stay out of the default arena feed.
	if !strings.EqualFold(statusFilter, "waiting_for_round") && !strings.EqualFold(statusFilter, StageWaitingForRound) {
		query += " AND l.status != 'waiting_for_round'"
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
		var classDays, classTime sql.NullString
		var snoozedUntil sql.NullTime
		var snoozeNote sql.NullString
		var offerReminderAt sql.NullTime
		var offerReminderNote sql.NullString
		var lastOfferMessageNumber int32
		var lastOfferMessageSentAt sql.NullTime
		var latestCompletedLevel int32
		var latestAttendedSessions int
		var lastOutcome sql.NullString
		var lastFinalGrade sql.NullString
		var refusedRenewalAt sql.NullTime
		var refusedRenewalReason string
		var refusedRenewalNotes string
		var lastRefusedMessageNumber int32
		var lastRefusedMessageSentAt sql.NullTime

		err := rows.Scan(
			&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status, &lead.OpsQueueReason, &lead.MentorHeadReturnReason, &lead.SentToClasses,
			&lead.IsReturning, &lead.RemainingCredits,
			&lead.CreatedByUserID, &lead.OfferSentAt, &lead.CreatedAt, &lead.UpdatedAt,
			&assignedLevel, &testDate,
			&classDays, &classTime,
			&remainingBalance, &amountPaid,
			&finalPrice,
			&snoozedUntil,
			&snoozeNote,
			&offerReminderAt,
			&offerReminderNote,
			&lastOfferMessageNumber,
			&lastOfferMessageSentAt,
			&latestCompletedLevel,
			&latestAttendedSessions,
			&lastOutcome,
			&lastFinalGrade,
			&refusedRenewalAt,
			&refusedRenewalReason,
			&refusedRenewalNotes,
			&lastRefusedMessageNumber,
			&lastRefusedMessageSentAt,
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
			Lead:                    lead,
			AssignedLevel:           assignedLevel,
			ClassDays:               classDays,
			ClassTime:               classTime,
			LatestCompletedLevel:    sql.NullInt32{Int32: int32(latestCompletedLevel), Valid: latestCompletedLevel > 0},
			LatestAttendedSessions:  latestAttendedSessions,
			LastOutcome:             lastOutcome,
			LastFinalGrade:          lastFinalGrade,
			RefusedRenewal:          refusedRenewalAt.Valid,
			RefusedRenewalAt:        refusedRenewalAt,
			RefusedFollowUpLastStep: int(lastRefusedMessageNumber),
			RefusedFollowUpLastSent: lastRefusedMessageSentAt,
			PaymentStatus:           GetPaymentStatus(remainingBalance, amountPaid),
			PaymentState:            paymentState,
			NextAction:              GetNextAction(lead.Status),
			TestDate:                testDate,
			AmountPaid:              amountPaid,
			FinalPrice:              finalPrice,
			RemainingBalance:        remainingBalance,
			OfferFollowUpLastStep:   int(lastOfferMessageNumber),
			OfferFollowUpLastSent:   lastOfferMessageSentAt,
			SnoozedUntil:            snoozedUntil,
			SnoozeNote:              snoozeNote,
			OfferReminderAt:         offerReminderAt,
			OfferReminderNote:       offerReminderNote,
		}
		if refusedRenewalReason != "" {
			item.RenewalRefusalReason = sql.NullString{String: refusedRenewalReason, Valid: true}
		}
		if refusedRenewalNotes != "" {
			item.RenewalRefusalNotes = sql.NullString{String: refusedRenewalNotes, Valid: true}
		}

		// Compute hot lead flags (needs finalPrice for proper payment state)
		ComputeLeadFlags(item)
		if refusedRenewalAt.Valid {
			nextStep, dueAt, dueNow, manualAvailable := ComputeRefusedRenewalFollowUpState(refusedRenewalAt.Time, int(lastRefusedMessageNumber), lastRefusedMessageSentAt, util.CairoNow())
			item.RefusedFollowUpStep = nextStep
			item.RefusedFollowUpDueNow = dueNow
			item.RefusedFollowUpManual = manualAvailable
			if !dueAt.IsZero() {
				item.RefusedFollowUpDueAt = sql.NullTime{Time: dueAt, Valid: true}
			}
		}
		applyLeadSnoozeState(item, util.CairoNow())

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

func computeSleepingLeadStep(createdAt time.Time, lastMessageNumber int32, lastMessageSentAt sql.NullTime, now time.Time) (int, time.Time, bool) {
	switch lastMessageNumber {
	case 0:
		dueAt := createdAt.Add(48 * time.Hour)
		return 1, dueAt, !now.Before(dueAt)
	case 1:
		if !lastMessageSentAt.Valid {
			return 0, time.Time{}, false
		}
		dueAt := lastMessageSentAt.Time.Add(72 * time.Hour)
		return 2, dueAt, !now.Before(dueAt)
	case 2:
		if !lastMessageSentAt.Valid {
			return 0, time.Time{}, false
		}
		dueAt := lastMessageSentAt.Time.Add(96 * time.Hour)
		return 3, dueAt, !now.Before(dueAt)
	default:
		return 0, time.Time{}, false
	}
}

func formatSleepingLeadReminderNextAction(reminderAt sql.NullTime, reminderDue bool) string {
	if !reminderAt.Valid {
		return ""
	}
	if reminderDue {
		return "Callback reminder due"
	}
	return "Callback scheduled"
}

func formatSleepingLeadNextAction(step int, dueAt time.Time, dueNow bool, lastMessageNumber int32) string {
	if step >= 1 && step <= 3 {
		return fmt.Sprintf("Message %d", step)
	}
	if lastMessageNumber >= 3 {
		return "Sequence completed"
	}
	return "Waiting"
}

func getSleepingLeads(extraCondition string, extraArgs ...interface{}) ([]*LeadListItem, error) {
	query := `
		SELECT
			l.id, l.full_name, l.phone, l.source, l.notes, l.status, l.ops_queue_reason, l.mentor_head_return_reason, l.sent_to_classes,
			COALESCE(l.is_returning, false),
			GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS remaining_credits_calc,
			l.created_by_user_id, l.offer_sent_at, l.created_at, l.updated_at,
			pt.assigned_level, pt.test_date,
			p.remaining_balance, p.amount_paid,
			o.final_price,
			ls.snoozed_until,
			ls.note,
			slr.follow_up_at,
			slr.note,
			COALESCE(last_msg.message_number, 0) AS last_message_number,
			last_msg.sent_at
		FROM leads l
		LEFT JOIN placement_tests pt ON pt.lead_id = l.id
		LEFT JOIN payments p ON p.lead_id = l.id
		LEFT JOIN offers o ON o.lead_id = l.id
		LEFT JOIN lead_snoozes ls ON ls.lead_id = l.id
		LEFT JOIN sleeping_lead_reminders slr ON slr.lead_id = l.id
		LEFT JOIN LATERAL (
			SELECT slf.message_number, slf.sent_at
			FROM sleeping_lead_follow_ups slf
			WHERE slf.lead_id = l.id
			ORDER BY slf.message_number DESC, slf.sent_at DESC
			LIMIT 1
		) last_msg ON true
		WHERE l.status = 'lead_created'
		  AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
		  AND COALESCE(l.ops_queue_reason, '') = ''
		  AND (ls.snoozed_until IS NULL OR ls.snoozed_until <= NOW())
		  AND COALESCE(pt.test_date::text, '') = ''
		  AND COALESCE(pt.test_time::text, '') = ''
		  AND (
		      slr.follow_up_at IS NOT NULL
		      OR
		      (slr.follow_up_at IS NULL AND l.created_at <= NOW() - INTERVAL '48 hours')
		  )
	`

	args := make([]interface{}, 0, len(extraArgs))
	if extraCondition != "" {
		query += extraCondition
		args = append(args, extraArgs...)
	}

	query += " ORDER BY l.created_at ASC, l.updated_at ASC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query sleeping leads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	now := util.CairoNow()
	items := make([]*LeadListItem, 0)
	for rows.Next() {
		lead := &Lead{}
		var assignedLevel sql.NullInt32
		var testDate sql.NullTime
		var remainingBalance sql.NullInt32
		var amountPaid sql.NullInt32
		var finalPrice sql.NullInt32
		var snoozedUntil sql.NullTime
		var snoozeNote sql.NullString
		var reminderAt sql.NullTime
		var reminderNote sql.NullString
		var lastMessageNumber int32
		var lastMessageSentAt sql.NullTime

		if err := rows.Scan(
			&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status, &lead.OpsQueueReason, &lead.MentorHeadReturnReason, &lead.SentToClasses,
			&lead.IsReturning, &lead.RemainingCredits,
			&lead.CreatedByUserID, &lead.OfferSentAt, &lead.CreatedAt, &lead.UpdatedAt,
			&assignedLevel, &testDate,
			&remainingBalance, &amountPaid,
			&finalPrice,
			&snoozedUntil,
			&snoozeNote,
			&reminderAt,
			&reminderNote,
			&lastMessageNumber,
			&lastMessageSentAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sleeping lead: %w", err)
		}

		reminderDue := reminderAt.Valid && !now.Before(reminderAt.Time)
		nextStep, dueAt, eligible := 0, time.Time{}, false
		nextAction := ""
		if reminderAt.Valid {
			dueAt = reminderAt.Time
			eligible = reminderDue
			nextAction = formatSleepingLeadReminderNextAction(reminderAt, reminderDue)
		} else {
			nextStep, dueAt, eligible = computeSleepingLeadStep(lead.CreatedAt, lastMessageNumber, lastMessageSentAt, now)
			nextAction = formatSleepingLeadNextAction(nextStep, dueAt, eligible, lastMessageNumber)
		}
		dueAtValue := sql.NullTime{}
		if !dueAt.IsZero() {
			dueAtValue = sql.NullTime{Time: dueAt, Valid: true}
		}

		item := &LeadListItem{
			Lead:                 lead,
			AssignedLevel:        assignedLevel,
			PaymentStatus:        GetPaymentStatus(remainingBalance, amountPaid),
			PaymentState:         GetPaymentState(amountPaid, finalPrice),
			NextAction:           nextAction,
			TestDate:             testDate,
			AmountPaid:           amountPaid,
			FinalPrice:           finalPrice,
			RemainingBalance:     remainingBalance,
			SnoozedUntil:         snoozedUntil,
			SnoozeNote:           snoozeNote,
			SleepingLeadStep:     nextStep,
			SleepingLeadLastStep: int(lastMessageNumber),
			SleepingLeadLastSent: lastMessageSentAt,
			SleepingLeadDueAt:    dueAtValue,
			SleepingLeadDueNow:   eligible,
			SleepingReminderAt:   reminderAt,
			SleepingReminderNote: reminderNote,
			SleepingReminderDue:  reminderDue,
		}
		applyLeadSnoozeState(item, now)

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating sleeping leads: %w", err)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].SleepingLeadDueNow != items[j].SleepingLeadDueNow {
			return items[i].SleepingLeadDueNow
		}
		iDeferredFuture := items[i].SleepingReminderAt.Valid && !items[i].SleepingReminderDue
		jDeferredFuture := items[j].SleepingReminderAt.Valid && !items[j].SleepingReminderDue
		if iDeferredFuture != jDeferredFuture {
			return !iDeferredFuture
		}
		if items[i].SleepingLeadDueAt.Valid && items[j].SleepingLeadDueAt.Valid && !items[i].SleepingLeadDueAt.Time.Equal(items[j].SleepingLeadDueAt.Time) {
			return items[i].SleepingLeadDueAt.Time.Before(items[j].SleepingLeadDueAt.Time)
		}
		return items[i].Lead.CreatedAt.Before(items[j].Lead.CreatedAt)
	})

	return items, nil
}

func GetSleepingLeads(searchFilter string) ([]*LeadListItem, error) {
	searchFilter = strings.TrimSpace(searchFilter)
	if searchFilter == "" {
		return getSleepingLeads("")
	}
	searchPattern := "%" + searchFilter + "%"
	return getSleepingLeads(" AND (LOWER(l.full_name) LIKE LOWER($1) OR l.phone LIKE $1)", searchPattern)
}

func GetSleepingLeadByID(leadID uuid.UUID) (*LeadListItem, error) {
	items, err := getSleepingLeads(" AND l.id = $1", leadID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items[0], nil
}

func GetLeadSnooze(leadID uuid.UUID) (*LeadSnooze, error) {
	snooze := &LeadSnooze{}
	err := db.DB.QueryRow(`
		SELECT lead_id, snoozed_until, note, scheduled_by_user_id::text, created_at, updated_at
		FROM lead_snoozes
		WHERE lead_id = $1
	`, leadID).Scan(
		&snooze.LeadID,
		&snooze.SnoozedUntil,
		&snooze.Note,
		&snooze.ScheduledByUserID,
		&snooze.CreatedAt,
		&snooze.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load lead snooze: %w", err)
	}
	return snooze, nil
}

func UpsertLeadSnooze(leadID uuid.UUID, snoozedUntil time.Time, note string, scheduledByUserID *uuid.UUID) error {
	var actor interface{}
	if scheduledByUserID != nil {
		actor = *scheduledByUserID
	}
	if _, err := db.DB.Exec(`
		INSERT INTO lead_snoozes (lead_id, snoozed_until, note, scheduled_by_user_id, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (lead_id) DO UPDATE
		SET snoozed_until = EXCLUDED.snoozed_until,
		    note = EXCLUDED.note,
		    scheduled_by_user_id = EXCLUDED.scheduled_by_user_id,
		    updated_at = CURRENT_TIMESTAMP
	`, leadID, snoozedUntil, strings.TrimSpace(note), actor); err != nil {
		return fmt.Errorf("failed to save lead snooze: %w", err)
	}
	return nil
}

func DeleteLeadSnooze(leadID uuid.UUID) error {
	if _, err := db.DB.Exec(`DELETE FROM lead_snoozes WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to clear lead snooze: %w", err)
	}
	return nil
}

func GetSnoozedLeads(searchFilter string) ([]*LeadListItem, error) {
	query := `
		SELECT
			l.id, l.full_name, l.phone, l.source, l.notes, l.status, l.ops_queue_reason, l.mentor_head_return_reason, l.sent_to_classes,
			COALESCE(l.is_returning, false),
			GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS remaining_credits_calc,
			l.created_by_user_id, l.offer_sent_at, l.created_at, l.updated_at,
			pt.assigned_level, pt.test_date,
			s.class_days,
			TO_CHAR(s.class_time, 'HH24:MI') AS class_time,
			p.remaining_balance, p.amount_paid,
			o.final_price,
			ls.snoozed_until,
			ls.note
		FROM lead_snoozes ls
		INNER JOIN leads l ON l.id = ls.lead_id
		LEFT JOIN placement_tests pt ON l.id = pt.lead_id
		LEFT JOIN scheduling s ON s.lead_id = l.id
		LEFT JOIN payments p ON l.id = p.lead_id
		LEFT JOIN offers o ON l.id = o.lead_id
		WHERE ls.snoozed_until > NOW()
	`

	args := []interface{}{}
	if trimmed := strings.TrimSpace(searchFilter); trimmed != "" {
		query += " AND (LOWER(l.full_name) LIKE LOWER($1) OR l.phone LIKE $1)"
		args = append(args, "%"+trimmed+"%")
	}
	query += " ORDER BY ls.snoozed_until ASC, l.updated_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query snoozed leads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*LeadListItem, 0)
	for rows.Next() {
		lead := &Lead{}
		var assignedLevel sql.NullInt32
		var testDate sql.NullTime
		var classDays, classTime sql.NullString
		var remainingBalance, amountPaid, finalPrice sql.NullInt32
		var snoozedUntil sql.NullTime
		var snoozeNote sql.NullString

		if err := rows.Scan(
			&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status, &lead.OpsQueueReason, &lead.MentorHeadReturnReason, &lead.SentToClasses,
			&lead.IsReturning, &lead.RemainingCredits,
			&lead.CreatedByUserID, &lead.OfferSentAt, &lead.CreatedAt, &lead.UpdatedAt,
			&assignedLevel, &testDate,
			&classDays, &classTime,
			&remainingBalance, &amountPaid,
			&finalPrice,
			&snoozedUntil,
			&snoozeNote,
		); err != nil {
			return nil, fmt.Errorf("failed to scan snoozed lead: %w", err)
		}

		paymentState := GetPaymentState(amountPaid, finalPrice)
		if lead.IsReturning && lead.RemainingCredits.Valid && lead.RemainingCredits.Int32 > 0 {
			switch lead.Status {
			case "waiting_for_round", "schedule_assigned", "ready_to_start":
				paymentState = PaymentStatePaidFull
			}
		}

		item := &LeadListItem{
			Lead:             lead,
			AssignedLevel:    assignedLevel,
			ClassDays:        classDays,
			ClassTime:        classTime,
			PaymentStatus:    GetPaymentStatus(remainingBalance, amountPaid),
			PaymentState:     paymentState,
			NextAction:       "مؤجل لحد معاد التذكير",
			TestDate:         testDate,
			AmountPaid:       amountPaid,
			FinalPrice:       finalPrice,
			RemainingBalance: remainingBalance,
			SnoozedUntil:     snoozedUntil,
			SnoozeNote:       snoozeNote,
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating snoozed leads: %w", err)
	}

	return items, nil
}

func RecordSleepingLeadFollowUp(leadID uuid.UUID, messageNumber int, sentByUserID uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO sleeping_lead_follow_ups (lead_id, message_number, sent_by_user_id, sent_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
	`, leadID, messageNumber, sentByUserID)
	if err != nil {
		return fmt.Errorf("failed to record sleeping lead follow-up: %w", err)
	}
	return nil
}

func GetSleepingLeadReminder(leadID uuid.UUID) (*SleepingLeadReminder, error) {
	reminder := &SleepingLeadReminder{}
	err := db.DB.QueryRow(`
		SELECT lead_id, follow_up_at, note, scheduled_by_user_id::text, created_at, updated_at
		FROM sleeping_lead_reminders
		WHERE lead_id = $1
	`, leadID).Scan(
		&reminder.LeadID,
		&reminder.FollowUpAt,
		&reminder.Note,
		&reminder.ScheduledByUserID,
		&reminder.CreatedAt,
		&reminder.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load sleeping lead reminder: %w", err)
	}
	return reminder, nil
}

func UpsertSleepingLeadReminder(leadID uuid.UUID, followUpAt time.Time, note string, scheduledByUserID *uuid.UUID) error {
	var actor interface{}
	if scheduledByUserID != nil {
		actor = *scheduledByUserID
	}
	if _, err := db.DB.Exec(`
		INSERT INTO sleeping_lead_reminders (lead_id, follow_up_at, note, scheduled_by_user_id, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (lead_id) DO UPDATE
		SET follow_up_at = EXCLUDED.follow_up_at,
		    note = EXCLUDED.note,
		    scheduled_by_user_id = EXCLUDED.scheduled_by_user_id,
		    updated_at = CURRENT_TIMESTAMP
	`, leadID, followUpAt, strings.TrimSpace(note), actor); err != nil {
		return fmt.Errorf("failed to save sleeping lead reminder: %w", err)
	}
	return nil
}

func DeleteSleepingLeadReminder(leadID uuid.UUID) error {
	if _, err := db.DB.Exec(`DELETE FROM sleeping_lead_reminders WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to clear sleeping lead reminder: %w", err)
	}
	return nil
}

func CountDueSleepingLeadReminders(now time.Time) (int, error) {
	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM sleeping_lead_reminders slr
		JOIN leads l ON l.id = slr.lead_id
		LEFT JOIN placement_tests pt ON pt.lead_id = l.id
		LEFT JOIN lead_snoozes ls ON ls.lead_id = l.id
		WHERE slr.follow_up_at <= $1
		  AND l.status = 'lead_created'
		  AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
		  AND COALESCE(l.ops_queue_reason, '') = ''
		  AND (ls.snoozed_until IS NULL OR ls.snoozed_until <= $1)
		  AND COALESCE(pt.test_date::text, '') = ''
		  AND COALESCE(pt.test_time::text, '') = ''
	`, now).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count due sleeping lead reminders: %w", err)
	}
	return count, nil
}

func GetOfferSentReminder(leadID uuid.UUID) (*OfferSentReminder, error) {
	reminder := &OfferSentReminder{}
	err := db.DB.QueryRow(`
		SELECT lead_id, follow_up_at, note, scheduled_by_user_id::text, created_at, updated_at
		FROM offer_sent_reminders
		WHERE lead_id = $1
	`, leadID).Scan(
		&reminder.LeadID,
		&reminder.FollowUpAt,
		&reminder.Note,
		&reminder.ScheduledByUserID,
		&reminder.CreatedAt,
		&reminder.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load offer reminder: %w", err)
	}
	return reminder, nil
}

func GetLatestOfferSentFollowUp(leadID uuid.UUID) (int, sql.NullTime, error) {
	var messageNumber int
	var sentAt sql.NullTime
	err := db.DB.QueryRow(`
		SELECT message_number, sent_at
		FROM offer_sent_follow_ups
		WHERE lead_id = $1
		ORDER BY message_number DESC, sent_at DESC
		LIMIT 1
	`, leadID).Scan(&messageNumber, &sentAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, sql.NullTime{}, nil
		}
		return 0, sql.NullTime{}, fmt.Errorf("failed to load latest offer follow-up: %w", err)
	}
	return messageNumber, sentAt, nil
}

func RecordOfferSentFollowUp(leadID uuid.UUID, messageNumber int, sentByUserID uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO offer_sent_follow_ups (lead_id, message_number, sent_by_user_id, sent_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
	`, leadID, messageNumber, sentByUserID)
	if err != nil {
		return fmt.Errorf("failed to record offer follow-up: %w", err)
	}
	return nil
}

func UpsertOfferSentReminder(leadID uuid.UUID, followUpAt time.Time, note string, scheduledByUserID *uuid.UUID) error {
	var actor interface{}
	if scheduledByUserID != nil {
		actor = *scheduledByUserID
	}
	if _, err := db.DB.Exec(`
		INSERT INTO offer_sent_reminders (lead_id, follow_up_at, note, scheduled_by_user_id, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (lead_id) DO UPDATE
		SET follow_up_at = EXCLUDED.follow_up_at,
		    note = EXCLUDED.note,
		    scheduled_by_user_id = EXCLUDED.scheduled_by_user_id,
		    updated_at = CURRENT_TIMESTAMP
	`, leadID, followUpAt, strings.TrimSpace(note), actor); err != nil {
		return fmt.Errorf("failed to save offer reminder: %w", err)
	}
	return nil
}

func DeleteOfferSentReminder(leadID uuid.UUID) error {
	if _, err := db.DB.Exec(`DELETE FROM offer_sent_reminders WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to clear offer reminder: %w", err)
	}
	return nil
}

func resetOfferSentFollowUpsIfNeeded(leadID uuid.UUID, status string) error {
	if status == "offer_sent" {
		return nil
	}
	if _, err := db.DB.Exec(`DELETE FROM offer_sent_follow_ups WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to clear offer follow-ups: %w", err)
	}
	if _, err := db.DB.Exec(`DELETE FROM offer_sent_reminders WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to clear offer reminders: %w", err)
	}
	return nil
}

func resetSleepingLeadFollowUpsIfNeeded(leadID uuid.UUID, status string) error {
	if status == "lead_created" {
		return nil
	}
	if _, err := db.DB.Exec(`DELETE FROM sleeping_lead_follow_ups WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to clear sleeping lead follow-ups: %w", err)
	}
	if _, err := db.DB.Exec(`DELETE FROM sleeping_lead_reminders WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to clear sleeping lead follow-up state: %w", err)
	}
	return nil
}

func resetLeadFollowUpState(leadID uuid.UUID, status string) error {
	if err := resetSleepingLeadFollowUpsIfNeeded(leadID, status); err != nil {
		return err
	}
	if err := resetOfferSentFollowUpsIfNeeded(leadID, status); err != nil {
		return err
	}
	return nil
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

func CountPresentedAttendanceForClass(leadID uuid.UUID, classKey string) (int, error) {
	var attendedSessions int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)::int
		FROM attendance a
		INNER JOIN class_sessions cs ON cs.id = a.session_id
		WHERE a.lead_id = $1
		  AND cs.class_key = $2
		  AND a.status = 'PRESENT'
	`, leadID, classKey).Scan(&attendedSessions)
	if err != nil {
		return 0, fmt.Errorf("failed to count presented attendance for class: %w", err)
	}
	return attendedSessions, nil
}

func GetLatestClassEnrollment(leadID uuid.UUID) (*ClassEnrollment, error) {
	out := &ClassEnrollment{}
	var classKey sql.NullString
	var level sql.NullInt32
	var classDays sql.NullString
	var classTime sql.NullString
	err := db.DB.QueryRow(`
		SELECT id, lead_id, class_key, level, class_days,
               class_time,
               mentor_name, final_grade, outcome,
               COALESCE(next_level_consumed_on_close, false),
               COALESCE(continuation_hold_active, false),
               continuation_hold_reason,
               continuation_hold_applied_by::text,
               continuation_hold_applied_at,
               continuation_hold_released_by::text,
               continuation_hold_released_at,
               enrolled_at, completed_at
		FROM class_enrollments
		WHERE lead_id = $1
		ORDER BY COALESCE(completed_at, enrolled_at) DESC
		LIMIT 1
	`, leadID).Scan(
		&out.ID,
		&out.LeadID,
		&classKey,
		&level,
		&classDays,
		&classTime,
		&out.MentorName,
		&out.FinalGrade,
		&out.Outcome,
		&out.NextLevelConsumedOnClose,
		&out.ContinuationHoldActive,
		&out.ContinuationHoldReason,
		&out.ContinuationHoldAppliedBy,
		&out.ContinuationHoldAppliedAt,
		&out.ContinuationHoldReleasedBy,
		&out.ContinuationHoldReleasedAt,
		&out.EnrolledAt,
		&out.CompletedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest class enrollment: %w", err)
	}
	if classKey.Valid {
		out.ClassKey = classKey.String
	}
	if level.Valid {
		out.Level = level.Int32
	}
	if classDays.Valid {
		out.ClassDays = classDays.String
	}
	if classTime.Valid {
		out.ClassTime = classTime.String
	}
	return out, nil
}

func GetLatestContinuationHoldCandidate(leadID uuid.UUID) (*ClassEnrollment, error) {
	out := &ClassEnrollment{}
	err := db.DB.QueryRow(`
		SELECT id,
		       COALESCE(continuation_hold_active, false),
		       continuation_hold_reason
		FROM class_enrollments
		WHERE lead_id = $1
		  AND COALESCE(next_level_consumed_on_close, false) = true
		ORDER BY COALESCE(completed_at, enrolled_at) DESC
		LIMIT 1
	`, leadID).Scan(
		&out.ID,
		&out.ContinuationHoldActive,
		&out.ContinuationHoldReason,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest continuation hold candidate: %w", err)
	}
	out.LeadID = leadID
	out.NextLevelConsumedOnClose = true
	return out, nil
}

func ApplyContinuationHold(leadID, userID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("hold reason is required")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin continuation hold tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var enrollmentID uuid.UUID
	var holdActive bool
	err = tx.QueryRow(`
		SELECT id, COALESCE(continuation_hold_active, false)
		FROM class_enrollments
		WHERE lead_id = $1
		  AND COALESCE(next_level_consumed_on_close, false) = true
		ORDER BY COALESCE(completed_at, enrolled_at) DESC
		LIMIT 1
	`, leadID).Scan(&enrollmentID, &holdActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no closed class with reserved continuation level found")
		}
		return fmt.Errorf("failed to load latest closed enrollment: %w", err)
	}
	if holdActive {
		return fmt.Errorf("continuation hold is already active for this student")
	}

	var status string
	var purchased, consumed int32
	err = tx.QueryRow(`
		SELECT status, COALESCE(levels_purchased_total, 0), COALESCE(levels_consumed, 0)
		FROM leads
		WHERE id = $1
		FOR UPDATE
	`, leadID).Scan(&status, &purchased, &consumed)
	if err != nil {
		return fmt.Errorf("failed to load lead for continuation hold: %w", err)
	}
	if status != "waiting_for_round" {
		return fmt.Errorf("continuation hold is only available for waiting-for-round students")
	}
	if consumed <= 0 {
		return fmt.Errorf("student has no consumed level to restore")
	}

	now := time.Now()
	newConsumed := consumed - 1
	newRemaining := purchased - newConsumed
	if newRemaining < 0 {
		newRemaining = 0
	}

	_, err = tx.Exec(`
		UPDATE leads
		SET levels_consumed = $1,
		    remaining_credits = $2,
		    status = 'paused',
		    sent_to_classes = false,
		    updated_at = $3
		WHERE id = $4
	`, newConsumed, newRemaining, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to apply continuation hold to lead: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE class_enrollments
		SET continuation_hold_active = true,
		    continuation_hold_reason = $1,
		    continuation_hold_applied_by = $2,
		    continuation_hold_applied_at = $3,
		    continuation_hold_released_by = NULL,
		    continuation_hold_released_at = NULL
		WHERE id = $4
	`, reason, userID, now, enrollmentID)
	if err != nil {
		return fmt.Errorf("failed to mark continuation hold on enrollment: %w", err)
	}

	return tx.Commit()
}

func ReleaseContinuationHold(leadID, userID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin continuation release tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var enrollmentID uuid.UUID
	err = tx.QueryRow(`
		SELECT id
		FROM class_enrollments
		WHERE lead_id = $1
		  AND COALESCE(next_level_consumed_on_close, false) = true
		  AND COALESCE(continuation_hold_active, false) = true
		ORDER BY COALESCE(completed_at, enrolled_at) DESC
		LIMIT 1
	`, leadID).Scan(&enrollmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("no active continuation hold found for this student")
		}
		return fmt.Errorf("failed to load active continuation hold: %w", err)
	}

	var status string
	var purchased, consumed int32
	err = tx.QueryRow(`
		SELECT status, COALESCE(levels_purchased_total, 0), COALESCE(levels_consumed, 0)
		FROM leads
		WHERE id = $1
		FOR UPDATE
	`, leadID).Scan(&status, &purchased, &consumed)
	if err != nil {
		return fmt.Errorf("failed to load lead for continuation release: %w", err)
	}
	if status != "paused" {
		return fmt.Errorf("continuation hold can only be released while the student is paused")
	}

	now := time.Now()
	newConsumed := consumed + 1
	newRemaining := purchased - newConsumed
	if newRemaining < 0 {
		newRemaining = 0
	}

	_, err = tx.Exec(`
		UPDATE leads
		SET levels_consumed = $1,
		    remaining_credits = $2,
		    status = 'ready_to_start',
		    sent_to_classes = false,
		    updated_at = $3
		WHERE id = $4
	`, newConsumed, newRemaining, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to release continuation hold on lead: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE class_enrollments
		SET continuation_hold_active = false,
		    continuation_hold_released_by = $1,
		    continuation_hold_released_at = $2
		WHERE id = $3
	`, userID, now, enrollmentID)
	if err != nil {
		return fmt.Errorf("failed to clear continuation hold on enrollment: %w", err)
	}

	return tx.Commit()
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
		SELECT id, full_name, phone, source, notes, status, ops_queue_reason, mentor_head_return_reason, sent_to_classes,
		       levels_purchased_total, levels_consumed, remaining_credits,
		       is_returning, high_priority_follow_up, created_by_user_id, offer_sent_at, created_at, updated_at
		FROM leads WHERE id = $1
	`, id).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status, &lead.OpsQueueReason, &lead.MentorHeadReturnReason,
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
		    ops_queue_reason = CASE
		        WHEN COALESCE(ops_queue_reason, '') IN ('private_track', 'refund_review') THEN ops_queue_reason
		        WHEN $5 = 'waiting_for_round' THEN ops_queue_reason
		        ELSE NULL
		    END,
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
		tx, err := db.DB.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		_, err = tx.Exec(`
			UPDATE scheduling
			SET class_group_index = NULL,
			    updated_at = CURRENT_TIMESTAMP
			WHERE lead_id = $1
		`, leadID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			UPDATE leads
			SET status = $1,
			    sent_to_classes = false,
			    ops_queue_reason = NULL,
			    offer_sent_at = CASE
			        WHEN $1 = 'offer_sent' AND status <> 'offer_sent' THEN CURRENT_TIMESTAMP
			        ELSE offer_sent_at
			    END,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, status, leadID)
		if err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return resetLeadFollowUpState(leadID, status)
	}

	_, err := db.DB.Exec(`
		UPDATE leads
		SET status = $1,
		    ops_queue_reason = NULL,
		    offer_sent_at = CASE
		        WHEN $1 = 'offer_sent' AND status <> 'offer_sent' THEN CURRENT_TIMESTAMP
		        ELSE offer_sent_at
		    END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`, status, leadID)
	if err != nil {
		return err
	}
	return resetLeadFollowUpState(leadID, status)
}

func SendLeadToPrivateTrack(leadID uuid.UUID) error {
	var status string
	err := db.DB.QueryRow(`SELECT status FROM leads WHERE id = $1`, leadID).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lead not found")
		}
		return fmt.Errorf("failed to load lead: %w", err)
	}
	if status == "cancelled" || status == "in_classes" {
		return fmt.Errorf("lead is not eligible for private track")
	}

	var finalPrice sql.NullInt32
	err = db.DB.QueryRow(`SELECT final_price FROM offers WHERE lead_id = $1`, leadID).Scan(&finalPrice)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to load offer: %w", err)
	}
	if !finalPrice.Valid || finalPrice.Int32 <= 0 {
		return fmt.Errorf("lead must have a course offer before moving to private track")
	}

	totalCoursePaid, err := GetTotalCoursePaidCurrentCycle(leadID)
	if err != nil {
		return fmt.Errorf("failed to check total course paid: %w", err)
	}
	if totalCoursePaid < finalPrice.Int32 {
		return fmt.Errorf("lead must be fully paid before moving to private track")
	}

	result, err := db.DB.Exec(`
		UPDATE leads
		SET ops_queue_reason = 'private_track',
		    sent_to_classes = false,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND status != 'cancelled'
		  AND status != 'in_classes'
	`, leadID)
	if err != nil {
		return fmt.Errorf("failed to send lead to private track: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm private-track update: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("lead is not eligible for private track")
	}
	return nil
}

func SetPrivateTrackLeadAssignedLevel(leadID uuid.UUID, level int32) error {
	if level < 1 || level > 10 {
		return fmt.Errorf("invalid assigned level")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin private-track level transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var queueReason sql.NullString
	err = tx.QueryRow(`SELECT ops_queue_reason FROM leads WHERE id = $1`, leadID).Scan(&queueReason)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("lead not found")
		}
		return fmt.Errorf("failed to load lead: %w", err)
	}
	if !queueReason.Valid || queueReason.String != "private_track" {
		return fmt.Errorf("lead is not in private track")
	}

	now := time.Now()
	_, err = tx.Exec(`
		INSERT INTO placement_tests (id, lead_id, assigned_level, updated_at)
		VALUES (COALESCE((SELECT id FROM placement_tests WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3)
		ON CONFLICT (lead_id) DO UPDATE SET
			assigned_level = EXCLUDED.assigned_level,
			updated_at = EXCLUDED.updated_at
	`, leadID, level, now)
	if err != nil {
		return fmt.Errorf("failed to update assigned level: %w", err)
	}

	return tx.Commit()
}

func ReturnPrivateTrackLeadToAdminFeed(leadID uuid.UUID) error {
	result, err := db.DB.Exec(`
		UPDATE leads
		SET ops_queue_reason = NULL,
		    sent_to_classes = false,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND ops_queue_reason = 'private_track'
	`, leadID)
	if err != nil {
		return fmt.Errorf("failed to return private-track lead to admin feed: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm private-track return: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("lead is not in private track")
	}
	return nil
}

// MarkRenewalRefusedAndSetCold moves a returning lead to cold_lead and writes an
// auditable refusal event for renewal reporting.
func MarkRenewalRefusedAndSetCold(leadID uuid.UUID, refusedByUserID *uuid.UUID, reason, notes string) error {
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
	cleanReason := strings.TrimSpace(reason)
	if !IsValidRefusedRenewalReason(cleanReason) {
		return fmt.Errorf("invalid refusal reason")
	}
	_, err = tx.Exec(`
		INSERT INTO renewal_refusals (id, lead_id, refused_at, refused_by_user_id, reason, notes, created_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $3)
	`, uuid.New(), leadID, now, refusedBy, cleanReason, strings.TrimSpace(notes))
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

// BookPlacementTest updates placement test fields and sets status to "test_booked".
// This is a lightweight update that doesn't require offer/pricing fields.
func BookPlacementTest(leadID uuid.UUID, placementTest *PlacementTest) error {
	if placementTest == nil {
		return fmt.Errorf("placement test details are required")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now()
	placementTestFee := sql.NullInt32{Int32: 60, Valid: true}
	if placementTest.PlacementTestFee.Valid {
		placementTestFee = placementTest.PlacementTestFee
	}
	placementTestFeePaid := sql.NullInt32{Int32: 0, Valid: true}
	if placementTest.PlacementTestFeePaid.Valid {
		placementTestFeePaid = placementTest.PlacementTestFeePaid
	}

	// Update or insert placement test with default fee of 60 if not exists.
	_, err = tx.Exec(`
		INSERT INTO placement_tests (
			id, lead_id, test_date, test_time, test_type, test_notes,
			placement_test_fee, placement_test_fee_paid, placement_test_payment_date, placement_test_payment_method, updated_at
		)
		VALUES (
			COALESCE((SELECT id FROM placement_tests WHERE lead_id = $1), gen_random_uuid()),
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
		ON CONFLICT (lead_id) DO UPDATE SET
			test_date = EXCLUDED.test_date,
			test_time = EXCLUDED.test_time,
			test_type = EXCLUDED.test_type,
			test_notes = EXCLUDED.test_notes,
			placement_test_fee = COALESCE(EXCLUDED.placement_test_fee, placement_tests.placement_test_fee, 60),
			placement_test_fee_paid = COALESCE(EXCLUDED.placement_test_fee_paid, placement_tests.placement_test_fee_paid, 0),
			placement_test_payment_date = EXCLUDED.placement_test_payment_date,
			placement_test_payment_method = EXCLUDED.placement_test_payment_method,
			updated_at = EXCLUDED.updated_at
	`, leadID, placementTest.TestDate, placementTest.TestTime, placementTest.TestType, placementTest.TestNotes,
		placementTestFee, placementTestFeePaid, placementTest.PlacementTestPaymentDate, placementTest.PlacementTestPaymentMethod, now)
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

// GetEligibleStudentsForClasses returns students eligible for classes board.
// Eligibility: status=ready_to_start or waiting_for_round, assigned_level set, class_days set, class_time set.
func GetEligibleStudentsForClasses() ([]*ClassStudent, error) {
	query := `
		SELECT l.id, l.full_name, l.phone, s.class_group_index
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.status IN ('ready_to_start', 'waiting_for_round')
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
		WHERE l.status IN ('ready_to_start', 'waiting_for_round', 'in_classes')
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
					group.SuggestedStartDate = wf.SuggestedStartDate
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
				       (SELECT COUNT(*) FROM class_sessions WHERE class_key = cg.class_key AND status = 'completed') + 1 AS current_session,
				       MIN(cs.scheduled_date) FILTER (WHERE cs.session_number = 1) AS started_on
				FROM class_groups cg
				LEFT JOIN class_sessions cs ON cs.class_key = cg.class_key
				WHERE cg.class_key = ANY($1)
				  AND COALESCE(cg.round_status, '') = 'active'
				GROUP BY cg.class_key
			`, pq.Array(activeKeys))
			if err == nil {
				type activeClassState struct {
					currentSession int32
					startedOn      sql.NullTime
				}
				sessionMap := make(map[string]activeClassState)
				for rows.Next() {
					var classKey string
					var currentSession int32
					var startedOn sql.NullTime
					if err := rows.Scan(&classKey, &currentSession, &startedOn); err != nil {
						continue
					}
					sessionMap[classKey] = activeClassState{
						currentSession: currentSession,
						startedOn:      startedOn,
					}
				}
				_ = rows.Close()

				for _, group := range groups {
					if state, ok := sessionMap[group.ClassKey]; ok {
						group.CurrentSession = sql.NullInt32{Int32: state.currentSession, Valid: true}
						group.StartedOn = state.startedOn
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
		AND l.status IN ('ready_to_start', 'waiting_for_round')
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
		  AND LEFT(class_time, 5) = TO_CHAR($3::time, 'HH24:MI')
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
			  AND LEFT(class_time, 5) = TO_CHAR($3::time, 'HH24:MI')
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
			WHERE l.status IN ('ready_to_start', 'waiting_for_round')
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
		  AND LEFT(class_time, 5) = TO_CHAR($3::time, 'HH24:MI')
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
		WHERE l.status IN ('ready_to_start', 'waiting_for_round')
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
		WHERE l.status IN ('ready_to_start', 'waiting_for_round')
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

// GetNextClassGroupIndexForLead returns the next unused class group index for the lead's
// current level + days + time. This must consider all existing groups, including
// active/sent/closed ones, otherwise mid-round new classes can incorrectly reuse older numbers.
func GetNextClassGroupIndexForLead(leadID uuid.UUID) (int32, error) {
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
		return 0, fmt.Errorf("failed to get student details: %w", err)
	}
	if !assignedLevel.Valid || !classDays.Valid || !classTime.Valid {
		return 0, fmt.Errorf("student is missing level or schedule")
	}

	var maxIndex int32
	err = db.DB.QueryRow(`
		WITH existing_groups AS (
			SELECT COALESCE(cg.class_number, 1) AS group_index
			FROM class_groups cg
			WHERE cg.level = $1
			  AND cg.class_days = $2
			  AND LEFT(cg.class_time, 5) = TO_CHAR($3::time, 'HH24:MI')
			UNION
			SELECT COALESCE(s.class_group_index, 1) AS group_index
			FROM scheduling s
			INNER JOIN placement_tests pt ON pt.lead_id = s.lead_id
			INNER JOIN leads l ON l.id = s.lead_id
			WHERE pt.assigned_level = $1
			  AND s.class_days = $2
			  AND s.class_time = $3
			  AND l.sent_to_classes = true
		)
		SELECT COALESCE(MAX(group_index), 0)
		FROM existing_groups
	`, assignedLevel.Int32, classDays.String, classTime.String).Scan(&maxIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to get next class group index: %w", err)
	}

	return maxIndex + 1, nil
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
			WHERE l.status IN ('ready_to_start', 'waiting_for_round')
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
		WHERE l.status IN ('ready_to_start', 'waiting_for_round')
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

// ReturnStudentToMainFeed removes a pre-start student from the classes board and
// returns them to the main pre-enrolment feed while preserving ready_to_start or waiting_for_round.
func ReturnStudentToMainFeed(leadID uuid.UUID) error {
	var assignedLevel sql.NullInt32
	var classDays, classTime sql.NullString
	var groupIndex sql.NullInt32
	var currentStatus string
	err := db.DB.QueryRow(`
		SELECT pt.assigned_level, s.class_days, s.class_time, s.class_group_index, l.status
		FROM leads l
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN scheduling s ON s.lead_id = l.id
		WHERE l.id = $1
		  AND l.status IN ('ready_to_start', 'waiting_for_round')
		  AND l.sent_to_classes = true
	`, leadID).Scan(&assignedLevel, &classDays, &classTime, &groupIndex, &currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("student is not attached to classes")
		}
		return fmt.Errorf("failed to load current class attachment: %w", err)
	}

	if assignedLevel.Valid && classDays.Valid && classTime.Valid {
		currentClassNumber := int32(1)
		if groupIndex.Valid {
			currentClassNumber = groupIndex.Int32
		}

		var roundStatus sql.NullString
		var sentToMentor sql.NullBool
		err = db.DB.QueryRow(`
			SELECT COALESCE(round_status, 'not_started'), COALESCE(sent_to_mentor, false)
			FROM class_groups
			WHERE level = $1
			  AND class_days = $2
			  AND LEFT(class_time, 5) = TO_CHAR($3::time, 'HH24:MI')
			  AND COALESCE(class_number, 1) = $4
		`, assignedLevel.Int32, classDays.String, classTime.String, currentClassNumber).Scan(&roundStatus, &sentToMentor)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("failed to verify current class state: %w", err)
		}
		if err == nil {
			if roundStatus.Valid && (roundStatus.String == "active" || roundStatus.String == "closed") {
				return fmt.Errorf("student is in a started or closed class")
			}
			if sentToMentor.Valid && sentToMentor.Bool {
				return fmt.Errorf("student is in a fixed class already sent to mentor head")
			}
		}
	}

	now := time.Now()
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin return-to-feed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`
		UPDATE scheduling
		SET class_group_index = NULL,
		    updated_at = $1
		WHERE lead_id = $2
	`, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to clear class attachment: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE leads
		SET status = $1,
		    sent_to_classes = false,
		    ops_queue_reason = NULL,
		    updated_at = $2
		WHERE id = $3
	`, currentStatus, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to return student to main feed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit return-to-feed transaction: %w", err)
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
		    mentor_head_return_reason = NULL,
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
	var sentAt, suggestedStartDate, returnedAt, hiddenAt, roundStartedAt, roundClosedAt sql.NullTime
	var hiddenBy, roundStartedBy, roundClosedBy sql.NullString
	var roundStatus sql.NullString
	err := db.DB.QueryRow(`
		SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, suggested_start_date, returned_at, updated_at,
		       hidden_in_ops, hidden_at, hidden_by::text,
		       COALESCE(round_status, 'not_started'), round_started_at, round_started_by::text, round_closed_at, round_closed_by::text
		FROM class_groups WHERE class_key = $1
	`, classKey).Scan(
		&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
		&wf.SentToMentor, &sentAt, &suggestedStartDate, &returnedAt, &wf.UpdatedAt,
		&wf.HiddenInOps, &hiddenAt, &hiddenBy,
		&roundStatus, &roundStartedAt, &roundStartedBy, &roundClosedAt, &roundClosedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Not found is OK - means not sent yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get class group workflow: %w", err)
	}
	wf.SentAt, wf.SuggestedStartDate, wf.ReturnedAt = sentAt, suggestedStartDate, returnedAt
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

// SendClassGroupToMentor marks a class group as sent to mentor head.
func SendClassGroupToMentor(classKey string, level int32, classDays, classTime string, classNumber int32, suggestedStartDate time.Time) error {
	if err := ValidateSuggestedStartDateNotPast(suggestedStartDate); err != nil {
		return err
	}
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, suggested_start_date, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, $6, $7::date, $6)
		ON CONFLICT (class_key) DO UPDATE SET
			sent_to_mentor = true,
			sent_at = $6,
			suggested_start_date = $7::date,
			returned_at = NULL,
			hidden_in_ops = false,
			hidden_at = NULL,
			hidden_by = NULL,
			updated_at = $6
	`, classKey, level, classDays, classTime, classNumber, now, util.FormatDateCairo(suggestedStartDate))
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
		SET status = 'ready_to_start',
		    mentor_head_return_reason = 'class_return',
		    updated_at = NOW()
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

	query := `SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, suggested_start_date, returned_at, updated_at,
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
		var sentAt, suggestedStartDate, returnedAt, hiddenAt, roundStartedAt, roundClosedAt sql.NullTime
		var hiddenBy, roundStartedBy, roundClosedBy, roundStatus sql.NullString
		err := rows.Scan(
			&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
			&wf.SentToMentor, &sentAt, &suggestedStartDate, &returnedAt, &wf.UpdatedAt,
			&wf.HiddenInOps, &hiddenAt, &hiddenBy,
			&roundStatus, &roundStartedAt, &roundStartedBy, &roundClosedAt, &roundClosedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class group workflow: %w", err)
		}
		wf.SentAt, wf.SuggestedStartDate, wf.ReturnedAt = sentAt, suggestedStartDate, returnedAt
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
	if currentStatus == "cancelled" ||
		currentStatus == "waiting_for_round" ||
		currentStatus == "schedule_assigned" ||
		currentStatus == "ready_to_start" ||
		currentStatus == "in_classes" {
		return nil
	}
	var finalPrice sql.NullInt32
	err = db.DB.QueryRow(`SELECT final_price FROM offers WHERE lead_id = $1`, leadID).Scan(&finalPrice)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get offer: %w", err)
	}
	if err == sql.ErrNoRows || !finalPrice.Valid {
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
		if err := resetLeadFollowUpState(leadID, newStatus); err != nil {
			return err
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
	case amount <= 1250:
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
	const singleLevelPriceEGP int32 = 1250

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

func validateLeadPaymentKind(kind string) error {
	allowedKinds := map[string]bool{
		"course":       true,
		"deposit":      true,
		"full_payment": true,
		"top_up":       true,
	}
	if !allowedKinds[kind] {
		return fmt.Errorf("invalid payment kind: %s", kind)
	}
	return nil
}

func validateFinancePaymentMethod(paymentMethod string) error {
	allowedMethods := map[string]bool{
		"vodafone_cash": true,
		"bank_transfer": true,
		"paypal":        true,
		"other":         true,
	}
	if !allowedMethods[paymentMethod] {
		return fmt.Errorf("invalid payment method: %s", paymentMethod)
	}
	return nil
}

func combinedReconciliationNotes(original, extra string) sql.NullString {
	original = strings.TrimSpace(original)
	extra = strings.TrimSpace(extra)

	switch {
	case original == "" && extra == "":
		return sql.NullString{}
	case original == "":
		return sql.NullString{String: extra, Valid: true}
	case extra == "":
		return sql.NullString{String: original, Valid: true}
	case strings.Contains(original, extra):
		return sql.NullString{String: original, Valid: true}
	default:
		return sql.NullString{String: original + "\nReconciled note: " + extra, Valid: true}
	}
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

	if err := validateLeadPaymentKind(kind); err != nil {
		return nil, err
	}

	if err := validateFinancePaymentMethod(paymentMethod); err != nil {
		return nil, err
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

func GetUnidentifiedTransfers() ([]*Transaction, error) {
	rows, err := db.DB.Query(`
		SELECT
			id, transaction_date, transaction_type, category, amount, payment_method,
			lead_id::text AS lead_id,
			notes, ref_type, ref_id, ref_sub_type, ref_key,
			original_category, reconciled_at, reconciled_by_user_id::text AS reconciled_by_user_id,
			created_at, updated_at
		FROM transactions
		WHERE transaction_type = 'IN'
		  AND category = 'unidentified_transfer'
		  AND lead_id IS NULL
		ORDER BY transaction_date DESC, created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query unidentified transfers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	transfers := make([]*Transaction, 0)
	for rows.Next() {
		tx := &Transaction{}
		var paymentMethod, leadID, notes, refType, refID, refSubType, refKey sql.NullString
		var originalCategory sql.NullString
		var reconciledAt sql.NullTime
		var reconciledByUser sql.NullString

		err := rows.Scan(
			&tx.ID,
			&tx.TransactionDate,
			&tx.TransactionType,
			&tx.Category,
			&tx.Amount,
			&paymentMethod,
			&leadID,
			&notes,
			&refType,
			&refID,
			&refSubType,
			&refKey,
			&originalCategory,
			&reconciledAt,
			&reconciledByUser,
			&tx.CreatedAt,
			&tx.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan unidentified transfer: %w", err)
		}

		tx.PaymentMethod = paymentMethod
		tx.LeadID = leadID
		tx.Notes = notes
		tx.RefType = refType
		tx.RefID = refID
		tx.RefSubType = refSubType
		tx.RefKey = refKey
		tx.OriginalCategory = originalCategory
		tx.ReconciledAt = reconciledAt
		tx.ReconciledByUser = reconciledByUser

		transfers = append(transfers, tx)
	}

	return transfers, rows.Err()
}

func ReconcileUnidentifiedTransferToLead(transferID, leadID uuid.UUID, kind string, notes string, reconciledByUserID *uuid.UUID) (*LeadPayment, error) {
	if err := validateLeadPaymentKind(kind); err != nil {
		return nil, err
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin unidentified transfer reconciliation: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	transfer := &Transaction{}
	var paymentMethod sql.NullString
	var existingLeadID sql.NullString
	var existingNotes sql.NullString
	var refType, refID, refSubType, refKey sql.NullString
	var originalCategory sql.NullString
	var reconciledAt sql.NullTime
	var reconciledByUser sql.NullString

	err = tx.QueryRow(`
		SELECT
			id, transaction_date, transaction_type, category, amount, payment_method,
			lead_id::text AS lead_id,
			notes, ref_type, ref_id, ref_sub_type, ref_key,
			original_category, reconciled_at, reconciled_by_user_id::text AS reconciled_by_user_id,
			created_at, updated_at
		FROM transactions
		WHERE id = $1
		FOR UPDATE
	`, transferID).Scan(
		&transfer.ID,
		&transfer.TransactionDate,
		&transfer.TransactionType,
		&transfer.Category,
		&transfer.Amount,
		&paymentMethod,
		&existingLeadID,
		&existingNotes,
		&refType,
		&refID,
		&refSubType,
		&refKey,
		&originalCategory,
		&reconciledAt,
		&reconciledByUser,
		&transfer.CreatedAt,
		&transfer.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("unidentified transfer not found")
		}
		return nil, fmt.Errorf("failed to load unidentified transfer: %w", err)
	}
	transfer.PaymentMethod = paymentMethod
	transfer.LeadID = existingLeadID
	transfer.Notes = existingNotes

	if transfer.TransactionType != "IN" || transfer.Category != "unidentified_transfer" || existingLeadID.Valid {
		return nil, fmt.Errorf("this transfer is no longer available for reconciliation")
	}
	if !paymentMethod.Valid || strings.TrimSpace(paymentMethod.String) == "" {
		return nil, fmt.Errorf("unidentified transfer is missing payment method")
	}
	if err := validateFinancePaymentMethod(paymentMethod.String); err != nil {
		return nil, err
	}
	if err := util.ValidateNotFutureDate(transfer.TransactionDate); err != nil {
		return nil, err
	}

	paymentNotes := combinedReconciliationNotes(existingNotes.String, notes)
	now := time.Now()
	createdAt := transfer.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	payment := &LeadPayment{
		ID:            uuid.New(),
		LeadID:        leadID,
		Kind:          kind,
		Amount:        transfer.Amount,
		PaymentMethod: paymentMethod.String,
		PaymentDate:   transfer.TransactionDate,
		Notes:         paymentNotes,
		CreatedAt:     createdAt,
		UpdatedAt:     now,
	}

	_, err = tx.Exec(`
		INSERT INTO lead_payments (id, lead_id, kind, amount, payment_method, payment_date, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, payment.ID, payment.LeadID, payment.Kind, payment.Amount, payment.PaymentMethod, payment.PaymentDate, payment.Notes, payment.CreatedAt, payment.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create reconciled lead payment: %w", err)
	}

	refKeyValue := fmt.Sprintf("lead:%s:course_payment:%s", leadID.String(), payment.ID.String())
	var reconciledBy interface{}
	if reconciledByUserID != nil {
		reconciledBy = *reconciledByUserID
	}

	_, err = tx.Exec(`
		UPDATE transactions
		SET category = 'course_payment',
		    lead_id = $2::uuid,
		    ref_type = 'lead',
		    ref_id = $3,
		    ref_sub_type = 'course_payment',
		    ref_key = $4,
		    notes = $5,
		    original_category = COALESCE(original_category, category),
		    reconciled_at = $6,
		    reconciled_by_user_id = $7,
		    updated_at = $6
		WHERE id = $1
	`, transferID, leadID, leadID.String(), refKeyValue, paymentNotes, now, reconciledBy)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile unidentified transfer: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit unidentified transfer reconciliation: %w", err)
	}

	if err := UpdateLeadStatusFromPayment(leadID); err != nil {
		log.Printf("WARNING: failed to auto-update lead status after reconciled payment: %v", err)
	}

	return payment, nil
}

func AddWaitingListBundleCredit(leadID uuid.UUID, addedLevels int32, amount int32, paymentMethod string, paymentDate time.Time, notes string) (*LeadPayment, error) {
	if addedLevels <= 0 || addedLevels > 4 {
		return nil, fmt.Errorf("added levels must be between 1 and 4")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if err := util.ValidateNotFutureDate(paymentDate); err != nil {
		return nil, err
	}

	if err := validateFinancePaymentMethod(paymentMethod); err != nil {
		return nil, err
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin add bundle credit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	var purchased, consumed int32
	err = tx.QueryRow(`
		SELECT status, COALESCE(levels_purchased_total, 0), COALESCE(levels_consumed, 0)
		FROM leads
		WHERE id = $1
		FOR UPDATE
	`, leadID).Scan(&status, &purchased, &consumed)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("lead not found")
		}
		return nil, fmt.Errorf("failed to load lead for bundle credit: %w", err)
	}
	if status != "waiting_for_round" {
		return nil, fmt.Errorf("bundle credit can only be added while the student is in waiting list")
	}

	now := time.Now()
	payment := &LeadPayment{
		ID:            uuid.New(),
		LeadID:        leadID,
		Kind:          "top_up",
		Amount:        amount,
		PaymentMethod: paymentMethod,
		PaymentDate:   paymentDate,
		Notes:         sql.NullString{String: notes, Valid: strings.TrimSpace(notes) != ""},
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err = tx.Exec(`
		INSERT INTO lead_payments (id, lead_id, kind, amount, payment_method, payment_date, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
	`, payment.ID, payment.LeadID, payment.Kind, payment.Amount, payment.PaymentMethod, payment.PaymentDate, payment.Notes, payment.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create bundle credit payment: %w", err)
	}

	refKey := fmt.Sprintf("lead:%s:course_payment:%s", leadID.String(), payment.ID.String())
	refIDStr := leadID.String()
	paymentDateValue := paymentDate.Format("2006-01-02")
	_, err = tx.Exec(`
		INSERT INTO transactions (id, transaction_date, transaction_type, category, amount, payment_method, lead_id, ref_type, ref_id, ref_sub_type, ref_key, notes, created_at, updated_at)
		VALUES ($1, $2::date, $3::text, $4::text, $5::integer, $6::text, $7::uuid, $8::text, $9::text, $10::text, $11::text, $12, $13::timestamp with time zone, $13::timestamp with time zone)
	`, uuid.New(), paymentDateValue, "IN", "course_payment", amount, paymentMethod, leadID, "lead", refIDStr, "bundle_credit", refKey, payment.Notes, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create bundle credit finance transaction: %w", err)
	}

	newPurchased := purchased + addedLevels
	newRemaining := newPurchased - consumed
	if newRemaining < 0 {
		newRemaining = 0
	}
	_, err = tx.Exec(`
		UPDATE leads
		SET levels_purchased_total = $1,
		    remaining_credits = $2,
		    status = 'waiting_for_round',
		    sent_to_classes = false,
		    ops_queue_reason = NULL,
		    high_priority_follow_up = false,
		    high_priority = false,
		    high_priority_reason = '',
		    updated_at = $3
		WHERE id = $4
	`, newPurchased, newRemaining, now, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to update bundle credits: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
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
	if err := validateFinancePaymentMethod(paymentMethod); err != nil {
		return nil, err
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
	return resetLeadFollowUpState(leadID, "cancelled")
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
	if _, err := db.DB.Exec(`DELETE FROM sleeping_lead_follow_ups WHERE lead_id = $1`, leadID); err != nil {
		return fmt.Errorf("failed to reset sleeping lead follow-ups on reopen: %w", err)
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
	if err := validateFinancePaymentMethod(paymentMethod); err != nil {
		return nil, err
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

// CreateRevenue creates an IN transaction for manual revenue.
func CreateRevenue(category string, amount int32, paymentMethod string, transactionDate time.Time, notes string) (*Transaction, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}

	if err := util.ValidateNotFutureDate(transactionDate); err != nil {
		return nil, err
	}

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
		TransactionType: "IN",
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
		return nil, fmt.Errorf("failed to create revenue: %w", err)
	}

	return tx, nil
}

// UpsertPlacementTestIncome creates or updates a finance transaction for placement test payment
func UpsertPlacementTestIncome(leadID uuid.UUID, amountPaid int32, paymentDate sql.NullTime, paymentMethod sql.NullString) error {
	refKey := fmt.Sprintf("lead:%s:placement_test", leadID.String())

	if amountPaid <= 0 {
		_, err := db.DB.Exec(`DELETE FROM transactions WHERE ref_key = $1`, refKey)
		if err != nil {
			return fmt.Errorf("failed to delete placement test income: %w", err)
		}
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
// Bundle prices: 1 level = 1250, 2 levels = 2400, 3 levels = 3300, 4 levels = 4000
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
	today := util.FormatDateCairo(util.CairoNow())

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

// CreateClassSessions creates 8 sessions for a class when round starts.
// Sessions follow the class's two-days-per-week cadence, e.g. Mon/Thu.
func CreateClassSessions(classKey string, startDate time.Time, startTime string) error {
	var classDays string
	if err := db.DB.QueryRow(`
		SELECT class_days
		FROM class_groups
		WHERE class_key = $1
	`, classKey).Scan(&classDays); err != nil {
		return fmt.Errorf("failed to load class days: %w", err)
	}

	sessionDates, err := BuildClassSessionDates(classDays, startDate, 8)
	if err != nil {
		return err
	}

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
		sessionDate := sessionDates[i-1]
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

func consumeMembershipLevelTx(tx *sql.Tx, membershipID, leadID uuid.UUID, triggerSession int32, now time.Time) error {
	_, err := tx.Exec(`
		UPDATE leads
		SET levels_consumed = COALESCE(levels_consumed, 0) + 1,
		    remaining_credits = GREATEST(COALESCE(levels_purchased_total, 0) - (COALESCE(levels_consumed, 0) + 1), 0),
		    updated_at = $1
		WHERE id = $2
	`, now, leadID)
	if err != nil {
		return fmt.Errorf("failed to consume level for lead %s: %w", leadID, err)
	}

	_, err = tx.Exec(`
		UPDATE class_memberships
		SET level_consumed_at_session_number = $1,
		    updated_at = $2
		WHERE id = $3
	`, triggerSession, now, membershipID)
	if err != nil {
		return fmt.Errorf("failed to mark membership consumption: %w", err)
	}
	return nil
}

func applyEligibleMembershipConsumptionsTx(tx *sql.Tx, classKey string, triggerSession int32, now time.Time) error {
	rows, err := tx.Query(`
		SELECT cm.id, cm.lead_id
		FROM class_memberships cm
		INNER JOIN leads l ON l.id = cm.lead_id
		WHERE cm.class_key = $1
		  AND cm.removed_at IS NULL
		  AND l.status != 'cancelled'
		  AND cm.level_consumed_at_session_number IS NULL
		  AND (
		      SELECT COUNT(*)
		      FROM class_sessions cs
		      WHERE cs.class_key = cm.class_key
		        AND cs.status = 'completed'
		        AND cs.session_number >= cm.joined_at_session_number
		  ) >= 2
	`, classKey)
	if err != nil {
		return fmt.Errorf("failed to query eligible membership consumptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type pendingConsumption struct {
		membershipID uuid.UUID
		leadID       uuid.UUID
	}
	var pending []pendingConsumption
	for rows.Next() {
		var item pendingConsumption
		if err := rows.Scan(&item.membershipID, &item.leadID); err != nil {
			return fmt.Errorf("failed to scan eligible membership consumption: %w", err)
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate eligible membership consumptions: %w", err)
	}

	for _, item := range pending {
		if err := consumeMembershipLevelTx(tx, item.membershipID, item.leadID, triggerSession, now); err != nil {
			return err
		}
	}
	return nil
}

func ensureMembershipLevelConsumedTx(tx *sql.Tx, membership *ClassMembership, triggerSession int32, now time.Time) (bool, error) {
	if membership == nil || membership.LevelConsumedAtSession.Valid {
		return false, nil
	}

	var completedSinceJoin int32
	err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM class_sessions
		WHERE class_key = $1
		  AND status = 'completed'
		  AND session_number >= $2
	`, membership.ClassKey, membership.JoinedAtSessionNumber).Scan(&completedSinceJoin)
	if err != nil {
		return false, fmt.Errorf("failed to count completed sessions since join: %w", err)
	}
	if completedSinceJoin < 2 {
		return false, nil
	}

	if err := consumeMembershipLevelTx(tx, membership.ID, membership.LeadID, triggerSession, now); err != nil {
		return false, err
	}
	membership.LevelConsumedAtSession = sql.NullInt32{Int32: triggerSession, Valid: true}
	return true, nil
}

// CompleteSession marks a session as completed and sets completed_at timestamp.
// Once 2 completed sessions have passed since a student's join point, the current level is consumed.
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

	if err := ensureClassMembershipsTx(tx, classKey); err != nil {
		return err
	}

	// Require attendance for all applicable students before completing the session.
	var missingCount int
	err = tx.QueryRow(`
		SELECT COUNT(*)
		FROM class_memberships cm
		INNER JOIN leads l ON l.id = cm.lead_id
		WHERE cm.class_key = $1
		  AND cm.joined_at_session_number <= $3
		  AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= $3)
		  AND cm.removed_at IS NULL
		  AND l.status != 'cancelled'
		  AND NOT EXISTS (
			  SELECT 1 FROM attendance a
			  WHERE a.session_id = $2 AND a.lead_id = cm.lead_id
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

	if err := applyEligibleMembershipConsumptionsTx(tx, classKey, sessionNumber, now); err != nil {
		return err
	}

	return tx.Commit()
}

// CancelAndRescheduleSession reschedules a session and records the previous schedule for reporting.
func CancelAndRescheduleSession(sessionID uuid.UUID, newDate time.Time, newTime string, changedByUserID uuid.UUID) error {
	// Parse new time to calculate end time
	startTimeParsed, err := time.Parse("15:04", newTime)
	if err != nil {
		return fmt.Errorf("invalid time format: %w", err)
	}
	endTimeParsed := startTimeParsed.Add(2 * time.Hour)
	endTime := endTimeParsed.Format("15:04")

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin reschedule transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var classKey string
	var sessionNumber int32
	var oldDate time.Time
	var oldTime string
	err = tx.QueryRow(`
		SELECT class_key, session_number, scheduled_date, COALESCE(scheduled_time::TEXT, '')
		FROM class_sessions
		WHERE id = $1
	`, sessionID).Scan(&classKey, &sessionNumber, &oldDate, &oldTime)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("session not found")
		}
		return fmt.Errorf("failed to load current session schedule: %w", err)
	}

	now := time.Now()
	_, err = tx.Exec(`
		INSERT INTO class_session_reschedules (
			id,
			class_session_id,
			class_key,
			session_number,
			old_scheduled_date,
			old_scheduled_time,
			new_scheduled_date,
			new_scheduled_time,
			changed_by_user_id,
			created_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5::time, $6, $7::time, $8, $9)
	`, sessionID, classKey, sessionNumber, oldDate.Format("2006-01-02"), oldTime, newDate.Format("2006-01-02"), newTime, changedByUserID, now)
	if err != nil {
		return fmt.Errorf("failed to record session reschedule: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE class_sessions
		SET scheduled_date = $1, scheduled_time = $2, scheduled_end_time = $3,
		    actual_date = NULL, actual_time = NULL, actual_end_time = NULL, completed_at = NULL,
		    status = 'scheduled', updated_at = $4
		WHERE id = $5
	`, newDate, newTime, endTime, now, sessionID)
	if err != nil {
		return fmt.Errorf("failed to reschedule session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit session reschedule: %w", err)
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

	sessionDates, err := BuildClassSessionDates(classDays, newStartDate, 8)
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

func BuildClassSessionDates(classDays string, startDate time.Time, count int) ([]time.Time, error) {
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

func buildClassSessionDates(classDays string, startDate time.Time, count int) ([]time.Time, error) {
	return BuildClassSessionDates(classDays, startDate, count)
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

func normalizeBusinessPMClock(clock time.Time) time.Time {
	if clock.Hour() > 0 && clock.Hour() < 12 {
		return clock.Add(12 * time.Hour)
	}
	return clock
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
	startBased := false
	if session.ScheduledTime.Valid {
		baseTime = session.ScheduledTime.String
		startBased = true
	} else if session.ScheduledEndTime.Valid {
		baseTime = session.ScheduledEndTime.String
	}
	if baseTime == "" {
		return time.Time{}, fmt.Errorf("session time is missing")
	}
	parsed, err := parseSessionClock(baseTime)
	if err != nil {
		return time.Time{}, err
	}
	parsed = normalizeBusinessPMClock(parsed)
	end := time.Date(year, month, day, parsed.Hour(), parsed.Minute(), parsed.Second(), 0, loc)
	if startBased {
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
	students, err := GetStudentsForMentorHeadClass(classKey)
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
		joinedAtSession := int32(1)
		if student.JoinedAtSessionNumber.Valid && student.JoinedAtSessionNumber.Int32 > 1 {
			joinedAtSession = student.JoinedAtSessionNumber.Int32
		}
		var absences int
		var completedTasks int
		var attendedSessions int
		var applicableSessions int
		var applicableTaskSessions int
		var participationTotal float64
		usedLegacyTaskFallback := false

		for _, session := range sessions {
			if session == nil {
				continue
			}
			if session.SessionNumber < joinedAtSession {
				continue
			}
			applicableSessions++
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
				applicableTaskSessions++
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
		} else if applicableSessions > 0 {
			attendanceScore = (float64(attendedSessions) / float64(applicableSessions)) * 50.0
		}

		taskScore := float64(0)
		if completedTasks > 1 && applicableTaskSessions > 0 {
			taskScore = (float64(completedTasks) / float64(applicableTaskSessions)) * 40.0
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

func getClassCurrentSession(classKey string) (int32, error) {
	var currentSession int32
	err := db.DB.QueryRow(`
		SELECT COALESCE(COUNT(*), 0) + 1
		FROM class_sessions
		WHERE class_key = $1
		  AND status = 'completed'
	`, classKey).Scan(&currentSession)
	if err != nil {
		return 0, fmt.Errorf("failed to get current session for %s: %w", classKey, err)
	}
	return currentSession, nil
}

func getClassCurrentSessionTx(tx *sql.Tx, classKey string) (int32, error) {
	var currentSession int32
	err := tx.QueryRow(`
		SELECT COALESCE(COUNT(*), 0) + 1
		FROM class_sessions
		WHERE class_key = $1
		  AND status = 'completed'
	`, classKey).Scan(&currentSession)
	if err != nil {
		return 0, fmt.Errorf("failed to get current session for %s: %w", classKey, err)
	}
	return currentSession, nil
}

func ensureClassMemberships(classKey string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin membership backfill transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := ensureClassMembershipsTx(tx, classKey); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureClassMembershipsTx(tx *sql.Tx, classKey string) error {
	var roundStatus string
	var sentToMentor bool
	err := tx.QueryRow(`
		SELECT COALESCE(round_status, 'not_started'), COALESCE(sent_to_mentor, false)
		FROM class_groups
		WHERE class_key = $1
	`, classKey).Scan(&roundStatus, &sentToMentor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("class not found")
		}
		return fmt.Errorf("failed to load class for membership backfill: %w", err)
	}

	if roundStatus == "closed" {
		return nil
	}
	if roundStatus != "active" && !(roundStatus == "not_started" && sentToMentor) {
		return nil
	}

	statuses := []string{"in_classes"}
	if roundStatus == "not_started" {
		// Keep pre-start mentor-head rosters aligned with the Ops classes board.
		// Sent classes can legitimately contain waiting-list students before the round starts.
		statuses = append(statuses, "ready_to_start", "waiting_for_round", "schedule_assigned")
	}

	_, err = tx.Exec(`
		INSERT INTO class_memberships (
			id,
			lead_id,
			class_key,
			joined_at_session_number,
			join_reason,
			added_by_user_id,
			created_at,
			updated_at
		)
		SELECT
			gen_random_uuid(),
			l.id,
			cg.class_key,
			COALESCE(lj.joined_at_session_number, 1),
			CASE
				WHEN lj.lead_id IS NOT NULL THEN 'late_join'
				WHEN $2 = 'active' THEN 'round_start'
				ELSE 'sent_to_mentor'
			END,
			lj.added_by_user_id,
			COALESCE(lj.created_at, NOW()),
			NOW()
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
		  AND l.status = ANY($3::text[])
		  AND l.status != 'cancelled'
		  AND NOT EXISTS (
			  SELECT 1
			  FROM class_memberships cm
			  WHERE cm.lead_id = l.id
			    AND cm.class_key = cg.class_key
		  )
	`, classKey, roundStatus, pq.Array(statuses))
	if err != nil {
		return fmt.Errorf("failed to backfill class memberships: %w", err)
	}

	return nil
}

func getApplicableMembershipCountTx(tx *sql.Tx, classKey string, sessionNumber int32) (int32, error) {
	var count int32
	err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM class_memberships cm
		INNER JOIN leads l ON l.id = cm.lead_id
		WHERE cm.class_key = $1
		  AND cm.joined_at_session_number <= $2
		  AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= $2)
		  AND cm.removed_at IS NULL
		  AND l.status != 'cancelled'
	`, classKey, sessionNumber).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count applicable memberships: %w", err)
	}
	return count, nil
}

func getApplicableMembershipTx(tx *sql.Tx, leadID uuid.UUID, classKey string, sessionNumber int32) (*ClassMembership, error) {
	item := &ClassMembership{}
	err := tx.QueryRow(`
		SELECT
			id,
			lead_id,
			class_key,
			joined_at_session_number,
			level_consumed_at_session_number,
			left_after_session_number,
			join_reason,
			leave_reason,
			added_by_user_id::text,
			removed_by_user_id::text,
			removed_at,
			created_at,
			updated_at
		FROM class_memberships
		WHERE lead_id = $1
		  AND class_key = $2
		  AND joined_at_session_number <= $3
		  AND (left_after_session_number IS NULL OR left_after_session_number >= $3)
		  AND removed_at IS NULL
		ORDER BY joined_at_session_number DESC, created_at DESC
		LIMIT 1
	`, leadID, classKey, sessionNumber).Scan(
		&item.ID,
		&item.LeadID,
		&item.ClassKey,
		&item.JoinedAtSessionNumber,
		&item.LevelConsumedAtSession,
		&item.LeftAfterSessionNumber,
		&item.JoinReason,
		&item.LeaveReason,
		&item.AddedByUserID,
		&item.RemovedByUserID,
		&item.RemovedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load applicable membership: %w", err)
	}
	return item, nil
}

func IsLeadApplicableToClassSession(leadID, sessionID uuid.UUID) (bool, error) {
	var classKey string
	var sessionNumber int32
	err := db.DB.QueryRow(`
		SELECT class_key, session_number
		FROM class_sessions
		WHERE id = $1
	`, sessionID).Scan(&classKey, &sessionNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("session not found")
		}
		return false, fmt.Errorf("failed to load session for attendance validation: %w", err)
	}

	if err := ensureClassMemberships(classKey); err != nil {
		return false, err
	}

	var applicable bool
	err = db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM class_memberships cm
			INNER JOIN leads l ON l.id = cm.lead_id
			WHERE cm.lead_id = $1
			  AND cm.class_key = $2
			  AND cm.joined_at_session_number <= $3
			  AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= $3)
			  AND cm.removed_at IS NULL
			  AND l.status != 'cancelled'
		)
	`, leadID, classKey, sessionNumber).Scan(&applicable)
	if err != nil {
		return false, fmt.Errorf("failed to validate class session applicability: %w", err)
	}
	if applicable {
		return true, nil
	}

	var fallback bool
	err = db.DB.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM leads l
			INNER JOIN scheduling s ON s.lead_id = l.id
			INNER JOIN placement_tests pt ON pt.lead_id = l.id
			INNER JOIN class_groups cg ON (
				cg.level = pt.assigned_level
				AND cg.class_days = s.class_days
				AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
				AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
			)
			WHERE l.id = $1
			  AND cg.class_key = $2
			  AND l.status = 'in_classes'
			  AND NOT EXISTS (
				  SELECT 1
				  FROM late_joiners lj
				  WHERE lj.lead_id = l.id
				    AND lj.class_key = cg.class_key
				    AND $3 < lj.joined_at_session_number
			  )
		)
	`, leadID, classKey, sessionNumber).Scan(&fallback)
	if err != nil {
		return false, fmt.Errorf("failed to validate legacy class session applicability: %w", err)
	}
	return fallback, nil
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

	var cycleStart sql.NullTime
	err = db.DB.QueryRow(`
		SELECT MAX(completed_at)
		FROM class_enrollments
		WHERE lead_id = $1
	`, leadID).Scan(&cycleStart)
	if err != nil {
		return 0, fmt.Errorf("failed to determine current cycle start: %w", err)
	}

	var consumedSessions int32
	err = db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM class_memberships cm
		INNER JOIN class_sessions cs ON cs.class_key = cm.class_key
		WHERE cm.lead_id = $1
		  AND cs.status = 'completed'
		  AND cs.completed_at IS NOT NULL
		  AND cs.session_number >= cm.joined_at_session_number
		  AND (cm.left_after_session_number IS NULL OR cs.session_number <= cm.left_after_session_number)
		  AND ($2::timestamp with time zone IS NULL OR cs.completed_at > $2)
	`, leadID, cycleStart).Scan(&consumedSessions)
	if err != nil {
		return 0, fmt.Errorf("failed to count consumed class sessions: %w", err)
	}

	if consumedSessions >= 2 {
		return 0, nil
	}
	if consumedSessions == 1 {
		return totalCoursePaid / 2, nil
	}

	// Legacy fallback for classes that predate class_memberships creation.
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
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("failed to get legacy class key: %w", err)
	}
	if err == nil && classKey.Valid {
		var session1Completed, session2Completed bool
		err = db.DB.QueryRow(`
			SELECT
				EXISTS(SELECT 1 FROM class_sessions WHERE class_key = $1 AND session_number = 1 AND completed_at IS NOT NULL),
				EXISTS(SELECT 1 FROM class_sessions WHERE class_key = $1 AND session_number = 2 AND completed_at IS NOT NULL)
		`, classKey.String).Scan(&session1Completed, &session2Completed)
		if err != nil {
			return 0, fmt.Errorf("failed to check legacy session completion: %w", err)
		}
		if session2Completed {
			return 0, nil
		}
		if session1Completed {
			return totalCoursePaid / 2, nil
		}
	}

	return totalCoursePaid, nil
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

// GetCertificateMentorName returns the best display name for the class mentor,
// preferring the active assignment and falling back to the closed-round mentor.
func GetCertificateMentorName(classKey string) (string, error) {
	var mentorName string
	err := db.DB.QueryRow(`
		SELECT COALESCE(NULLIF(TRIM(u.full_name), ''), NULLIF(TRIM(u.email), ''), '')
		FROM class_groups cg
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
		LEFT JOIN users u ON u.id = COALESCE(ma.mentor_user_id, cg.closed_mentor_user_id)
		WHERE cg.class_key = $1
	`, classKey).Scan(&mentorName)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get certificate mentor name: %w", err)
	}
	return strings.TrimSpace(mentorName), nil
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

	classRows, err := db.DB.Query(`
		SELECT ma.class_key
		FROM mentor_assignments ma
		INNER JOIN class_groups cg ON cg.class_key = ma.class_key
		WHERE ma.mentor_user_id = $1
		  AND COALESCE(cg.round_status, '') != 'closed'
	`, mentorUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to load mentor classes for membership sync: %w", err)
	}
	for classRows.Next() {
		var classKey string
		if err := classRows.Scan(&classKey); err == nil {
			if ensureErr := ensureClassMemberships(classKey); ensureErr != nil {
				_ = classRows.Close()
				return nil, ensureErr
			}
		}
	}
	if err := classRows.Close(); err != nil {
		return nil, fmt.Errorf("failed to close mentor class rows: %w", err)
	}

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
			  SELECT 1
			  FROM class_memberships cm
			  INNER JOIN leads l ON l.id = cm.lead_id
			  WHERE cm.class_key = cg.class_key
				AND cm.joined_at_session_number <= cs.session_number
				AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= cs.session_number)
				AND cm.removed_at IS NULL
				AND l.status != 'cancelled'
				AND NOT EXISTS (
					SELECT 1 FROM attendance a
					WHERE a.session_id = cs.id AND a.lead_id = cm.lead_id
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
			  SELECT 1
			  FROM class_memberships cm
			  INNER JOIN leads l ON l.id = cm.lead_id
			  WHERE cm.class_key = cg.class_key
				AND cm.joined_at_session_number <= 8
				AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= 8)
				AND cm.removed_at IS NULL
				AND l.status != 'cancelled'
				AND NOT EXISTS (
					SELECT 1 FROM grades g
					WHERE g.lead_id = cm.lead_id
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

	absenceOverrideApproved, err := isAbsencePromotionOverrideApprovedTx(tx, leadID, classKey)
	if err != nil {
		return err
	}

	// Determine outcome: repeat if absences > 2 OR grade = 'F', unless Mentor Head approved an absence override.
	shouldRepeat := (absences > 2 && !absenceOverrideApproved) || (finalGrade.Valid && finalGrade.String == "F")
	outcome := "promoted"
	if shouldRepeat {
		outcome = "repeated"
	}

	// 5. Snapshot to class_enrollments
	membership, err := getApplicableMembershipTx(tx, leadID, classKey, 8)
	if err != nil {
		return fmt.Errorf("failed to load class membership for close-round consumption: %w", err)
	}
	if _, err := ensureMembershipLevelConsumedTx(tx, membership, 8, now); err != nil {
		return fmt.Errorf("failed to finalize current-level consumption: %w", err)
	}

	// 5. Credit check and status update (compute remaining credits from purchased - consumed)
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

	nextLevelConsumedOnClose := creditsRemaining > 0
	newCredits := creditsRemaining
	newConsumed := int32(0)
	if consumed.Valid {
		newConsumed = consumed.Int32
	}
	if nextLevelConsumedOnClose {
		newCredits -= 1
		if newCredits < 0 {
			newCredits = 0
		}
		newConsumed++
	}

	_, err = tx.Exec(`
		INSERT INTO class_enrollments (
			lead_id, class_key, level, class_days, class_time, mentor_name,
			final_grade, outcome, next_level_consumed_on_close, enrolled_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (lead_id, class_key) DO UPDATE SET
			final_grade = EXCLUDED.final_grade,
			outcome = EXCLUDED.outcome,
			next_level_consumed_on_close = class_enrollments.next_level_consumed_on_close OR EXCLUDED.next_level_consumed_on_close,
			completed_at = EXCLUDED.completed_at
	`, leadID, classKey, level, classDays, classTime, mentorName, finalGrade, outcome, nextLevelConsumedOnClose, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert class enrollment: %w", err)
	}

	// Status is based on credits before the close-round continuation consumption.
	// If the student had any prepaid continuation level when they finished, they wait for a round.
	newStatus := "renewal_pending"
	if creditsRemaining > 0 {
		newStatus = "waiting_for_round"
	}

	highPriorityFollowUp := newStatus == "renewal_pending"

	// 7. Set returning flag and remaining credits snapshot
	_, err = tx.Exec(`
		UPDATE leads 
		SET levels_consumed = $1,
		    remaining_credits = $2,
		    status = $3,
		    is_returning = true,
		    high_priority_follow_up = $6,
		    high_priority = false,
		    high_priority_reason = '',
		    updated_at = $4
		WHERE id = $5
	`, newConsumed, newCredits, newStatus, now, leadID, highPriorityFollowUp)
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

	var roundStatus string
	err = tx.QueryRow(`
		SELECT COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
		FOR UPDATE
	`, classKey).Scan(&roundStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot close round: class not found")
		}
		return fmt.Errorf("failed to lock class for close round: %w", err)
	}
	if roundStatus == "closed" {
		return fmt.Errorf("cannot close round: class is already closed")
	}

	if err := ensureClassMembershipsTx(tx, classKey); err != nil {
		return err
	}

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
		SELECT cm.lead_id
		FROM class_memberships cm
		INNER JOIN leads l ON l.id = cm.lead_id
		WHERE cm.class_key = $1
		  AND cm.joined_at_session_number <= 8
		  AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= 8)
		  AND cm.removed_at IS NULL
		  AND l.status != 'cancelled'
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

	pendingOverrideCount, err := countPendingAbsencePromotionOverridesTx(tx, classKey)
	if err != nil {
		return err
	}
	if pendingOverrideCount > 0 {
		return fmt.Errorf("cannot close round: %d absence promotion override request(s) still need Mentor Head review", pendingOverrideCount)
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

	_, err = tx.Exec(`
		UPDATE class_memberships
		SET left_after_session_number = COALESCE(left_after_session_number, 8),
		    leave_reason = COALESCE(leave_reason, 'round_closed'),
		    removed_by_user_id = COALESCE(removed_by_user_id, $2),
		    removed_at = COALESCE(removed_at, $1),
		    updated_at = $1
		WHERE class_key = $3
		  AND removed_at IS NULL
	`, now, closedByUserID, classKey)
	if err != nil {
		return fmt.Errorf("failed to close class memberships: %w", err)
	}

	return tx.Commit()
}

type AbsencePromotionOverrideItem struct {
	ID                uuid.UUID      `json:"id"`
	LeadID            uuid.UUID      `json:"lead_id"`
	ClassKey          string         `json:"class_key"`
	FullName          string         `json:"full_name"`
	Phone             string         `json:"phone"`
	Absences          int            `json:"absences"`
	FinalGrade        sql.NullString `json:"final_grade"`
	Status            sql.NullString `json:"status"`
	Reason            sql.NullString `json:"reason"`
	RequestedByName   sql.NullString `json:"requested_by_name"`
	RequestedAt       sql.NullTime   `json:"requested_at"`
	ReviewedByName    sql.NullString `json:"reviewed_by_name"`
	ReviewedAt        sql.NullTime   `json:"reviewed_at"`
	ReviewNote        sql.NullString `json:"review_note"`
	OverrideAvailable bool           `json:"override_available"`
}

func GetClassAbsencePromotionOverrides(classKey string) ([]AbsencePromotionOverrideItem, error) {
	rows, err := db.DB.Query(`
		WITH active_students AS (
			SELECT cm.lead_id
			FROM class_memberships cm
			INNER JOIN leads l ON l.id = cm.lead_id
			WHERE cm.class_key = $1
			  AND cm.joined_at_session_number <= 8
			  AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= 8)
			  AND cm.removed_at IS NULL
			  AND l.status != 'cancelled'
		),
		absence_counts AS (
			SELECT a.lead_id, COUNT(*)::int AS absences
			FROM attendance a
			INNER JOIN class_sessions cs ON cs.id = a.session_id
			WHERE cs.class_key = $1
			  AND a.status IN ('ABSENT', 'LATE')
			GROUP BY a.lead_id
		),
		final_grades AS (
			SELECT lead_id, grade
			FROM grades
			WHERE class_key = $1
			  AND session_number = 8
		)
		SELECT
			COALESCE(apo.id, '00000000-0000-0000-0000-000000000000'::uuid),
			l.id,
			$1,
			l.full_name,
			l.phone,
			COALESCE(ac.absences, 0),
			fg.grade,
			apo.status,
			apo.reason,
			COALESCE(NULLIF(TRIM(requested_by.full_name), ''), requested_by.email),
			apo.requested_at,
			COALESCE(NULLIF(TRIM(reviewed_by.full_name), ''), reviewed_by.email),
			apo.reviewed_at,
			apo.review_note,
			(COALESCE(ac.absences, 0) > 2 AND fg.grade IS NOT NULL AND fg.grade <> 'F') AS override_available
		FROM active_students ast
		INNER JOIN leads l ON l.id = ast.lead_id
		LEFT JOIN absence_counts ac ON ac.lead_id = l.id
		LEFT JOIN final_grades fg ON fg.lead_id = l.id
		LEFT JOIN absence_promotion_overrides apo ON apo.lead_id = l.id AND apo.class_key = $1
		LEFT JOIN users requested_by ON requested_by.id = apo.requested_by_user_id
		LEFT JOIN users reviewed_by ON reviewed_by.id = apo.reviewed_by_user_id
		WHERE COALESCE(ac.absences, 0) > 2 OR apo.id IS NOT NULL
		ORDER BY l.full_name
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query absence promotion overrides: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []AbsencePromotionOverrideItem{}
	for rows.Next() {
		var item AbsencePromotionOverrideItem
		if err := rows.Scan(
			&item.ID,
			&item.LeadID,
			&item.ClassKey,
			&item.FullName,
			&item.Phone,
			&item.Absences,
			&item.FinalGrade,
			&item.Status,
			&item.Reason,
			&item.RequestedByName,
			&item.RequestedAt,
			&item.ReviewedByName,
			&item.ReviewedAt,
			&item.ReviewNote,
			&item.OverrideAvailable,
		); err != nil {
			return nil, fmt.Errorf("failed to scan absence promotion override: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func RequestAbsencePromotionOverride(leadID uuid.UUID, classKey, reason string, requestedBy uuid.UUID) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("justification note is required")
	}
	eligible, err := isAbsencePromotionOverrideEligible(leadID, classKey)
	if err != nil {
		return err
	}
	if !eligible {
		return fmt.Errorf("absence promotion override is available only for students with more than 2 absences and a non-F final grade")
	}
	_, err = db.DB.Exec(`
		INSERT INTO absence_promotion_overrides (
			lead_id, class_key, reason, status, requested_by_user_id, requested_at, updated_at
		)
		VALUES ($1, $2, $3, 'pending', $4, NOW(), NOW())
		ON CONFLICT (lead_id, class_key) DO UPDATE SET
			reason = EXCLUDED.reason,
			status = 'pending',
			requested_by_user_id = EXCLUDED.requested_by_user_id,
			requested_at = NOW(),
			reviewed_by_user_id = NULL,
			reviewed_at = NULL,
			review_note = NULL,
			updated_at = NOW()
	`, leadID, classKey, reason, requestedBy)
	if err != nil {
		return fmt.Errorf("failed to request absence promotion override: %w", err)
	}
	return nil
}

func ReviewAbsencePromotionOverride(leadID uuid.UUID, classKey, status, reviewNote string, reviewedBy uuid.UUID) error {
	status = strings.TrimSpace(status)
	if status != "approved" && status != "rejected" {
		return fmt.Errorf("review status must be approved or rejected")
	}
	res, err := db.DB.Exec(`
		UPDATE absence_promotion_overrides
		SET status = $1,
		    review_note = NULLIF($2, ''),
		    reviewed_by_user_id = $3,
		    reviewed_at = NOW(),
		    updated_at = NOW()
		WHERE lead_id = $4
		  AND class_key = $5
	`, status, strings.TrimSpace(reviewNote), reviewedBy, leadID, classKey)
	if err != nil {
		return fmt.Errorf("failed to review absence promotion override: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("absence promotion override request not found")
	}
	return nil
}

func isAbsencePromotionOverrideEligible(leadID uuid.UUID, classKey string) (bool, error) {
	var absences int
	var finalGrade sql.NullString
	err := db.DB.QueryRow(`
		WITH absence_count AS (
			SELECT COUNT(*)::int AS absences
			FROM attendance a
			INNER JOIN class_sessions cs ON cs.id = a.session_id
			WHERE a.lead_id = $1
			  AND cs.class_key = $2
			  AND a.status IN ('ABSENT', 'LATE')
		)
		SELECT ac.absences, g.grade
		FROM absence_count ac
		LEFT JOIN grades g ON g.lead_id = $1 AND g.class_key = $2 AND g.session_number = 8
	`, leadID, classKey).Scan(&absences, &finalGrade)
	if err != nil {
		return false, fmt.Errorf("failed to verify absence promotion override eligibility: %w", err)
	}
	return absences > 2 && finalGrade.Valid && finalGrade.String != "F", nil
}

func isAbsencePromotionOverrideApprovedTx(tx *sql.Tx, leadID uuid.UUID, classKey string) (bool, error) {
	var approved bool
	err := tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM absence_promotion_overrides
			WHERE lead_id = $1
			  AND class_key = $2
			  AND status = 'approved'
		)
	`, leadID, classKey).Scan(&approved)
	if err != nil {
		return false, fmt.Errorf("failed to check absence promotion override: %w", err)
	}
	return approved, nil
}

func countPendingAbsencePromotionOverridesTx(tx *sql.Tx, classKey string) (int, error) {
	var count int
	err := tx.QueryRow(`
		SELECT COUNT(*)::int
		FROM absence_promotion_overrides
		WHERE class_key = $1
		  AND status = 'pending'
	`, classKey).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending absence promotion overrides: %w", err)
	}
	return count, nil
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

func normalizeSessionQualityBySession(raw []int) []int {
	out := make([]int, 8)
	for i := 0; i < len(out) && i < len(raw); i++ {
		value := raw[i]
		if value < 0 {
			value = 0
		}
		if value > 10 {
			value = 10
		}
		out[i] = value
	}
	return out
}

func parseSessionQualityBySessionJSON(raw sql.NullString, fallback int) []int {
	if raw.Valid {
		var parsed []int
		if err := json.Unmarshal([]byte(raw.String), &parsed); err == nil {
			normalized := normalizeSessionQualityBySession(parsed)
			if countRecordedSessionQualities(normalized) > 0 {
				return normalized
			}
		}
	}
	out := make([]int, 8)
	if fallback > 0 {
		out[0] = fallback
	}
	return out
}

func countRecordedSessionQualities(values []int) int {
	count := 0
	for _, value := range values {
		if value > 0 {
			count++
		}
	}
	return count
}

func averageRecordedSessionQuality(values []int) int {
	total := 0
	count := 0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		total += value
		count++
	}
	if count == 0 {
		return 0
	}
	return int(math.Round(float64(total) / float64(count)))
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
			me.kpi_session_quality_by_session,
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
		var sessionQualityBySessionJSON sql.NullString
		var trelloJSON sql.NullString

		if err := rows.Scan(
			&user.ID, &user.Email, &user.FullName, &user.Phone, &user.PasswordHash, &user.Role, &user.CreatedAt,
			&classItem.ClassKey, &classItem.Level, &classItem.ClassDays, &classItem.ClassTime, &classItem.ClassNumber, &classItem.RoundStatus,
			&classItem.KPISessionQuality, &classItem.KPIStudentsFeedback, &sessionQualityBySessionJSON, &trelloJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor evaluation class row: %w", err)
		}

		statuses, punctuality, whatsapp, err := computeAttendanceFromCompliance(classItem.ClassKey)
		if err != nil {
			return nil, fmt.Errorf("failed to compute compliance metrics for class %s: %w", classItem.ClassKey, err)
		}

		classItem.KPISessionQualityByS = parseSessionQualityBySessionJSON(sessionQualityBySessionJSON, classItem.KPISessionQuality)
		classItem.KPISessionQuality = averageRecordedSessionQuality(classItem.KPISessionQualityByS)
		classItem.RecordedSessionCount = countRecordedSessionQualities(classItem.KPISessionQualityByS)
		classItem.TrelloSessionChecks = parseTrelloChecksJSON(trelloJSON)
		classItem.AttendanceStatuses = statuses
		classItem.AttendancePercent = punctuality
		classItem.AutoWhatsAppPercent = whatsapp
		checked := 0
		for _, ok := range classItem.TrelloSessionChecks {
			if ok {
				checked++
			}
		}
		trelloPercent := (checked * 100) / 8
		classItem.ClassCollectiveScore = computeCollectiveClassScore(classItem.KPISessionQuality, classItem.KPIStudentsFeedback, trelloPercent, punctuality, whatsapp)

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
			me.kpi_session_quality_by_session,
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
		var sessionQualityBySessionJSON sql.NullString
		var trelloJSON sql.NullString
		if err := rows.Scan(
			&item.ClassKey, &item.Level, &item.Days, &item.Time,
			&item.StartDate, &item.EndDate,
			&sessionQuality, &feedback, &sessionQualityBySessionJSON, &trelloJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor class history row: %w", err)
		}

		metrics, err := getClassComplianceMetrics(item.ClassKey)
		if err != nil {
			return nil, fmt.Errorf("failed to compute compliance for class %s: %w", item.ClassKey, err)
		}

		sessionQualityBySession := parseSessionQualityBySessionJSON(sessionQualityBySessionJSON, sessionQuality)
		sessionQuality = averageRecordedSessionQuality(sessionQualityBySession)
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
func UpsertMentorEvaluationByClass(mentorID uuid.UUID, classKey string, evaluatorID uuid.UUID, sessionQualityBySession []int, studentsFeedback int, trelloSessionChecks []bool) error {
	if classKey == "" {
		return fmt.Errorf("class_key is required")
	}
	sessionQualityBySession = normalizeSessionQualityBySession(sessionQualityBySession)
	for _, value := range sessionQualityBySession {
		if value < 0 || value > 10 {
			return fmt.Errorf("session_quality_by_session must be between 0 and 10")
		}
	}
	if studentsFeedback < 1 || studentsFeedback > 10 {
		return fmt.Errorf("students_feedback must be between 1 and 10")
	}
	sessionQuality := averageRecordedSessionQuality(sessionQualityBySession)
	sessionQualityJSON, err := json.Marshal(sessionQualityBySession)
	if err != nil {
		return fmt.Errorf("failed to encode session quality by session: %w", err)
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
		    kpi_session_quality_by_session = $5::jsonb,
		    trello_session_checks = $6::jsonb,
		    evaluator_id = $7,
		    updated_at = NOW()
		WHERE mentor_id = $1
		  AND class_key = $2
	`, mentorID, classKey, sessionQuality, studentsFeedback, string(sessionQualityJSON), string(trelloJSON), evaluatorID)
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
				mentor_id, class_key, kpi_session_quality, kpi_students_feedback, kpi_session_quality_by_session, trello_session_checks, evaluator_id, updated_at
			)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, NOW())
		`, mentorID, classKey, sessionQuality, studentsFeedback, string(sessionQualityJSON), string(trelloJSON), evaluatorID)
		if err != nil {
			// If another request inserted concurrently, retry as update in same tx.
			if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
				_, err = tx.Exec(`
					UPDATE mentor_evaluations
					SET kpi_session_quality = $3,
					    kpi_students_feedback = $4,
					    kpi_session_quality_by_session = $5::jsonb,
					    trello_session_checks = $6::jsonb,
					    evaluator_id = $7,
					    updated_at = NOW()
					WHERE mentor_id = $1
					  AND class_key = $2
				`, mentorID, classKey, sessionQuality, studentsFeedback, string(sessionQualityJSON), string(trelloJSON), evaluatorID)
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
		       sent_to_mentor, sent_at, suggested_start_date, returned_at, updated_at,
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
		var sentAt, suggestedStartDate, returnedAt, hiddenAt sql.NullTime
		var hiddenBy sql.NullString

		err := rows.Scan(
			&g.ClassKey, &g.Level, &g.ClassDays, &g.ClassTime, &g.ClassNumber,
			&g.SentToMentor, &sentAt, &suggestedStartDate, &returnedAt, &g.UpdatedAt,
			&g.HiddenInOps, &hiddenAt, &hiddenBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class group: %w", err)
		}

		g.SentAt = sentAt
		g.SuggestedStartDate = suggestedStartDate
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
	if err := ensureClassMemberships(classKey); err != nil {
		return nil, err
	}

	currentSession, err := getClassCurrentSession(classKey)
	if err != nil {
		return nil, err
	}

	rows, err := db.DB.Query(`
		SELECT l.id, l.full_name, l.phone, COALESCE(l.is_returning, false), s.class_group_index, cm.joined_at_session_number
		FROM class_memberships cm
		INNER JOIN leads l ON l.id = cm.lead_id
		LEFT JOIN scheduling s ON s.lead_id = l.id
		WHERE cm.class_key = $1
		  AND cm.joined_at_session_number <= $2
		  AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= $2)
		  AND cm.removed_at IS NULL
		  AND l.status != 'cancelled'
		ORDER BY l.full_name
	`, classKey, currentSession)
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
	return GetStudentsInClassGroup(classKey)
}

// GetEligibleClassesForLateJoin returns classes a student is eligible to join as a late joiner.
// Standard rule: class must be same level, session <= 2, and current enrollment below 6.
// Manager override: allow classes beyond session 2 while keeping the same level/state/capacity checks.
// Eligibility includes:
// - active classes, and
// - sent_to_mentor + not_started classes (pre-start exception).
func GetEligibleClassesForLateJoin(leadID uuid.UUID) ([]*EligibleClass, error) {
	return getEligibleClassesForLateJoin(leadID, false)
}

func GetEligibleClassesForLateJoinWithManagerOverride(leadID uuid.UUID) ([]*EligibleClass, error) {
	return getEligibleClassesForLateJoin(leadID, true)
}

func getEligibleClassesForLateJoin(leadID uuid.UUID, allowBeyondSessionTwo bool) ([]*EligibleClass, error) {
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

		// Standard late join stops at session 2. Manager can override this cap.
		if !allowBeyondSessionTwo && ec.CurrentSession > 2 {
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

		// Filter by capacity (any roster below 6 students)
		if ec.CurrentEnrollment < 6 {
			eligible = append(eligible, ec)
		}
	}

	return eligible, nil
}

// AddLateJoiner adds a student to an active class group after the round has started.
// It updates scheduling, lead status, creates an audit record, and backfills N/A attendance.
func AddLateJoiner(leadID uuid.UUID, classKey string, reason string, userID uuid.UUID) error {
	return addLateJoiner(leadID, classKey, reason, userID, false)
}

func AddLateJoinerWithManagerOverride(leadID uuid.UUID, classKey string, reason string, userID uuid.UUID) error {
	return addLateJoiner(leadID, classKey, reason, userID, true)
}

func addLateJoiner(leadID uuid.UUID, classKey string, reason string, userID uuid.UUID, allowBeyondSessionTwo bool) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Standard late join stops at session 2. Manager can override this cap.
	var currentSession int32
	err = tx.QueryRow(`
		SELECT COUNT(*) + 1 
		FROM class_sessions 
		WHERE class_key = $1 AND status = 'completed'
	`, classKey).Scan(&currentSession)
	if err != nil {
		return fmt.Errorf("failed to get current session: %w", err)
	}
	if !allowBeyondSessionTwo && currentSession > 2 {
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

	// 2. Validate capacity (any roster below 6 students)
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
	if studentCount >= 6 {
		return fmt.Errorf("cannot join class: invalid capacity (current students: %d, required: fewer than 6)", studentCount)
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
		SET status = 'in_classes',
		    sent_to_classes = true,
		    mentor_head_return_reason = NULL,
		    updated_at = NOW()
		WHERE id = $1
	`, leadID)
	if err != nil {
		return fmt.Errorf("failed to update lead status: %w", err)
	}

	// 8. Add explicit membership window for the late join.
	_, err = tx.Exec(`
		INSERT INTO class_memberships (
			id,
			lead_id,
			class_key,
			joined_at_session_number,
			join_reason,
			added_by_user_id,
			created_at,
			updated_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, 'late_join', $4, NOW(), NOW())
		ON CONFLICT (lead_id, class_key, joined_at_session_number) DO UPDATE SET
			updated_at = EXCLUDED.updated_at
	`, leadID, classKey, currentSession, userID)
	if err != nil {
		return fmt.Errorf("failed to create late-join membership: %w", err)
	}

	// 9. Backfill 'N/A' attendance for past sessions
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

	// 10. Insert notifications.
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

	// 7. Delete late-join membership window.
	_, err = tx.Exec(`
		DELETE FROM class_memberships
		WHERE lead_id = $1
		  AND class_key = $2
		  AND joined_at_session_number = $3
		  AND join_reason = 'late_join'
	`, leadID, lj.ClassKey, lj.JoinedAtSessionNumber)
	if err != nil {
		return fmt.Errorf("failed to delete late-join membership: %w", err)
	}

	// 8. Delete late_joiners record
	_, err = tx.Exec(`DELETE FROM late_joiners WHERE lead_id = $1`, leadID)
	if err != nil {
		return fmt.Errorf("failed to delete audit record: %w", err)
	}

	return tx.Commit()
}

func GetEligibleTransferClasses(leadID uuid.UUID, sourceClassKey string) ([]*ClassTransferOption, error) {
	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transfer-options transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceRoundStatus string
	err = tx.QueryRow(`
		SELECT COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
	`, sourceClassKey).Scan(&sourceRoundStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("source class not found")
		}
		return nil, fmt.Errorf("failed to load source class: %w", err)
	}
	if sourceRoundStatus != "active" {
		return nil, fmt.Errorf("source class must be active")
	}

	sourceCurrentSession, err := getClassCurrentSessionTx(tx, sourceClassKey)
	if err != nil {
		return nil, err
	}
	if sourceCurrentSession > 8 {
		return nil, fmt.Errorf("source class has no future sessions")
	}

	if err := ensureClassMembershipsTx(tx, sourceClassKey); err != nil {
		return nil, err
	}
	sourceMembership, err := getApplicableMembershipTx(tx, leadID, sourceClassKey, sourceCurrentSession)
	if err != nil {
		return nil, err
	}
	if sourceMembership == nil {
		return nil, fmt.Errorf("student is not in the source class")
	}

	rows, err := tx.Query(`
		SELECT
			cg.class_key,
			cg.level,
			cg.class_days,
			cg.class_time,
			COALESCE(cg.class_number, 1),
			COALESCE(cg.round_status, 'not_started'),
			COALESCE(cg.sent_to_mentor, false),
			(SELECT COALESCE(COUNT(*), 0) + 1 FROM class_sessions cs WHERE cs.class_key = cg.class_key AND cs.status = 'completed') AS current_session
		FROM class_groups cg
		WHERE cg.class_key <> $1
		  AND COALESCE(cg.round_status, 'not_started') != 'closed'
		ORDER BY cg.level, cg.class_days, cg.class_time, COALESCE(cg.class_number, 1)
	`, sourceClassKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query transfer targets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type candidateTransferClass struct {
		option       *ClassTransferOption
		sentToMentor bool
	}

	var candidates []*candidateTransferClass
	for rows.Next() {
		option := &ClassTransferOption{}
		var sentToMentor bool
		if err := rows.Scan(
			&option.ClassKey,
			&option.Level,
			&option.ClassDays,
			&option.ClassTime,
			&option.ClassNumber,
			&option.RoundStatus,
			&sentToMentor,
			&option.CurrentSession,
		); err != nil {
			return nil, fmt.Errorf("failed to scan transfer target: %w", err)
		}
		if option.CurrentSession > 8 {
			continue
		}
		if option.RoundStatus != "active" && !(option.RoundStatus == "not_started" && sentToMentor) {
			continue
		}
		candidates = append(candidates, &candidateTransferClass{
			option:       option,
			sentToMentor: sentToMentor,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transfer targets: %w", err)
	}
	_ = rows.Close()

	var options []*ClassTransferOption
	for _, candidate := range candidates {
		if candidate == nil || candidate.option == nil {
			continue
		}
		if err := ensureClassMembershipsTx(tx, candidate.option.ClassKey); err != nil {
			return nil, err
		}
		count, err := getApplicableMembershipCountTx(tx, candidate.option.ClassKey, candidate.option.CurrentSession)
		if err != nil {
			return nil, err
		}
		if count >= 6 {
			continue
		}
		candidate.option.CurrentEnrollment = count
		options = append(options, candidate.option)
	}

	return options, nil
}

func TransferStudentBetweenActiveClasses(leadID uuid.UUID, sourceClassKey, targetClassKey, reason, notes string, userID uuid.UUID) (*ClassRosterChangeResult, error) {
	reason = strings.TrimSpace(reason)
	notes = strings.TrimSpace(notes)
	if targetClassKey == "" {
		return nil, fmt.Errorf("target class is required")
	}
	if sourceClassKey == targetClassKey {
		return nil, fmt.Errorf("source and target classes must be different")
	}
	switch reason {
	case "schedule_change", "promotion", "demotion", "other":
	default:
		return nil, fmt.Errorf("invalid transfer reason")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transfer transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceLevel int32
	var sourceRoundStatus string
	err = tx.QueryRow(`
		SELECT level, COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
	`, sourceClassKey).Scan(&sourceLevel, &sourceRoundStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("source class not found")
		}
		return nil, fmt.Errorf("failed to load source class: %w", err)
	}
	if sourceRoundStatus != "active" {
		return nil, fmt.Errorf("source class must be active")
	}

	sourceCurrentSession, err := getClassCurrentSessionTx(tx, sourceClassKey)
	if err != nil {
		return nil, err
	}
	if sourceCurrentSession > 8 {
		return nil, fmt.Errorf("source class has no future sessions")
	}

	if err := ensureClassMembershipsTx(tx, sourceClassKey); err != nil {
		return nil, err
	}
	sourceMembership, err := getApplicableMembershipTx(tx, leadID, sourceClassKey, sourceCurrentSession)
	if err != nil {
		return nil, err
	}
	if sourceMembership == nil {
		return nil, fmt.Errorf("student is not in the source class")
	}

	var targetLevel, targetClassNumber int32
	var targetClassDays, targetClassTime, targetRoundStatus string
	var targetSentToMentor bool
	err = tx.QueryRow(`
		SELECT
			level,
			class_days,
			class_time,
			COALESCE(class_number, 1),
			COALESCE(round_status, 'not_started'),
			COALESCE(sent_to_mentor, false)
		FROM class_groups
		WHERE class_key = $1
	`, targetClassKey).Scan(&targetLevel, &targetClassDays, &targetClassTime, &targetClassNumber, &targetRoundStatus, &targetSentToMentor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("target class not found")
		}
		return nil, fmt.Errorf("failed to load target class: %w", err)
	}
	if targetRoundStatus != "active" && !(targetRoundStatus == "not_started" && targetSentToMentor) {
		return nil, fmt.Errorf("target class must be active or sent to mentor")
	}

	switch reason {
	case "schedule_change":
		if targetLevel != sourceLevel {
			return nil, fmt.Errorf("schedule change must stay within the same level")
		}
	case "promotion":
		if targetLevel <= sourceLevel {
			return nil, fmt.Errorf("promotion requires a higher target level")
		}
	case "demotion":
		if targetLevel >= sourceLevel {
			return nil, fmt.Errorf("demotion requires a lower target level")
		}
	}

	targetCurrentSession, err := getClassCurrentSessionTx(tx, targetClassKey)
	if err != nil {
		return nil, err
	}
	if targetCurrentSession > 8 {
		return nil, fmt.Errorf("target class has no future sessions")
	}

	if err := ensureClassMembershipsTx(tx, targetClassKey); err != nil {
		return nil, err
	}
	targetEnrollment, err := getApplicableMembershipCountTx(tx, targetClassKey, targetCurrentSession)
	if err != nil {
		return nil, err
	}
	if targetEnrollment >= 6 {
		return nil, fmt.Errorf("target class is full")
	}

	now := time.Now()
	sourceExitAfter := sourceCurrentSession - 1

	_, err = tx.Exec(`
		UPDATE class_memberships
		SET left_after_session_number = $1,
		    leave_reason = $2,
		    removed_by_user_id = $3,
		    removed_at = $4,
		    updated_at = $4
		WHERE id = $5
	`, sourceExitAfter, reason, userID, now, sourceMembership.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to close source membership: %w", err)
	}

	var targetMembershipID uuid.UUID
	err = tx.QueryRow(`
		INSERT INTO class_memberships (
			id,
			lead_id,
			class_key,
			joined_at_session_number,
			level_consumed_at_session_number,
			join_reason,
			added_by_user_id,
			created_at,
			updated_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $7)
		ON CONFLICT (lead_id, class_key, joined_at_session_number) DO UPDATE SET
			level_consumed_at_session_number = COALESCE(class_memberships.level_consumed_at_session_number, EXCLUDED.level_consumed_at_session_number),
			left_after_session_number = NULL,
			leave_reason = NULL,
			removed_by_user_id = NULL,
			removed_at = NULL,
			updated_at = EXCLUDED.updated_at
		RETURNING id
	`, leadID, targetClassKey, targetCurrentSession, sourceMembership.LevelConsumedAtSession, reason, userID, now).Scan(&targetMembershipID)
	if err != nil {
		return nil, fmt.Errorf("failed to create target membership: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE scheduling
		SET class_days = $1,
		    class_time = $2,
		    class_group_index = $3,
		    updated_at = $4
		WHERE lead_id = $5
	`, targetClassDays, targetClassTime, targetClassNumber, now, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to update scheduling for transfer: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE placement_tests
		SET assigned_level = $1,
		    updated_at = $2
		WHERE lead_id = $3
	`, targetLevel, now, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to update assigned level for transfer: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE leads
		SET status = 'in_classes',
		    sent_to_classes = true,
		    ops_queue_reason = NULL,
		    mentor_head_return_reason = NULL,
		    updated_at = $1
		WHERE id = $2
	`, now, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to update lead after transfer: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO attendance (id, session_id, lead_id, status, notes, marked_by_user_id, created_at, updated_at)
		SELECT
			gen_random_uuid(),
			target_cs.id,
			$1,
			source_att.status,
			source_att.notes,
			source_att.marked_by_user_id,
			$4,
			$4
		FROM class_sessions source_cs
		INNER JOIN attendance source_att
			ON source_att.session_id = source_cs.id
		   AND source_att.lead_id = $1
		INNER JOIN class_sessions target_cs
			ON target_cs.class_key = $2
		   AND target_cs.session_number = source_cs.session_number
		WHERE source_cs.class_key = $5
		  AND source_cs.session_number <= $3
		  AND source_cs.session_number < $6
		ON CONFLICT (session_id, lead_id) DO UPDATE SET
			status = EXCLUDED.status,
			notes = EXCLUDED.notes,
			marked_by_user_id = EXCLUDED.marked_by_user_id,
			updated_at = EXCLUDED.updated_at
	`, leadID, targetClassKey, sourceExitAfter, now, sourceClassKey, targetCurrentSession)
	if err != nil {
		return nil, fmt.Errorf("failed to mirror transferred-student attendance: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO session_performance (id, class_session_id, lead_id, task_completed, participation_score, created_at, updated_at)
		SELECT
			gen_random_uuid(),
			target_cs.id,
			$1,
			source_perf.task_completed,
			source_perf.participation_score,
			$4,
			$4
		FROM class_sessions source_cs
		INNER JOIN session_performance source_perf
			ON source_perf.class_session_id = source_cs.id
		   AND source_perf.lead_id = $1
		INNER JOIN class_sessions target_cs
			ON target_cs.class_key = $2
		   AND target_cs.session_number = source_cs.session_number
		WHERE source_cs.class_key = $5
		  AND source_cs.session_number <= $3
		  AND source_cs.session_number < $6
		ON CONFLICT (class_session_id, lead_id) DO UPDATE SET
			task_completed = EXCLUDED.task_completed,
			participation_score = EXCLUDED.participation_score,
			updated_at = EXCLUDED.updated_at
	`, leadID, targetClassKey, sourceExitAfter, now, sourceClassKey, targetCurrentSession)
	if err != nil {
		return nil, fmt.Errorf("failed to mirror transferred-student performance: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO attendance (id, session_id, lead_id, status, created_at, updated_at)
		SELECT gen_random_uuid(), cs.id, $1, 'N/A', $4, $4
		FROM class_sessions cs
		WHERE cs.class_key = $2
		  AND cs.session_number < $3
		ON CONFLICT (session_id, lead_id) DO NOTHING
	`, leadID, targetClassKey, targetCurrentSession, now)
	if err != nil {
		return nil, fmt.Errorf("failed to backfill transferred-student attendance: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO class_transfers (
			id,
			lead_id,
			source_class_key,
			target_class_key,
			source_membership_id,
			target_membership_id,
			source_exit_after_session_number,
			target_joined_at_session_number,
			reason,
			notes,
			created_by_user_id,
			created_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11)
	`, leadID, sourceClassKey, targetClassKey, sourceMembership.ID, targetMembershipID, sourceExitAfter, targetCurrentSession, reason, notes, userID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to write class transfer audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ClassRosterChangeResult{
		LeadID:                       leadID,
		SourceClassKey:               sourceClassKey,
		TargetClassKey:               sql.NullString{String: targetClassKey, Valid: true},
		SourceExitAfterSessionNumber: sourceExitAfter,
		TargetJoinedAtSessionNumber:  sql.NullInt32{Int32: targetCurrentSession, Valid: true},
		Reason:                       reason,
	}, nil
}

func ReturnStudentToAdminFromClass(leadID uuid.UUID, sourceClassKey, reason, notes string, userID uuid.UUID) (*ClassRosterChangeResult, error) {
	reason = strings.TrimSpace(reason)
	notes = strings.TrimSpace(notes)
	if reason != "refund_to_admin" && reason != "private_track_to_admin" && reason != "other_to_admin" {
		return nil, fmt.Errorf("invalid admin-return reason")
	}
	if reason == "other_to_admin" && notes == "" {
		return nil, fmt.Errorf("notes are required for the 'other' admin-return reason")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin admin-return transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceRoundStatus string
	err = tx.QueryRow(`
		SELECT COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
	`, sourceClassKey).Scan(&sourceRoundStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("source class not found")
		}
		return nil, fmt.Errorf("failed to load source class: %w", err)
	}
	if sourceRoundStatus != "active" {
		return nil, fmt.Errorf("source class must be active")
	}

	sourceCurrentSession, err := getClassCurrentSessionTx(tx, sourceClassKey)
	if err != nil {
		return nil, err
	}
	if sourceCurrentSession > 8 {
		return nil, fmt.Errorf("source class has no future sessions")
	}

	if err := ensureClassMembershipsTx(tx, sourceClassKey); err != nil {
		return nil, err
	}
	sourceMembership, err := getApplicableMembershipTx(tx, leadID, sourceClassKey, sourceCurrentSession)
	if err != nil {
		return nil, err
	}
	if sourceMembership == nil {
		return nil, fmt.Errorf("student is not in the source class")
	}

	now := time.Now()
	sourceExitAfter := sourceCurrentSession - 1
	queueReason := sql.NullString{String: "refund_review", Valid: true}
	newStatus := "ready_to_start"
	mentorHeadReturnReason := sql.NullString{}
	if reason == "private_track_to_admin" {
		queueReason = sql.NullString{String: "private_track", Valid: true}
		newStatus = "waiting_for_round"
	} else if reason == "other_to_admin" {
		queueReason = sql.NullString{}
		newStatus = "ready_to_start"
		mentorHeadReturnReason = sql.NullString{String: "class_return", Valid: true}
	}

	_, err = tx.Exec(`
		UPDATE class_memberships
		SET left_after_session_number = $1,
		    leave_reason = $2,
		    removed_by_user_id = $3,
		    removed_at = $4,
		    updated_at = $4
		WHERE id = $5
	`, sourceExitAfter, reason, userID, now, sourceMembership.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to close source membership: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE scheduling
		SET class_group_index = NULL,
		    updated_at = $1
		WHERE lead_id = $2
	`, now, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to detach scheduling on admin return: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE leads
		SET status = $1,
		    sent_to_classes = false,
		    ops_queue_reason = $2,
		    mentor_head_return_reason = $3,
		    updated_at = $4
		WHERE id = $5
	`, newStatus, queueReason, mentorHeadReturnReason, now, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to update lead for admin return: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO class_transfers (
			id,
			lead_id,
			source_class_key,
			source_membership_id,
			source_exit_after_session_number,
			reason,
			notes,
			created_by_user_id,
			created_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8)
	`, leadID, sourceClassKey, sourceMembership.ID, sourceExitAfter, reason, notes, userID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to write admin-return audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ClassRosterChangeResult{
		LeadID:                       leadID,
		SourceClassKey:               sourceClassKey,
		SourceExitAfterSessionNumber: sourceExitAfter,
		Reason:                       reason,
		OpsQueueReason:               queueReason,
	}, nil
}

func ReturnStudentToAdminAsEarlyRepeat(leadID uuid.UUID, sourceClassKey, notes string, userID uuid.UUID) (*ClassRosterChangeResult, error) {
	notes = strings.TrimSpace(notes)

	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin early-repeat transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var sourceRoundStatus string
	err = tx.QueryRow(`
		SELECT COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
	`, sourceClassKey).Scan(&sourceRoundStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("source class not found")
		}
		return nil, fmt.Errorf("failed to load source class: %w", err)
	}
	if sourceRoundStatus != "active" {
		return nil, fmt.Errorf("source class must be active")
	}

	sourceCurrentSession, err := getClassCurrentSessionTx(tx, sourceClassKey)
	if err != nil {
		return nil, err
	}
	if sourceCurrentSession > 8 {
		return nil, fmt.Errorf("source class has no future sessions")
	}

	if err := ensureClassMembershipsTx(tx, sourceClassKey); err != nil {
		return nil, err
	}
	sourceMembership, err := getApplicableMembershipTx(tx, leadID, sourceClassKey, sourceCurrentSession)
	if err != nil {
		return nil, err
	}
	if sourceMembership == nil {
		return nil, fmt.Errorf("student is not in the source class")
	}

	sourceExitAfter := sourceCurrentSession - 1

	var absenceCount int
	err = tx.QueryRow(`
		SELECT COUNT(*)
		FROM attendance a
		INNER JOIN class_sessions cs ON cs.id = a.session_id
		WHERE a.lead_id = $1
		  AND cs.class_key = $2
		  AND cs.session_number >= $3
		  AND cs.session_number <= $4
		  AND UPPER(COALESCE(a.status, '')) = 'ABSENT'
	`, leadID, sourceClassKey, sourceMembership.JoinedAtSessionNumber, sourceExitAfter).Scan(&absenceCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count absences for early repeat: %w", err)
	}
	if absenceCount <= 2 {
		return nil, fmt.Errorf("student must have more than 2 missed sessions for early repeat")
	}

	var level int32
	var classDays, classTime string
	var mentorName sql.NullString
	err = tx.QueryRow(`
		SELECT cg.level, cg.class_days, cg.class_time, u.email
		FROM class_groups cg
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id::uuid
		WHERE cg.class_key = $1
	`, sourceClassKey).Scan(&level, &classDays, &classTime, &mentorName)
	if err != nil {
		return nil, fmt.Errorf("failed to get class details for early repeat: %w", err)
	}

	var currentLevel sql.NullInt32
	err = tx.QueryRow(`SELECT assigned_level FROM placement_tests WHERE lead_id = $1`, leadID).Scan(&currentLevel)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to get assigned level for early repeat: %w", err)
	}
	if !currentLevel.Valid {
		currentLevel = sql.NullInt32{Int32: level, Valid: true}
	}

	var purchased, consumed sql.NullInt32
	err = tx.QueryRow(`
		SELECT COALESCE(levels_purchased_total, 0), COALESCE(levels_consumed, 0)
		FROM leads
		WHERE id = $1
	`, leadID).Scan(&purchased, &consumed)
	if err != nil {
		return nil, fmt.Errorf("failed to load credits for early repeat: %w", err)
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

	newStatus := "renewal_pending"
	if creditsRemaining > 0 {
		newStatus = "waiting_for_round"
	}
	highPriorityFollowUp := newStatus == "renewal_pending"
	now := time.Now()

	_, err = tx.Exec(`
		UPDATE class_memberships
		SET left_after_session_number = $1,
		    leave_reason = 'early_repeat_absence',
		    removed_by_user_id = $2,
		    removed_at = $3,
		    updated_at = $3
		WHERE id = $4
	`, sourceExitAfter, userID, now, sourceMembership.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to close source membership for early repeat: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE scheduling
		SET class_group_index = NULL,
		    updated_at = $1
		WHERE lead_id = $2
	`, now, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to detach scheduling for early repeat: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE leads
		SET remaining_credits = $1,
		    status = $2,
		    is_returning = true,
		    high_priority_follow_up = $5,
		    high_priority = false,
		    high_priority_reason = '',
		    sent_to_classes = false,
		    ops_queue_reason = NULL,
		    mentor_head_return_reason = 'early_repeat_absence',
		    updated_at = $3
		WHERE id = $4
	`, creditsRemaining, newStatus, now, leadID, highPriorityFollowUp)
	if err != nil {
		return nil, fmt.Errorf("failed to update lead for early repeat: %w", err)
	}

	_, err = tx.Exec(`DELETE FROM payments WHERE lead_id = $1`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear payment snapshot for early repeat: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM offers WHERE lead_id = $1`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to clear offer snapshot for early repeat: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO class_enrollments (
			lead_id, class_key, level, class_days, class_time, mentor_name,
			final_grade, outcome, enrolled_at, completed_at
		) VALUES ($1, $2, $3, $4, $5, $6, NULL, 'repeated', $7, $8)
		ON CONFLICT (lead_id, class_key) DO UPDATE SET
			final_grade = EXCLUDED.final_grade,
			outcome = EXCLUDED.outcome,
			completed_at = EXCLUDED.completed_at
	`, leadID, sourceClassKey, currentLevel.Int32, classDays, classTime, mentorName, sourceMembership.CreatedAt, now)
	if err != nil {
		return nil, fmt.Errorf("failed to snapshot early repeat enrollment: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO class_transfers (
			id,
			lead_id,
			source_class_key,
			source_membership_id,
			source_exit_after_session_number,
			reason,
			notes,
			created_by_user_id,
			created_at
		)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 'early_repeat_absence_to_admin', NULLIF($5, ''), $6, $7)
	`, leadID, sourceClassKey, sourceMembership.ID, sourceExitAfter, notes, userID, now)
	if err != nil {
		return nil, fmt.Errorf("failed to write early-repeat transfer audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &ClassRosterChangeResult{
		LeadID:                       leadID,
		SourceClassKey:               sourceClassKey,
		SourceExitAfterSessionNumber: sourceExitAfter,
		Reason:                       "early_repeat_absence_to_admin",
	}, nil
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
	var classDays string
	err = tx.QueryRow(`
		SELECT class_days
		FROM class_groups
		WHERE class_key = $1
	`, classKey).Scan(&classDays)
	if err != nil {
		return fmt.Errorf("failed to load class days: %w", err)
	}
	if allowedWeekdays, ok := allowedRoundStartWeekdays(classDays); ok {
		normalizedStart := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
		for !containsWeekday(allowedWeekdays, normalizedStart.Weekday()) {
			normalizedStart = normalizedStart.AddDate(0, 0, 1)
		}
		startDate = normalizedStart
	}
	sessionDates, err := BuildClassSessionDates(classDays, startDate, 8)
	if err != nil {
		return err
	}

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
		sessionDate := sessionDates[i-1]
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
		SET status = 'in_classes',
		    mentor_head_return_reason = NULL,
		    updated_at = NOW()
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
		  AND l.status IN ('ready_to_start', 'schedule_assigned', 'waiting_for_round')
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
	`
	args := []interface{}{classKey}
	argIdx := 2

	if filter != "" && filter != "all" {
		switch filter {
		case "unresolved":
			query += " AND (f.resolved IS NULL OR f.resolved = false)"
		case "resolved":
			query += " AND f.resolved = true"
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
	followUpIDs := make([]uuid.UUID, 0)
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
			followUpIDs = append(followUpIDs, fid)
		}

		results = append(results, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	notesByCaseID, err := getFollowUpNotesByCaseIDs(followUpIDs)
	if err != nil {
		return nil, err
	}
	for _, item := range results {
		if item.FollowUp != nil {
			item.FollowUp.Notes = notesByCaseID[item.FollowUp.ID]
		}
	}

	return results, nil
}

// CreateFollowUp creates or updates a follow-up note
func CreateFollowUp(classKey string, leadID uuid.UUID, sessionNumber int, note string, status string, createdBy uuid.UUID) error {
	standardizedStatus := normalizeFollowUpStatus(status)
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var followUpID uuid.UUID
	var previousStatus sql.NullString
	err = tx.QueryRow(`
		SELECT id, status
		FROM followups
		WHERE class_key = $1 AND lead_id = $2 AND session_number = $3 AND deleted_at IS NULL
	`, classKey, leadID, sessionNumber).Scan(&followUpID, &previousStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.QueryRow(`
			INSERT INTO followups (class_key, lead_id, session_number, note, status, created_by, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
			RETURNING id
		`, classKey, leadID, sessionNumber, note, standardizedStatus, createdBy).Scan(&followUpID); err != nil {
			return err
		}
		if err := writeFollowUpAuditNotesTx(tx, followUpID, standardizedStatus, note, false, true, createdBy); err != nil {
			return err
		}
		return tx.Commit()
	}

	_, err = tx.Exec(`
		UPDATE followups
		SET note = $1, status = $2, created_by = $3, updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL
	`, note, standardizedStatus, createdBy, followUpID)
	if err != nil {
		return err
	}
	if err := writeFollowUpAuditNotesTx(tx, followUpID, standardizedStatus, note, false, previousStatus.String != standardizedStatus, createdBy); err != nil {
		return err
	}
	return tx.Commit()
}

// ResolveFollowUp marks a follow-up as resolved
func ResolveFollowUp(id uuid.UUID, resolvedBy uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`
		UPDATE followups 
		SET resolved = true, resolved_at = NOW(), resolved_by_user_id = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, resolvedBy, id)
	if err != nil {
		return err
	}
	if err := CreateFollowUpNoteTx(tx, id, "Case resolved", "resolution", resolvedBy); err != nil {
		return err
	}
	return tx.Commit()
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
	standardizedStatus := normalizeFollowUpStatus(status)
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var previousStatus sql.NullString
	err = tx.QueryRow(`SELECT status FROM followups WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&previousStatus)
	if err != nil {
		return err
	}

	if resolved {
		_, err = tx.Exec(`
			UPDATE followups 
			SET status = $1, note = $2, resolved = true, resolved_at = NOW(), resolved_by_user_id = $3, updated_at = NOW()
			WHERE id = $4 AND deleted_at IS NULL
		`, standardizedStatus, note, userID, id)
	} else {
		_, err = tx.Exec(`
			UPDATE followups 
			SET status = $1, note = $2, updated_at = NOW()
			WHERE id = $3 AND deleted_at IS NULL
		`, standardizedStatus, note, id)
	}
	if err != nil {
		return err
	}
	if err := writeFollowUpAuditNotesTx(tx, id, standardizedStatus, note, resolved, previousStatus.String != standardizedStatus, userID); err != nil {
		return err
	}
	return tx.Commit()
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
	followUpIDs := make([]uuid.UUID, 0)
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
		followUpIDs = append(followUpIDs, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	notesByCaseID, err := getFollowUpNotesByCaseIDs(followUpIDs)
	if err != nil {
		return nil, err
	}
	for _, item := range results {
		item.Notes = notesByCaseID[item.ID]
	}
	return results, nil
}

// ResolveAbsence marks an absence as resolved, creating a follow-up record if necessary
func ResolveAbsence(classKey string, leadID uuid.UUID, sessionNumber int, note string, status string, resolvedBy uuid.UUID) error {
	standardizedStatus := normalizeFollowUpStatus(status)
	if standardizedStatus == "NOT_CONTACTED" {
		standardizedStatus = "RESOLVED"
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var followUpID uuid.UUID
	var previousStatus sql.NullString
	err = tx.QueryRow(`
		SELECT id, status
		FROM followups
		WHERE class_key = $1 AND lead_id = $2 AND session_number = $3 AND deleted_at IS NULL
	`, classKey, leadID, sessionNumber).Scan(&followUpID, &previousStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.QueryRow(`
			INSERT INTO followups (class_key, lead_id, session_number, note, status, created_by, updated_at, resolved, resolved_at, resolved_by_user_id)
			VALUES ($1, $2, $3, $4, $5, $6, NOW(), true, NOW(), $6)
			RETURNING id
		`, classKey, leadID, sessionNumber, note, standardizedStatus, resolvedBy).Scan(&followUpID); err != nil {
			return err
		}
		if err := writeFollowUpAuditNotesTx(tx, followUpID, standardizedStatus, note, true, true, resolvedBy); err != nil {
			return err
		}
		return tx.Commit()
	}

	_, err = tx.Exec(`
		UPDATE followups
		SET note = $1,
		    status = $2,
		    resolved = true,
		    resolved_at = NOW(),
		    resolved_by_user_id = $3,
		    updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL
	`, note, standardizedStatus, resolvedBy, followUpID)
	if err != nil {
		return err
	}
	if err := writeFollowUpAuditNotesTx(tx, followUpID, standardizedStatus, note, true, previousStatus.String != standardizedStatus, resolvedBy); err != nil {
		return err
	}
	return tx.Commit()
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

func CreateFollowUpNoteTx(tx *sql.Tx, caseID uuid.UUID, noteText, noteType string, userID uuid.UUID) error {
	_, err := tx.Exec(`
		INSERT INTO followup_case_notes (case_id, note_text, note_type, created_by_user_id)
		VALUES ($1, $2, $3, $4)
	`, caseID, noteText, noteType, userID)
	return err
}

func writeFollowUpAuditNotesTx(tx *sql.Tx, caseID uuid.UUID, status, note string, resolved bool, statusChanged bool, userID uuid.UUID) error {
	if statusChanged {
		if err := CreateFollowUpNoteTx(tx, caseID, "Status changed to: "+status, "status_change", userID); err != nil {
			return err
		}
	}
	if strings.TrimSpace(note) != "" {
		noteType := "comment"
		if resolved {
			noteType = "resolution"
		}
		if err := CreateFollowUpNoteTx(tx, caseID, note, noteType, userID); err != nil {
			return err
		}
	} else if resolved {
		if err := CreateFollowUpNoteTx(tx, caseID, "Case resolved", "resolution", userID); err != nil {
			return err
		}
	}
	return nil
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

func getFollowUpNotesByCaseIDs(caseIDs []uuid.UUID) (map[uuid.UUID][]*FollowUpCaseNote, error) {
	if len(caseIDs) == 0 {
		return map[uuid.UUID][]*FollowUpCaseNote{}, nil
	}

	rows, err := db.DB.Query(`
		SELECT fcn.id, fcn.case_id, fcn.note_text, fcn.note_type, fcn.created_at,
		       fcn.created_by_user_id, COALESCE(u.email, '') as created_by_email
		FROM followup_case_notes fcn
		LEFT JOIN users u ON u.id = fcn.created_by_user_id
		WHERE fcn.case_id = ANY($1)
		ORDER BY fcn.created_at DESC
	`, pq.Array(caseIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query follow-up notes by case ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	notesByCaseID := make(map[uuid.UUID][]*FollowUpCaseNote)
	for rows.Next() {
		note := &FollowUpCaseNote{}
		err := rows.Scan(
			&note.ID,
			&note.CaseID,
			&note.NoteText,
			&note.NoteType,
			&note.CreatedAt,
			&note.CreatedByUserID,
			&note.CreatedByEmail,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan follow-up note: %w", err)
		}
		notesByCaseID[note.CaseID] = append(notesByCaseID[note.CaseID], note)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return notesByCaseID, nil
}

func gradeMirrorNoteID(gradeID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("grade-note:"+gradeID.String()))
}

func validateFinalGradeNotes(notes string) error {
	if len(strings.Fields(strings.TrimSpace(notes))) < 10 {
		return fmt.Errorf("final grading comment must be at least 10 words")
	}
	return nil
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
	if err := validateFinalGradeNotes(notes); err != nil {
		return uuid.Nil, err
	}

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
	if err := validateFinalGradeNotes(notes); err != nil {
		return err
	}

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
// Daily reports become ready at 02:00 Africa/Cairo on the next day.
func LatestReadyDailyReportWindow(now time.Time) (time.Time, time.Time) {
	loc := util.CairoLocation()
	cairoNow := now.In(loc)
	today := time.Date(cairoNow.Year(), cairoNow.Month(), cairoNow.Day(), 0, 0, 0, 0, loc)
	reportDate := today.AddDate(0, 0, -1)
	readyAt := time.Date(cairoNow.Year(), cairoNow.Month(), cairoNow.Day(), 2, 0, 0, 0, loc)
	if cairoNow.Before(readyAt) {
		reportDate = reportDate.AddDate(0, 0, -1)
		readyAt = readyAt.AddDate(0, 0, -1)
	}
	return reportDate, readyAt
}

// GetDailyReportPayload builds the Mentor Head/Manager daily report for a date.
func GetDailyReportPayload(inputDate time.Time, rankingFrom, rankingTo *time.Time) (*DailyReportPayload, error) {
	normalizedDate := util.CairoStartOfDay(inputDate)
	var normalizedRankingFrom *time.Time
	var normalizedRankingTo *time.Time
	if rankingFrom != nil {
		value := util.CairoStartOfDay(*rankingFrom)
		normalizedRankingFrom = &value
	}
	if rankingTo != nil {
		value := util.CairoStartOfDay(*rankingTo)
		normalizedRankingTo = &value
	}
	if normalizedRankingFrom != nil && normalizedRankingTo != nil && normalizedRankingTo.Before(*normalizedRankingFrom) {
		normalizedRankingFrom, normalizedRankingTo = normalizedRankingTo, normalizedRankingFrom
	}
	readyAt := normalizedDate.AddDate(0, 0, 1).Add(2 * time.Hour)

	now := util.CairoNow()

	hasRescheduleAudit, err := managerOpsHasRescheduleAudit()
	if err != nil {
		return nil, err
	}

	query := `
		SELECT cs.id, cs.class_key, cs.session_number, cs.scheduled_date,
		       COALESCE(cs.scheduled_time::TEXT, '') AS scheduled_time,
		       COALESCE(cs.actual_time::TEXT, '') AS actual_time,
		       cs.status,
		       ma.mentor_user_id::TEXT,
		       COALESCE(NULLIF(TRIM(u.full_name), ''), u.email, 'Unassigned') AS mentor_name,
		       COALESCE(u.email, 'Unassigned') AS mentor_email,
		       cg.level, cg.class_days, cg.class_time, cg.class_number,
		       (msc.id IS NOT NULL) AS compliance_checked,
		       COALESCE(msc.delay_minutes, 0) AS delay_minutes,
		       COALESCE(msc.is_absent, false) AS mentor_absent,
		       COALESCE(att.marked_count, 0) AS attendance_marked,
		       COALESCE(att.attended_count, 0) AS attended_students,
		       COALESCE(att.absent_count, 0) AS absent_students`
	if hasRescheduleAudit {
		query += `,
		       (latest_reschedule.id IS NOT NULL) AS was_rescheduled,
		       COALESCE(latest_reschedule.old_scheduled_date::TEXT, '') AS previous_date,
		       COALESCE(latest_reschedule.old_scheduled_time::TEXT, '') AS previous_time,
		       COALESCE(latest_reschedule.created_at::TEXT, '') AS rescheduled_at,
		       COALESCE(NULLIF(TRIM(changed_by.full_name), ''), changed_by.email, '') AS rescheduled_by`
	} else {
		query += `,
		       false AS was_rescheduled,
		       '' AS previous_date,
		       '' AS previous_time,
		       '' AS rescheduled_at,
		       '' AS rescheduled_by`
	}
	query += `
		FROM class_sessions cs
		INNER JOIN class_groups cg ON cg.class_key = cs.class_key
		LEFT JOIN mentor_assignments ma ON ma.class_key = cs.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		LEFT JOIN mentor_session_checks msc ON msc.class_session_id = cs.id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*)::int AS marked_count,
				COUNT(*) FILTER (WHERE a.status IN ('PRESENT', 'LATE'))::int AS attended_count,
				COUNT(*) FILTER (WHERE a.status = 'ABSENT')::int AS absent_count
			FROM attendance a
			WHERE a.session_id = cs.id
		) att ON true`
	if hasRescheduleAudit {
		query += `
		LEFT JOIN LATERAL (
			SELECT csr.id, csr.old_scheduled_date, csr.old_scheduled_time, csr.created_at, csr.changed_by_user_id
			FROM class_session_reschedules csr
			WHERE csr.class_session_id = cs.id
			ORDER BY csr.created_at DESC
			LIMIT 1
		) latest_reschedule ON true
		LEFT JOIN users changed_by ON changed_by.id = latest_reschedule.changed_by_user_id`
	}
	query += `
		WHERE cs.scheduled_date = $1
		  AND COALESCE(cg.round_status, 'not_started') = 'active'
		ORDER BY cs.scheduled_time, cg.level, cg.class_number, cs.class_key
	`

	rows, err := db.DB.Query(query, normalizedDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to query daily report sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	report := &DailyReportPayload{
		ReportDate:  normalizedDate.Format("2006-01-02"),
		ReadyAt:     readyAt.Format(time.RFC3339),
		GeneratedAt: now.Format(time.RFC3339),
		SessionRows: []*ManagerOpsSessionRow{},
	}
	if normalizedRankingFrom != nil {
		report.RankingFrom = normalizedRankingFrom.Format("2006-01-02")
	}
	if normalizedRankingTo != nil {
		report.RankingTo = normalizedRankingTo.Format("2006-01-02")
	}

	for rows.Next() {
		var mentorIDStr sql.NullString
		var level, classNumber int32
		var classDays, classTime string
		var scheduledDate time.Time
		var previousDate string
		var previousTime string
		var rescheduledAt string
		var rescheduledBy string
		row := &ManagerOpsSessionRow{}

		if err := rows.Scan(
			&row.SessionID,
			&row.ClassKey,
			&row.SessionNumber,
			&scheduledDate,
			&row.ScheduledTime,
			&row.ActualTime,
			&row.SessionStatus,
			&mentorIDStr,
			&row.MentorName,
			&row.MentorEmail,
			&level,
			&classDays,
			&classTime,
			&classNumber,
			&row.ComplianceChecked,
			&row.DelayMinutes,
			&row.MentorAbsent,
			&row.AttendanceMarked,
			&row.AttendedStudents,
			&row.AbsentStudents,
			&row.WasRescheduled,
			&previousDate,
			&previousTime,
			&rescheduledAt,
			&rescheduledBy,
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
		row.ExpectedStudents = expected
		row.AttendanceStatus = computeManagerAttendanceStatus(expected, row.AttendanceMarked)
		row.SessionPhase = computeManagerSessionPhase(normalizedDate, row.ScheduledTime, row.SessionStatus, now)
		row.MentorStatus = computeManagerMentorStatus(row.ComplianceChecked, row.MentorAbsent, row.DelayMinutes)
		row.PreviousDate = previousDate
		row.PreviousTime = previousTime
		row.RescheduledAt = rescheduledAt
		row.RescheduledBy = rescheduledBy

		report.ClassesScheduled++
		report.ExpectedStudents += row.ExpectedStudents
		report.AbsentStudents += row.AbsentStudents
		if row.SessionStatus == "completed" {
			report.ClassesTaught++
		} else {
			report.ClassesMissingReport++
		}
		if row.SessionPhase == "live_now" {
			report.SessionsLiveNow++
		}
		report.SessionRows = append(report.SessionRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate daily report sessions: %w", err)
	}

	sort.SliceStable(report.SessionRows, func(i, j int) bool {
		left := managerSessionPhasePriority(report.SessionRows[i].SessionPhase)
		right := managerSessionPhasePriority(report.SessionRows[j].SessionPhase)
		if left != right {
			return left < right
		}
		if report.SessionRows[i].ScheduledTime != report.SessionRows[j].ScheduledTime {
			return report.SessionRows[i].ScheduledTime < report.SessionRows[j].ScheduledTime
		}
		return report.SessionRows[i].ClassKey < report.SessionRows[j].ClassKey
	})

	studentsInClassesCount, _, err := getManagerOpsLeadCounts()
	if err != nil {
		return nil, err
	}
	report.StudentsInClassesCount = studentsInClassesCount
	report.AbsentStudentsRanking, report.LateStartsRanking, report.StudentsOverAbsenceRanking, err = getActiveClassRankingSummary(normalizedRankingFrom, normalizedRankingTo, 2)
	if err != nil {
		return nil, err
	}

	return report, nil
}

// GetManagerOpsPayload builds a manager-only operational view for one Cairo business day.
func GetManagerOpsPayload(inputDate time.Time, rankingFrom, rankingTo *time.Time) (*ManagerOpsPayload, error) {
	reportDate := util.CairoStartOfDay(inputDate)
	var normalizedRankingFrom *time.Time
	var normalizedRankingTo *time.Time
	if rankingFrom != nil {
		value := util.CairoStartOfDay(*rankingFrom)
		normalizedRankingFrom = &value
	}
	if rankingTo != nil {
		value := util.CairoStartOfDay(*rankingTo)
		normalizedRankingTo = &value
	}
	if normalizedRankingFrom != nil && normalizedRankingTo != nil && normalizedRankingTo.Before(*normalizedRankingFrom) {
		normalizedRankingFrom, normalizedRankingTo = normalizedRankingTo, normalizedRankingFrom
	}
	now := util.CairoNow()

	hasRescheduleAudit, err := managerOpsHasRescheduleAudit()
	if err != nil {
		return nil, err
	}

	query := `
		SELECT cs.id, cs.class_key, cs.session_number, cs.scheduled_date,
		       COALESCE(cs.scheduled_time::TEXT, '') AS scheduled_time,
		       COALESCE(cs.actual_time::TEXT, '') AS actual_time,
		       cs.status,
		       ma.mentor_user_id::TEXT,
		       COALESCE(NULLIF(TRIM(u.full_name), ''), u.email, 'Unassigned') AS mentor_name,
		       COALESCE(u.email, 'Unassigned') AS mentor_email,
		       cg.level, cg.class_days, cg.class_time, cg.class_number,
		       (msc.id IS NOT NULL) AS compliance_checked,
		       COALESCE(msc.delay_minutes, 0) AS delay_minutes,
		       COALESCE(msc.is_absent, false) AS mentor_absent,
		       COALESCE(att.marked_count, 0) AS attendance_marked,
		       COALESCE(att.attended_count, 0) AS attended_students,
		       COALESCE(att.absent_count, 0) AS absent_students`
	if hasRescheduleAudit {
		query += `,
		       (latest_reschedule.id IS NOT NULL) AS was_rescheduled,
		       COALESCE(latest_reschedule.old_scheduled_date::TEXT, '') AS previous_date,
		       COALESCE(latest_reschedule.old_scheduled_time::TEXT, '') AS previous_time,
		       COALESCE(latest_reschedule.created_at::TEXT, '') AS rescheduled_at,
		       COALESCE(NULLIF(TRIM(changed_by.full_name), ''), changed_by.email, '') AS rescheduled_by`
	} else {
		query += `,
		       false AS was_rescheduled,
		       '' AS previous_date,
		       '' AS previous_time,
		       '' AS rescheduled_at,
		       '' AS rescheduled_by`
	}
	query += `
		FROM class_sessions cs
		INNER JOIN class_groups cg ON cg.class_key = cs.class_key
		LEFT JOIN mentor_assignments ma ON ma.class_key = cs.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		LEFT JOIN mentor_session_checks msc ON msc.class_session_id = cs.id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*)::int AS marked_count,
				COUNT(*) FILTER (WHERE a.status IN ('PRESENT', 'LATE'))::int AS attended_count,
				COUNT(*) FILTER (WHERE a.status = 'ABSENT')::int AS absent_count
			FROM attendance a
			WHERE a.session_id = cs.id
		) att ON true`
	if hasRescheduleAudit {
		query += `
		LEFT JOIN LATERAL (
			SELECT csr.id, csr.old_scheduled_date, csr.old_scheduled_time, csr.created_at, csr.changed_by_user_id
			FROM class_session_reschedules csr
			WHERE csr.class_session_id = cs.id
			ORDER BY csr.created_at DESC
			LIMIT 1
		) latest_reschedule ON true
		LEFT JOIN users changed_by ON changed_by.id = latest_reschedule.changed_by_user_id`
	}
	query += `
		WHERE cs.scheduled_date = $1
		  AND COALESCE(cg.round_status, 'not_started') = 'active'
		ORDER BY cs.scheduled_time, cg.level, cg.class_number, cs.class_key
	`

	rows, err := db.DB.Query(query, reportDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to query manager ops sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	payload := &ManagerOpsPayload{
		ReportDate:    reportDate.Format("2006-01-02"),
		Timezone:      util.CairoTimeZone,
		GeneratedAt:   now.Format(time.RFC3339),
		WeeklySummary: ManagerOpsWeeklySummary{},
		SessionRows:   []*ManagerOpsSessionRow{},
	}
	if normalizedRankingFrom != nil {
		payload.RankingFrom = normalizedRankingFrom.Format("2006-01-02")
	}
	if normalizedRankingTo != nil {
		payload.RankingTo = normalizedRankingTo.Format("2006-01-02")
	}

	for rows.Next() {
		var mentorIDStr sql.NullString
		var scheduledDate time.Time
		var level, classNumber int32
		var classDays, classTime string
		var previousDate string
		var previousTime string
		var rescheduledAt string
		var rescheduledBy string

		row := &ManagerOpsSessionRow{}
		if err := rows.Scan(
			&row.SessionID,
			&row.ClassKey,
			&row.SessionNumber,
			&scheduledDate,
			&row.ScheduledTime,
			&row.ActualTime,
			&row.SessionStatus,
			&mentorIDStr,
			&row.MentorName,
			&row.MentorEmail,
			&level,
			&classDays,
			&classTime,
			&classNumber,
			&row.ComplianceChecked,
			&row.DelayMinutes,
			&row.MentorAbsent,
			&row.AttendanceMarked,
			&row.AttendedStudents,
			&row.AbsentStudents,
			&row.WasRescheduled,
			&previousDate,
			&previousTime,
			&rescheduledAt,
			&rescheduledBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan manager ops session: %w", err)
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
		row.ExpectedStudents = expected
		row.AttendanceStatus = computeManagerAttendanceStatus(expected, row.AttendanceMarked)
		row.SessionPhase = computeManagerSessionPhase(reportDate, row.ScheduledTime, row.SessionStatus, now)
		row.MentorStatus = computeManagerMentorStatus(row.ComplianceChecked, row.MentorAbsent, row.DelayMinutes)
		row.PreviousDate = previousDate
		row.PreviousTime = previousTime
		row.RescheduledAt = rescheduledAt
		row.RescheduledBy = rescheduledBy

		payload.Summary.SessionsScheduled++
		payload.Summary.ExpectedStudents += row.ExpectedStudents
		payload.Summary.AttendedStudents += row.AttendedStudents
		if row.SessionStatus == "completed" {
			payload.Summary.SessionsCompleted++
		}
		if row.SessionPhase == "live_now" {
			payload.Summary.SessionsLiveNow++
		}
		if row.AttendanceStatus == "done" || row.AttendanceStatus == "none_expected" {
			payload.Summary.SessionsAttendanceDone++
		} else {
			payload.Summary.SessionsAttendancePending++
		}
		switch row.MentorStatus {
		case "late":
			payload.Summary.LateMentorSessions++
		case "absent":
			payload.Summary.AbsentMentorSessions++
		case "not_checked":
			payload.Summary.UncheckedMentorSessions++
		}

		payload.SessionRows = append(payload.SessionRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate manager ops sessions: %w", err)
	}

	sort.SliceStable(payload.SessionRows, func(i, j int) bool {
		left := managerSessionPhasePriority(payload.SessionRows[i].SessionPhase)
		right := managerSessionPhasePriority(payload.SessionRows[j].SessionPhase)
		if left != right {
			return left < right
		}
		if payload.SessionRows[i].ScheduledTime != payload.SessionRows[j].ScheduledTime {
			return payload.SessionRows[i].ScheduledTime < payload.SessionRows[j].ScheduledTime
		}
		return payload.SessionRows[i].ClassKey < payload.SessionRows[j].ClassKey
	})

	payload.AbsentStudentsRanking, payload.LateStartsRanking, payload.StudentsOverAbsenceRanking, err = getActiveClassRankingSummary(normalizedRankingFrom, normalizedRankingTo, 2)
	if err != nil {
		return nil, err
	}

	revenue, payingLeads, err := getRevenueInForBusinessDate(reportDate)
	if err != nil {
		return nil, err
	}
	payload.Summary.TodayRevenue = revenue
	payload.Summary.PayingLeadsCount = payingLeads

	placementScheduled, placementCompleted, err := getPlacementTestProgressForDate(reportDate)
	if err != nil {
		return nil, err
	}
	payload.Summary.PlacementTestsScheduled = placementScheduled
	payload.Summary.PlacementTestsCompleted = placementCompleted
	payload.Summary.PlacementTestsPending = placementScheduled - placementCompleted

	studentsInClassesCount, preEnrolmentStudentsCount, err := getManagerOpsLeadCounts()
	if err != nil {
		return nil, err
	}
	payload.Summary.StudentsInClassesCount = studentsInClassesCount
	payload.Summary.PreEnrolmentStudentsCount = preEnrolmentStudentsCount

	weekStart, weekEnd := util.LastCompletedCairoBusinessWeek(reportDate)
	weeklySummary, err := getManagerOpsWeeklySummary(weekStart, weekEnd)
	if err != nil {
		return nil, err
	}
	payload.WeeklySummary = weeklySummary

	return payload, nil
}

func managerOpsHasRescheduleAudit() (bool, error) {
	var exists bool
	if err := db.DB.QueryRow(`SELECT to_regclass('public.class_session_reschedules') IS NOT NULL`).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check class session reschedule audit table: %w", err)
	}
	return exists, nil
}

func GetManagerOverviewPayload() (*ManagerOverviewPayload, error) {
	now := util.CairoNow()
	payload := &ManagerOverviewPayload{
		Timezone:    util.CairoTimeZone,
		GeneratedAt: now.Format(time.RFC3339),
	}

	studentsInClassesCount, preEnrolmentCount, err := getManagerOpsLeadCounts()
	if err != nil {
		return nil, err
	}
	payload.Summary.StudentsInClassesCount = studentsInClassesCount
	payload.Summary.PreEnrolmentCount = preEnrolmentCount

	waitingListCount, waitingLevelBuckets, err := getManagerWaitingListSummary()
	if err != nil {
		return nil, err
	}
	payload.Summary.WaitingListCount = waitingListCount
	payload.WaitingListLevelBuckets = waitingLevelBuckets

	currentCashBalance, err := GetCurrentCashBalance()
	if err != nil {
		return nil, err
	}
	payload.Summary.CurrentCashBalance = currentCashBalance

	runningClassesCount, activeMentorsCount, err := getManagerOverviewClassAndMentorCounts()
	if err != nil {
		return nil, err
	}
	payload.Summary.RunningClassesCount = runningClassesCount
	payload.Summary.ActiveMentorsCount = activeMentorsCount

	statusBuckets, err := getManagerOverviewPreEnrolmentStatusBreakdown()
	if err != nil {
		return nil, err
	}
	payload.PreEnrolmentStatusBuckets = statusBuckets

	return payload, nil
}

func getManagerOpsLeadCounts() (studentsInClasses int, preEnrolmentStudents int, err error) {
	row := db.DB.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE l.status = 'in_classes')::int AS students_in_classes,
			COUNT(*) FILTER (
				WHERE l.status != 'in_classes'
				  AND l.status != 'waiting_for_round'
				  AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
				  AND l.status != 'cancelled'
				  AND COALESCE(l.ops_queue_reason, '') != 'private_track'
			)::int AS pre_enrolment_students
		FROM leads l
	`)
	if err := row.Scan(&studentsInClasses, &preEnrolmentStudents); err != nil {
		return 0, 0, fmt.Errorf("failed to query manager ops lead counts: %w", err)
	}
	return studentsInClasses, preEnrolmentStudents, nil
}

func getManagerOverviewClassAndMentorCounts() (runningClasses int, activeMentors int, err error) {
	row := db.DB.QueryRow(`
		SELECT
			COUNT(DISTINCT cg.class_key) FILTER (WHERE COALESCE(cg.round_status, 'not_started') = 'active')::int AS running_classes,
			COUNT(DISTINCT ma.mentor_user_id) FILTER (
				WHERE COALESCE(cg.round_status, 'not_started') = 'active'
				  AND ma.mentor_user_id IS NOT NULL
			)::int AS active_mentors
		FROM class_groups cg
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
	`)
	if err := row.Scan(&runningClasses, &activeMentors); err != nil {
		return 0, 0, fmt.Errorf("failed to query manager overview class and mentor counts: %w", err)
	}
	return runningClasses, activeMentors, nil
}

func getManagerOverviewPreEnrolmentStatusBreakdown() ([]ManagerOverviewStatusBreakdown, error) {
	rows, err := db.DB.Query(`
		SELECT l.status, COUNT(*)::int AS total
		FROM leads l
		WHERE l.status != 'in_classes'
		  AND l.status != 'waiting_for_round'
		  AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
		  AND l.status != 'cancelled'
		  AND COALESCE(l.ops_queue_reason, '') != 'private_track'
		GROUP BY l.status
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query manager overview pre-enrolment statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	countsByStatus := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan manager overview pre-enrolment status row: %w", err)
		}
		countsByStatus[status] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate manager overview pre-enrolment statuses: %w", err)
	}

	statusOrder := []struct {
		key   string
		label string
	}{
		{key: "lead_created", label: "No placement test time set"},
		{key: "test_booked", label: "Placement test booked"},
		{key: "tested", label: "Placement test done"},
		{key: "offer_sent", label: "Offer sent / not paid"},
		{key: "booking_confirmed", label: "Booking confirmed / not paid"},
		{key: "deposit_paid", label: "Deposit paid"},
		{key: "paid_full", label: "Paid in full"},
		{key: "waiting_for_round", label: "Waiting list"},
		{key: "schedule_assigned", label: "Schedule assigned"},
		{key: "ready_to_start", label: "Ready to start"},
		{key: "paused", label: "Paused"},
		{key: "renewal_pending", label: "Renewal pending"},
		{key: "cold_lead", label: "Cold lead"},
	}

	breakdown := make([]ManagerOverviewStatusBreakdown, 0, len(countsByStatus))
	for _, item := range statusOrder {
		count := countsByStatus[item.key]
		if count <= 0 {
			continue
		}
		breakdown = append(breakdown, ManagerOverviewStatusBreakdown{
			StatusKey: item.key,
			Label:     item.label,
			Count:     count,
		})
		delete(countsByStatus, item.key)
	}

	if len(countsByStatus) > 0 {
		extras := make([]string, 0, len(countsByStatus))
		for status := range countsByStatus {
			extras = append(extras, status)
		}
		sort.Strings(extras)
		for _, status := range extras {
			breakdown = append(breakdown, ManagerOverviewStatusBreakdown{
				StatusKey: status,
				Label:     strings.ReplaceAll(status, "_", " "),
				Count:     countsByStatus[status],
			})
		}
	}

	return breakdown, nil
}

func getManagerWaitingListSummary() (int, []ManagerWaitingListLevelBucket, error) {
	rows, err := db.DB.Query(`
		SELECT COALESCE(pt.assigned_level, 0) AS level, COUNT(*)::int AS total
		FROM leads l
		LEFT JOIN placement_tests pt ON pt.lead_id = l.id
		WHERE l.status = 'waiting_for_round'
		  AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
		  AND l.status != 'cancelled'
		  AND COALESCE(l.ops_queue_reason, '') != 'private_track'
		GROUP BY COALESCE(pt.assigned_level, 0)
		ORDER BY COALESCE(pt.assigned_level, 0)
	`)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to query waiting list summary: %w", err)
	}
	defer func() { _ = rows.Close() }()

	total := 0
	buckets := make([]ManagerWaitingListLevelBucket, 0)
	for rows.Next() {
		var bucket ManagerWaitingListLevelBucket
		if err := rows.Scan(&bucket.Level, &bucket.Count); err != nil {
			return 0, nil, fmt.Errorf("failed to scan waiting list summary row: %w", err)
		}
		total += bucket.Count
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("failed to iterate waiting list summary: %w", err)
	}

	return total, buckets, nil
}

func countExpectedStudentsForSession(classKey string, sessionNumber int32) (int, error) {
	if err := ensureClassMemberships(classKey); err != nil {
		return 0, err
	}

	var count int
	err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM class_memberships cm
		INNER JOIN leads l ON l.id = cm.lead_id
		WHERE cm.class_key = $1
		  AND cm.joined_at_session_number <= $2
		  AND (cm.left_after_session_number IS NULL OR cm.left_after_session_number >= $2)
		  AND cm.removed_at IS NULL
		  AND l.status != 'cancelled'
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
	loc := util.CairoLocation()
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

func computeManagerAttendanceStatus(expectedStudents, attendanceMarked int) string {
	switch {
	case expectedStudents <= 0:
		return "none_expected"
	case attendanceMarked <= 0:
		return "not_started"
	case attendanceMarked < expectedStudents:
		return "partial"
	default:
		return "done"
	}
}

func computeManagerMentorStatus(complianceChecked, mentorAbsent bool, delayMinutes int) string {
	if !complianceChecked {
		return "not_checked"
	}
	if mentorAbsent {
		return "absent"
	}
	if delayMinutes > 0 {
		return "late"
	}
	return "on_time"
}

func computeManagerSessionPhase(reportDate time.Time, scheduledTime, sessionStatus string, now time.Time) string {
	if sessionStatus == "completed" {
		return "completed"
	}

	day := util.CairoStartOfDay(reportDate)
	today := util.CairoStartOfDay(now)
	if day.Before(today) {
		return "ended_unfinished"
	}
	if day.After(today) {
		return "upcoming"
	}

	clock, err := parseSessionClock(scheduledTime)
	if err != nil {
		return "scheduled"
	}
	clock = normalizeBusinessPMClock(clock)
	loc := util.CairoLocation()
	startAt := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
	endAt := startAt.Add(2 * time.Hour)
	current := now.In(loc)
	if current.Before(startAt) {
		return "upcoming"
	}
	if current.After(endAt) {
		return "ended_unfinished"
	}
	return "live_now"
}

func managerSessionPhasePriority(phase string) int {
	switch phase {
	case "live_now":
		return 0
	case "ended_unfinished":
		return 1
	case "upcoming":
		return 2
	case "completed":
		return 3
	default:
		return 4
	}
}

type mentorWeeklyAggregate struct {
	MentorID       string
	MentorName     string
	MentorEmail    string
	AbsentStudents int
	LateStarts     int
}

func getManagerOpsWeeklySummary(weekStart, weekEnd time.Time) (ManagerOpsWeeklySummary, error) {
	summary := ManagerOpsWeeklySummary{
		Label:     "Last Week",
		WeekStart: weekStart.Format("2006-01-02"),
		WeekEnd:   weekEnd.Format("2006-01-02"),
	}
	mentorAggregates := make(map[string]*mentorWeeklyAggregate)

	rows, err := db.DB.Query(`
		SELECT cs.id, cs.class_key, cs.session_number,
		       ma.mentor_user_id::TEXT,
		       COALESCE(NULLIF(TRIM(u.full_name), ''), u.email, 'Unassigned') AS mentor_name,
		       COALESCE(u.email, 'Unassigned') AS mentor_email,
		       COALESCE(att.marked_count, 0) AS attendance_marked,
		       COALESCE(att.attended_count, 0) AS attended_students,
		       COALESCE(att.absent_count, 0) AS absent_students,
		       COALESCE(msc.id IS NOT NULL, false) AS compliance_checked,
		       COALESCE(msc.delay_minutes, 0) AS delay_minutes,
		       COALESCE(msc.is_absent, false) AS mentor_absent,
		       cs.status
		FROM class_sessions cs
		INNER JOIN class_groups cg ON cg.class_key = cs.class_key
		LEFT JOIN mentor_assignments ma ON ma.class_key = cs.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		LEFT JOIN mentor_session_checks msc ON msc.class_session_id = cs.id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*)::int AS marked_count,
				COUNT(*) FILTER (WHERE a.status IN ('PRESENT', 'LATE'))::int AS attended_count,
				COUNT(*) FILTER (WHERE a.status = 'ABSENT')::int AS absent_count
			FROM attendance a
			WHERE a.session_id = cs.id
		) att ON true
		WHERE cs.scheduled_date BETWEEN $1 AND $2
		  AND COALESCE(cg.round_status, 'not_started') = 'active'
	`, weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02"))
	if err != nil {
		return summary, fmt.Errorf("failed to query weekly manager ops sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var sessionID uuid.UUID
		var classKey string
		var sessionNumber int32
		var mentorID sql.NullString
		var mentorName string
		var mentorEmail string
		var attendanceMarked int
		var attendedStudents int
		var absentStudents int
		var complianceChecked bool
		var delayMinutes int
		var mentorAbsent bool
		var sessionStatus string

		if err := rows.Scan(
			&sessionID,
			&classKey,
			&sessionNumber,
			&mentorID,
			&mentorName,
			&mentorEmail,
			&attendanceMarked,
			&attendedStudents,
			&absentStudents,
			&complianceChecked,
			&delayMinutes,
			&mentorAbsent,
			&sessionStatus,
		); err != nil {
			return summary, fmt.Errorf("failed to scan weekly manager ops session: %w", err)
		}

		expectedStudents, err := countExpectedStudentsForSession(classKey, sessionNumber)
		if err != nil {
			return summary, err
		}

		attendanceStatus := computeManagerAttendanceStatus(expectedStudents, attendanceMarked)
		mentorStatus := computeManagerMentorStatus(complianceChecked, mentorAbsent, delayMinutes)

		summary.SessionsScheduled++
		summary.ExpectedStudents += expectedStudents
		summary.AttendedStudents += attendedStudents
		if sessionStatus == "completed" {
			summary.SessionsCompleted++
		}
		if attendanceStatus == "done" || attendanceStatus == "none_expected" {
			summary.SessionsAttendanceDone++
		} else {
			summary.SessionsAttendancePending++
		}
		switch mentorStatus {
		case "late":
			summary.LateMentorSessions++
		case "absent":
			summary.AbsentMentorSessions++
		case "not_checked":
			summary.UncheckedMentorSessions++
		}
		if mentorID.Valid && strings.TrimSpace(mentorID.String) != "" {
			aggregate, ok := mentorAggregates[mentorID.String]
			if !ok {
				aggregate = &mentorWeeklyAggregate{
					MentorID:    mentorID.String,
					MentorName:  mentorName,
					MentorEmail: mentorEmail,
				}
				mentorAggregates[mentorID.String] = aggregate
			}
			aggregate.AbsentStudents += absentStudents
			if mentorStatus == "late" {
				aggregate.LateStarts++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return summary, fmt.Errorf("failed to iterate weekly manager ops sessions: %w", err)
	}
	summary.AbsentStudentsRanking = buildManagerOpsMentorRanking(mentorAggregates, func(item *mentorWeeklyAggregate) int {
		return item.AbsentStudents
	})
	summary.LateStartsRanking = buildManagerOpsMentorRanking(mentorAggregates, func(item *mentorWeeklyAggregate) int {
		return item.LateStarts
	})
	summary.TopAbsentStudentsMentor = pickTopManagerOpsMentorLeader(mentorAggregates, func(item *mentorWeeklyAggregate) int {
		return item.AbsentStudents
	})
	summary.TopLateStartsMentor = pickTopManagerOpsMentorLeader(mentorAggregates, func(item *mentorWeeklyAggregate) int {
		return item.LateStarts
	})

	revenue, payingLeads, err := getRevenueInForDateRange(weekStart, weekEnd)
	if err != nil {
		return summary, err
	}
	summary.Revenue = revenue
	summary.PayingLeadsCount = payingLeads

	placementScheduled, placementCompleted, err := getPlacementTestProgressForDateRange(weekStart, weekEnd)
	if err != nil {
		return summary, err
	}
	summary.PlacementTestsScheduled = placementScheduled
	summary.PlacementTestsCompleted = placementCompleted
	summary.PlacementTestsPending = placementScheduled - placementCompleted

	transferEvents, returnsToAdmin, err := getClassTransferEventCountsForDateRange(weekStart, weekEnd)
	if err != nil {
		return summary, err
	}
	summary.TransferEvents = transferEvents
	summary.ReturnsToAdmin = returnsToAdmin

	return summary, nil
}

func pickTopManagerOpsMentorLeader(aggregates map[string]*mentorWeeklyAggregate, metric func(*mentorWeeklyAggregate) int) ManagerOpsWeeklyMentorLeader {
	var leader *mentorWeeklyAggregate
	bestValue := 0
	for _, item := range aggregates {
		value := metric(item)
		if value <= 0 {
			continue
		}
		if leader == nil || value > bestValue || (value == bestValue && compareManagerOpsMentorAggregate(item, leader) < 0) {
			leader = item
			bestValue = value
		}
	}
	if leader == nil {
		return ManagerOpsWeeklyMentorLeader{}
	}
	return ManagerOpsWeeklyMentorLeader{
		MentorID:    leader.MentorID,
		MentorName:  leader.MentorName,
		MentorEmail: leader.MentorEmail,
		MetricValue: bestValue,
	}
}

func compareManagerOpsMentorAggregate(left, right *mentorWeeklyAggregate) int {
	leftName := strings.ToLower(strings.TrimSpace(left.MentorName))
	rightName := strings.ToLower(strings.TrimSpace(right.MentorName))
	if leftName < rightName {
		return -1
	}
	if leftName > rightName {
		return 1
	}
	leftEmail := strings.ToLower(strings.TrimSpace(left.MentorEmail))
	rightEmail := strings.ToLower(strings.TrimSpace(right.MentorEmail))
	if leftEmail < rightEmail {
		return -1
	}
	if leftEmail > rightEmail {
		return 1
	}
	if left.MentorID < right.MentorID {
		return -1
	}
	if left.MentorID > right.MentorID {
		return 1
	}
	return 0
}

func buildManagerOpsMentorRanking(aggregates map[string]*mentorWeeklyAggregate, metric func(*mentorWeeklyAggregate) int) []ManagerOpsWeeklyMentorLeader {
	ranking := make([]ManagerOpsWeeklyMentorLeader, 0, len(aggregates))
	for _, item := range aggregates {
		value := metric(item)
		if value <= 0 {
			continue
		}
		ranking = append(ranking, ManagerOpsWeeklyMentorLeader{
			MentorID:    item.MentorID,
			MentorName:  item.MentorName,
			MentorEmail: item.MentorEmail,
			MetricValue: value,
		})
	}
	sort.SliceStable(ranking, func(i, j int) bool {
		if ranking[i].MetricValue != ranking[j].MetricValue {
			return ranking[i].MetricValue > ranking[j].MetricValue
		}
		left := &mentorWeeklyAggregate{MentorID: ranking[i].MentorID, MentorName: ranking[i].MentorName, MentorEmail: ranking[i].MentorEmail}
		right := &mentorWeeklyAggregate{MentorID: ranking[j].MentorID, MentorName: ranking[j].MentorName, MentorEmail: ranking[j].MentorEmail}
		return compareManagerOpsMentorAggregate(left, right) < 0
	})
	return ranking
}

func getActiveClassRankingSummary(startDate, endDate *time.Time, absenceThreshold int) ([]ManagerOpsWeeklyMentorLeader, []ManagerOpsWeeklyMentorLeader, []DailyReportStudentLeader, error) {
	var queryArgs []interface{}
	dateClause := ""
	if startDate != nil && endDate != nil {
		dateClause = "AND cs.scheduled_date BETWEEN $1::date AND $2::date"
		queryArgs = append(queryArgs, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	} else if startDate != nil {
		dateClause = "AND cs.scheduled_date >= $1::date"
		queryArgs = append(queryArgs, startDate.Format("2006-01-02"))
	} else if endDate != nil {
		dateClause = "AND cs.scheduled_date <= $1::date"
		queryArgs = append(queryArgs, endDate.Format("2006-01-02"))
	}

	mentorAggregates := make(map[string]*mentorWeeklyAggregate)
	rows, err := db.DB.Query(fmt.Sprintf(`
		SELECT
			ma.mentor_user_id::TEXT,
			COALESCE(NULLIF(TRIM(u.full_name), ''), u.email, 'Unassigned') AS mentor_name,
			COALESCE(u.email, 'Unassigned') AS mentor_email,
			COALESCE(att.absent_count, 0) AS absent_students,
			COALESCE(msc.id IS NOT NULL, false) AS compliance_checked,
			COALESCE(msc.delay_minutes, 0) AS delay_minutes,
			COALESCE(msc.is_absent, false) AS mentor_absent
		FROM class_sessions cs
		INNER JOIN class_groups cg ON cg.class_key = cs.class_key
		LEFT JOIN mentor_assignments ma ON ma.class_key = cs.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		LEFT JOIN mentor_session_checks msc ON msc.class_session_id = cs.id
		LEFT JOIN LATERAL (
			SELECT COUNT(*) FILTER (WHERE a.status = 'ABSENT')::int AS absent_count
			FROM attendance a
			WHERE a.session_id = cs.id
		) att ON true
		WHERE COALESCE(cg.round_status, 'not_started') = 'active'
		  %s
	`, dateClause), queryArgs...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to query active class ranking summary: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var mentorID sql.NullString
		var mentorName, mentorEmail string
		var absentStudents, delayMinutes int
		var complianceChecked, mentorAbsent bool
		if err := rows.Scan(&mentorID, &mentorName, &mentorEmail, &absentStudents, &complianceChecked, &delayMinutes, &mentorAbsent); err != nil {
			return nil, nil, nil, fmt.Errorf("failed to scan active class ranking session: %w", err)
		}
		if !mentorID.Valid || strings.TrimSpace(mentorID.String) == "" {
			continue
		}
		aggregate, ok := mentorAggregates[mentorID.String]
		if !ok {
			aggregate = &mentorWeeklyAggregate{
				MentorID:    mentorID.String,
				MentorName:  mentorName,
				MentorEmail: mentorEmail,
			}
			mentorAggregates[mentorID.String] = aggregate
		}
		aggregate.AbsentStudents += absentStudents
		if computeManagerMentorStatus(complianceChecked, mentorAbsent, delayMinutes) == "late" {
			aggregate.LateStarts++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to iterate active class ranking sessions: %w", err)
	}

	absentRanking := buildManagerOpsMentorRanking(mentorAggregates, func(item *mentorWeeklyAggregate) int {
		return item.AbsentStudents
	})
	lateRanking := buildManagerOpsMentorRanking(mentorAggregates, func(item *mentorWeeklyAggregate) int {
		return item.LateStarts
	})
	studentRanking, err := getStudentsOverAbsenceThresholdRanking(startDate, endDate, absenceThreshold)
	if err != nil {
		return nil, nil, nil, err
	}
	return absentRanking, lateRanking, studentRanking, nil
}

func getStudentsOverAbsenceThresholdRanking(startDate, endDate *time.Time, threshold int) ([]DailyReportStudentLeader, error) {
	var queryArgs []interface{}
	paramIndex := 1
	dateClause := ""
	if startDate != nil && endDate != nil {
		dateClause = fmt.Sprintf("AND cs.scheduled_date BETWEEN $%d::date AND $%d::date", paramIndex, paramIndex+1)
		queryArgs = append(queryArgs, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
		paramIndex += 2
	} else if startDate != nil {
		dateClause = fmt.Sprintf("AND cs.scheduled_date >= $%d::date", paramIndex)
		queryArgs = append(queryArgs, startDate.Format("2006-01-02"))
		paramIndex++
	} else if endDate != nil {
		dateClause = fmt.Sprintf("AND cs.scheduled_date <= $%d::date", paramIndex)
		queryArgs = append(queryArgs, endDate.Format("2006-01-02"))
		paramIndex++
	}
	thresholdPlaceholder := fmt.Sprintf("$%d", paramIndex)
	queryArgs = append(queryArgs, threshold)

	rows, err := db.DB.Query(fmt.Sprintf(`
		SELECT
			l.id::text,
			COALESCE(NULLIF(TRIM(l.full_name), ''), 'Unknown student') AS student_name,
			COALESCE(l.phone, '') AS student_phone,
			COUNT(*)::int AS absence_count
		FROM attendance a
		INNER JOIN leads l ON l.id = a.lead_id
		INNER JOIN class_sessions cs ON cs.id = a.session_id
		INNER JOIN class_groups cg ON cg.class_key = cs.class_key
		WHERE a.status = 'ABSENT'
		  AND l.status = 'in_classes'
		  AND COALESCE(cg.round_status, 'not_started') = 'active'
		  %s
		GROUP BY l.id, l.full_name, l.phone
		HAVING COUNT(*) > %s
		ORDER BY COUNT(*) DESC, COALESCE(NULLIF(TRIM(l.full_name), ''), 'Unknown student') ASC, COALESCE(l.phone, '') ASC
	`, dateClause, thresholdPlaceholder), queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query students over absence threshold: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ranking := make([]DailyReportStudentLeader, 0)
	for rows.Next() {
		var item DailyReportStudentLeader
		if err := rows.Scan(&item.LeadID, &item.StudentName, &item.StudentPhone, &item.MetricValue); err != nil {
			return nil, fmt.Errorf("failed to scan student absence ranking: %w", err)
		}
		ranking = append(ranking, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate student absence ranking: %w", err)
	}
	return ranking, nil
}

func getRevenueInForBusinessDate(reportDate time.Time) (int32, int, error) {
	var total sql.NullInt32
	var payingLeads int
	if err := db.DB.QueryRow(`
		SELECT
			COALESCE(SUM(amount), 0)::int,
			COUNT(DISTINCT lead_id)::int
		FROM transactions
		WHERE transaction_date = $1::date
		  AND transaction_type = 'IN'
	`, reportDate.Format("2006-01-02")).Scan(&total, &payingLeads); err != nil {
		return 0, 0, fmt.Errorf("failed to load daily revenue: %w", err)
	}
	if !total.Valid {
		return 0, payingLeads, nil
	}
	return total.Int32, payingLeads, nil
}

func getRevenueInForDateRange(startDate, endDate time.Time) (int32, int, error) {
	var total sql.NullInt32
	var payingLeads int
	if err := db.DB.QueryRow(`
		SELECT
			COALESCE(SUM(amount), 0)::int,
			COUNT(DISTINCT lead_id)::int
		FROM transactions
		WHERE transaction_date BETWEEN $1::date AND $2::date
		  AND transaction_type = 'IN'
	`, startDate.Format("2006-01-02"), endDate.Format("2006-01-02")).Scan(&total, &payingLeads); err != nil {
		return 0, 0, fmt.Errorf("failed to load ranged revenue: %w", err)
	}
	if !total.Valid {
		return 0, payingLeads, nil
	}
	return total.Int32, payingLeads, nil
}

func getPlacementTestProgressForDate(reportDate time.Time) (int, int, error) {
	var scheduled int
	var completed int
	if err := db.DB.QueryRow(`
		SELECT
			COUNT(*)::int AS scheduled_count,
			COUNT(*) FILTER (WHERE pt.assigned_level IS NOT NULL)::int AS completed_count
		FROM placement_tests pt
		INNER JOIN leads l ON l.id = pt.lead_id
		WHERE pt.test_date = $1::date
		  AND l.status != 'cancelled'
	`, reportDate.Format("2006-01-02")).Scan(&scheduled, &completed); err != nil {
		return 0, 0, fmt.Errorf("failed to load placement test progress: %w", err)
	}
	return scheduled, completed, nil
}

func getPlacementTestProgressForDateRange(startDate, endDate time.Time) (int, int, error) {
	var scheduled int
	var completed int
	if err := db.DB.QueryRow(`
		SELECT
			COUNT(*)::int AS scheduled_count,
			COUNT(*) FILTER (WHERE pt.assigned_level IS NOT NULL)::int AS completed_count
		FROM placement_tests pt
		INNER JOIN leads l ON l.id = pt.lead_id
		WHERE pt.test_date BETWEEN $1::date AND $2::date
		  AND l.status != 'cancelled'
	`, startDate.Format("2006-01-02"), endDate.Format("2006-01-02")).Scan(&scheduled, &completed); err != nil {
		return 0, 0, fmt.Errorf("failed to load ranged placement test progress: %w", err)
	}
	return scheduled, completed, nil
}

func getClassTransferEventCountsForDateRange(startDate, endDate time.Time) (int, int, error) {
	var transferEvents int
	var returnsToAdmin int
	if err := db.DB.QueryRow(`
		SELECT
			COUNT(*) FILTER (
				WHERE target_class_key IS NOT NULL
				  AND reason IN ('schedule_change', 'promotion', 'demotion', 'late_join', 'other')
			)::int AS transfer_events,
			COUNT(*) FILTER (
				WHERE reason IN ('refund_to_admin', 'private_track_to_admin', 'other_to_admin', 'early_repeat_absence_to_admin')
			)::int AS returns_to_admin
		FROM class_transfers
		WHERE created_at >= $1
		  AND created_at < $2
	`, weekRangeStartTime(startDate), weekRangeEndExclusive(endDate)).Scan(&transferEvents, &returnsToAdmin); err != nil {
		return 0, 0, fmt.Errorf("failed to load weekly class transfer events: %w", err)
	}
	return transferEvents, returnsToAdmin, nil
}

func weekRangeStartTime(day time.Time) time.Time {
	return util.CairoStartOfDay(day)
}

func weekRangeEndExclusive(day time.Time) time.Time {
	return util.CairoStartOfDay(day).AddDate(0, 0, 1)
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

// GetOpsNotificationSummary returns unread operational banners for MH/Manager.
func GetOpsNotificationSummary(userID uuid.UUID, role string, now time.Time) (*OpsNotificationSummary, error) {
	summary := &OpsNotificationSummary{}
	if role == "mentor_head" || role == "manager" {
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
			report, err := GetDailyReportPayload(reportDate, nil, nil)
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
	}

	if role == "mentor_head" {
		classSent, err := GetUnreadClassSentNotification(userID)
		if err != nil {
			return nil, err
		}
		summary.ClassSent = classSent
	}
	if role == "student_success" {
		sessionReschedule, err := GetUnreadSessionRescheduleNotification(userID)
		if err != nil {
			return nil, err
		}
		summary.SessionReschedule = sessionReschedule
	}
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

// MarkClassSentNotificationRead marks a new-class banner as dismissed for a user.
func MarkClassSentNotificationRead(userID uuid.UUID, classKey string) error {
	_, err := db.DB.Exec(`
		INSERT INTO class_sent_notification_reads (user_id, class_key, read_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id, class_key) DO UPDATE SET read_at = EXCLUDED.read_at
	`, userID, classKey)
	if err != nil {
		return fmt.Errorf("failed to mark class-sent notification read: %w", err)
	}
	return nil
}

// MarkAllClassSentNotificationsRead dismisses the entire unread new-class banner batch for a user.
func MarkAllClassSentNotificationsRead(userID uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO class_sent_notification_reads (user_id, class_key, read_at)
		SELECT $1, cg.class_key, NOW()
		FROM class_groups cg
		LEFT JOIN class_sent_notification_reads csr
			ON csr.class_key = cg.class_key AND csr.user_id = $1
		WHERE cg.sent_to_mentor = true
		  AND COALESCE(cg.round_status, '') != 'closed'
		  AND csr.id IS NULL
		ON CONFLICT (user_id, class_key) DO UPDATE
		SET read_at = EXCLUDED.read_at
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to mark all class-sent notifications read: %w", err)
	}
	return nil
}

// GetUnreadClassSentNotification returns the newest unread sent-to-MH class for the user.
func GetUnreadClassSentNotification(userID uuid.UUID) (*ClassSentNotification, error) {
	var unreadCount int
	if err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM class_groups cg
		LEFT JOIN class_sent_notification_reads csr
			ON csr.class_key = cg.class_key AND csr.user_id = $1
		WHERE cg.sent_to_mentor = true
		  AND COALESCE(cg.round_status, '') != 'closed'
		  AND csr.id IS NULL
	`, userID).Scan(&unreadCount); err != nil {
		return nil, fmt.Errorf("failed to count unread class-sent notifications: %w", err)
	}
	if unreadCount == 0 {
		return nil, nil
	}

	n := &ClassSentNotification{UnreadCount: unreadCount}
	var sentAt sql.NullTime
	err := db.DB.QueryRow(`
		SELECT cg.class_key, cg.level, COALESCE(cg.class_number, 1), cg.class_days, cg.class_time, cg.sent_at
		FROM class_groups cg
		LEFT JOIN class_sent_notification_reads csr
			ON csr.class_key = cg.class_key AND csr.user_id = $1
		WHERE cg.sent_to_mentor = true
		  AND COALESCE(cg.round_status, '') != 'closed'
		  AND csr.id IS NULL
		ORDER BY cg.sent_at DESC NULLS LAST, cg.updated_at DESC
		LIMIT 1
	`, userID).Scan(&n.ClassKey, &n.Level, &n.ClassNumber, &n.Days, &n.Time, &sentAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query unread class-sent notification: %w", err)
	}
	if sentAt.Valid {
		n.SentAt = sentAt.Time
	}

	students, err := GetStudentsForMentorHeadClass(n.ClassKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load class-sent notification students: %w", err)
	}
	n.StudentCount = len(students)
	return n, nil
}

// MarkSessionRescheduleNotificationRead marks one SS reschedule banner as dismissed for a user.
func MarkSessionRescheduleNotificationRead(userID, rescheduleID uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO session_reschedule_notification_reads (user_id, reschedule_id, read_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id, reschedule_id) DO UPDATE SET read_at = EXCLUDED.read_at
	`, userID, rescheduleID)
	if err != nil {
		return fmt.Errorf("failed to mark session reschedule notification read: %w", err)
	}
	return nil
}

// GetUnreadSessionRescheduleNotification returns the newest unread MH-driven session reschedule for Student Success.
func GetUnreadSessionRescheduleNotification(userID uuid.UUID) (*SessionRescheduleNotification, error) {
	var unreadCount int
	if err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM class_session_reschedules csr
		INNER JOIN users changed_by ON changed_by.id = csr.changed_by_user_id
		LEFT JOIN session_reschedule_notification_reads srr
			ON srr.reschedule_id = csr.id AND srr.user_id = $1
		WHERE changed_by.role = 'mentor_head'
		  AND srr.id IS NULL
	`, userID).Scan(&unreadCount); err != nil {
		return nil, fmt.Errorf("failed to count unread session reschedule notifications: %w", err)
	}
	if unreadCount == 0 {
		return nil, nil
	}

	n := &SessionRescheduleNotification{UnreadCount: unreadCount}
	err := db.DB.QueryRow(`
		SELECT csr.id,
		       csr.class_key,
		       cg.level,
		       COALESCE(cg.class_number, 1),
		       cg.class_days,
		       cg.class_time,
		       csr.session_number,
		       csr.old_scheduled_date::text,
		       COALESCE(csr.old_scheduled_time::text, ''),
		       csr.new_scheduled_date::text,
		       COALESCE(csr.new_scheduled_time::text, ''),
		       COALESCE(NULLIF(TRIM(u.full_name), ''), u.email, 'Unknown'),
		       csr.created_at
		FROM class_session_reschedules csr
		INNER JOIN users u ON u.id = csr.changed_by_user_id
		LEFT JOIN class_groups cg ON cg.class_key = csr.class_key
		LEFT JOIN session_reschedule_notification_reads srr
			ON srr.reschedule_id = csr.id AND srr.user_id = $1
		WHERE u.role = 'mentor_head'
		  AND srr.id IS NULL
		ORDER BY csr.created_at DESC, csr.id DESC
		LIMIT 1
	`, userID).Scan(
		&n.ID,
		&n.ClassKey,
		&n.Level,
		&n.ClassNumber,
		&n.Days,
		&n.Time,
		&n.SessionNumber,
		&n.OldDate,
		&n.OldTime,
		&n.NewDate,
		&n.NewTime,
		&n.ChangedBy,
		&n.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query unread session reschedule notification: %w", err)
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
