package models

import (
	"database/sql"
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
		"lead_created":      StageNewLead,
		"test_booked":       StageTestBooked,
		"tested":            StageTested,
		"offer_sent":        StageOfferSent,
		"ready_to_start":    StageReadyToStart,
		// Payment-based statuses need context (handled separately with payment state)
		"booking_confirmed": StageOfferSent, // Default mapping, will be upgraded based on payment
		"paid_full":         StageBookingConfirmedPaidFull,
		"deposit_paid":      StageBookingConfirmedDeposit,
		// Schedule-based statuses
		"waiting_for_round": StageScheduleSet,
		"schedule_assigned":  StageScheduleSet,
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
				StageNewLead:                  true,
				StageTestBooked:               true,
				StageTested:                   true,
				StageOfferSent:                true,
				StageBookingConfirmedDeposit:  true,
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

func GetAllLeads(statusFilter, searchFilter, paymentFilter, hotFilter string, includeCancelled bool) ([]*LeadListItem, error) {
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
			FinalPrice:        finalPrice,
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
		SELECT id, full_name, phone, source, notes, status, sent_to_classes, created_by_user_id, created_at, updated_at
		FROM leads WHERE id = $1
	`, id).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status,
		&lead.SentToClasses, &lead.CreatedByUserID, &lead.CreatedAt, &lead.UpdatedAt,
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
	err = db.DB.QueryRow(`
		SELECT id, lead_id, expected_round, class_days, class_time, start_date, start_time, class_group_index, updated_at
		FROM scheduling WHERE lead_id = $1
	`, id).Scan(
		&scheduling.ID, &scheduling.LeadID, &scheduling.ExpectedRound, &scheduling.ClassDays,
		&scheduling.ClassTime, &scheduling.StartDate, &scheduling.StartTime, &scheduling.ClassGroupIndex, &scheduling.UpdatedAt,
	)
	if err == nil {
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
		ID:             leadID,
		FullName:       fullName,
		Phone:          phone,
		Source:         sourceVal,
		Notes:          notesVal,
		Status:         "lead_created",
		CreatedByUserID: createdByID,
		CreatedAt:      now,
		UpdatedAt:      now,
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
		_, err = tx.Exec(`
			INSERT INTO scheduling (id, lead_id, expected_round, class_days, class_time, start_date, start_time, class_group_index, updated_at)
			VALUES (COALESCE((SELECT id FROM scheduling WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (lead_id) DO UPDATE SET
				expected_round = EXCLUDED.expected_round,
				class_days = EXCLUDED.class_days,
				class_time = EXCLUDED.class_time,
				start_date = EXCLUDED.start_date,
				start_time = EXCLUDED.start_time,
				class_group_index = EXCLUDED.class_group_index,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.Scheduling.ExpectedRound, detail.Scheduling.ClassDays,
			detail.Scheduling.ClassTime, detail.Scheduling.StartDate, detail.Scheduling.StartTime, detail.Scheduling.ClassGroupIndex, now)
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
	err := db.DB.QueryRow(`
		SELECT id, email, password_hash, role, created_at
		FROM users WHERE email = $1
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
			LeadID:    leadID,
			FullName:  fullName,
			Phone:     phone,
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
	err := db.DB.QueryRow(`
		SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, returned_at, updated_at
		FROM class_groups WHERE class_key = $1
	`, classKey).Scan(
		&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
		&wf.SentToMentor, &wf.SentAt, &wf.ReturnedAt, &wf.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Not found is OK - means not sent yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get class group workflow: %w", err)
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

// ReturnClassGroupFromMentor clears the sent_to_mentor flag
func ReturnClassGroupFromMentor(classKey string) error {
	now := time.Now()
	_, err := db.DB.Exec(`
		UPDATE class_groups
		SET sent_to_mentor = false,
			returned_at = $2,
			updated_at = $2
		WHERE class_key = $1
	`, classKey, now)
	return err
}

// GetClassGroupWorkflowsBatch gets workflow state for multiple class keys
func GetClassGroupWorkflowsBatch(classKeys []string) (map[string]*ClassGroupWorkflow, error) {
	if len(classKeys) == 0 {
		return make(map[string]*ClassGroupWorkflow), nil
	}

	// Build query with placeholders
	query := `SELECT class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, returned_at, updated_at
		FROM class_groups WHERE class_key = ANY($1)`
	
	rows, err := db.DB.Query(query, classKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to query class group workflows: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*ClassGroupWorkflow)
	for rows.Next() {
		wf := &ClassGroupWorkflow{}
		err := rows.Scan(
			&wf.ClassKey, &wf.Level, &wf.ClassDays, &wf.ClassTime, &wf.ClassNumber,
			&wf.SentToMentor, &wf.SentAt, &wf.ReturnedAt, &wf.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan class group workflow: %w", err)
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
	
	// Validate refund doesn't exceed total course paid
	totalCoursePaid, err := GetTotalCoursePaid(leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate refund amount: %w", err)
	}
	if amount > totalCoursePaid {
		return nil, fmt.Errorf("refund amount (%d) cannot exceed total course paid (%d)", amount, totalCoursePaid)
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
		INByCategory: make(map[string]int32),
		OUTByCategory: make(map[string]int32),
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
