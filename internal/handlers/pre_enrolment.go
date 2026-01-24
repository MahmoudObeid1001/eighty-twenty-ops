package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"
	"eighty-twenty-ops/internal/util"

	"github.com/google/uuid"
)

type PreEnrolmentHandler struct {
	cfg *config.Config
}

func NewPreEnrolmentHandler(cfg *config.Config) *PreEnrolmentHandler {
	return &PreEnrolmentHandler{cfg: cfg}
}

func isValidAssignedLevel(level int) bool {
	return level >= 1 && level <= 8
}

func (h *PreEnrolmentHandler) List(w http.ResponseWriter, r *http.Request) {
	// Read filter parameters from query string
	statusFilter := r.URL.Query().Get("status")
	searchFilter := r.URL.Query().Get("search")
	paymentFilter := r.URL.Query().Get("payment")
	hotFilter := r.URL.Query().Get("hot") // Changed from "filter" to "hot"
	includeCancelled := r.URL.Query().Get("include_cancelled") == "1" || r.URL.Query().Get("include_cancelled") == "true"
	// When explicitly filtering by status=cancelled, include cancelled even if checkbox off
	if statusFilter == "cancelled" {
		includeCancelled = true
	}

	// Check for flash messages in query params (separate from filter status)
	flashMessage := ""
	savedParam := r.URL.Query().Get("saved")
	deletedParam := r.URL.Query().Get("deleted")
	statusFlashParam := r.URL.Query().Get("status_flash")
	sentToClassesParam := r.URL.Query().Get("sentToClasses")

	if sentToClassesParam == "1" {
		flashMessage = "Lead sent to classes board successfully!"
	} else if deletedParam == "1" {
		flashMessage = "Lead cancelled successfully!"
	} else if r.URL.Query().Get("cancelled") == "1" {
		flashMessage = "Lead cancelled successfully!"
	} else if savedParam == "1" {
		flashMessage = "Lead saved successfully!"
	} else if statusFlashParam != "" {
		statusMessages := map[string]string{
			"test_booked": "Placement test booked successfully!",
			"tested":      "Lead marked as tested!",
			"offer_sent":  "Offer sent successfully!",
			"waiting":     "Lead moved to waiting list!",
			"ready":       "Lead marked as ready to start!",
		}
		if msg, ok := statusMessages[statusFlashParam]; ok {
			flashMessage = msg
		}
	}

	h.cfg.Debugf("List: statusFilter=%q, searchFilter=%q, paymentFilter=%q, hotFilter=%q, includeCancelled=%v", statusFilter, searchFilter, paymentFilter, hotFilter, includeCancelled)

	// Get filtered leads
	leads, err := models.GetAllLeads(statusFilter, searchFilter, paymentFilter, hotFilter, includeCancelled)
	if err != nil {
		log.Printf("ERROR: Failed to load leads: %v", err)
		http.Error(w, fmt.Sprintf("Failed to load leads: %v", err), http.StatusInternalServerError)
		return
	}

	h.cfg.Debugf("List: returned %d leads", len(leads))

	// Count follow-ups due for banner
	// Get total count of hot leads (need to fetch all leads without hot filter)
	var followUpCount int
	if hotFilter == "1" || hotFilter == "hot" {
		// All leads in filtered list are hot leads
		followUpCount = len(leads)
	} else {
		// Get all leads to count hot leads accurately (exclude cancelled)
		allLeads, err := models.GetAllLeads("", "", "", "", false)
		if err == nil {
			for _, lead := range allLeads {
				if lead.FollowUpDue {
					followUpCount++
				}
			}
		}
	}

	data := map[string]interface{}{
		"Title":            "Pre-Enrolment - Eighty Twenty",
		"Leads":            leads,
		"UserRole":         middleware.GetUserRole(r),
		"FlashMessage":     flashMessage,
		"StatusFilter":     statusFilter,
		"SearchFilter":     searchFilter,
		"PaymentFilter":    paymentFilter,
		"HotFilter":        hotFilter,
		"IncludeCancelled": includeCancelled,
		"FollowUpCount":    followUpCount,
	}
	renderTemplate(w, "pre_enrolment_list.html", data)
}

func (h *PreEnrolmentHandler) NewForm(w http.ResponseWriter, r *http.Request) {
	h.cfg.Debugf("📝 NewForm() called - rendering pre_enrolment_new.html template")
	data := map[string]interface{}{
		"Title":    "New Lead - Eighty Twenty",
		"UserRole": middleware.GetUserRole(r),
	}
	renderTemplate(w, "pre_enrolment_new.html", data)
	h.cfg.Debugf("  → Template render complete")
}

func (h *PreEnrolmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fullName := r.FormValue("full_name")
	phone := r.FormValue("phone")
	source := r.FormValue("source")
	notes := r.FormValue("notes")

	if fullName == "" || phone == "" {
		data := map[string]interface{}{
			"Title":    "New Lead - Eighty Twenty",
			"Error":    "Full name and phone are required",
			"UserRole": middleware.GetUserRole(r),
		}
		renderTemplate(w, "pre_enrolment_new.html", data)
		return
	}

	// Validate source is one of allowed options
	allowedSources := map[string]bool{
		"Facebook": true,
		"WhatsApp": true,
		"Admin":    true,
		"Referral": true,
		"Other":    true,
	}
	if source == "" || !allowedSources[source] {
		source = "Other" // Default to Other if invalid
	}

	userID := middleware.GetUserID(r)
	lead, err := models.CreateLead(fullName, phone, source, notes, userID)
	if err != nil {
		// Check if it's a phone constraint error
		var phoneErr *models.PhoneAlreadyExistsError
		if errors.As(err, &phoneErr) {
			// phoneErr already has the details from CreateLead
		} else if phoneConstraintErr := models.IsPhoneConstraintError(err); phoneConstraintErr != nil {
			// Try to get existing lead
			if existingLead, findErr := models.GetLeadByPhone(phone); findErr == nil {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:          phone,
					ExistingLeadID: &existingLead.ID,
					Message:        "Phone number already exists",
				}
			} else {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:   phone,
					Message: "Phone number already exists",
				}
			}
		}
		
		if phoneErr != nil {
			// Show form again with error and preserved values
			data := map[string]interface{}{
				"Title":            "New Lead - Eighty Twenty",
				"Error":            phoneErr.Message,
				"PhoneError":       phoneErr.Message,
				"ExistingLeadID":   phoneErr.ExistingLeadID,
				"PreservedFullName": fullName,
				"PreservedPhone":    phone,
				"PreservedSource":   source,
				"PreservedNotes":    notes,
				"UserRole":         middleware.GetUserRole(r),
			}
			renderTemplate(w, "pre_enrolment_new.html", data)
			return
		}
		
		http.Error(w, fmt.Sprintf("Failed to create lead: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s", lead.ID.String()), http.StatusFound)
}

