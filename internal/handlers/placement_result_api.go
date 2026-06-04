package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"eighty-twenty-ops/internal/db"
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

	// Helper functions to normalize days and times
	normalizeDays := func(d string) string {
		d = strings.ReplaceAll(d, " ", "")
		d = strings.ReplaceAll(d, "/", "-")
		d = strings.ReplaceAll(d, "Tues", "Tue")
		return d
	}

	normalizeTime := func(t string) string {
		t = strings.ReplaceAll(t, " ", "")
		if t == "07:30" || t == "7:30" {
			return "7:30PM"
		}
		if t == "10:00" {
			return "10:00PM"
		}
		return t
	}

	type ClassResult struct {
		Days       string
		Time       string
		Seats      int
		Enrollment int
	}

	// Standard 6 slots defined by the school system
	type StandardSlot struct {
		Days string
		Time string
	}
	stdSlots := []StandardSlot{
		{Days: "Sun-Wed", Time: "7:30PM"},
		{Days: "Sat-Tue", Time: "10:00PM"},
		{Days: "Mon-Thu", Time: "7:30PM"},
		{Days: "Sun-Wed", Time: "10:00PM"},
		{Days: "Sat-Tue", Time: "7:30PM"},
		{Days: "Mon-Thu", Time: "10:00PM"},
	}

	var openClasses []ClassResult
	
	// Query active or sent-to-mentor classes matching level directly
	// to avoid blocking non-ready_to_start leads from showing open classes.
	rows, queryErr := db.DB.Query(`
		SELECT cg.class_key, cg.class_days, cg.class_time,
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
	`, level)

	if queryErr == nil {
		defer rows.Close()
		for rows.Next() {
			var classKey, classDays, classTime, roundStatus string
			if scanErr := rows.Scan(&classKey, &classDays, &classTime, &roundStatus); scanErr == nil {
				// Count enrollment
				var currentEnrollment int
				enrollErr := db.DB.QueryRow(`
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
				`, classKey, roundStatus).Scan(&currentEnrollment)
				
				if enrollErr == nil {
					seats := 6 - currentEnrollment
					if seats < 0 {
						seats = 0
					}
					if seats > 0 {
						openClasses = append(openClasses, ClassResult{
							Days:       normalizeDays(classDays),
							Time:       normalizeTime(classTime),
							Seats:      int(seats),
							Enrollment: currentEnrollment,
						})
					}
				}
			}
		}
	}

	// Sort real classes by enrollment descending (most populated first)
	sort.Slice(openClasses, func(i, j int) bool {
		return openClasses[i].Enrollment > openClasses[j].Enrollment
	})

	// Select highly populated class, then less populated class (if it exists)
	var selected []ClassResult
	if len(openClasses) > 0 {
		// Highly populated one
		selected = append(selected, openClasses[0])
		// Less populated one
		if len(openClasses) > 1 {
			selected = append(selected, openClasses[len(openClasses)-1])
		}
	}

	// Fill the remaining slots up to 4 using the standard slots
	for _, std := range stdSlots {
		if len(selected) >= 4 {
			break
		}
		// Skip if this slot is already selected
		alreadySelected := false
		for _, sel := range selected {
			if sel.Days == std.Days && sel.Time == std.Time {
				alreadySelected = true
				break
			}
		}
		if !alreadySelected {
			selected = append(selected, ClassResult{
				Days:  std.Days,
				Time:  std.Time,
				Seats: 6, // default available seats
			})
		}
	}

	// Format output classes parameter
	var classParts []string
	for _, s := range selected {
		classParts = append(classParts, fmt.Sprintf("%s_%s|%d", s.Days, s.Time, s.Seats))
	}
	classesStr := strings.Join(classParts, ",")

	jsonResponse(w, http.StatusOK, PlacementResultData{
		Name:    strings.Split(name, " ")[0], // First name only
		Level:   level,
		Classes: classesStr,
	})
}
