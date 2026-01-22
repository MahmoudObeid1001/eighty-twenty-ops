package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

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

	// Check for flash messages in query params (separate from filter status)
	flashMessage := ""
	savedParam := r.URL.Query().Get("saved")
	deletedParam := r.URL.Query().Get("deleted")
	statusFlashParam := r.URL.Query().Get("status_flash")
	sentToClassesParam := r.URL.Query().Get("sentToClasses")

	if sentToClassesParam == "1" {
		flashMessage = "Lead sent to classes board successfully!"
	} else if deletedParam == "1" {
		flashMessage = "Lead deleted successfully!"
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

	h.cfg.Debugf("List: statusFilter=%q, searchFilter=%q, paymentFilter=%q, hotFilter=%q", statusFilter, searchFilter, paymentFilter, hotFilter)

	// Get filtered leads
	leads, err := models.GetAllLeads(statusFilter, searchFilter, paymentFilter, hotFilter)
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
		// Get all leads to count hot leads accurately
		allLeads, err := models.GetAllLeads("", "", "", "")
		if err == nil {
			for _, lead := range allLeads {
				if lead.FollowUpDue {
					followUpCount++
				}
			}
		}
	}

	data := map[string]interface{}{
		"Title":         "Pre-Enrolment - Eighty Twenty",
		"Leads":         leads,
		"UserRole":      middleware.GetUserRole(r),
		"FlashMessage":  flashMessage,
		"StatusFilter":  statusFilter,
		"SearchFilter":  searchFilter,
		"PaymentFilter": paymentFilter,
		"HotFilter":     hotFilter,
		"FollowUpCount": followUpCount,
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

	data := map[string]interface{}{
		"Title":                  fmt.Sprintf("Pre-Enrolment Detail - %s", detail.Lead.FullName),
		"Detail":                 detail,
		"UserRole":               userRole,
		"IsModerator":            isModerator,
		"PlacementTestRemaining": placementTestRemaining,
		"FollowUpDue":            tempItem.FollowUpDue,
		"HotLevel":               tempItem.HotLevel,
		"DaysSinceLastProgress":  tempItem.DaysSinceLastProgress,
	}
	renderTemplate(w, "pre_enrolment_detail.html", data)
}