func (h *PreEnrolmentHandler) Detail(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
		return
	}

	userRole := middleware.GetUserRole(r)
	isModerator := userRole == "moderator"

	// Calculate placement test remaining fee
	var placementTestRemaining int32 = 0
	if detail.PlacementTest != nil {
		if detail.PlacementTest.PlacementTestFee.Valid && detail.PlacementTest.PlacementTestFeePaid.Valid {
			placementTestRemaining = detail.PlacementTest.PlacementTestFee.Int32 - detail.PlacementTest.PlacementTestFeePaid.Int32
			if placementTestRemaining < 0 {
				placementTestRemaining = 0
			}
		} else if detail.PlacementTest.PlacementTestFee.Valid {
			placementTestRemaining = detail.PlacementTest.PlacementTestFee.Int32
		} else {
			placementTestRemaining = 100 // default
		}
	} else {
		placementTestRemaining = 100 // default
	}

	// Compute hot lead flags for detail page
	// Create a temporary LeadListItem to compute flags
	var amountPaid, finalPrice sql.NullInt32
	if detail.Payment != nil {
		amountPaid = detail.Payment.AmountPaid
	}
	if detail.Offer != nil {
		finalPrice = detail.Offer.FinalPrice
	}
	var testDate sql.NullTime
	if detail.PlacementTest != nil {
		testDate = detail.PlacementTest.TestDate
	}

	tempItem := &models.LeadListItem{
		Lead:       detail.Lead,
		TestDate:   testDate,
		AmountPaid: amountPaid,
		FinalPrice: finalPrice,
	}
	models.ComputeLeadFlags(tempItem)

	today := time.Now().Format("2006-01-02")
	// Get lead payments for display
	leadPayments, err := models.GetLeadPayments(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to get lead payments: %v", err)
		leadPayments = []*models.LeadPayment{} // Empty slice on error
	}
	
	// Check for error messages
	errorMsg := ""
	phoneError := ""
	var existingLeadID *uuid.UUID
	errorType := r.URL.Query().Get("error")
	switch errorType {
	case "future_date":
		errorMsg = "Refund date cannot be in the future"
	case "refund_required":
		errorMsg = "Refund amount is required when cancelling a lead with course payments"
	case "invalid_amount":
		errorMsg = "Invalid refund amount. Amount must be greater than 0"
	case "amount_exceeds":
		maxStr := r.URL.Query().Get("max")
		if maxStr != "" {
			errorMsg = fmt.Sprintf("Refund amount cannot exceed total course paid (%s EGP)", maxStr)
		} else {
			errorMsg = "Refund amount cannot exceed total course paid"
		}
	case "method_required":
		errorMsg = "Refund payment method is required when cancelling a lead with course payments"
	case "invalid_method":
		errorMsg = "Invalid refund payment method"
	case "date_required":
		errorMsg = "Refund date is required when cancelling a lead with course payments"
	case "invalid_date":
		errorMsg = "Invalid refund date format. Please use YYYY-MM-DD format"
	case "refund_failed":
		errorMsg = "Failed to create refund. Please try again"
	case "phone_exists":
		errorMsg = "Phone number already exists"
		phoneError = "Phone number already exists"
		// Try to parse existing lead ID
		if existingIDStr := r.URL.Query().Get("existing_lead_id"); existingIDStr != "" {
			if parsedID, err := uuid.Parse(existingIDStr); err == nil {
				existingLeadID = &parsedID
			}
		}
	}
	
	// Check for success messages
	successMsg := ""
	if r.URL.Query().Get("cancelled") == "1" && r.URL.Query().Get("refund_recorded") == "1" {
		successMsg = "Lead cancelled and refund recorded."
	} else if r.URL.Query().Get("cancelled") == "1" {
		successMsg = "Lead cancelled successfully."
	}
	
	// Check if we should show cancel modal
	showCancelModal := r.URL.Query().Get("action") == "cancel"
	
	// Calculate payment totals for UI
	var finalPriceValue int32 = 0
	if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
		finalPriceValue = detail.Offer.FinalPrice.Int32
	}
	
	totalCoursePaid, err := models.GetTotalCoursePaid(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to get total course paid: %v", err)
		totalCoursePaid = 0
	}
	
	var remainingBalance int32 = 0
	if finalPriceValue > 0 {
		remainingBalance = finalPriceValue - totalCoursePaid
		if remainingBalance < 0 {
			remainingBalance = 0
		}
	}
	
	// Check if fully paid: offer final price exists and total course paid equals final price
	isFullyPaid := finalPriceValue > 0 && totalCoursePaid >= finalPriceValue
	
	// Compute follow-up banner logic: only show if NOT fully paid AND in pipeline statuses
	// Pipeline statuses that require chasing: lead_created, test_booked, tested, offer_sent
	pipelineStatuses := map[string]bool{
		"lead_created": true,
		"test_booked":  true,
		"tested":       true,
		"offer_sent":   true,
	}
	showFollowUpBanner := !isFullyPaid && tempItem.FollowUpDue && pipelineStatuses[detail.Lead.Status]
	
	// Get status display info for unified status banner.
	// When paid in full, always show "Paid in Full" (override DB status which may still be offer_sent).
	statusInfo := models.GetStatusDisplayInfo(detail.Lead.Status)
	if isFullyPaid {
		statusInfo = models.GetStatusDisplayInfo("paid_full")
		// Sync DB: set status to paid_full if still offer_sent (or other pipeline status)
		if detail.Lead.Status != "paid_full" && detail.Lead.Status != "cancelled" {
			_ = models.UpdateLeadStatusFromPayment(leadID)
		}
	}
	
	data := map[string]interface{}{
		"Title":                  fmt.Sprintf("Pre-Enrolment Detail - %s", detail.Lead.FullName),
		"Detail":                 detail,
		"UserRole":               userRole,
		"IsModerator":            isModerator,
		"PlacementTestRemaining": placementTestRemaining,
		"FollowUpDue":            tempItem.FollowUpDue,
		"ShowFollowUpBanner":     showFollowUpBanner,
		"HotLevel":               tempItem.HotLevel,
		"DaysSinceLastProgress":  tempItem.DaysSinceLastProgress,
		"Today":                  today,
		"LeadPayments":           leadPayments,
		"FinalPrice":             finalPriceValue,
		"TotalCoursePaid":        totalCoursePaid,
		"RemainingBalance":       remainingBalance,
		"IsFullyPaid":            isFullyPaid,
		"StatusDisplayName":      statusInfo.DisplayName,
		"StatusBgColor":          statusInfo.BgColor,
		"StatusTextColor":        statusInfo.TextColor,
		"StatusBorderColor":      statusInfo.BorderColor,
		"Error":                  errorMsg,
		"PhoneError":             phoneError,
		"ExistingLeadID":         existingLeadID,
		"SuccessMessage":         successMsg,
		"ShowCancelModal":        showCancelModal,
	}
	
	// If showing cancel modal, calculate additional fields needed for modal
	if showCancelModal {
		// Calculate placement test paid (read-only, not refundable)
		var placementTestPaid int32 = 0
		if detail.PlacementTest != nil && detail.PlacementTest.PlacementTestFeePaid.Valid {
			placementTestPaid = detail.PlacementTest.PlacementTestFeePaid.Int32
		}
		data["PlacementTestPaid"] = placementTestPaid
	}
	renderTemplate(w, "pre_enrolment_detail.html", data)
}

// renderDetailWithError fetches the lead, builds detail page data with Error set, and renders.
// Used when validation fails (e.g. schedule required for mark_ready).
// It includes all the same data as Detail() to ensure template variables are available.
func (h *PreEnrolmentHandler) renderDetailWithError(w http.ResponseWriter, r *http.Request, leadID uuid.UUID, errMsg string) {
	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
		return
	}
	userRole := middleware.GetUserRole(r)
	isModerator := userRole == "moderator"
	var placementTestRemaining int32 = 0
	if detail.PlacementTest != nil {
		if detail.PlacementTest.PlacementTestFee.Valid && detail.PlacementTest.PlacementTestFeePaid.Valid {
			placementTestRemaining = detail.PlacementTest.PlacementTestFee.Int32 - detail.PlacementTest.PlacementTestFeePaid.Int32
			if placementTestRemaining < 0 {
				placementTestRemaining = 0
			}
		} else if detail.PlacementTest.PlacementTestFee.Valid {
			placementTestRemaining = detail.PlacementTest.PlacementTestFee.Int32
		} else {
			placementTestRemaining = 100
		}
	} else {
		placementTestRemaining = 100
	}
	var amountPaid, finalPrice sql.NullInt32
	if detail.Payment != nil {
		amountPaid = detail.Payment.AmountPaid
	}
	if detail.Offer != nil {
		finalPrice = detail.Offer.FinalPrice
	}
	var testDate sql.NullTime
	if detail.PlacementTest != nil {
		testDate = detail.PlacementTest.TestDate
	}
	tempItem := &models.LeadListItem{
		Lead:       detail.Lead,
		TestDate:   testDate,
		AmountPaid: amountPaid,
		FinalPrice: finalPrice,
	}
	models.ComputeLeadFlags(tempItem)
	
	today := time.Now().Format("2006-01-02")
	
	// Get lead payments for display
	leadPayments, err := models.GetLeadPayments(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to get lead payments: %v", err)
		leadPayments = []*models.LeadPayment{} // Empty slice on error
	}
	
	// Calculate payment totals for UI
	var finalPriceValue int32 = 0
	if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
		finalPriceValue = detail.Offer.FinalPrice.Int32
	}
	
	totalCoursePaid, err := models.GetTotalCoursePaid(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to get total course paid: %v", err)
		totalCoursePaid = 0
	}
	
	var remainingBalance int32 = 0
	if finalPriceValue > 0 {
		remainingBalance = finalPriceValue - totalCoursePaid
		if remainingBalance < 0 {
			remainingBalance = 0
		}
	}

	// Check if fully paid: offer final price exists and total course paid equals final price
	isFullyPaid := finalPriceValue > 0 && totalCoursePaid >= finalPriceValue

	data := map[string]interface{}{
		"Title":                  fmt.Sprintf("Pre-Enrolment Detail - %s", detail.Lead.FullName),
		"Detail":                 detail,
		"UserRole":               userRole,
		"IsModerator":            isModerator,
		"PlacementTestRemaining": placementTestRemaining,
		"FollowUpDue":            tempItem.FollowUpDue,
		"HotLevel":               tempItem.HotLevel,
		"DaysSinceLastProgress":  tempItem.DaysSinceLastProgress,
		"Today":                  today,
		"LeadPayments":           leadPayments,
		"FinalPrice":             finalPriceValue,
		"TotalCoursePaid":        totalCoursePaid,
		"RemainingBalance":       remainingBalance,
		"IsFullyPaid":            isFullyPaid,
		"Error":                  errMsg,
		"SuccessMessage":         "", // No success message in error rendering
	}
	renderTemplate(w, "pre_enrolment_detail.html", data)
}

