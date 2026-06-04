package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"
)

// PlacementResultData contains the data needed for the result page
type PlacementResultData struct {
	Name    string `json:"name"`
	Level   int32  `json:"level"`
	Classes string `json:"classes"` // Format: Days_Time|seatsLeft,Days_Time|seatsLeft
}

// GetPlacementResultData returns lead info and available classes formatted for the result URL
func (h *APIHandler) GetPlacementResultData(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	var leadIDStr string
	if len(parts) >= 4 {
		leadIDStr = parts[3]
	}

	if leadIDStr == "" {
		jsonError(w, http.StatusBadRequest, "lead_id is required")
		return
	}

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid lead_id format")
		return
	}

	// Get lead name and level
	var name string
	var level int32
	err = db.DB.QueryRow(`
		SELECT l.full_name, pt.assigned_level 
		FROM leads l 
		JOIN placement_tests pt ON pt.lead_id = l.id 
		WHERE l.id = $1
	`, leadID).Scan(&name, &level)

	if err != nil {
		if err == sql.ErrNoRows {
			jsonError(w, http.StatusNotFound, "lead or placement test not found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "failed to get lead info")
		return
	}

	// Try to get eligible classes for late join
	classesStr := ""
	eligibleClasses, err := models.GetEligibleClassesForLateJoin(leadID)
	if err == nil && len(eligibleClasses) > 0 {
		var classParts []string
		for _, c := range eligibleClasses {
			days := strings.ReplaceAll(c.ClassDays, "/", "-")
			days = strings.ReplaceAll(days, " ", "")
			timeStr := strings.ReplaceAll(c.ClassTime, " ", "")
			seats := 6 - c.CurrentEnrollment
			if seats < 0 {
				seats = 0
			}
			classParts = append(classParts, fmt.Sprintf("%s_%s|%d", days, timeStr, seats))
		}
		classesStr = strings.Join(classParts, ",")
	}

	// Also maybe get classes that are just not started for this level if late join didn't find any?
	// The late join query already includes "sent_to_mentor + not_started classes (pre-start exception)."
	// So GetEligibleClassesForLateJoin should be sufficient.

	jsonResponse(w, http.StatusOK, PlacementResultData{
		Name:    strings.Split(name, " ")[0], // First name only
		Level:   level,
		Classes: classesStr,
	})
}
