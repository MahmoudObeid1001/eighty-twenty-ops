package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

func (h *PreEnrolmentHandler) PrivateTrackList(w http.ResponseWriter, r *http.Request) {
	searchFilter := strings.TrimSpace(r.URL.Query().Get("search"))
	flashMessage, flashMessageType := flashFromQuery(r)

	leads, err := models.GetAllLeads("", searchFilter, "", "", false, "", "", "", "", "private_track")
	if err != nil {
		if flashMessage == "" {
			flashMessage = "Couldn't load private track students. Please refresh and try again."
			flashMessageType = "error"
		}
		leads = []*models.LeadListItem{}
	}

	data := map[string]interface{}{
		"Title":            "Private Track - Eighty Twenty",
		"UserRole":         middleware.GetUserRole(r),
		"IsModerator":      IsModerator(r),
		"IsAdmin":          IsAdmin(r),
		"Leads":            leads,
		"SearchFilter":     searchFilter,
		"FlashMessage":     flashMessage,
		"FlashMessageType": flashMessageType,
	}
	renderTemplate(w, r, "private_track_list.html", data)
}

func (h *PreEnrolmentHandler) PrivateTrackAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" && userRole != "manager" {
		http.Error(w, "You don't have permission to edit private track leads.", http.StatusForbidden)
		return
	}

	leadIDStr := strings.Trim(strings.TrimPrefix(r.URL.Path, "/private-track/"), "/")
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch strings.TrimSpace(r.FormValue("action")) {
	case "adjust_level":
		level, err := strconv.Atoi(strings.TrimSpace(r.FormValue("assigned_level")))
		if err != nil || !isValidAssignedLevel(level) {
			redirectWithError(w, r, "/private-track", "Choose a valid level from 1 to 10.")
			return
		}
		if err := models.SetPrivateTrackLeadAssignedLevel(leadID, int32(level)); err != nil {
			redirectWithError(w, r, "/private-track", err.Error())
			return
		}
		redirectWithFlash(w, r, "/private-track", "success", fmt.Sprintf("Assigned level updated to Level %d.", level))
		return

	case "return_to_admin_feed":
		if err := models.ReturnPrivateTrackLeadToAdminFeed(leadID); err != nil {
			redirectWithError(w, r, "/private-track", err.Error())
			return
		}
		redirectWithFlash(w, r, "/private-track", "success", "Student returned to the admin feed.")
		return

	default:
		redirectWithError(w, r, "/private-track", "Unknown action.")
		return
	}
}