func (h *PreEnrolmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	// Read action parameter
	action := r.FormValue("action")
	h.cfg.Debugf("🔄 Update: leadID=%s, action=%s, userRole=%s, path=%s", leadID, action, userRole, r.URL.Path)

	// Handle different actions
	switch action {
	case "mark_test_booked":
		h.cfg.Debugf("  → Action: mark_test_booked")
		// Validate placement test fields
		testDate := r.FormValue("test_date")
		testTime := r.FormValue("test_time")
		testType := r.FormValue("test_type")
		if testDate == "" || testTime == "" || testType == "" {
			log.Printf("ERROR: Validation failed for mark_test_booked: test_date=%q, test_time=%q, test_type=%q", testDate, testTime, testType)
			http.Error(w, "Test date, time, and type are required to book placement test", http.StatusBadRequest)
			return
		}

		// Parse and book test
		var testDateVal sql.NullTime
		if t, err := time.Parse("2006-01-02", testDate); err == nil {
			testDateVal = sql.NullTime{Time: t, Valid: true}
		}
		var testTimeVal sql.NullString
		if testTime != "" {
			testTimeVal = sql.NullString{String: testTime, Valid: true}
		}
		var testTypeVal sql.NullString
		if testType != "" {
			testTypeVal = sql.NullString{String: testType, Valid: true}
		}
		var testNotes sql.NullString
		if notes := r.FormValue("test_notes"); notes != "" {
			testNotes = sql.NullString{String: notes, Valid: true}
		}

		err = models.BookPlacementTest(leadID, testDateVal, testTimeVal, testTypeVal, testNotes)
		if err != nil {
			log.Printf("ERROR: Failed to book placement test: %v", err)
			http.Error(w, fmt.Sprintf("Failed to book placement test: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Test booked successfully, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=test_booked", http.StatusFound)
		return

	case "mark_tested":
		h.cfg.Debugf("  → Action: mark_tested")
		// Server-side check: moderators cannot update status
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
			return
		}

		// Update placement test if fields are provided
		if r.FormValue("assigned_level") != "" || r.FormValue("test_notes") != "" {
			detail, err := models.GetLeadByID(leadID)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
				return
			}

			if detail.PlacementTest == nil {
				detail.PlacementTest = &models.PlacementTest{LeadID: leadID}
			}

			if assignedLevel := r.FormValue("assigned_level"); assignedLevel != "" {
				level, err := strconv.Atoi(assignedLevel)
				if err != nil || !isValidAssignedLevel(level) {
					h.renderDetailWithError(w, r, leadID, "Invalid assigned level. Allowed: 1–8.")
					return
				}
				detail.PlacementTest.AssignedLevel = sql.NullInt32{Int32: int32(level), Valid: true}
			}
			if testNotes := r.FormValue("test_notes"); testNotes != "" {
				detail.PlacementTest.TestNotes = sql.NullString{String: testNotes, Valid: true}
			}

			if err := models.UpdatePlacementTest(detail.PlacementTest); err != nil {
				http.Error(w, fmt.Sprintf("Failed to update placement test: %v", err), http.StatusInternalServerError)
				return
			}
		}

		err = models.UpdateLeadStatus(leadID, "tested")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to tested, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=tested", http.StatusFound)
		return

	case "mark_offer_sent":
		h.cfg.Debugf("  → Action: mark_offer_sent")
		// Server-side check: moderators cannot update status
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
			return
		}

		// Validate offer fields are present
		bundle := r.FormValue("bundle")
		finalPrice := r.FormValue("final_price")
		if bundle == "" || finalPrice == "" {
			log.Printf("ERROR: Validation failed for mark_offer_sent: bundle=%q, final_price=%q", bundle, finalPrice)
			http.Error(w, "Bundle and Final Price are required to send offer", http.StatusBadRequest)
			return
		}

		// Update or create offer
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
			return
		}

		if detail.Offer == nil {
			detail.Offer = &models.Offer{LeadID: leadID}
		}

		if b, err := strconv.Atoi(bundle); err == nil {
			detail.Offer.BundleLevels = sql.NullInt32{Int32: int32(b), Valid: true}
		}
		if fp, err := strconv.Atoi(finalPrice); err == nil {
			detail.Offer.FinalPrice = sql.NullInt32{Int32: int32(fp), Valid: true}
		}
		if basePrice := r.FormValue("base_price"); basePrice != "" {
			if bp, err := strconv.Atoi(basePrice); err == nil {
				detail.Offer.BasePrice = sql.NullInt32{Int32: int32(bp), Valid: true}
			}
		}
		if discount := r.FormValue("discount"); discount != "" {
			if strings.HasSuffix(discount, "%") {
				if pct, err := strconv.Atoi(strings.TrimSuffix(discount, "%")); err == nil {
					detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(pct), Valid: true}
					detail.Offer.DiscountType = sql.NullString{String: "percent", Valid: true}
				}
			} else {
				if amt, err := strconv.Atoi(discount); err == nil {
					detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(amt), Valid: true}
					detail.Offer.DiscountType = sql.NullString{String: "amount", Valid: true}
				}
			}
		}

		if err := models.UpdateOffer(detail.Offer); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update offer: %v", err), http.StatusInternalServerError)
			return
		}

		err = models.UpdateLeadStatus(leadID, "offer_sent")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to offer_sent, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=offer_sent", http.StatusFound)
		return

	case "move_waiting":
		h.cfg.Debugf("  → Action: move_waiting")
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
			return
		}

		// WAITING allowed regardless of course payments. No refund; payments stay.
		err = models.UpdateLeadStatus(leadID, "waiting_for_round")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to waiting_for_round, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=waiting", http.StatusFound)
		return

	case "mark_ready":
		h.cfg.Debugf("  → Action: mark_ready")
		// Server-side check: moderators cannot update status
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
			return
		}

		// Get lead detail to validate prerequisites
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
			return
		}

		// Check if fully paid
		var finalPriceValue int32 = 0
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			finalPriceValue = detail.Offer.FinalPrice.Int32
		}
		
		totalCoursePaid, err := models.GetTotalCoursePaid(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get total course paid: %v", err)
			totalCoursePaid = 0
		}
		
		isFullyPaid := finalPriceValue > 0 && totalCoursePaid >= finalPriceValue
		if !isFullyPaid {
			h.renderDetailWithError(w, r, leadID, "Cannot mark READY_TO_START before full payment. Course must be fully paid first.")
			return
		}

		// Check assigned level exists
		if detail.PlacementTest == nil || !detail.PlacementTest.AssignedLevel.Valid {
			h.renderDetailWithError(w, r, leadID, "Cannot mark READY_TO_START: Assigned level must be set first.")
			return
		}

		// Schedule required: both Class Days and Class Time must be present
		classDaysMR := r.FormValue("class_days")
		classTimeMR := r.FormValue("class_time")
		if classDaysMR == "" || classTimeMR == "" {
			h.renderDetailWithError(w, r, leadID, "Cannot mark READY_TO_START: Both Class Days and Class Time are required.")
			return
		}
		allowedClassDaysMR := map[string]bool{"Sun/Wed": true, "Sat/Tues": true, "Mon/Thu": true}
		allowedClassTimesMR := map[string]bool{"07:30": true, "10:00": true}
		if !allowedClassDaysMR[classDaysMR] {
			h.renderDetailWithError(w, r, leadID, "Invalid class days. Allowed: Sun/Wed, Sat/Tues, Mon/Thu.")
			return
		}
		if !allowedClassTimesMR[classTimeMR] {
			h.renderDetailWithError(w, r, leadID, "Invalid class time. Allowed: 07:30, 10:00.")
			return
		}

		if err := models.UpsertSchedulingClassDaysTime(leadID, classDaysMR, classTimeMR); err != nil {
			log.Printf("ERROR: Failed to save schedule: %v", err)
			http.Error(w, fmt.Sprintf("Failed to save schedule: %v", err), http.StatusInternalServerError)
			return
		}
		err = models.UpdateLeadStatus(leadID, "ready_to_start")
		if err != nil {
			log.Printf("ERROR: Failed to update status: %v", err)
			http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Status updated to ready_to_start, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?status_flash=ready", http.StatusFound)
		return

	case "send_to_classes":
		h.cfg.Debugf("  → Action: send_to_classes")
		// Server-side check: moderators cannot send to classes
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot send leads to classes", http.StatusForbidden)
			return
		}

		// Verify lead is ready (has level, days, time)
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
			return
		}

		// Check eligibility: status must be ready_to_start, and must have assigned_level, class_days, class_time
		if detail.Lead.Status != "ready_to_start" {
			h.renderDetailWithError(w, r, leadID, "Lead must be READY_TO_START to send to classes.")
			return
		}
		if detail.PlacementTest == nil || !detail.PlacementTest.AssignedLevel.Valid {
			h.renderDetailWithError(w, r, leadID, "Lead must have an assigned level to send to classes.")
			return
		}
		if detail.Scheduling == nil || !detail.Scheduling.ClassDays.Valid || !detail.Scheduling.ClassTime.Valid {
			h.renderDetailWithError(w, r, leadID, "Lead must have class days and class time set to send to classes.")
			return
		}

		// Send to classes
		err = models.SendLeadToClasses(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to send lead to classes: %v", err)
			// Check if AJAX request
			if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"success": false, "error": "Failed to send lead to classes"}`))
				return
			}
			http.Error(w, fmt.Sprintf("Failed to send lead to classes: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Lead sent to classes, redirecting to list")
		// Check if AJAX request - return JSON instead of redirect
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true, "message": "Lead sent to classes board successfully"}`))
			return
		}
		http.Redirect(w, r, "/pre-enrolment?sentToClasses=1", http.StatusFound)
		return

	case "cancel":
		h.cfg.Debugf("  → Action: cancel")
		// Server-side check: moderators cannot cancel
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot cancel leads", http.StatusForbidden)
			return
		}

		// Get lead detail for cancel modal
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
			return
		}

		// If this is a POST with refund data, process cancellation + refund
		if r.Method == http.MethodPost {
			refundAmountStr := r.FormValue("refund_amount")
			refundMethod := r.FormValue("refund_method")
			refundDateStr := r.FormValue("refund_date")
			refundNotes := r.FormValue("refund_notes")
			
			// Get course payments total
			totalCoursePaid, err := models.GetTotalCoursePaid(leadID)
			if err != nil {
				h.renderDetailWithError(w, r, leadID, fmt.Sprintf("Failed to calculate course payments: %v", err))
				return
			}
			
		// If there are course payments, refund is required
		if totalCoursePaid > 0 {
			// Validate refund amount
			if refundAmountStr == "" {
				// Show modal again with error - redirect to detail page with action=cancel and error
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=refund_required", leadID.String()), http.StatusFound)
				return
			}
				
			refundAmount, err := strconv.Atoi(refundAmountStr)
			if err != nil || refundAmount <= 0 {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=invalid_amount", leadID.String()), http.StatusFound)
				return
			}
			
			if int32(refundAmount) > totalCoursePaid {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=amount_exceeds&max=%d", leadID.String(), totalCoursePaid), http.StatusFound)
				return
			}
			
			// Validate payment method
			if refundMethod == "" {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=method_required", leadID.String()), http.StatusFound)
				return
			}
			
			allowedMethods := map[string]bool{
				"vodafone_cash": true,
				"bank_transfer": true,
				"paypal":        true,
				"other":         true,
			}
			if !allowedMethods[refundMethod] {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=invalid_method", leadID.String()), http.StatusFound)
				return
			}
			
			// Validate refund date
			if refundDateStr == "" {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=date_required", leadID.String()), http.StatusFound)
				return
			}
			
			refundDate, err := util.ParseDateLocal(refundDateStr)
			if err != nil {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=invalid_date", leadID.String()), http.StatusFound)
				return
			}
			
			// Validate refund date is not in the future
			if err := util.ValidateNotFutureDate(refundDate); err != nil {
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=future_date", leadID.String()), http.StatusFound)
				return
			}
				
				// Build notes: combine default message with user notes
				refundNotesText := "Refund for cancelled lead"
				if refundNotes != "" {
					refundNotesText = refundNotesText + ". " + refundNotes
				}
				
				// Create refund transaction
				_, err = models.CreateRefund(leadID, int32(refundAmount), refundMethod, refundDate, refundNotesText)
				if err != nil {
					log.Printf("ERROR: Failed to create refund: %v", err)
				// Check if it's a validation error
				if err.Error() == "payment date cannot be in the future" {
					http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=future_date", leadID.String()), http.StatusFound)
					return
				}
				if strings.Contains(err.Error(), "cannot exceed total course paid") {
					http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=amount_exceeds", leadID.String()), http.StatusFound)
					return
				}
				http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?action=cancel&error=refund_failed", leadID.String()), http.StatusFound)
				return
				}
			}
			
			// Cancel the lead (soft cancel)
			err = models.CancelLead(leadID)
			if err != nil {
				log.Printf("ERROR: Failed to cancel lead: %v", err)
				http.Error(w, fmt.Sprintf("Failed to cancel lead: %v", err), http.StatusInternalServerError)
				return
			}
			
			h.cfg.Debugf("  ✅ Lead cancelled successfully, redirecting to list")
			// Redirect with success message
			http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?cancelled=1&refund_recorded=1", leadID.String()), http.StatusFound)
			return
		}
		
		// GET request: show cancel modal with refund options
		// Calculate placement test paid (read-only, not refundable)
		var placementTestPaid int32 = 0
		if detail.PlacementTest != nil && detail.PlacementTest.PlacementTestFeePaid.Valid {
			placementTestPaid = detail.PlacementTest.PlacementTestFeePaid.Int32
		}
		
		// Calculate course paid total
		totalCoursePaid, err := models.GetTotalCoursePaid(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get total course paid: %v", err)
			totalCoursePaid = 0
		}
		
		// Get offer final price for remaining balance calculation
		var remainingBalance int32 = 0
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			remainingBalance = detail.Offer.FinalPrice.Int32 - totalCoursePaid
			if remainingBalance < 0 {
				remainingBalance = 0
			}
		}
		
		// Get lead payments for display
		leadPayments, err := models.GetLeadPayments(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get lead payments: %v", err)
			leadPayments = []*models.LeadPayment{} // Empty slice on error
		}
		
		// Calculate final price
		var finalPriceValue int32 = 0
		if detail.Offer != nil && detail.Offer.FinalPrice.Valid {
			finalPriceValue = detail.Offer.FinalPrice.Int32
		}
		
		today := time.Now().Format("2006-01-02")
		data := map[string]interface{}{
			"Title":                  fmt.Sprintf("Cancel Lead - %s", detail.Lead.FullName),
			"Detail":                 detail,
			"UserRole":               userRole,
			"IsModerator":            false,
			"ShowCancelModal":        true,
			"PlacementTestPaid":      placementTestPaid,
			"TotalCoursePaid":        totalCoursePaid,
			"RemainingBalance":       remainingBalance,
			"FinalPrice":             finalPriceValue,
			"LeadPayments":           leadPayments,
			"Today":                  today,
		}
		renderTemplate(w, "pre_enrolment_detail.html", data)
		return

	case "reopen":
		h.cfg.Debugf("  → Action: reopen")
		// Server-side check: moderators cannot reopen
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot reopen leads", http.StatusForbidden)
			return
		}
		
		// Reopen the cancelled lead
		err = models.ReopenLead(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to reopen lead: %v", err)
			http.Error(w, fmt.Sprintf("Failed to reopen lead: %v", err), http.StatusInternalServerError)
			return
		}
		
		h.cfg.Debugf("  ✅ Lead reopened successfully, redirecting to detail")
		http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?reopened=1", leadID.String()), http.StatusFound)
		return

	case "save", "":
		// Default action: save all fields without forcing status change
		h.cfg.Debugf("  → Action: save (default)")
		// Use SaveFull logic but allow moderators for basic info only
		h.SaveFull(w, r)
		return

	default:
		h.cfg.Debugf("  ⚠️  Unknown action: %s, treating as save", action)
		// Unknown action, treat as save
		h.SaveFull(w, r)
		return
	}
}