// renderDetailWithError fetches the lead, builds detail page data with Error set, and renders.
// Used when validation fails (e.g. schedule required for mark_ready).
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
	data := map[string]interface{}{
		"Title":                  fmt.Sprintf("Pre-Enrolment Detail - %s", detail.Lead.FullName),
		"Detail":                 detail,
		"UserRole":               userRole,
		"IsModerator":            isModerator,
		"PlacementTestRemaining": placementTestRemaining,
		"FollowUpDue":            tempItem.FollowUpDue,
		"HotLevel":               tempItem.HotLevel,
		"DaysSinceLastProgress":  tempItem.DaysSinceLastProgress,
		"Error":                  errMsg,
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
		// Server-side check: moderators cannot update status
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot update lead status", http.StatusForbidden)
			return
		}

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

		// Schedule required: both Class Days and Class Time must be present
		classDaysMR := r.FormValue("class_days")
		classTimeMR := r.FormValue("class_time")
		if classDaysMR == "" || classTimeMR == "" {
			h.renderDetailWithError(w, r, leadID, "Both Class Days and Class Time are required to mark as ready to start.")
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

	case "delete":
		h.cfg.Debugf("  → Action: delete")
		// Server-side check: moderators cannot delete
		if userRole == "moderator" {
			http.Error(w, "Forbidden: Moderators cannot delete leads", http.StatusForbidden)
			return
		}

		// Require confirmation
		confirmDelete := r.FormValue("confirm_delete")
		if confirmDelete != "yes" {
			// Show confirmation page
			detail, err := models.GetLeadByID(leadID)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
				return
			}

			data := map[string]interface{}{
				"Title":             fmt.Sprintf("Delete Lead - %s", detail.Lead.FullName),
				"Detail":            detail,
				"UserRole":          userRole,
				"IsModerator":       false,
				"ShowDeleteConfirm": true,
			}
			renderTemplate(w, "pre_enrolment_detail.html", data)
			return
		}

		// Delete the lead
		err = models.DeleteLead(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to delete lead: %v", err)
			http.Error(w, fmt.Sprintf("Failed to delete lead: %v", err), http.StatusInternalServerError)
			return
		}

		h.cfg.Debugf("  ✅ Lead deleted successfully, redirecting to list")
		http.Redirect(w, r, "/pre-enrolment?deleted=1", http.StatusFound)
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
	fullName := r.FormValue("full_name")
	phone := r.FormValue("phone")
	if fullName == "" || phone == "" {
		log.Printf("ERROR: Validation failed for SaveFull: fullName=%q, phone=%q", fullName, phone)
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
		// Keep existing status
		existingDetail, err := models.GetLeadByID(leadID)
		if err == nil {
			detail.Lead.Status = existingDetail.Lead.Status
		} else {
			detail.Lead.Status = "lead_created"
		}

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

	// Load existing detail to get current status and preserve other fields
	existingDetail, err := models.GetLeadByID(leadID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
		return
	}

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
		detail.PlacementTest = pt
	}

	// Offer
	if r.FormValue("bundle") != "" || r.FormValue("final_price") != "" {
		offer := &models.Offer{LeadID: leadID}
		if bundle := r.FormValue("bundle"); bundle != "" {
			if b, err := strconv.Atoi(bundle); err == nil {
				offer.BundleLevels = sql.NullInt32{Int32: int32(b), Valid: true}
			}
		}
		if basePrice := r.FormValue("base_price"); basePrice != "" {
			if bp, err := strconv.Atoi(basePrice); err == nil {
				offer.BasePrice = sql.NullInt32{Int32: int32(bp), Valid: true}
			}
		}
		if discount := r.FormValue("discount"); discount != "" {
			// Parse discount (could be "500" or "10%")
			if strings.HasSuffix(discount, "%") {
				if pct, err := strconv.Atoi(strings.TrimSuffix(discount, "%")); err == nil {
					offer.DiscountValue = sql.NullInt32{Int32: int32(pct), Valid: true}
					offer.DiscountType = sql.NullString{String: "percent", Valid: true}
				}
			} else {
				if amt, err := strconv.Atoi(discount); err == nil {
					offer.DiscountValue = sql.NullInt32{Int32: int32(amt), Valid: true}
					offer.DiscountType = sql.NullString{String: "amount", Valid: true}
				}
			}
		}
		if finalPrice := r.FormValue("final_price"); finalPrice != "" {
			if fp, err := strconv.Atoi(finalPrice); err == nil {
				offer.FinalPrice = sql.NullInt32{Int32: int32(fp), Valid: true}
			}
		}
		detail.Offer = offer
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

	// Payment
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

	// If user is setting schedule (either field provided), both must be present
	if classDays != "" || classTime != "" {
		if classDays == "" || classTime == "" {
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
			// Re-render detail page with error message
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
			data := map[string]interface{}{
				"Title":                  fmt.Sprintf("Pre-Enrolment Detail - %s", detail.Lead.FullName),
				"Detail":                 detail,
				"UserRole":               userRole,
				"IsModerator":            isModerator,
				"PlacementTestRemaining": placementTestRemaining,
				"Error":                  "Invalid class days value. Allowed values: Sun/Wed, Sat/Tues, Mon/Thu",
			}
			renderTemplate(w, "pre_enrolment_detail.html", data)
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
			// Re-render detail page with error message
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
			data := map[string]interface{}{
				"Title":                  fmt.Sprintf("Pre-Enrolment Detail - %s", detail.Lead.FullName),
				"Detail":                 detail,
				"UserRole":               userRole,
				"IsModerator":            isModerator,
				"PlacementTestRemaining": placementTestRemaining,
				"Error":                  "Invalid class time value. Allowed values: 07:30, 10:00",
			}
			renderTemplate(w, "pre_enrolment_detail.html", data)
			return
		}
	}

	// Create/update scheduling if class days or time is provided
	// Note: Auto-stage classification (below) will handle status upgrade to READY_TO_START when schedule is filled
	if classDays != "" || classTime != "" {
		scheduling := &models.Scheduling{LeadID: leadID}
		if classDays != "" {
			scheduling.ClassDays = sql.NullString{String: classDays, Valid: true}
		}
		if classTime != "" {
			scheduling.ClassTime = sql.NullString{String: classTime, Valid: true}
		}
		// Preserve existing expected_round, start_date, start_time if they exist (data compatibility)
		existingDetail, err := models.GetLeadByID(leadID)
		if err == nil && existingDetail.Scheduling != nil {
			scheduling.ExpectedRound = existingDetail.Scheduling.ExpectedRound
			scheduling.StartDate = existingDetail.Scheduling.StartDate
			scheduling.StartTime = existingDetail.Scheduling.StartTime
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

	// Ensure we have existing offer data if form didn't modify it (for stage computation)
	if detail.Offer == nil && existingDetail.Offer != nil {
		detail.Offer = existingDetail.Offer
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
	newStage, dbStatus := models.ComputeStageFromFormCompletion(detail, currentStatus)

	// Validation: If stage reaches OFFER_SENT or later, final_price must be valid
	if newStage == models.StageOfferSent || newStage == models.StageBookingConfirmedPaidFull || newStage == models.StageBookingConfirmedDeposit {
		if detail.Offer == nil || !detail.Offer.FinalPrice.Valid || detail.Offer.FinalPrice.Int32 <= 0 {
			// Re-render detail page with error
			existingDetail, err := models.GetLeadByID(leadID)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to load lead: %v", err), http.StatusInternalServerError)
				return
			}
			userRole := middleware.GetUserRole(r)
			isModerator := userRole == "moderator"
			var placementTestRemaining int32 = 0
			if existingDetail.PlacementTest != nil {
				if existingDetail.PlacementTest.PlacementTestFee.Valid && existingDetail.PlacementTest.PlacementTestFeePaid.Valid {
					placementTestRemaining = existingDetail.PlacementTest.PlacementTestFee.Int32 - existingDetail.PlacementTest.PlacementTestFeePaid.Int32
					if placementTestRemaining < 0 {
						placementTestRemaining = 0
					}
				} else if existingDetail.PlacementTest.PlacementTestFee.Valid {
					placementTestRemaining = existingDetail.PlacementTest.PlacementTestFee.Int32
				} else {
					placementTestRemaining = 100
				}
			} else {
				placementTestRemaining = 100
			}
			data := map[string]interface{}{
				"Title":                  fmt.Sprintf("Pre-Enrolment Detail - %s", existingDetail.Lead.FullName),
				"Detail":                 existingDetail,
				"UserRole":               userRole,
				"IsModerator":            isModerator,
				"PlacementTestRemaining": placementTestRemaining,
				"Error":                  "Final price is required when sending an offer. Please fill in the Offer & Pricing section.",
			}
			renderTemplate(w, "pre_enrolment_detail.html", data)
			return
		}
	}

	detail.Lead.Status = dbStatus
	h.cfg.Debugf("  📊 Auto-stage: computed stage=%s, dbStatus=%s (was %s)", newStage, dbStatus, currentStatus)

	err = models.UpdateLeadDetail(detail)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update lead: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/pre-enrolment?saved=1", http.StatusFound)
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
