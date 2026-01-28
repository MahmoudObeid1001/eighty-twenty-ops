package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

type ClassesHandler struct {
	cfg *config.Config
}

func NewClassesHandler(cfg *config.Config) *ClassesHandler {
	return &ClassesHandler{cfg: cfg}
}

// List renders the classes board page. Admin and mentor_head can access (mentor_head read-only).
// Moderator gets 403 access-restricted.
func (h *ClassesHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole == "moderator" {
		w.WriteHeader(http.StatusForbidden)
		data := map[string]interface{}{
			"Title":       "Access Restricted – Eighty Twenty",
			"SectionName": "Classes Board",
			"IsModerator": true,
			"UserRole":    userRole,
		}
		renderTemplate(w, r, "access_restricted.html", data)
		return
	}
	if userRole != "admin" && userRole != "mentor_head" {
		w.WriteHeader(http.StatusForbidden)
		data := map[string]interface{}{
			"Title":       "Access Restricted – Eighty Twenty",
			"SectionName": "Classes Board",
			"IsModerator": IsModerator(r),
			"UserRole":    userRole,
		}
		renderTemplate(w, r, "access_restricted.html", data)
		return
	}

	// Get class groups
	groups, err := models.GetClassGroups()
	if err != nil {
		log.Printf("ERROR: Failed to get class groups: %v", err)
		http.Error(w, fmt.Sprintf("Failed to load classes: %v", err), http.StatusInternalServerError)
		return
	}

	// Get current round
	currentRound, err := models.GetCurrentRound()
	if err != nil {
		log.Printf("ERROR: Failed to get current round: %v", err)
		currentRound = 1 // Default
	}

	// Check for flash messages
	flashMessage := ""
	if r.URL.Query().Get("moved") == "1" {
		flashMessage = "Student moved successfully"
	}
	if r.URL.Query().Get("round_started") == "1" {
		flashMessage = "Round started successfully. READY and LOCKED classes moved to IN_CLASSES."
	}
	if r.URL.Query().Get("sent") == "1" {
		flashMessage = "Class sent to mentor head successfully"
	}
	if r.URL.Query().Get("returned") == "1" {
		flashMessage = "Class returned from mentor head"
	}

	// Auto-assign students without group_index
	// Get all eligible students and assign those without group_index
	eligibleStudents, err := models.GetEligibleStudentsForClasses()
	if err == nil {
		for _, student := range eligibleStudents {
			if !student.GroupIndex.Valid {
				// Auto-assign to a group
				_, err := models.AssignClassGroup(student.LeadID)
				if err != nil {
					h.cfg.Debugf("Failed to auto-assign student %s: %v", student.LeadID, err)
				}
			}
		}
	}

	// Re-fetch groups after auto-assignment
	groups, err = models.GetClassGroups()
	if err != nil {
		log.Printf("ERROR: Failed to re-fetch class groups: %v", err)
		http.Error(w, fmt.Sprintf("Failed to load classes: %v", err), http.StatusInternalServerError)
		return
	}

	// Compute available groups for each student (for move dropdown)
	for _, group := range groups {
		for _, student := range group.Students {
			availableGroups, err := models.GetAvailableGroupsForMove(student.LeadID)
			if err == nil {
				// Store as a simple slice of ints for template
				student.AvailableGroups = availableGroups
			}
		}
	}

	data := map[string]interface{}{
		"Title":             "Classes Board - Eighty Twenty",
		"Groups":            groups,
		"CurrentRound":      currentRound,
		"UserRole":          userRole,
		"IsModerator":       IsModerator(r),
		"FlashMessage":      flashMessage,
		"IsClassesReadOnly": userRole == "mentor_head",
	}
	renderTemplate(w, r, "classes.html", data)
}