func (h *PreEnrolmentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	status := r.FormValue("status")
	if status == "" {
		http.Error(w, "Status is required", http.StatusBadRequest)
		return
	}

	err = models.UpdateLeadStatus(leadID, status)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?saved=1", http.StatusFound)
}

// SaveFull performs a full save of all form fields and redirects to list.
// IMPORTANT: This function now auto-classifies stage based on form completion.
// Stage is computed from the furthest completed block and automatically upgraded.
// Never downgrades stage - only upgrades based on what's filled.
// Validation: only validates basic lead fields (name, phone) + final_price if stage reaches OFFER_SENT
// Does NOT require offer/pricing fields for test booking - can save test info without offer
func (h *PreEnrolmentHandler) SaveFull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	h.cfg.Debugf("💾 SaveFull: leadID=%s, userRole=%s", leadID, userRole)

	// Validate basic lead fields (name and phone are required)
	// Load existing lead first to get current values if form fields are missing
	existingDetail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
		return
	}
	
	fullName := r.FormValue("full_name")
	phone := r.FormValue("phone")
	
	// If fields are empty, use existing values (might happen with some form submissions)
	if fullName == "" {
		fullName = existingDetail.Lead.FullName
		h.cfg.Debugf("  ⚠️  full_name empty in form, using existing: %q", fullName)
	}
	if phone == "" {
		phone = existingDetail.Lead.Phone
		h.cfg.Debugf("  ⚠️  phone empty in form, using existing: %q", phone)
	}
	
	if fullName == "" || phone == "" {
		log.Printf("ERROR: Validation failed for SaveFull: fullName=%q, phone=%q, leadID=%s", fullName, phone, leadID)
		http.Error(w, "Full name and phone are required", http.StatusBadRequest)
		return
	}

	// Parse form values
	detail := &models.LeadDetail{
		Lead: &models.Lead{
			ID:       leadID,
			FullName: fullName,
			Phone:    phone,
		},
	}

	// Moderator restrictions: only allow editing Lead Info section
	if userRole == "moderator" {
		h.cfg.Debugf("  🔒 Moderator save: only updating Lead Info section")
		// Only update lead info fields, ignore all other sections
		if source := r.FormValue("source"); source != "" {
			allowedSources := map[string]bool{
				"Facebook":  true,
				"WhatsApp":  true,
				"Instagram": true,
				"Referral":  true,
				"Walk-in":   true,
				"Other":     true,
			}
			if !allowedSources[source] {
				source = "Other"
			}
			detail.Lead.Source = sql.NullString{String: source, Valid: true}
		}
		if notes := r.FormValue("notes"); notes != "" {
			detail.Lead.Notes = sql.NullString{String: notes, Valid: true}
		}
		// Keep existing status (existingDetail already loaded above)
		detail.Lead.Status = existingDetail.Lead.Status

		// Update only lead info
		err = models.UpdateLeadBasicInfo(detail.Lead)
		if err != nil {
			log.Printf("ERROR: Failed to update lead: %v", err)
			http.Error(w, fmt.Sprintf("Failed to update lead: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Moderator save successful")
		http.Redirect(w, r, "/pre-enrolment?saved=1", http.StatusFound)
		return
	}

	// Admin: can update all sections
	h.cfg.Debugf("  👤 Admin save: updating all sections")

	if source := r.FormValue("source"); source != "" {
		// Validate source is one of allowed options
		allowedSources := map[string]bool{
			"Facebook":  true,
			"WhatsApp":  true,
			"Instagram": true,
			"Referral":  true,
			"Walk-in":   true,
			"Other":     true,
		}
		if !allowedSources[source] {
			source = "Other" // Default to Other if invalid
		}
		detail.Lead.Source = sql.NullString{String: source, Valid: true}
	}
	if notes := r.FormValue("notes"); notes != "" {
		detail.Lead.Notes = sql.NullString{String: notes, Valid: true}
	}

	// existingDetail already loaded above for validation
	currentStatus := existingDetail.Lead.Status

	// Auto-compute stage from form completion (before parsing all sections)
	// This will be used after all sections are parsed

	// Placement test
	if r.FormValue("test_date") != "" || r.FormValue("assigned_level") != "" || r.FormValue("placement_test_fee") != "" {
		pt := &models.PlacementTest{LeadID: leadID}
		if testDate := r.FormValue("test_date"); testDate != "" {
			if t, err := time.Parse("2006-01-02", testDate); err == nil {
				pt.TestDate = sql.NullTime{Time: t, Valid: true}
			}
		}
		if testTime := r.FormValue("test_time"); testTime != "" {
			pt.TestTime = sql.NullString{String: testTime, Valid: true}
		}
		if testType := r.FormValue("test_type"); testType != "" {
			pt.TestType = sql.NullString{String: testType, Valid: true}
		}
		if assignedLevel := r.FormValue("assigned_level"); assignedLevel != "" {
			level, err := strconv.Atoi(assignedLevel)
			if err != nil || !isValidAssignedLevel(level) {
				h.renderDetailWithError(w, r, leadID, "Invalid assigned level. Allowed: 1–8.")
				return
			}
			pt.AssignedLevel = sql.NullInt32{Int32: int32(level), Valid: true}
		}
		if testNotes := r.FormValue("test_notes"); testNotes != "" {
			pt.TestNotes = sql.NullString{String: testNotes, Valid: true}
		}
		// Placement test fee fields
		if feeStr := r.FormValue("placement_test_fee"); feeStr != "" {
			if fee, err := strconv.Atoi(feeStr); err == nil {
				pt.PlacementTestFee = sql.NullInt32{Int32: int32(fee), Valid: true}
			}
		}
		if paidStr := r.FormValue("placement_test_fee_paid"); paidStr != "" {
			if paid, err := strconv.Atoi(paidStr); err == nil {
				pt.PlacementTestFeePaid = sql.NullInt32{Int32: int32(paid), Valid: true}
			}
		}

		// Payment method/date: required only if paid > 0. Otherwise keep them NULL.
		amountPaid := int32(0)
		if pt.PlacementTestFeePaid.Valid {
			amountPaid = pt.PlacementTestFeePaid.Int32
		}
		if amountPaid > 0 {
			paymentDateStr := r.FormValue("placement_test_payment_date")
			if paymentDateStr == "" {
				h.renderDetailWithError(w, r, leadID, "Payment date is required when placement test fee is paid.")
				return
			}
			t, err := util.ParseDateLocal(paymentDateStr)
			if err != nil {
				h.renderDetailWithError(w, r, leadID, "Invalid payment date for placement test.")
				return
			}
			if err := util.ValidateNotFutureDate(t); err != nil {
				h.renderDetailWithError(w, r, leadID, "Payment date cannot be in the future")
				return
			}
			pt.PlacementTestPaymentDate = sql.NullTime{Time: t, Valid: true}

			paymentMethod := r.FormValue("placement_test_payment_method")
			if paymentMethod == "" {
				h.renderDetailWithError(w, r, leadID, "Payment method is required when placement test fee is paid.")
				return
			}
			pt.PlacementTestPaymentMethod = sql.NullString{String: paymentMethod, Valid: true}
		} else {
			pt.PlacementTestPaymentDate = sql.NullTime{Valid: false}
			pt.PlacementTestPaymentMethod = sql.NullString{Valid: false}
		}

		detail.PlacementTest = pt
	}

	// Offer
	// Only process offer if bundle is explicitly provided OR if save_offer action is triggered
	// This prevents auto-selecting bundle when saving other sections (e.g., placement test payment)
	finalPriceStr := r.FormValue("final_price")
	bundleStr := r.FormValue("bundle")
	basePriceStr := r.FormValue("base_price")
	discountStr := r.FormValue("discount")
	
	// Check if this is an explicit offer save action
	action := r.FormValue("action")
	isExplicitOfferSave := action == "save_offer" || action == "mark_offer_sent"
	
	// Check if offer already exists (existingDetail loaded above)
	existingOffer := existingDetail.Offer
	if existingOffer != nil {
		h.cfg.Debugf("  💰 Existing offer found: FinalPrice.Valid=%v, FinalPrice.Int32=%d, leadID=%s",
			existingOffer.FinalPrice.Valid, func() int32 {
				if existingOffer.FinalPrice.Valid {
					return existingOffer.FinalPrice.Int32
				}
				return 0
			}(), leadID)
	}
	
	// Process offer ONLY if:
	// 1. Bundle is explicitly provided (not empty), OR
	// 2. Explicit offer save action is triggered, OR
	// 3. Final price is explicitly provided (user manually set it)
	// Do NOT process if only existing offer exists (prevents auto-updating when saving other sections)
	shouldProcessOffer := bundleStr != "" || isExplicitOfferSave || finalPriceStr != ""
	h.cfg.Debugf("  💰 Offer processing check: bundleStr=%q, finalPriceStr=%q, isExplicitOfferSave=%v, existingOffer!=nil=%v, shouldProcess=%v, leadID=%s",
		bundleStr, finalPriceStr, isExplicitOfferSave, existingOffer != nil, shouldProcessOffer, leadID)
	
	if shouldProcessOffer {
		offer := &models.Offer{LeadID: leadID}
		
		// If offer exists, start with existing values
		if existingOffer != nil {
			offer.BundleLevels = existingOffer.BundleLevels
			offer.BasePrice = existingOffer.BasePrice
			offer.DiscountValue = existingOffer.DiscountValue
			offer.DiscountType = existingOffer.DiscountType
			offer.FinalPrice = existingOffer.FinalPrice
		}
		
		// Hardcoded bundle prices
		bundlePrices := map[int32]int32{
			1: 1300,
			2: 2400,
			3: 3300,
			4: 4000,
		}
		
		// Update with form values
		var basePrice int32 = 0
		if bundleStr != "" {
			if b, err := strconv.Atoi(bundleStr); err == nil && b >= 1 && b <= 4 {
				bundleLevel := int32(b)
				offer.BundleLevels = sql.NullInt32{Int32: bundleLevel, Valid: true}
				// Auto-set base price from hardcoded bundle prices
				if price, ok := bundlePrices[bundleLevel]; ok {
					basePrice = price
					offer.BasePrice = sql.NullInt32{Int32: basePrice, Valid: true}
					h.cfg.Debugf("  💰 Bundle %d selected: auto-set base_price=%d, leadID=%s", bundleLevel, basePrice, leadID)
				}
			}
		}
		
		// If base price was set from bundle, use it; otherwise use form value
		if basePrice == 0 && basePriceStr != "" {
			if bp, err := strconv.Atoi(basePriceStr); err == nil {
				basePrice = int32(bp)
				offer.BasePrice = sql.NullInt32{Int32: basePrice, Valid: true}
			}
		}
		
		// If base price exists from existing offer and wasn't set above, use it
		if basePrice == 0 && existingOffer != nil && existingOffer.BasePrice.Valid {
			basePrice = existingOffer.BasePrice.Int32
		}
		
		// Parse discount (could be "500" or "10%")
		var discountAmount int32 = 0
		if discountStr != "" {
			if strings.HasSuffix(discountStr, "%") {
				if pct, err := strconv.Atoi(strings.TrimSuffix(discountStr, "%")); err == nil && basePrice > 0 {
					discountAmount = (basePrice * int32(pct)) / 100
					offer.DiscountValue = sql.NullInt32{Int32: int32(pct), Valid: true}
					offer.DiscountType = sql.NullString{String: "percent", Valid: true}
					h.cfg.Debugf("  💰 Discount: %d%% = %d EGP (from base %d), leadID=%s", pct, discountAmount, basePrice, leadID)
				}
			} else {
				if amt, err := strconv.Atoi(discountStr); err == nil {
					discountAmount = int32(amt)
					offer.DiscountValue = sql.NullInt32{Int32: discountAmount, Valid: true}
					offer.DiscountType = sql.NullString{String: "amount", Valid: true}
					h.cfg.Debugf("  💰 Discount: %d EGP, leadID=%s", discountAmount, leadID)
				}
			}
		}
		
		// Calculate final price: base - discount (if base price is set)
		// PRIORITY: If final_price is explicitly provided, use it (highest priority)
		// Otherwise, calculate from base - discount
		// Otherwise, preserve existing
		if finalPriceStr != "" {
			if fp, err := strconv.Atoi(finalPriceStr); err == nil {
				offer.FinalPrice = sql.NullInt32{Int32: int32(fp), Valid: true}
				h.cfg.Debugf("  💰 Offer Final Price: EXPLICIT from form=%d, leadID=%s", fp, leadID)
			} else {
				h.cfg.Debugf("  ⚠️  Failed to parse final_price: %q, error: %v, leadID=%s", finalPriceStr, err, leadID)
			}
		} else if basePrice > 0 {
			// Auto-calculate final price from base - discount (only if final_price not explicitly provided)
			calculatedFinalPrice := basePrice - discountAmount
			if calculatedFinalPrice < 0 {
				calculatedFinalPrice = 0
			}
			offer.FinalPrice = sql.NullInt32{Int32: calculatedFinalPrice, Valid: true}
			h.cfg.Debugf("  💰 Offer Final Price: AUTO-CALCULATED=%d (base=%d - discount=%d), leadID=%s", 
				calculatedFinalPrice, basePrice, discountAmount, leadID)
		} else if existingOffer != nil && existingOffer.FinalPrice.Valid {
			// Preserve existing final price if not provided in form and can't calculate
			offer.FinalPrice = existingOffer.FinalPrice
			h.cfg.Debugf("  💰 Offer Final Price: PRESERVED existing=%d, leadID=%s", existingOffer.FinalPrice.Int32, leadID)
		} else {
			// No final price set - this is OK if it's a new offer
			h.cfg.Debugf("  ⚠️  Offer Final Price: NOT SET (new offer or no existing), leadID=%s", leadID)
		}
		
		detail.Offer = offer
		finalPriceVal := int32(0)
		if offer.FinalPrice.Valid {
			finalPriceVal = offer.FinalPrice.Int32
		}
		h.cfg.Debugf("  💰 Offer prepared for save: FinalPrice.Valid=%v, FinalPrice.Int32=%d, leadID=%s", 
			offer.FinalPrice.Valid, finalPriceVal, leadID)
	} else {
		h.cfg.Debugf("  ⚠️  Offer NOT processed: no fields provided and no existing offer, leadID=%s", leadID)
	}

	// Booking
	if r.FormValue("book_format") != "" {
		booking := &models.Booking{LeadID: leadID}
		if bookFormat := r.FormValue("book_format"); bookFormat != "" {
			booking.BookFormat = sql.NullString{String: bookFormat, Valid: true}
		}
		if address := r.FormValue("address"); address != "" {
			booking.Address = sql.NullString{String: address, Valid: true}
		}
		if city := r.FormValue("city"); city != "" {
			booking.City = sql.NullString{String: city, Valid: true}
		}
		if deliveryNotes := r.FormValue("delivery_notes"); deliveryNotes != "" {
			booking.DeliveryNotes = sql.NullString{String: deliveryNotes, Valid: true}
		}
		detail.Booking = booking
	}

	// Payment (legacy Payment model - still used for display)
	var amountPaidValue int32 = 0
	if r.FormValue("payment_type") != "" || r.FormValue("amount_paid") != "" {
		payment := &models.Payment{LeadID: leadID}
		if paymentType := r.FormValue("payment_type"); paymentType != "" {
			payment.PaymentType = sql.NullString{String: paymentType, Valid: true}
		}
		if amountPaid := r.FormValue("amount_paid"); amountPaid != "" {
			if ap, err := strconv.Atoi(amountPaid); err == nil {
				amountPaidValue = int32(ap)
				payment.AmountPaid = sql.NullInt32{Int32: amountPaidValue, Valid: true}
			}
		}
		if remainingBalance := r.FormValue("remaining_balance"); remainingBalance != "" {
			if rb, err := strconv.Atoi(remainingBalance); err == nil {
				payment.RemainingBalance = sql.NullInt32{Int32: int32(rb), Valid: true}
			}
		}
		if paymentDate := r.FormValue("payment_date"); paymentDate != "" {
			if t, err := time.Parse("2006-01-02", paymentDate); err == nil {
				payment.PaymentDate = sql.NullTime{Time: t, Valid: true}
			}
		}
		detail.Payment = payment
	}
	
	// Course payment (new LeadPayment model for multiple payments)
	// Parse course payment fields if provided
	coursePaymentType := r.FormValue("course_payment_type")
	coursePaymentAmountStr := r.FormValue("course_payment_amount")
	coursePaymentMethod := r.FormValue("course_payment_method")
	coursePaymentDateStr := r.FormValue("course_payment_date")
	coursePaymentNotes := r.FormValue("course_payment_notes")

	// Auto-move to WAITING when payment is recorded (only for admin, only if status is before WAITING)
	if amountPaidValue > 0 {
		currentStatus := detail.Lead.Status
		// Statuses that come before waiting_for_round in the workflow
		statusesBeforeWaiting := map[string]bool{
			"lead_created":      true,
			"test_booked":       true,
			"tested":            true,
			"offer_sent":        true,
			"booking_confirmed": true,
			"deposit_paid":      true,
		}

		if statusesBeforeWaiting[currentStatus] {
			oldStatus := currentStatus
			detail.Lead.Status = "waiting_for_round"
			h.cfg.Debugf("  💰 Payment recorded (AmountPaid=%d): Auto-moving status %s → waiting_for_round", amountPaidValue, oldStatus)
		} else {
			h.cfg.Debugf("  💰 Payment recorded (AmountPaid=%d): Status is %s (not before WAITING), keeping current status", amountPaidValue, currentStatus)
		}
	}

	// Scheduling - validate and process class days and time
	classDays := r.FormValue("class_days")
	classTime := r.FormValue("class_time")

	// If user is setting schedule (either field provided), validate payment first
	if classDays != "" || classTime != "" {
		// Check if fully paid before allowing schedule updates
		var finalPriceValue int32 = 0
		if existingDetail.Offer != nil && existingDetail.Offer.FinalPrice.Valid {
			finalPriceValue = existingDetail.Offer.FinalPrice.Int32
		}
		
		totalCoursePaid, err := models.GetTotalCoursePaid(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get total course paid: %v", err)
			totalCoursePaid = 0
		}
		
		isFullyPaid := finalPriceValue > 0 && totalCoursePaid >= finalPriceValue
		
		if !isFullyPaid {
			h.renderDetailWithError(w, r, leadID, "Cannot schedule before full payment. Course must be fully paid before setting class days and time.")
			return
		}
		
		// Both fields must be present when setting NEW schedule
		// But if one is already set, allow updating just the other one
		existingScheduling := existingDetail.Scheduling
		hasExistingSchedule := existingScheduling != nil && existingScheduling.ClassDays.Valid && existingScheduling.ClassTime.Valid
		
		if !hasExistingSchedule && (classDays == "" || classTime == "") {
			h.renderDetailWithError(w, r, leadID, "Both Class Days and Class Time are required when setting schedule.")
			return
		}
	}

	// Validate class days (if provided)
	if classDays != "" {
		allowedClassDays := map[string]bool{
			"Sun/Wed":  true,
			"Sat/Tues": true,
			"Mon/Thu":  true,
		}
		if !allowedClassDays[classDays] {
			log.Printf("ERROR: Invalid class_days value: %q", classDays)
			h.renderDetailWithError(w, r, leadID, "Invalid class days value. Allowed values: Sun/Wed, Sat/Tues, Mon/Thu")
			return
		}
	}

	// Validate class time (if provided)
	if classTime != "" {
		allowedClassTimes := map[string]bool{
			"07:30": true,
			"10:00": true,
		}
		if !allowedClassTimes[classTime] {
			log.Printf("ERROR: Invalid class_time value: %q", classTime)
			h.renderDetailWithError(w, r, leadID, "Invalid class time value. Allowed values: 07:30, 10:00")
			return
		}
	}

	// Create/update scheduling if class days or time is provided
	// Note: Auto-stage classification (below) will handle status upgrade to READY_TO_START when schedule is filled
	// IMPORTANT: Always preserve existing scheduling values if form fields are not provided (e.g., when disabled)
	if classDays != "" || classTime != "" {
		// Load existing scheduling to preserve values not in form
		// Note: existingDetail was already loaded earlier, but we need fresh data for scheduling
		existingSchedulingDetail, err := models.GetLeadByID(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to load existing detail for scheduling preservation: %v", err)
			existingSchedulingDetail = nil
		}
		
		scheduling := &models.Scheduling{LeadID: leadID}
		
		// Set class_days if provided, otherwise preserve existing
		if classDays != "" {
			scheduling.ClassDays = sql.NullString{String: classDays, Valid: true}
		} else if existingSchedulingDetail != nil && existingSchedulingDetail.Scheduling != nil {
			scheduling.ClassDays = existingSchedulingDetail.Scheduling.ClassDays
		}
		
		// Set class_time if provided, otherwise preserve existing
		if classTime != "" {
			scheduling.ClassTime = sql.NullString{String: classTime, Valid: true}
		} else if existingSchedulingDetail != nil && existingSchedulingDetail.Scheduling != nil {
			scheduling.ClassTime = existingSchedulingDetail.Scheduling.ClassTime
		}
		
		// Preserve existing expected_round, start_date, start_time, class_group_index if they exist
		if existingSchedulingDetail != nil && existingSchedulingDetail.Scheduling != nil {
			scheduling.ExpectedRound = existingSchedulingDetail.Scheduling.ExpectedRound
			scheduling.StartDate = existingSchedulingDetail.Scheduling.StartDate
			scheduling.StartTime = existingSchedulingDetail.Scheduling.StartTime
			scheduling.ClassGroupIndex = existingSchedulingDetail.Scheduling.ClassGroupIndex
		}
		
		detail.Scheduling = scheduling
	}

	// Shipping
	if r.FormValue("shipment_status") != "" {
		shipping := &models.Shipping{LeadID: leadID}
		if shipmentStatus := r.FormValue("shipment_status"); shipmentStatus != "" {
			shipping.ShipmentStatus = sql.NullString{String: shipmentStatus, Valid: true}
		}
		if shipmentDate := r.FormValue("shipment_date"); shipmentDate != "" {
			if t, err := time.Parse("2006-01-02", shipmentDate); err == nil {
				shipping.ShipmentDate = sql.NullTime{Time: t, Valid: true}
			}
		}
		detail.Shipping = shipping
	}

	// CRITICAL: Preserve existing offer for display/computation, but track if it was explicitly changed
	// This ensures that if user only updates other sections, offer final_price is not lost
	// BUT: We only use it for status upgrade if it was explicitly changed
	offerWasExplicitlyChanged := shouldProcessOffer
	if detail.Offer == nil && existingDetail.Offer != nil {
		detail.Offer = existingDetail.Offer
		h.cfg.Debugf("  💰 Preserving existing offer: FinalPrice.Valid=%v, FinalPrice.Int32=%d, leadID=%s",
			existingDetail.Offer.FinalPrice.Valid, func() int32 {
				if existingDetail.Offer.FinalPrice.Valid {
					return existingDetail.Offer.FinalPrice.Int32
				}
				return 0
			}(), leadID)
	}
	// Ensure we have existing payment data if form didn't modify it (for stage computation)
	if detail.Payment == nil && existingDetail.Payment != nil {
		detail.Payment = existingDetail.Payment
	}
	// Ensure we have existing scheduling data if form didn't modify it (for stage computation)
	if detail.Scheduling == nil && existingDetail.Scheduling != nil {
		detail.Scheduling = existingDetail.Scheduling
	}

	// Auto-compute stage from form completion and update status
	// This happens after all form sections are parsed
	// IMPORTANT: Only upgrade to OFFER_SENT if offer was explicitly changed
	// Create a copy of detail for stage computation, removing offer if it wasn't explicitly changed
	stageDetail := detail
	if !offerWasExplicitlyChanged && detail.Offer != nil {
		// Create a copy without the offer to prevent status upgrade
		stageDetailCopy := *detail
		stageDetailCopy.Offer = nil
		stageDetail = &stageDetailCopy
		h.cfg.Debugf("  📊 Stage computation: Offer NOT explicitly changed, excluding from status upgrade, leadID=%s", leadID)
	} else if offerWasExplicitlyChanged {
		h.cfg.Debugf("  📊 Stage computation: Offer WAS explicitly changed, including in status upgrade, leadID=%s", leadID)
	}
	newStage, dbStatus := models.ComputeStageFromFormCompletion(stageDetail, currentStatus)

	// Validation: If stage reaches OFFER_SENT or later, final_price must be valid
	if newStage == models.StageOfferSent || newStage == models.StageBookingConfirmedPaidFull || newStage == models.StageBookingConfirmedDeposit {
		if detail.Offer == nil || !detail.Offer.FinalPrice.Valid || detail.Offer.FinalPrice.Int32 <= 0 {
			h.renderDetailWithError(w, r, leadID, "Final price is required when sending an offer. Please fill in the Offer & Pricing section.")
			return
		}
	}

	detail.Lead.Status = dbStatus
	h.cfg.Debugf("  📊 Auto-stage: computed stage=%s, dbStatus=%s (was %s)", newStage, dbStatus, currentStatus)

	// Log offer final price before saving
	if detail.Offer != nil {
		finalPriceVal := int32(0)
		if detail.Offer.FinalPrice.Valid {
			finalPriceVal = detail.Offer.FinalPrice.Int32
		}
		h.cfg.Debugf("  💾 About to save: Offer.FinalPrice.Valid=%v, Offer.FinalPrice.Int32=%d, leadID=%s", 
			detail.Offer.FinalPrice.Valid, finalPriceVal, leadID)
	} else {
		h.cfg.Debugf("  💾 About to save: Offer is nil, leadID=%s", leadID)
	}
	
	err = models.UpdateLeadDetail(detail)
	if err != nil {
		// Check if it's a phone constraint error
		var phoneErr *models.PhoneAlreadyExistsError
		if errors.As(err, &phoneErr) {
			// phoneErr already has the details from UpdateLeadDetail
		} else if phoneConstraintErr := models.IsPhoneConstraintError(err); phoneConstraintErr != nil {
			// Try to get the full error with existing lead ID
			if existingLead, findErr := models.GetLeadByPhone(detail.Lead.Phone); findErr == nil && existingLead.ID != leadID {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:          detail.Lead.Phone,
					ExistingLeadID: &existingLead.ID,
					Message:        "Phone number already exists",
				}
			} else {
				phoneErr = &models.PhoneAlreadyExistsError{
					Phone:   detail.Lead.Phone,
					Message: "Phone number already exists",
				}
			}
		}
		
		if phoneErr != nil {
			// Redirect back to detail page with error
			redirectURL := fmt.Sprintf("/pre-enrolment/%s?error=phone_exists", leadID.String())
			if phoneErr.ExistingLeadID != nil {
				redirectURL += fmt.Sprintf("&existing_lead_id=%s", phoneErr.ExistingLeadID.String())
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}
		
		http.Error(w, fmt.Sprintf("Failed to update lead: %v", err), http.StatusInternalServerError)
		return
	}
	
	// Log after saving - reload to verify
	reloadedDetail, err := models.GetLeadByID(leadID)
	if err == nil && reloadedDetail.Offer != nil {
		finalPriceVal := int32(0)
		if reloadedDetail.Offer.FinalPrice.Valid {
			finalPriceVal = reloadedDetail.Offer.FinalPrice.Int32
		}
		h.cfg.Debugf("  ✅ After save: Offer.FinalPrice.Valid=%v, Offer.FinalPrice.Int32=%d, leadID=%s", 
			reloadedDetail.Offer.FinalPrice.Valid, finalPriceVal, leadID)
	} else if err == nil {
		h.cfg.Debugf("  ⚠️  After save: Offer is nil, leadID=%s", leadID)
	}

	// Sync finance transactions for placement test
	if detail.PlacementTest != nil {
		amountPaid := int32(0)
		if detail.PlacementTest.PlacementTestFeePaid.Valid {
			amountPaid = detail.PlacementTest.PlacementTestFeePaid.Int32
		}
		
		// Validate: if amount > 0, date and method must be provided
		if amountPaid > 0 {
			if !detail.PlacementTest.PlacementTestPaymentDate.Valid || !detail.PlacementTest.PlacementTestPaymentMethod.Valid {
				h.renderDetailWithError(w, r, leadID, "Payment date and method are required when placement test fee is paid.")
				return
			}
		}
		
		err = models.UpsertPlacementTestIncome(leadID, amountPaid, detail.PlacementTest.PlacementTestPaymentDate, detail.PlacementTest.PlacementTestPaymentMethod)
		if err != nil {
			log.Printf("ERROR: Failed to sync placement test finance transaction: %v", err)
			// Check if it's a validation error (future date)
			errorMsg := fmt.Sprintf("Failed to sync placement test payment: %v", err)
			if err.Error() == "payment date cannot be in the future" {
				errorMsg = "Payment date cannot be in the future"
			}
			h.renderDetailWithError(w, r, leadID, errorMsg)
			return
		}
	}

	// Sync finance transactions for course payment (create new LeadPayment if provided)
	if coursePaymentAmountStr != "" && coursePaymentMethod != "" && coursePaymentDateStr != "" {
		// Validate payment type is provided
		if coursePaymentType == "" {
			h.renderDetailWithError(w, r, leadID, "Payment type is required (Deposit, Full Payment, or Top-up).")
			return
		}
		
		// Validate payment type value
		allowedPaymentTypes := map[string]bool{
			"deposit":      true,
			"full_payment": true,
			"top_up":       true,
		}
		if !allowedPaymentTypes[coursePaymentType] {
			h.renderDetailWithError(w, r, leadID, "Invalid payment type. Must be: deposit, full_payment, or top_up.")
			return
		}
		
		amount, err := strconv.Atoi(coursePaymentAmountStr)
		if err != nil || amount <= 0 {
			h.renderDetailWithError(w, r, leadID, "Invalid course payment amount.")
			return
		}
		
		paymentDate, err := util.ParseDateLocal(coursePaymentDateStr)
		if err != nil {
			h.renderDetailWithError(w, r, leadID, "Invalid course payment date.")
			return
		}
		
		_, err = models.CreateLeadPayment(leadID, coursePaymentType, int32(amount), coursePaymentMethod, paymentDate, coursePaymentNotes)
		if err != nil {
			log.Printf("ERROR: Failed to create course payment: %v", err)
			// Check if it's a validation error (future date)
			errorMsg := fmt.Sprintf("Failed to create course payment: %v", err)
			if err.Error() == "payment date cannot be in the future" {
				errorMsg = "Payment date cannot be in the future"
			}
			h.renderDetailWithError(w, r, leadID, errorMsg)
			return
		}
		
		// Update lead credits based on all payments
		var bundleLevels sql.NullInt32
		if detail.Offer != nil && detail.Offer.BundleLevels.Valid {
			bundleLevels = detail.Offer.BundleLevels
		}
		err = models.UpdateLeadCreditsFromPayments(leadID, bundleLevels)
		if err != nil {
			log.Printf("ERROR: Failed to update lead credits: %v", err)
			// Don't fail the save, just log
		}
	}

	// Redirect back to detail page to show saved changes
	http.Redirect(w, r, fmt.Sprintf("/pre-enrolment/%s?saved=1", leadID.String()), http.StatusFound)
}

// MarkTested sets status to "tested" and optionally saves test notes/assigned level
func (h *PreEnrolmentHandler) MarkTested(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	// Update placement test if fields are provided
	if r.FormValue("assigned_level") != "" || r.FormValue("test_notes") != "" {
		detail, err := models.GetLeadByID(leadID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
			return
		}

		if detail.PlacementTest == nil {
			detail.PlacementTest = &models.PlacementTest{LeadID: leadID}
		}

		if assignedLevel := r.FormValue("assigned_level"); assignedLevel != "" {
			level, parseErr := strconv.Atoi(assignedLevel)
			if parseErr != nil || !isValidAssignedLevel(level) {
				h.renderDetailWithError(w, r, leadID, "Invalid assigned level. Allowed: 1–8.")
				return
			}
			detail.PlacementTest.AssignedLevel = sql.NullInt32{Int32: int32(level), Valid: true}
		}
		if testNotes := r.FormValue("test_notes"); testNotes != "" {
			detail.PlacementTest.TestNotes = sql.NullString{String: testNotes, Valid: true}
		}

		// Update placement test only
		if err := models.UpdatePlacementTest(detail.PlacementTest); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update placement test: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Update status
	err = models.UpdateLeadStatus(leadID, "tested")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=tested", http.StatusFound)
}

// MarkOfferSent sets status to "offer_sent" and validates offer fields
func (h *PreEnrolmentHandler) MarkOfferSent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	// Validate offer fields are present
	bundle := r.FormValue("bundle")
	finalPrice := r.FormValue("final_price")
	if bundle == "" || finalPrice == "" {
		http.Error(w, "Bundle and Final Price are required to send offer", http.StatusBadRequest)
		return
	}

	// Update or create offer
	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
		return
	}

	if detail.Offer == nil {
		detail.Offer = &models.Offer{LeadID: leadID}
	}

	if b, err := strconv.Atoi(bundle); err == nil {
		detail.Offer.BundleLevels = sql.NullInt32{Int32: int32(b), Valid: true}
	}
	if fp, err := strconv.Atoi(finalPrice); err == nil {
		detail.Offer.FinalPrice = sql.NullInt32{Int32: int32(fp), Valid: true}
	}
	if basePrice := r.FormValue("base_price"); basePrice != "" {
		if bp, err := strconv.Atoi(basePrice); err == nil {
			detail.Offer.BasePrice = sql.NullInt32{Int32: int32(bp), Valid: true}
		}
	}
	if discount := r.FormValue("discount"); discount != "" {
		// Parse discount (could be "500" or "10%")
		if strings.HasSuffix(discount, "%") {
			if pct, err := strconv.Atoi(strings.TrimSuffix(discount, "%")); err == nil {
				detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(pct), Valid: true}
				detail.Offer.DiscountType = sql.NullString{String: "percent", Valid: true}
			}
		} else {
			if amt, err := strconv.Atoi(discount); err == nil {
				detail.Offer.DiscountValue = sql.NullInt32{Int32: int32(amt), Valid: true}
				detail.Offer.DiscountType = sql.NullString{String: "amount", Valid: true}
			}
		}
	}

	// Update offer
	if err := models.UpdateOffer(detail.Offer); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update offer: %v", err), http.StatusInternalServerError)
		return
	}

	// Update status
	err = models.UpdateLeadStatus(leadID, "offer_sent")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=offer_sent", http.StatusFound)
}

