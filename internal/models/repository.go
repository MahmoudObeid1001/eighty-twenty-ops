package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/util"

	"github.com/google/uuid"
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
)

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
		"waiting_for_round": StageScheduleSet,
		"schedule_assigned": StageScheduleSet,
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
			StageNewLead:    true,
			StageTestBooked: true,
			StageTested:     true,
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

	// Calculate days since last progress (test_date or updated_at)
	var progressTime time.Time
	if item.TestDate.Valid {
		progressTime = item.TestDate.Time
	} else {
		progressTime = item.Lead.UpdatedAt
		if progressTime.IsZero() {
			progressTime = item.Lead.CreatedAt
		}
	}

	now := time.Now()
	daysSince := int(now.Sub(progressTime).Hours() / 24)
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

func GetAllLeads(statusFilter, searchFilter, paymentFilter, hotFilter string, includeCancelled bool, followUpFilter string) ([]*LeadListItem, error) {
	query := `
		SELECT 
			l.id, l.full_name, l.phone, l.source, l.notes, l.status, l.sent_to_classes,
			l.created_by_user_id, l.created_at, l.updated_at,
			pt.assigned_level, pt.test_date,
			p.remaining_balance, p.amount_paid,
			o.final_price
		FROM leads l
		LEFT JOIN placement_tests pt ON l.id = pt.lead_id
		LEFT JOIN payments p ON l.id = p.lead_id
		LEFT JOIN offers o ON l.id = o.lead_id
		WHERE 1=1
		AND l.status != 'in_classes'
		AND (l.sent_to_classes IS NULL OR l.sent_to_classes = false)
	`

	args := []interface{}{}
	argIndex := 1

	// Apply follow-up filter (high priority follow-up)
	if followUpFilter == "high_priority" {
		query += fmt.Sprintf(" AND l.high_priority_follow_up = true")
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
		}

		// If it's a new stage constant, map it; otherwise use as-is (backward compat)
		dbStatus := statusFilter
		if mapped, ok := stageToStatusMap[statusFilter]; ok {
			dbStatus = mapped
		}

		// For booking confirmed stages, we'll filter by status in SQL
		// but payment state filtering happens after computing flags
		query += fmt.Sprintf(" AND l.status = $%d", argIndex)
		args = append(args, dbStatus)
		argIndex++
	}

	// Apply search filter (name or phone)
	if searchFilter != "" {
		query += fmt.Sprintf(" AND (LOWER(l.full_name) LIKE LOWER($%d) OR l.phone LIKE $%d)", argIndex, argIndex)
		searchPattern := "%" + searchFilter + "%"
		args = append(args, searchPattern)
		argIndex++
	}

	// Default sorting (unless hot filter is active, then we sort after computing flags in Go)
	if hotFilter != "hot" {
		query += " ORDER BY l.created_at DESC"
	} else {
		// For hot filter, we'll sort in Go after computing flags, but still need an ORDER BY for SQL
		query += " ORDER BY l.created_at DESC"
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
	defer rows.Close()

	var leads []*LeadListItem
	for rows.Next() {
		lead := &Lead{}
		var assignedLevel sql.NullInt32
		var remainingBalance, amountPaid, finalPrice sql.NullInt32
		var testDate sql.NullTime

		err := rows.Scan(
			&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status, &lead.SentToClasses,
			&lead.CreatedByUserID, &lead.CreatedAt, &lead.UpdatedAt,
			&assignedLevel, &testDate,
			&remainingBalance, &amountPaid,
			&finalPrice,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lead: %w", err)
		}

		// Compute payment state
		paymentState := GetPaymentState(amountPaid, finalPrice)

		item := &LeadListItem{
			Lead:             lead,
			AssignedLevel:    assignedLevel,
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

	// For BOOKING_CONFIRMED_PAID_FULL and BOOKING_CONFIRMED_DEPOSIT status filters,
	// also filter by payment state after computing it
	if statusFilter == StageBookingConfirmedPaidFull {
		var filteredLeads []*LeadListItem
		for _, lead := range leads {
			// Must have booking_confirmed/paid_full status AND paid_full payment state
			if (lead.Lead.Status == "paid_full" || lead.Lead.Status == "booking_confirmed") && lead.PaymentState == PaymentStatePaidFull {
				filteredLeads = append(filteredLeads, lead)
			}
		}
		leads = filteredLeads
	} else if statusFilter == StageBookingConfirmedDeposit {
		var filteredLeads []*LeadListItem
		for _, lead := range leads {
			// Must have booking_confirmed/deposit_paid status AND deposit payment state
			if (lead.Lead.Status == "deposit_paid" || lead.Lead.Status == "booking_confirmed") && lead.PaymentState == PaymentStateDeposit {
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

func GetLeadByID(id uuid.UUID) (*LeadDetail, error) {
	// Get lead
	lead := &Lead{}
	err := db.DB.QueryRow(`
		SELECT id, full_name, phone, source, notes, status, sent_to_classes, high_priority_follow_up, created_by_user_id, created_at, updated_at
		FROM leads WHERE id = $1
	`, id).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status,
		&lead.SentToClasses, &lead.HighPriorityFollowUp, &lead.CreatedByUserID, &lead.CreatedAt, &lead.UpdatedAt,
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
	defer tx.Rollback()

	now := time.Now()

	// Update lead
	_, err = tx.Exec(`
		UPDATE leads SET full_name = $1, phone = $2, source = $3, notes = $4, status = $5, sent_to_classes = $6, updated_at = $7
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
				assigned_level = EXCLUDED.assigned_level,
				test_notes = EXCLUDED.test_notes,
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
	_, err := db.DB.Exec(`
		UPDATE leads SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`, status, leadID)
	return err
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

// BookPlacementTest updates placement test fields and sets status to "test_booked"
// This is a lightweight update that doesn't require offer/pricing fields
func BookPlacementTest(leadID uuid.UUID, testDate sql.NullTime, testTime sql.NullString, testType sql.NullString, testNotes sql.NullString) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

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
		SELECT id, email, password_hash, role, created_at
		FROM users WHERE LOWER(TRIM(email)) = LOWER(TRIM($1))
	`, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func CreateUser(email, passwordHash, role string) (*User, error) {
	userID := uuid.New()
	_, err := db.DB.Exec(`
		INSERT INTO users (id, email, password_hash, role, created_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
	`, userID, email, passwordHash, role)
	if err != nil {
		return nil, err
	}
	return &User{
		ID:           userID,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         role,
	}, nil
}

// DeleteLead deletes a lead and all associated data (cascade delete)
func DeleteLead(leadID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

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
		ON CONFLICT (key) DO UPDATE SET value = (CAST(value AS INTEGER) + 1)::TEXT, updated_at = CURRENT_TIMESTAMP
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
	defer rows.Close()

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
	// Only show students that have been manually sent to classes (sent_to_classes = true)
	query := `
		SELECT l.id, l.full_name, l.phone, pt.assigned_level, s.class_days, s.class_time, s.class_group_index
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.status = 'ready_to_start'
		AND l.sent_to_classes = true
		AND pt.assigned_level IS NOT NULL
		AND s.class_days IS NOT NULL
		AND s.class_time IS NOT NULL
		ORDER BY pt.assigned_level, s.class_days, s.class_time, COALESCE(s.class_group_index, 1), l.full_name
	`
	rows, err := db.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query class groups: %w", err)
	}
	defer rows.Close()

	// Group by (level, days, time, group_index)
	groupsMap := make(map[string]*ClassGroup)
	for rows.Next() {
		var leadID uuid.UUID
		var fullName, phone string
		var assignedLevel sql.NullInt32
		var classDays, classTime sql.NullString
		var groupIndex sql.NullInt32

		err := rows.Scan(&leadID, &fullName, &phone, &assignedLevel, &classDays, &classTime, &groupIndex)
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
				StudentCount: 0,
				Students:     []*ClassStudent{},
			}
			groupsMap[key] = group
		}

		group.Students = append(group.Students, &ClassStudent{
			LeadID:     leadID,
			FullName:   fullName,
			Phone:      phone,
			GroupIndex: groupIndex,
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
		// Compute readiness: 6=LOCKED, 4-5=READY, <4=NOT READY
		if group.StudentCount >= 6 {
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
			for _, group := range groups {
				if wf, ok := workflows[group.ClassKey]; ok {
					group.SentToMentor = wf.SentToMentor
					group.SentAt = wf.SentAt
					group.ReturnedAt = wf.ReturnedAt
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
	// Get student's level, days, time
	// Note: We don't check sent_to_classes here because GetClassGroups already filters for it
	var assignedLevel sql.NullInt32
	var classDays, classTime sql.NullString
	err := db.DB.QueryRow(`
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

	// Find existing groups for this key (level+days+time) that are not locked
	// Check each group index 1, 2, 3... until we find one with < 6 students
	for groupIndex := int32(1); ; groupIndex++ {
		var count int
		err := db.DB.QueryRow(`
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
			_, err = db.DB.Exec(`
				UPDATE scheduling SET class_group_index = $1, updated_at = CURRENT_TIMESTAMP
				WHERE lead_id = $2
		`, groupIndex, leadID)
			if err != nil {
				return 0, fmt.Errorf("failed to assign class group: %w", err)
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

// StartRound moves students in READY/LOCKED groups to in_classes status and increments round
func StartRound() error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Find all students in READY (4-5) or LOCKED (6) groups
	// We need to identify groups by counting students per (level, days, time, group_index)
	// Then update status for students in those groups

	// First, get all eligible students with their group info
	rows, err := tx.Query(`
		SELECT l.id, pt.assigned_level, s.class_days, s.class_time, COALESCE(s.class_group_index, 1)
		FROM leads l
		INNER JOIN placement_tests pt ON l.id = pt.lead_id
		INNER JOIN scheduling s ON l.id = s.lead_id
		WHERE l.status = 'ready_to_start'
		AND l.sent_to_classes = true
		AND pt.assigned_level IS NOT NULL
		AND s.class_days IS NOT NULL
		AND s.class_time IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to query students: %w", err)
	}
	defer rows.Close()

	// Group by (level, days, time, group_index) and count
	type groupKey struct {
		Level      int32
		Days       string
		Time       string
		GroupIndex int32
	}
	groupCounts := make(map[groupKey]int)
	studentGroups := make(map[uuid.UUID]groupKey)

	for rows.Next() {
		var leadID uuid.UUID
		var assignedLevel sql.NullInt32
		var classDays, classTime sql.NullString
		var groupIndex int32

		err := rows.Scan(&leadID, &assignedLevel, &classDays, &classTime, &groupIndex)
		if err != nil {
			return fmt.Errorf("failed to scan: %w", err)
		}

		key := groupKey{
			Level:      assignedLevel.Int32,
			Days:       classDays.String,
			Time:       classTime.String,
			GroupIndex: groupIndex,
		}
		groupCounts[key]++
		studentGroups[leadID] = key
	}
	rows.Close()

	// Collect lead IDs for READY (4-5) or LOCKED (6) groups
	var leadIDsToUpdate []uuid.UUID
	for leadID, key := range studentGroups {
		count := groupCounts[key]
		if count >= 4 { // READY or LOCKED
			leadIDsToUpdate = append(leadIDsToUpdate, leadID)
		}
	}

	// Group students by class_key to create sessions per class
	classGroups := make(map[string]struct {
		StartDate time.Time
		StartTime string
	})

	// Get start_date and start_time from scheduling for each class
	for leadID, key := range studentGroups {
		count := groupCounts[key]
		if count >= 4 { // READY or LOCKED
			// Get class_key and start_date/start_time
			var classKey string
			var startDate sql.NullTime
			var startTime sql.NullString

			err = tx.QueryRow(`
				SELECT 
					COALESCE(cg.class_key, 'L' || pt.assigned_level::TEXT || '|' || s.class_days || '|' || s.class_time || '|' || COALESCE(s.class_group_index, 1)::TEXT),
					s.start_date,
					TO_CHAR(s.start_time, 'HH24:MI') as start_time
				FROM scheduling s
				INNER JOIN placement_tests pt ON pt.lead_id = s.lead_id
				LEFT JOIN class_groups cg ON (
					cg.level = pt.assigned_level
					AND cg.class_days = s.class_days
					AND cg.class_time = s.class_time::text::text
					AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
				)
				WHERE s.lead_id = $1
			`, leadID).Scan(&classKey, &startDate, &startTime)

			if err == nil {
				// Ensure class_groups record exists
				_, err = tx.Exec(`
					INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, updated_at)
					VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
					ON CONFLICT (class_key) DO UPDATE SET updated_at = CURRENT_TIMESTAMP
				`, classKey, key.Level, key.Days, key.Time, key.GroupIndex)
				if err != nil {
					return fmt.Errorf("failed to ensure class group: %w", err)
				}

				// Store start date/time for this class (use first student's schedule)
				if _, exists := classGroups[classKey]; !exists {
					startDateVal := time.Now() // Default to today if not set
					if startDate.Valid {
						startDateVal = startDate.Time
					}
					startTimeVal := "07:30" // Default
					if startTime.Valid {
						startTimeVal = startTime.String
					}
					classGroups[classKey] = struct {
						StartDate time.Time
						StartTime string
					}{StartDate: startDateVal, StartTime: startTimeVal}
				}
			}
		}
	}

	// Update status to in_classes
	if len(leadIDsToUpdate) > 0 {
		// Update each lead individually (PostgreSQL array handling can be tricky)
		for _, leadID := range leadIDsToUpdate {
			_, err = tx.Exec(`UPDATE leads SET status = 'in_classes', updated_at = CURRENT_TIMESTAMP WHERE id = $1`, leadID)
			if err != nil {
				return fmt.Errorf("failed to update status for lead %s: %w", leadID, err)
			}
		}
	}

	// Create 8 sessions for each class
	for classKey, schedule := range classGroups {
		for i := 1; i <= 8; i++ {
			sessionDate := schedule.StartDate.AddDate(0, 0, (i-1)*7) // Weekly sessions
			startTimeParsed, err := time.Parse("15:04", schedule.StartTime)
			if err != nil {
				startTimeParsed, _ = time.Parse("15:04", "07:30") // Fallback
			}
			endTimeParsed := startTimeParsed.Add(2 * time.Hour)
			endTime := endTimeParsed.Format("15:04")

			_, err = tx.Exec(`
				INSERT INTO class_sessions (id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time, status, created_at, updated_at)
				VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'scheduled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
				ON CONFLICT (class_key, session_number) DO NOTHING
			`, classKey, i, sessionDate, schedule.StartTime, endTime)
			if err != nil {
				return fmt.Errorf("failed to create session %d for class %s: %w", i, classKey, err)
			}
		}
	}

	// Increment round
	_, err = tx.Exec(`
		INSERT INTO settings (key, value) VALUES ('current_round', '1')
		ON CONFLICT (key) DO UPDATE SET value = (CAST(value AS INTEGER) + 1)::TEXT, updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return fmt.Errorf("failed to increment round: %w", err)
	}

	return tx.Commit()
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
		WHERE l.status = 'ready_to_start'
		AND l.sent_to_classes = true
		AND pt.assigned_level = $1
		AND s.class_days = $2
		AND s.class_time = $3
		GROUP BY COALESCE(s.class_group_index, 1)
		ORDER BY COALESCE(s.class_group_index, 1)
	`, assignedLevel.Int32, classDays.String, classTime.String)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}
	defer rows.Close()

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

// SendLeadToClasses marks a lead as sent to classes board
func SendLeadToClasses(leadID uuid.UUID) error {
	_, err := db.DB.Exec(`
		UPDATE leads SET sent_to_classes = true, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, leadID)
	return err
}

// GenerateClassKey creates a stable class key from level, days, time, and group index
func GenerateClassKey(level int32, classDays, classTime string, groupIndex int32) string {
	return fmt.Sprintf("L%d|%s|%s|%d", level, classDays, classTime, groupIndex)
}

// GetClassGroupWorkflow gets workflow state for a class group by class_key
func GetClassGroupWorkflow(classKey string) (*ClassGroupWorkflow, error) {
	wf := &ClassGroupWorkflow{}
	var sentAt, returnedAt, roundStartedAt, roundClosedAt sql.NullTime
	var roundStartedBy, roundClosedBy sql.NullString
	var roundStatus sql.NullString
	err := db.DB.QueryRow(`
		SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, returned_at, updated_at,
		       COALESCE(round_status, 'not_started'), round_started_at, round_started_by::text, round_closed_at, round_closed_by::text
		FROM class_groups WHERE class_key = $1
	`, classKey).Scan(
		&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
		&wf.SentToMentor, &sentAt, &returnedAt, &wf.UpdatedAt,
		&roundStatus, &roundStartedAt, &roundStartedBy, &roundClosedAt, &roundClosedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Not found is OK - means not sent yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get class group workflow: %w", err)
	}
	wf.SentAt, wf.ReturnedAt = sentAt, returnedAt
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
			updated_at = $6
	`, classKey, level, classDays, classTime, classNumber, now)
	return err
}

// ReturnClassGroupFromMentor clears the sent_to_mentor flag and removes mentor assignment.
// Dashboard uses GetClassGroupsSentToMentor() which selects WHERE sent_to_mentor = true;
// this UPDATE sets sent_to_mentor = false so the class no longer matches and disappears from the list.
func ReturnClassGroupFromMentor(classKey string) error {
	now := time.Now()
	res, err := db.DB.Exec(`
		UPDATE class_groups
		SET sent_to_mentor = false,
			returned_at = $2,
			updated_at = $2
		WHERE class_key = $1
	`, classKey, now)
	if err != nil {
		return fmt.Errorf("return class_groups update: %w", err)
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		return fmt.Errorf("no class_group updated for class_key %q (not found or already returned)", classKey)
	}
	_, err = db.DB.Exec(`DELETE FROM mentor_assignments WHERE class_key = $1`, classKey)
	if err != nil {
		return fmt.Errorf("return mentor_assignments delete: %w", err)
	}
	return nil
}

// GetClassGroupWorkflowsBatch gets workflow state for multiple class keys
func GetClassGroupWorkflowsBatch(classKeys []string) (map[string]*ClassGroupWorkflow, error) {
	if len(classKeys) == 0 {
		return make(map[string]*ClassGroupWorkflow), nil
	}

	query := `SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, returned_at, updated_at,
		COALESCE(round_status, 'not_started'), round_started_at, round_started_by::text, round_closed_at, round_closed_by::text
		FROM class_groups WHERE class_key = ANY($1)`
	rows, err := db.DB.Query(query, classKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to query class group workflows: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*ClassGroupWorkflow)
	for rows.Next() {
		wf := &ClassGroupWorkflow{}
		var sentAt, returnedAt, roundStartedAt, roundClosedAt sql.NullTime
		var roundStartedBy, roundClosedBy, roundStatus sql.NullString
		err := rows.Scan(
			&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
			&wf.SentToMentor, &sentAt, &returnedAt, &wf.UpdatedAt,
			&roundStatus, &roundStartedAt, &roundStartedBy, &roundClosedAt, &roundClosedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class group workflow: %w", err)
		}
		wf.SentAt, wf.ReturnedAt = sentAt, returnedAt
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
	if currentStatus == "cancelled" {
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
	totalCoursePaid, err := GetTotalCoursePaid(leadID)
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
	defer rows.Close()

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
		db.DB.Exec(`DELETE FROM lead_payments WHERE id = $1`, payment.ID)
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
	refundableAmount, err := GetRefundableAmount(leadID)
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

	_, err = db.DB.Exec(`
		UPDATE leads SET 
			levels_purchased_total = $1,
			bundle_type = $2,
			updated_at = $3
		WHERE id = $4
	`, levelsPurchased, bundleType, time.Now(), leadID)

	return err
}

// GetFinanceSummary returns aggregated finance data for today and date range
func GetFinanceSummary(dateFrom, dateTo sql.NullTime) (*FinanceSummary, error) {
	today := time.Now().Format("2006-01-02")

	summary := &FinanceSummary{
		INByCategory:     make(map[string]int32),
		OUTByCategory:    make(map[string]int32),
		CreditsBreakdown: make(map[string]int),
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
		argIndex++
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
	defer categoryRows.Close()

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

	// Credits breakdown (levels remaining)
	var totalRemaining sql.NullInt32
	err = db.DB.QueryRow(`
		SELECT COALESCE(SUM(levels_purchased_total - COALESCE(levels_consumed, 0)), 0)
		FROM leads
		WHERE levels_purchased_total > 0
	`).Scan(&totalRemaining)
	if err == nil && totalRemaining.Valid {
		summary.TotalRemainingLevels = totalRemaining.Int32
	}

	// Credits breakdown by count
	creditsRows, err := db.DB.Query(`
		SELECT 
			CASE 
				WHEN (levels_purchased_total - COALESCE(levels_consumed, 0)) = 0 THEN '0'
				WHEN (levels_purchased_total - COALESCE(levels_consumed, 0)) = 1 THEN '1'
				WHEN (levels_purchased_total - COALESCE(levels_consumed, 0)) = 2 THEN '2'
				ELSE '3+'
			END as bucket,
			COUNT(*)
		FROM leads
		WHERE levels_purchased_total > 0
		GROUP BY bucket
	`)
	if err == nil {
		defer creditsRows.Close()
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
	defer rows.Close()
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
	defer rows.Close()

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
		if tx.TransactionType == "IN" {
			group.InTotal += tx.Amount
		} else if tx.TransactionType == "OUT" {
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
	defer rows.Close()

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
	defer rows.Close()

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
	defer tx.Rollback()

	// Get session info
	var classKey string
	var sessionNumber int32
	err = tx.QueryRow(`
		SELECT class_key, session_number FROM class_sessions WHERE id = $1
	`, sessionID).Scan(&classKey, &sessionNumber)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
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
					AND cg.class_time = s.class_time::text::text
					AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
				)
				WHERE cg.class_key = $2
			)
		`, now, classKey)
		if err != nil {
			return fmt.Errorf("failed to increment levels_consumed: %w", err)
		}
	}

	// Create default attendance records (all PRESENT) for all students in class who don't have records yet
	// This ensures students who weren't manually marked are treated as present by default
	_, err = tx.Exec(`
		INSERT INTO attendance (id, session_id, lead_id, status, created_at, updated_at)
		SELECT gen_random_uuid(), $1, s.lead_id, 'PRESENT', $2, $2
		FROM scheduling s
		INNER JOIN class_groups cg ON (
			cg.level = (SELECT pt.assigned_level FROM placement_tests pt WHERE pt.lead_id = s.lead_id)
			AND cg.class_days = s.class_days
			AND cg.class_time = s.class_time::text
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE cg.class_key = $3
		ON CONFLICT (session_id, lead_id) DO NOTHING
	`, sessionID, now, classKey)
	if err != nil {
		return fmt.Errorf("failed to create attendance records: %w", err)
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
		    status = 'scheduled', updated_at = $4
		WHERE id = $5
	`, newDate, newTime, endTime, now, sessionID)
	if err != nil {
		return fmt.Errorf("failed to reschedule session: %w", err)
	}
	return nil
}

// MarkAttendance upserts attendance record for a student in a session
func MarkAttendance(sessionID, leadID uuid.UUID, status string, notes string, markedByUserID uuid.UUID) error {
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
	defer rows.Close()

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
func AddStudentNote(leadID uuid.UUID, classKey string, sessionNumber sql.NullInt32, noteText string, createdByUserID uuid.UUID) error {
	now := time.Now()
	var classKeyNull sql.NullString
	if classKey != "" {
		classKeyNull = sql.NullString{String: classKey, Valid: true}
	}

	var noteID uuid.UUID
	err := db.DB.QueryRow(`
		INSERT INTO student_notes (id, lead_id, class_key, session_number, note_text, created_by_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $6)
		RETURNING id
	`, leadID, classKeyNull, sessionNumber, noteText, createdByUserID, now).Scan(&noteID)
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
		SELECT sn.id, sn.lead_id, sn.class_key, sn.session_number, sn.note_text, 
		       sn.created_by_user_id, u.email as created_by_email, sn.created_at, sn.updated_at
		FROM student_notes sn
		LEFT JOIN users u ON u.id = sn.created_by_user_id
		WHERE sn.lead_id::uuid = $1
		ORDER BY sn.created_at DESC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query student notes: %w", err)
	}
	defer rows.Close()

	var notes []*StudentNote
	for rows.Next() {
		n := &StudentNote{}
		var classKey sql.NullString
		var sessionNumberInt sql.NullInt32
		var createdByUserID sql.NullString
		var createdByEmail sql.NullString

		err := rows.Scan(
			&n.ID, &n.LeadID, &classKey, &sessionNumberInt,
			&n.NoteText, &createdByUserID, &createdByEmail, &n.CreatedAt, &n.UpdatedAt,
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
			AND cg.class_time = s.class_time::text
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
		WHERE ma.mentor_user_id = $1 AND cg.sent_to_mentor = true
		ORDER BY cg.level, cg.class_days, cg.class_time
	`, mentorUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor classes: %w", err)
	}
	defer rows.Close()

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

// CloseRound computes outcomes for all students in a class and sets high_priority_follow_up flag.
// Returns to Operations by setting sent_to_mentor = false and round_status = 'closed'.
func CloseRound(classKey string, closedByUserID uuid.UUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get all students in the class
	rows, err := tx.Query(`
		SELECT s.lead_id
		FROM scheduling s
		INNER JOIN placement_tests pt ON pt.lead_id = s.lead_id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND cg.class_time = s.class_time::text
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE cg.class_key = $1
	`, classKey)
	if err != nil {
		return fmt.Errorf("failed to query students: %w", err)
	}
	defer rows.Close()

	var leadIDs []uuid.UUID
	for rows.Next() {
		var leadID uuid.UUID
		if err := rows.Scan(&leadID); err != nil {
			return fmt.Errorf("failed to scan lead ID: %w", err)
		}
		leadIDs = append(leadIDs, leadID)
	}
	rows.Close()

	now := time.Now()
	// For each student, compute outcome and set follow-up flag if needed
	for _, leadID := range leadIDs {
		// Count absences
		var absences int
		err = tx.QueryRow(`
			SELECT COUNT(*)
			FROM attendance a
			INNER JOIN class_sessions cs ON a.session_id = cs.id
			WHERE a.lead_id = $1 AND cs.class_key = $2 AND a.attended = false
		`, leadID, classKey).Scan(&absences)
		if err != nil {
			return fmt.Errorf("failed to count absences: %w", err)
		}

		// Get grade
		var grade sql.NullString
		err = tx.QueryRow(`
			SELECT grade FROM grades WHERE lead_id = $1 AND class_key = $2 AND session_number = 8
		`, leadID, classKey).Scan(&grade)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("failed to get grade: %w", err)
		}

		// Compute decision: repeat if absences > 2 OR grade = 'F'
		shouldRepeat := absences > 2 || (grade.Valid && grade.String == "F")
		_ = shouldRepeat // outcome: repeat vs promote (stored when outcome column exists)

		// Check if student has no remaining credits
		var levelsPurchased, levelsConsumed sql.NullInt32
		err = tx.QueryRow(`
			SELECT levels_purchased_total, levels_consumed FROM leads WHERE id = $1
		`, leadID).Scan(&levelsPurchased, &levelsConsumed)
		if err != nil {
			return fmt.Errorf("failed to get levels: %w", err)
		}

		purchased := int32(0)
		if levelsPurchased.Valid {
			purchased = levelsPurchased.Int32
		}
		consumed := int32(0)
		if levelsConsumed.Valid {
			consumed = levelsConsumed.Int32
		}

		// Set high_priority_follow_up if no remaining credits
		highPriority := consumed >= purchased
		_, err = tx.Exec(`
			UPDATE leads SET high_priority_follow_up = $1, updated_at = $2 WHERE id = $3
		`, highPriority, now, leadID)
		if err != nil {
			return fmt.Errorf("failed to update follow-up flag: %w", err)
		}
	}

	// Return class to Operations and mark round closed
	_, err = tx.Exec(`
		UPDATE class_groups
		SET sent_to_mentor = false, returned_at = $1, updated_at = $1,
		    round_status = 'closed', round_closed_at = $1, round_closed_by = $3
		WHERE class_key = $2
	`, now, classKey, closedByUserID)
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

// SubmitFeedback submits community officer feedback for a student at session 4 or 8
func SubmitFeedback(leadID uuid.UUID, classKey string, sessionNumber int32, feedbackText string, followUpRequired bool, createdByUserID uuid.UUID) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		INSERT INTO community_officer_feedback (id, lead_id, class_key, session_number, feedback_text, follow_up_required, created_by_user_id, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $7)
		ON CONFLICT (lead_id, class_key, session_number) DO UPDATE SET
			feedback_text = EXCLUDED.feedback_text,
			follow_up_required = EXCLUDED.follow_up_required,
			updated_at = EXCLUDED.updated_at
	`, leadID, classKey, sessionNumber, feedbackText, followUpRequired, createdByUserID, now)
	return err
}

// GetClassFeedbackRecords returns all feedback records for a given class.
func GetClassFeedbackRecords(classKey string) ([]*CommunityOfficerFeedback, error) {
	rows, err := db.DB.Query(`
		SELECT id, lead_id, class_key, session_number, feedback_text, follow_up_required, status, created_by_user_id, created_at, updated_at
		FROM community_officer_feedback
		WHERE class_key = $1
		ORDER BY session_number, created_at DESC
	`, classKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*CommunityOfficerFeedback
	for rows.Next() {
		f := &CommunityOfficerFeedback{}
		var status sql.NullString
		if err := rows.Scan(&f.ID, &f.LeadID, &f.ClassKey, &f.SessionNumber, &f.FeedbackText, &f.FollowUpRequired, &status, &f.CreatedByUserID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		if status.Valid {
			f.Status = status.String
		} else {
			f.Status = "sent" // Default for existing records
		}
		results = append(results, f)
	}
	return results, rows.Err()
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
			AND cg.class_time = s.class_time::text
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		INNER JOIN class_sessions cs ON cs.class_key = cg.class_key AND cs.session_number = $1
		WHERE cs.status = 'completed'
		AND NOT EXISTS (
			SELECT 1 FROM community_officer_feedback cof
			WHERE cof.lead_id = l.id AND cof.class_key = cs.class_key AND cof.session_number = $1
		)
		ORDER BY l.full_name
	`, sessionNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending feedback: %w", err)
	}
	defer rows.Close()

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
	defer rows.Close()

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
		SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE role = $1
		ORDER BY email
	`, role)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u)
	}

	return users, rows.Err()
}

// GetAssignedMentors returns all distinct mentors assigned to at least one class, with their class count and evaluation data
func GetAssignedMentors() ([]struct {
	User               *User
	AssignedClassCount int
	Evaluation         *MentorEvaluation
}, error) {
	rows, err := db.DB.Query(`
		SELECT 
			u.id, u.email, u.password_hash, u.role, u.created_at, 
			COUNT(ma.class_key) as assigned_class_count,
			me.kpi_session_quality, me.kpi_trello, me.kpi_whatsapp, me.kpi_students_feedback,
			me.attendance_statuses
		FROM users u
		INNER JOIN mentor_assignments ma ON u.id = ma.mentor_user_id
		LEFT JOIN mentor_evaluations me ON u.id = me.mentor_id
		WHERE u.role = 'mentor'
		GROUP BY u.id, u.email, u.password_hash, u.role, u.created_at,
		         me.kpi_session_quality, me.kpi_trello, me.kpi_whatsapp, me.kpi_students_feedback,
		         me.attendance_statuses
		ORDER BY u.email
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query assigned mentors: %w", err)
	}
	defer rows.Close()

	var results []struct {
		User               *User
		AssignedClassCount int
		Evaluation         *MentorEvaluation
	}

	for rows.Next() {
		u := &User{}
		var count int
		var kpiSessionQuality, kpiTrello, kpiWhatsapp, kpiStudentsFeedback sql.NullInt32
		var attendanceStatusesJSON sql.NullString

		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &count,
			&kpiSessionQuality, &kpiTrello, &kpiWhatsapp, &kpiStudentsFeedback, &attendanceStatusesJSON); err != nil {
			return nil, fmt.Errorf("failed to scan mentor: %w", err)
		}

		var eval *MentorEvaluation
		if kpiSessionQuality.Valid {
			eval = &MentorEvaluation{
				MentorID:            u.ID,
				KPISessionQuality:   int(kpiSessionQuality.Int32),
				KPITrello:           int(kpiTrello.Int32),
				KPIWhatsapp:         int(kpiWhatsapp.Int32),
				KPIStudentsFeedback: int(kpiStudentsFeedback.Int32),
			}
			if attendanceStatusesJSON.Valid {
				// Parse JSON array
				var statuses []string
				if err := json.Unmarshal([]byte(attendanceStatusesJSON.String), &statuses); err == nil {
					eval.AttendanceStatuses = statuses
				} else {
					// Default to unknown if parse fails
					eval.AttendanceStatuses = []string{"unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown"}
				}
			} else {
				eval.AttendanceStatuses = []string{"unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown"}
			}
		}

		results = append(results, struct {
			User               *User
			AssignedClassCount int
			Evaluation         *MentorEvaluation
		}{
			User:               u,
			AssignedClassCount: count,
			Evaluation:         eval,
		})
	}

	return results, rows.Err()
}

// UpsertMentorEvaluation creates or updates a mentor evaluation
func UpsertMentorEvaluation(mentorID uuid.UUID, evaluatorID uuid.UUID, kpiSessionQuality, kpiTrello, kpiWhatsapp, kpiStudentsFeedback int, attendanceStatuses []string) error {
	// Validate attendance statuses
	if len(attendanceStatuses) != 8 {
		return fmt.Errorf("attendance statuses must have exactly 8 elements")
	}
	for _, status := range attendanceStatuses {
		if status != "on-time" && status != "late" && status != "absent" && status != "unknown" {
			return fmt.Errorf("invalid attendance status: %s (must be on-time, late, absent, or unknown)", status)
		}
	}

	// Validate KPIs (0-100)
	if kpiSessionQuality < 0 || kpiSessionQuality > 100 {
		return fmt.Errorf("kpi_session_quality must be between 0 and 100")
	}
	if kpiTrello < 0 || kpiTrello > 100 {
		return fmt.Errorf("kpi_trello must be between 0 and 100")
	}
	if kpiWhatsapp < 0 || kpiWhatsapp > 100 {
		return fmt.Errorf("kpi_whatsapp must be between 0 and 100")
	}
	if kpiStudentsFeedback < 0 || kpiStudentsFeedback > 100 {
		return fmt.Errorf("kpi_students_feedback must be between 0 and 100")
	}

	// Convert attendance statuses to JSON
	statusesJSON, err := json.Marshal(attendanceStatuses)
	if err != nil {
		return fmt.Errorf("failed to marshal attendance statuses: %w", err)
	}

	// Upsert (INSERT ... ON CONFLICT UPDATE)
	_, err = db.DB.Exec(`
		INSERT INTO mentor_evaluations (mentor_id, kpi_session_quality, kpi_trello, kpi_whatsapp, kpi_students_feedback, attendance_statuses, evaluator_id, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, NOW())
		ON CONFLICT (mentor_id) DO UPDATE SET
			kpi_session_quality = EXCLUDED.kpi_session_quality,
			kpi_trello = EXCLUDED.kpi_trello,
			kpi_whatsapp = EXCLUDED.kpi_whatsapp,
			kpi_students_feedback = EXCLUDED.kpi_students_feedback,
			attendance_statuses = EXCLUDED.attendance_statuses,
			evaluator_id = EXCLUDED.evaluator_id,
			updated_at = NOW()
	`, mentorID, kpiSessionQuality, kpiTrello, kpiWhatsapp, kpiStudentsFeedback, string(statusesJSON), evaluatorID)
	if err != nil {
		return fmt.Errorf("failed to upsert mentor evaluation: %w", err)
	}

	return nil
}

// GetUserByID returns a user by ID
func GetUserByID(userID string) (*User, error) {
	u := &User{}
	err := db.DB.QueryRow(`
		SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return u, nil
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

// GetClassGroupsSentToMentor returns all class groups where sent_to_mentor = true
func GetClassGroupsSentToMentor() ([]*ClassGroupWorkflow, error) {
	rows, err := db.DB.Query(`
		SELECT class_key, level, class_days, class_time, class_number,
		       sent_to_mentor, sent_at, returned_at, updated_at
		FROM class_groups
		WHERE sent_to_mentor = true
		ORDER BY level, class_days, class_time
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query class groups: %w", err)
	}
	defer rows.Close()

	var groups []*ClassGroupWorkflow
	for rows.Next() {
		g := &ClassGroupWorkflow{}
		var sentAt, returnedAt sql.NullTime

		err := rows.Scan(
			&g.ClassKey, &g.Level, &g.ClassDays, &g.ClassTime, &g.ClassNumber,
			&g.SentToMentor, &sentAt, &returnedAt, &g.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class group: %w", err)
		}

		g.SentAt = sentAt
		g.ReturnedAt = returnedAt
		groups = append(groups, g)
	}

	return groups, rows.Err()
}

// StudentSuccessClassRow represents one active class for Student Success list.
type StudentSuccessClassRow struct {
	ClassKey     string
	Level        int32
	ClassDays    string
	ClassTime    string
	ClassNumber  int32
	MentorEmail  string
	MentorName   string
	MentorUserID string
	StudentCount int
}

// GetActiveClassesForStudentSuccess returns all classes where round_status = 'active'.
// Includes mentor email/name (if assigned), schedule, level, class_number, student_count.
func GetActiveClassesForStudentSuccess() ([]StudentSuccessClassRow, error) {
	rows, err := db.DB.Query(`
		SELECT cg.class_key, cg.level, cg.class_days, cg.class_time, cg.class_number,
		       COALESCE(u.email, ''), COALESCE(u.email, ''), COALESCE(ma.mentor_user_id::text, '')
		FROM class_groups cg
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		WHERE cg.round_status = 'active'
		ORDER BY cg.level, cg.class_days, cg.class_time
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query active classes: %w", err)
	}
	defer rows.Close()

	var out []StudentSuccessClassRow
	for rows.Next() {
		var r StudentSuccessClassRow
		if err := rows.Scan(&r.ClassKey, &r.Level, &r.ClassDays, &r.ClassTime, &r.ClassNumber,
			&r.MentorEmail, &r.MentorName, &r.MentorUserID); err != nil {
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
	defer rows.Close()

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

// GetStudentSuccessClassDetail returns class + students (with missed_sessions) + sessions + feedback for round_status='active' classes.
func GetStudentSuccessClassDetail(classKey string) (classGroup *ClassGroupWorkflow, students []*ClassStudent, sessions []*ClassSession, missedSessions map[uuid.UUID][]int32, feedbackRecords []*CommunityOfficerFeedback, completedCount int, err error) {
	classGroup, err = GetClassGroupByKey(classKey)
	if err != nil || classGroup == nil {
		return nil, nil, nil, nil, nil, 0, err
	}
	if classGroup.RoundStatus != "active" {
		return nil, nil, nil, nil, nil, 0, fmt.Errorf("class is not active")
	}
	students, err = GetStudentsInClassGroup(classKey)
	if err != nil {
		return nil, nil, nil, nil, nil, 0, err
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
		feedbackRecords = []*CommunityOfficerFeedback{}
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
		SELECT l.id, l.full_name, l.phone, s.class_group_index
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND cg.class_time = s.class_time::text
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE cg.class_key = $1
		ORDER BY l.full_name
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to query students: %w", err)
	}
	defer rows.Close()

	var students []*ClassStudent
	for rows.Next() {
		s := &ClassStudent{}
		var groupIndex sql.NullInt32

		err := rows.Scan(&s.LeadID, &s.FullName, &s.Phone, &groupIndex)
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

// StartClassRound starts the round for a class group: sets status to 'active' and creates 8 sessions
func StartClassRound(classKey string, startedByUserID uuid.UUID, startDate time.Time, startTime string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()

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
			f.resolved_at
		FROM class_sessions s
		JOIN attendance a ON s.id = a.session_id
		JOIN leads l ON a.lead_id = l.id
		LEFT JOIN users u ON a.marked_by_user_id = u.id
		LEFT JOIN followups f ON f.class_key = s.class_key AND f.lead_id = l.id AND f.session_number = s.session_number
		WHERE s.class_key = $1 
		  AND a.status IN ('ABSENT', 'LATE')
		  AND (f.resolved IS NULL OR f.resolved = false)
		  AND (f.status IS NULL OR f.status != 'no_response')
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
		argIdx++
	}

	query += " ORDER BY s.session_number DESC, a.created_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query absence feed: %w", err)
	}
	defer rows.Close()

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
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan absence feed item: %w", err)
		}

		item.SessionDate = sDate.Format("2006-01-02")
		item.MentorNote = mNote.String

		if fID.Valid {
			fid, _ := uuid.Parse(fID.String)
			item.FollowUp = &FollowUpInfo{
				ID:         fid,
				Status:     fStatus.String,
				LastNote:   fNote.String,
				UpdatedAt:  fUpdatedAt.Time,
				Resolved:   fResolved.Bool,
				ResolvedAt: fResolvedAt,
			}
		}

		results = append(results, item)
	}

	return results, rows.Err()
}

// CreateFollowUp creates or updates a follow-up note
func CreateFollowUp(classKey string, leadID uuid.UUID, sessionNumber int, note string, status string, createdBy uuid.UUID) error {
	_, err := db.DB.Exec(`
		INSERT INTO followups (class_key, lead_id, session_number, note, status, created_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (class_key, lead_id, session_number) 
		DO UPDATE SET note = $4, status = $5, created_by = $6, updated_at = NOW()
	`, classKey, leadID, sessionNumber, note, status, createdBy)
	return err
}

// ResolveFollowUp marks a follow-up as resolved
func ResolveFollowUp(id uuid.UUID, resolvedBy uuid.UUID) error {
	_, err := db.DB.Exec(`
		UPDATE followups 
		SET resolved = true, resolved_at = NOW(), resolved_by_user_id = $1, updated_at = NOW()
		WHERE id = $2
	`, resolvedBy, id)
	return err
}

// UpdateFollowUpStatus updates the status of a follow-up
func UpdateFollowUpStatus(id uuid.UUID, status string) error {
	_, err := db.DB.Exec(`
		UPDATE followups SET status = $1, updated_at = NOW() WHERE id = $2
	`, status, id)
	return err
}

// UpdateFollowUp handles generic update of follow-up details
func UpdateFollowUp(id uuid.UUID, status, note string, resolved bool, userID uuid.UUID) error {
	var err error
	if resolved {
		_, err = db.DB.Exec(`
			UPDATE followups 
			SET status = $1, note = $2, resolved = true, resolved_at = NOW(), resolved_by_user_id = $3, updated_at = NOW()
			WHERE id = $4
		`, status, note, userID, id)
	} else {
		_, err = db.DB.Exec(`
			UPDATE followups 
			SET status = $1, note = $2, updated_at = NOW()
			WHERE id = $3
		`, status, note, id)
	}
	return err
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
		WHERE f.class_key = $1 AND f.resolved = $2 AND f.status = 'no_response'
		ORDER BY f.created_at DESC
	`, classKey, resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to query follow-ups: %w", err)
	}
	defer rows.Close()

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
		VALUES ($1, $2, $3, '', 'none', $4, NOW(), true, NOW(), $4)
		ON CONFLICT (class_key, lead_id, session_number) 
		DO UPDATE SET resolved = true, resolved_at = NOW(), resolved_by_user_id = $4, updated_at = NOW()
	`, classKey, leadID, sessionNumber, resolvedBy)
	return err
}
