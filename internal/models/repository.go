package models

import (
	"database/sql"
	"fmt"
	"time"

	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
)

func GetNextAction(status string) string {
	actions := map[string]string{
		"lead_created":        "Book placement test",
		"test_booked":         "Run placement test",
		"tested":              "Send offer",
		"offer_sent":          "Wait for booking",
		"booking_confirmed":   "Collect payment",
		"deposit_paid":        "Collect remaining",
		"paid_full":           "Assign schedule",
		"waiting_for_round":   "Assign to round",
		"schedule_assigned":   "Mark ready to start",
		"ready_to_start":      "Ready for activation",
	}
	if action, ok := actions[status]; ok {
		return action
	}
	return "Review"
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

func GetAllLeads(statusFilter, searchFilter string) ([]*LeadListItem, error) {
	query := `
		SELECT 
			l.id, l.full_name, l.phone, l.source, l.notes, l.status, 
			l.created_by_user_id, l.created_at, l.updated_at,
			pt.assigned_level,
			p.remaining_balance, p.amount_paid
		FROM leads l
		LEFT JOIN placement_tests pt ON l.id = pt.lead_id
		LEFT JOIN payments p ON l.id = p.lead_id
		WHERE 1=1
	`
	
	args := []interface{}{}
	argIndex := 1
	
	// Apply status filter
	if statusFilter != "" {
		query += fmt.Sprintf(" AND l.status = $%d", argIndex)
		args = append(args, statusFilter)
		argIndex++
	}
	
	// Apply search filter (name or phone)
	if searchFilter != "" {
		query += fmt.Sprintf(" AND (LOWER(l.full_name) LIKE LOWER($%d) OR l.phone LIKE $%d)", argIndex, argIndex)
		searchPattern := "%" + searchFilter + "%"
		args = append(args, searchPattern)
		argIndex++
	}
	
	query += " ORDER BY l.created_at DESC"

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
		var remainingBalance, amountPaid sql.NullInt32

		err := rows.Scan(
			&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status,
			&lead.CreatedByUserID, &lead.CreatedAt, &lead.UpdatedAt,
			&assignedLevel,
			&remainingBalance, &amountPaid,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lead: %w", err)
		}

		leads = append(leads, &LeadListItem{
			Lead:          lead,
			AssignedLevel: assignedLevel,
			PaymentStatus: GetPaymentStatus(remainingBalance, amountPaid),
			NextAction:    GetNextAction(lead.Status),
		})
	}

	return leads, nil
}

func GetLeadByID(id uuid.UUID) (*LeadDetail, error) {
	// Get lead
	lead := &Lead{}
	err := db.DB.QueryRow(`
		SELECT id, full_name, phone, source, notes, status, created_by_user_id, created_at, updated_at
		FROM leads WHERE id = $1
	`, id).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status,
		&lead.CreatedByUserID, &lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get lead: %w", err)
	}

	detail := &LeadDetail{Lead: lead}

	// Get placement test
	pt := &PlacementTest{}
	err = db.DB.QueryRow(`
		SELECT id, lead_id, test_date, test_time, test_type, assigned_level, test_notes, run_by_user_id, placement_test_fee, placement_test_fee_paid, updated_at
		FROM placement_tests WHERE lead_id = $1
	`, id).Scan(
		&pt.ID, &pt.LeadID, &pt.TestDate, &pt.TestTime, &pt.TestType, &pt.AssignedLevel,
		&pt.TestNotes, &pt.RunByUserID, &pt.PlacementTestFee, &pt.PlacementTestFeePaid, &pt.UpdatedAt,
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
		SELECT id, lead_id, expected_round, class_days, class_time, start_date, start_time, updated_at
		FROM scheduling WHERE lead_id = $1
	`, id).Scan(
		&scheduling.ID, &scheduling.LeadID, &scheduling.ExpectedRound, &scheduling.ClassDays,
		&scheduling.ClassTime, &scheduling.StartDate, &scheduling.StartTime, &scheduling.UpdatedAt,
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
		INSERT INTO leads (id, full_name, phone, source, notes, status, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, leadID, fullName, phone, sourceVal, notesVal, "lead_created", createdByID, now, now)
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
		UPDATE leads SET full_name = $1, phone = $2, source = $3, notes = $4, status = $5, updated_at = $6
		WHERE id = $7
	`, detail.Lead.FullName, detail.Lead.Phone, detail.Lead.Source, detail.Lead.Notes, detail.Lead.Status, now, detail.Lead.ID)
	if err != nil {
		return fmt.Errorf("failed to update lead: %w", err)
	}

	// Upsert placement test
	if detail.PlacementTest != nil {
		_, err = tx.Exec(`
			INSERT INTO placement_tests (id, lead_id, test_date, test_time, test_type, assigned_level, test_notes, run_by_user_id, placement_test_fee, placement_test_fee_paid, updated_at)
			VALUES (COALESCE((SELECT id FROM placement_tests WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (lead_id) DO UPDATE SET
				test_date = EXCLUDED.test_date,
				test_time = EXCLUDED.test_time,
				test_type = EXCLUDED.test_type,
				assigned_level = EXCLUDED.assigned_level,
				test_notes = EXCLUDED.test_notes,
				run_by_user_id = EXCLUDED.run_by_user_id,
				placement_test_fee = EXCLUDED.placement_test_fee,
				placement_test_fee_paid = EXCLUDED.placement_test_fee_paid,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.PlacementTest.TestDate, detail.PlacementTest.TestTime,
			detail.PlacementTest.TestType, detail.PlacementTest.AssignedLevel,
			detail.PlacementTest.TestNotes, detail.PlacementTest.RunByUserID,
			detail.PlacementTest.PlacementTestFee, detail.PlacementTest.PlacementTestFeePaid, now)
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
			INSERT INTO scheduling (id, lead_id, expected_round, class_days, class_time, start_date, start_time, updated_at)
			VALUES (COALESCE((SELECT id FROM scheduling WHERE lead_id = $1), gen_random_uuid()), $1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (lead_id) DO UPDATE SET
				expected_round = EXCLUDED.expected_round,
				class_days = EXCLUDED.class_days,
				class_time = EXCLUDED.class_time,
				start_date = EXCLUDED.start_date,
				start_time = EXCLUDED.start_time,
				updated_at = EXCLUDED.updated_at
		`, detail.Lead.ID, detail.Scheduling.ExpectedRound, detail.Scheduling.ClassDays,
			detail.Scheduling.ClassTime, detail.Scheduling.StartDate, detail.Scheduling.StartTime, now)
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