// MarkWaiting sets status to "waiting_for_round"
func (h *PreEnrolmentHandler) MarkWaiting(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	err = models.UpdateLeadStatus(leadID, "waiting_for_round")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=waiting", http.StatusFound)
}

// MarkReady sets status to "ready_to_start"
func (h *PreEnrolmentHandler) MarkReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot update status
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	err = models.UpdateLeadStatus(leadID, "ready_to_start")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update status: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?status_flash=ready", http.StatusFound)
}

func (h *PreEnrolmentHandler) BookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Server-side check: moderators cannot book tests
	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		http.Error(w, "Forbidden: Moderators cannot book placement tests", http.StatusForbidden)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(pathParts[2])
	if err != nil {
		http.Error(w, "Invalid lead ID", http.StatusBadRequest)
		return
	}

	// Parse placement test fields
	var testDate sql.NullTime
	if dateStr := r.FormValue("test_date"); dateStr != "" {
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			testDate = sql.NullTime{Time: t, Valid: true}
		}
	}

	var testTime sql.NullString
	if timeStr := r.FormValue("test_time"); timeStr != "" {
		testTime = sql.NullString{String: timeStr, Valid: true}
	}

	var testType sql.NullString
	if typeStr := r.FormValue("test_type"); typeStr != "" {
		testType = sql.NullString{String: typeStr, Valid: true}
	}

	var testNotes sql.NullString
	if notesStr := r.FormValue("test_notes"); notesStr != "" {
		testNotes = sql.NullString{String: notesStr, Valid: true}
	}

	h.cfg.Debugf("📅 BookTest: leadID=%s, testDate=%v, testTime=%v, testType=%v", leadID, testDate, testTime, testType)

	// Book the placement test (updates test fields and sets status to test_booked)
	err = models.BookPlacementTest(leadID, testDate, testTime, testType, testNotes)
	if err != nil {
		log.Printf("ERROR: Failed to book placement test: %v", err)
		http.Error(w, fmt.Sprintf("Failed to book placement test: %v", err), http.StatusInternalServerError)
		return
	}

	h.cfg.Debugf("  ✅ Test booked successfully, redirecting to list")
	http.Redirect(w, r, "/pre-enrolment?status_flash=test_booked", http.StatusFound)
}
