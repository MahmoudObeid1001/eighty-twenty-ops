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
		parts := strings.Split(t, ":")
		if len(parts) >= 2 {
			t = parts[0] + ":" + parts[1]
		}
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
		Status     string
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

	// Fetch popularity counts for standard slots
	popRows, popErr := db.DB.Query(`
		SELECT class_days, class_time, COUNT(*) 
		FROM class_groups 
		GROUP BY class_days, class_time
	`)
	if popErr == nil {
		counts := make(map[string]int)
		for popRows.Next() {
			var d, t string
			var count int
			if err := popRows.Scan(&d, &t, &count); err == nil {
				key := normalizeDays(d) + "_" + normalizeTime(t)
				counts[key] = count
			}
		}
		popRows.Close()

		// Sort stdSlots based on counts (descending)
		sort.SliceStable(stdSlots, func(i, j int) bool {
			keyI := stdSlots[i].Days + "_" + stdSlots[i].Time
			keyJ := stdSlots[j].Days + "_" + stdSlots[j].Time
			return counts[keyI] > counts[keyJ]
		})
	}

	var openClasses []ClassResult
	
	// Query active or underconstruction classes matching level directly
	rows, queryErr := db.DB.Query(`
		SELECT cg.class_key, cg.class_days, cg.class_time,
		       COALESCE(cg.round_status, 'not_started') as round_status
		FROM class_groups cg
		WHERE cg.level = $1
		  AND COALESCE(cg.round_status, 'not_started') IN ('active', 'not_started')
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
							Status:     roundStatus,
						})
					}
				}
			}
		}
	}

	// Sort classes:
	// Priority 1: underconstruction ('not_started') classes with >= 1 student (descending by enrollment)
	// Priority 2: active classes (descending by enrollment)
	// Priority 3: underconstruction classes with 0 students (descending by enrollment)
	sort.Slice(openClasses, func(i, j int) bool {
		cI, cJ := openClasses[i], openClasses[j]
		
		isPriI := cI.Status == "not_started" && cI.Enrollment >= 1
		isPriJ := cJ.Status == "not_started" && cJ.Enrollment >= 1
		
		if isPriI && !isPriJ {
			return true
		}
		if !isPriI && isPriJ {
			return false
		}
		if isPriI && isPriJ {
			return cI.Enrollment > cJ.Enrollment
		}
		
		isActiveI := cI.Status == "active"
		isActiveJ := cJ.Status == "active"
		
		if isActiveI && !isActiveJ {
			return true
		}
		if !isActiveI && isActiveJ {
			return false
		}
		if isActiveI && isActiveJ {
			return cI.Enrollment > cJ.Enrollment
		}
		
		return cI.Enrollment > cJ.Enrollment
	})

	var selected []ClassResult
	if len(openClasses) > 0 {
		// Suggest the first prioritized open class (underconstruction with students, active, or empty underconstruction)
		selected = append(selected, openClasses[0])
		
		// Fill the remaining slots up to 4 using the standard slots (so 1 open + 3 standard slots)
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
	} else {
		// If no class is open, fill with all 6 standard slots
		for _, std := range stdSlots {
			selected = append(selected, ClassResult{
				Days:  std.Days,
				Time:  std.Time,
				Seats: 6,
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