// Move handles moving a student between groups
func (h *ClassesHandler) Move(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	leadIDStr := r.FormValue("lead_id")
	targetGroupStr := r.FormValue("target_group")

	if leadIDStr == "" || targetGroupStr == "" {
		http.Error(w, "lead_id and target_group are required", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		http.Error(w, "Invalid lead_id", http.StatusBadRequest)
		return
	}

	targetGroup, err := strconv.Atoi(targetGroupStr)
	if err != nil {
		http.Error(w, "Invalid target_group", http.StatusBadRequest)
		return
	}

	// If target_group is 0, create new group (find next available index)
	if targetGroup == 0 {
		availableGroups, err := models.GetAvailableGroupsForMove(leadID)
		if err != nil {
			log.Printf("ERROR: Failed to get available groups: %v", err)
			http.Error(w, fmt.Sprintf("Failed to get available groups: %v", err), http.StatusInternalServerError)
			return
		}

		// Find next group index (max + 1)
		maxIndex := int32(0)
		for _, idx := range availableGroups {
			if idx > maxIndex {
				maxIndex = idx
			}
		}
		targetGroup = int(maxIndex + 1)
	}

	err = models.MoveStudentBetweenGroups(leadID, int32(targetGroup))
	if err != nil {
		log.Printf("ERROR: Failed to move student: %v", err)
		http.Error(w, fmt.Sprintf("Failed to move student: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/classes?moved=1", http.StatusFound)
}

// StartRound handles starting a new round
func (h *ClassesHandler) StartRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	err := models.StartRound()
	if err != nil {
		log.Printf("ERROR: Failed to start round: %v", err)
		http.Error(w, fmt.Sprintf("Failed to start round: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/classes?round_started=1", http.StatusFound)
}

// SendToMentor handles sending a class group to mentor head
func (h *ClassesHandler) SendToMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	classKey := r.FormValue("class_key")
	levelStr := r.FormValue("level")
	classDays := r.FormValue("class_days")
	classTime := r.FormValue("class_time")
	classNumberStr := r.FormValue("class_number")

	// If class_key is provided, use it; otherwise construct from form fields
	if classKey == "" {
		if levelStr == "" || classDays == "" || classTime == "" || classNumberStr == "" {
			http.Error(w, "class_key or (level, class_days, class_time, class_number) required", http.StatusBadRequest)
			return
		}
		level, err := strconv.Atoi(levelStr)
		if err != nil {
			http.Error(w, "Invalid level", http.StatusBadRequest)
			return
		}
		classNumber, err := strconv.Atoi(classNumberStr)
		if err != nil {
			http.Error(w, "Invalid class_number", http.StatusBadRequest)
			return
		}
		classKey = models.GenerateClassKey(int32(level), classDays, classTime, int32(classNumber))
		// Use the parsed values
		levelInt, _ := strconv.Atoi(levelStr)
		classNumberInt, _ := strconv.Atoi(classNumberStr)
		err = models.SendClassGroupToMentor(classKey, int32(levelInt), classDays, classTime, int32(classNumberInt))
		if err != nil {
			log.Printf("ERROR: Failed to send class to mentor: %v", err)
			http.Error(w, fmt.Sprintf("Failed to send class to mentor: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Parse class key to get components
		// Format: "L{level}|{days}|{time}|{index}"
		parts := strings.Split(classKey, "|")
		if len(parts) != 4 || !strings.HasPrefix(parts[0], "L") {
			http.Error(w, "Invalid class_key format", http.StatusBadRequest)
			return
		}
		level, err := strconv.Atoi(strings.TrimPrefix(parts[0], "L"))
		if err != nil {
			http.Error(w, "Invalid level in class_key", http.StatusBadRequest)
			return
		}
		classNumber, err := strconv.Atoi(parts[3])
		if err != nil {
			http.Error(w, "Invalid class_number in class_key", http.StatusBadRequest)
			return
		}
		err = models.SendClassGroupToMentor(classKey, int32(level), parts[1], parts[2], int32(classNumber))
		if err != nil {
			log.Printf("ERROR: Failed to send class to mentor: %v", err)
			http.Error(w, fmt.Sprintf("Failed to send class to mentor: %v", err), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/classes?sent=1", http.StatusFound)
}

// ReturnFromMentor handles returning a class group from mentor head.
// Uses POST /classes/return with form field class_key (not path) because classKey can contain "/".
func (h *ClassesHandler) ReturnFromMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	classKey := r.FormValue("class_key")
	if classKey == "" {
		http.Error(w, "class_key is required", http.StatusBadRequest)
		return
	}

	err := models.ReturnClassGroupFromMentor(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to return class from mentor: %v", err)
		http.Error(w, fmt.Sprintf("Failed to return class from mentor: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/classes?returned=1", http.StatusFound)
}
