package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"eighty-twenty-ops/internal/db"
	"github.com/google/uuid"
)

// PlacementResultData contains the data needed for the result page
type PlacementResultData struct {
	Name    string                `json:"name"`
	Level   int32                 `json:"level"`
	Classes string                `json:"classes"` // Legacy format: Days_Time|seatsLeft,Days_Time|seatsLeft
	Slots   []PlacementResultSlot `json:"slots"`
}

type PlacementResultSlot struct {
	Days       string `json:"days"`
	Time       string `json:"time"`
	Seats      int    `json:"seats"`
	Kind       string `json:"kind"`
	Status     string `json:"status,omitempty"`
	Enrollment int    `json:"enrollment,omitempty"`
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
		Kind       string
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

	var underconstructionClasses []ClassResult

	// Build underconstruction candidates from the same scheduling-based grouping
	// used by the Ops board. This catches real pre-start classes even before a
	// class_groups row has been created for them.
	rows, queryErr := db.DB.Query(`
		SELECT
			s.class_days,
			TO_CHAR(s.class_time, 'HH24:MI') AS class_time,
			COALESCE(cg.round_status, 'not_started') AS round_status,
			COUNT(DISTINCT l.id) AS current_enrollment
		FROM leads l
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN scheduling s ON s.lead_id = l.id
		LEFT JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND LEFT(cg.class_time, 5) = TO_CHAR(s.class_time, 'HH24:MI')
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		WHERE pt.assigned_level = $1
		  AND COALESCE(l.sent_to_classes, false) = true
		  AND l.status IN ('ready_to_start', 'waiting_for_round', 'in_classes')
		  AND s.class_days IS NOT NULL
		  AND s.class_time IS NOT NULL
		  AND COALESCE(cg.round_status, 'not_started') != 'closed'
		GROUP BY
			s.class_days,
			TO_CHAR(s.class_time, 'HH24:MI'),
			COALESCE(s.class_group_index, 1),
			COALESCE(cg.round_status, 'not_started')
		HAVING COALESCE(cg.round_status, 'not_started') = 'not_started'
	`, level)

	if queryErr == nil {
		defer rows.Close()
		for rows.Next() {
			var classDays, classTime, roundStatus string
			var currentEnrollment int
			if scanErr := rows.Scan(&classDays, &classTime, &roundStatus, &currentEnrollment); scanErr == nil {
				seats := 6 - currentEnrollment
				if seats < 0 {
					seats = 0
				}
				if seats > 0 && currentEnrollment >= 1 {
					underconstructionClasses = append(underconstructionClasses, ClassResult{
						Days:       normalizeDays(classDays),
						Time:       normalizeTime(classTime),
						Seats:      int(seats),
						Enrollment: currentEnrollment,
						Status:     roundStatus,
						Kind:       "underconstruction",
					})
				}
			}
		}
	}

	sort.SliceStable(underconstructionClasses, func(i, j int) bool {
		cI, cJ := underconstructionClasses[i], underconstructionClasses[j]
		if cI.Enrollment != cJ.Enrollment {
			return cI.Enrollment > cJ.Enrollment
		}
		if cI.Seats != cJ.Seats {
			return cI.Seats < cJ.Seats
		}
		if cI.Days != cJ.Days {
			return cI.Days < cJ.Days
		}
		return cI.Time < cJ.Time
	})

	var selected []ClassResult
	hasUnderconstruction := len(underconstructionClasses) > 0

	if hasUnderconstruction {
		// Suggest the top underconstruction class first, then fill with the three
		// most popular standard slots.
		selected = append(selected, underconstructionClasses[0])

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
					Kind:  "standard",
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
				Kind:  "standard",
			})
		}
	}

	// Format output classes parameter
	var classParts []string
	slots := make([]PlacementResultSlot, 0, len(selected))
	for _, s := range selected {
		classParts = append(classParts, fmt.Sprintf("%s_%s|%d", s.Days, s.Time, s.Seats))
		slots = append(slots, PlacementResultSlot{
			Days:       s.Days,
			Time:       s.Time,
			Seats:      s.Seats,
			Kind:       s.Kind,
			Status:     s.Status,
			Enrollment: s.Enrollment,
		})
	}
	classesStr := strings.Join(classParts, ",")

	jsonResponse(w, http.StatusOK, PlacementResultData{
		Name:    strings.Split(name, " ")[0], // First name only
		Level:   level,
		Classes: classesStr,
		Slots:   slots,
	})
}
