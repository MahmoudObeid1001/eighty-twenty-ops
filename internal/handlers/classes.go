package handlers

import (
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
		http.Error(w, "This action isn't available.", http.StatusMethodNotAllowed)
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

	flashMessage, flashMessageType := flashFromQuery(r)

	// Get class groups
	groups, err := models.GetClassGroups()
	if err != nil {
		log.Printf("ERROR: Failed to get class groups: %v", err)
		if flashMessage == "" {
			flashMessage = "Couldn't load classes. Please refresh and try again."
			flashMessageType = "error"
		}
		groups = []*models.ClassGroup{}
	}

	// Get current round
	currentRound, err := models.GetCurrentRound()
	if err != nil {
		log.Printf("ERROR: Failed to get current round: %v", err)
		currentRound = 1 // Default
	}

	// Check for flash messages
	if flashMessage == "" {
		if r.URL.Query().Get("moved") == "1" {
			flashMessage = "Student moved successfully"
			flashMessageType = "success"
		}
		if r.URL.Query().Get("sent") == "1" {
			flashMessage = "Class sent to mentor head successfully"
			flashMessageType = "success"
		}
		if r.URL.Query().Get("returned") == "1" {
			flashMessage = "Class returned from mentor head"
			flashMessageType = "success"
		}
		if r.URL.Query().Get("archived") == "1" {
			flashMessage = "Class archived from Ops view"
			flashMessageType = "success"
		}
		if r.URL.Query().Get("unarchived") == "1" {
			flashMessage = "Class restored to Ops view"
			flashMessageType = "success"
		}
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
		if flashMessage == "" {
			flashMessage = "Couldn't load classes. Please refresh and try again."
			flashMessageType = "error"
		}
		groups = []*models.ClassGroup{}
	}

	// Build available options by level from current groups (for move dropdown)
	optionsByLevel := map[int32][]models.MoveClassOption{}
	for _, group := range groups {
		if group.Readiness == "STARTED" || group.Readiness == "LOCKED" || group.SentToMentor {
			continue
		}
		opts := optionsByLevel[group.Level]
		opts = append(opts, models.MoveClassOption{
			Value: "class_key:" + group.ClassKey,
			Label: group.ClassDays + " @ " + group.ClassTime + " (Class #" + strconv.Itoa(int(group.GroupIndex)) + ")",
		})
		optionsByLevel[group.Level] = opts
	}

	// Compute available groups/options for each student (for move dropdown)
	for _, group := range groups {
		for _, student := range group.Students {
			availableGroups, err := models.GetAvailableGroupsForMove(student.LeadID)
			if err == nil {
				// Store as a simple slice of ints for template
				student.AvailableGroups = availableGroups
			}
			// Always include "create new with same days/time"
			student.AvailableClassOptions = []models.MoveClassOption{{
				Value: "new_same",
				Label: "Create New Class (same days/time)",
			}}
			// Append available classes for this level
			level := group.Level
			if opts, ok := optionsByLevel[level]; ok {
				for _, opt := range opts {
					// Remove current class from suggestions.
					if opt.Value == "class_key:"+group.ClassKey {
						continue
					}
					student.AvailableClassOptions = append(student.AvailableClassOptions, opt)
				}
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
		"FlashMessageType":  flashMessageType,
		"IsClassesReadOnly": userRole == "mentor_head",
	}
	renderTemplate(w, r, "classes.html", data)
}

// Move handles moving a student between groups
func (h *ClassesHandler) Move(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/classes", "This action isn't available.")
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		redirectWithError(w, r, "/classes", "You don't have permission to do this.")
		return
	}

	leadIDStr := r.FormValue("lead_id")
	targetGroupStr := r.FormValue("target_group")

	if leadIDStr == "" || targetGroupStr == "" {
		redirectWithError(w, r, "/classes", "Please choose a student and a target class.")
		return
	}

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		redirectWithError(w, r, "/classes", "We couldn't find that student. Please refresh and try again.")
		return
	}

	if strings.HasPrefix(targetGroupStr, "class_key:") {
		classKey := strings.TrimPrefix(targetGroupStr, "class_key:")
		err = models.MoveStudentToClassKey(leadID, classKey)
	} else {
		targetGroup, err := strconv.Atoi(targetGroupStr)
		if err != nil {
			redirectWithError(w, r, "/classes", "Please choose a valid target class.")
			return
		}

		// If target_group is 0 or new_same, create new group (find next available index)
		if targetGroupStr == "new_same" || targetGroup == 0 {
			availableGroups, err := models.GetAvailableGroupsForMove(leadID)
			if err != nil {
				log.Printf("ERROR: Failed to get available groups: %v", err)
				redirectWithError(w, r, "/classes", "Couldn't load available classes. Please try again.")
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
	}
	if err != nil {
		log.Printf("ERROR: Failed to move student: %v", err)
		redirectWithError(w, r, "/classes", "We couldn't move this student. Please try again.")
		return
	}

	http.Redirect(w, r, "/classes?moved=1", http.StatusFound)
}

// SendToMentor handles sending a class group to mentor head
func (h *ClassesHandler) SendToMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/classes", "This action isn't available.")
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		redirectWithError(w, r, "/classes", "You don't have permission to do this.")
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
			redirectWithError(w, r, "/classes", "Please select the class details before sending.")
			return
		}
		level, err := strconv.Atoi(levelStr)
		if err != nil {
			redirectWithError(w, r, "/classes", "Please choose a valid level.")
			return
		}
		classNumber, err := strconv.Atoi(classNumberStr)
		if err != nil {
			redirectWithError(w, r, "/classes", "Please choose a valid class number.")
			return
		}
		classKey = models.GenerateClassKey(int32(level), classDays, classTime, int32(classNumber))
		// Use the parsed values
		levelInt, _ := strconv.Atoi(levelStr)
		classNumberInt, _ := strconv.Atoi(classNumberStr)
		err = models.SendClassGroupToMentor(classKey, int32(levelInt), classDays, classTime, int32(classNumberInt))
		if err != nil {
			log.Printf("ERROR: Failed to send class to mentor: %v", err)
			redirectWithError(w, r, "/classes", "We couldn't send this class to Mentor Head. Please try again.")
			return
		}
	} else {
		// Parse class key to get components
		// Format: "L{level}|{days}|{time}|{index}"
		parts := strings.Split(classKey, "|")
		if len(parts) != 4 || !strings.HasPrefix(parts[0], "L") {
			redirectWithError(w, r, "/classes", "We couldn't read that class reference. Please refresh and try again.")
			return
		}
		level, err := strconv.Atoi(strings.TrimPrefix(parts[0], "L"))
		if err != nil {
			redirectWithError(w, r, "/classes", "We couldn't read that class level. Please refresh and try again.")
			return
		}
		classNumber, err := strconv.Atoi(parts[3])
		if err != nil {
			redirectWithError(w, r, "/classes", "We couldn't read that class number. Please refresh and try again.")
			return
		}
		err = models.SendClassGroupToMentor(classKey, int32(level), parts[1], parts[2], int32(classNumber))
		if err != nil {
			log.Printf("ERROR: Failed to send class to mentor: %v", err)
			redirectWithError(w, r, "/classes", "We couldn't send this class to Mentor Head. Please try again.")
			return
		}
	}

	http.Redirect(w, r, "/classes?sent=1", http.StatusFound)
}

// ReturnFromMentor handles returning a class group from mentor head.
// Uses POST /classes/return with form field class_key (not path) because classKey can contain "/".
func (h *ClassesHandler) ReturnFromMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/classes", "This action isn't available.")
		return
	}

	// Admin only
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		redirectWithError(w, r, "/classes", "You don't have permission to do this.")
		return
	}

	classKey := r.FormValue("class_key")
	if classKey == "" {
		redirectWithError(w, r, "/classes", "Missing class reference. Please refresh and try again.")
		return
	}

	err := models.ReturnClassGroupFromMentor(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to return class from mentor: %v", err)
		redirectWithError(w, r, "/classes", "Cannot return this class. If the round already started, please archive or close it instead.")
		return
	}

	http.Redirect(w, r, "/classes?returned=1", http.StatusFound)
}

// ArchiveClass hides a started class from the Ops Classes board.
// Only allowed when class is sent_to_mentor and active.
func (h *ClassesHandler) ArchiveClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/classes", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		redirectWithError(w, r, "/classes", "You don't have permission to do this.")
		return
	}

	classKey := r.FormValue("class_key")
	if classKey == "" {
		redirectWithError(w, r, "/classes", "Missing class reference. Please refresh and try again.")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		redirectWithError(w, r, "/classes", "We couldn't verify your account. Please refresh and try again.")
		return
	}

	if err := models.ArchiveClassInOps(classKey, userID); err != nil {
		log.Printf("ERROR: Failed to archive class: %v", err)
		redirectWithError(w, r, "/classes", "Only started classes that were sent to Mentor Head can be archived.")
		return
	}

	http.Redirect(w, r, "/classes?archived=1", http.StatusFound)
}

// UnarchiveClass restores a class to the Ops Classes board.
func (h *ClassesHandler) UnarchiveClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/classes/archived", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "admin" {
		redirectWithError(w, r, "/classes/archived", "You don't have permission to do this.")
		return
	}

	classKey := r.FormValue("class_key")
	if classKey == "" {
		redirectWithError(w, r, "/classes/archived", "Missing class reference. Please refresh and try again.")
		return
	}

	if err := models.UnarchiveClassInOps(classKey); err != nil {
		log.Printf("ERROR: Failed to unarchive class: %v", err)
		redirectWithError(w, r, "/classes/archived", "Could not restore this class. Please try again.")
		return
	}

	http.Redirect(w, r, "/classes/archived?unarchived=1", http.StatusFound)
}

// Archived shows archived classes for Ops with basic filters.
func (h *ClassesHandler) Archived(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		redirectWithError(w, r, "/classes", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "admin" && userRole != "mentor_head" {
		redirectWithError(w, r, "/classes", "You don't have permission to access this page.")
		return
	}

	classKeyFilter := strings.TrimSpace(r.URL.Query().Get("class_key"))
	var fromDate *time.Time
	var toDate *time.Time
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			fromDate = &t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			toDate = &t
		}
	}

	flashMessage, flashMessageType := flashFromQuery(r)
	archived, err := models.GetArchivedOpsClasses(classKeyFilter, fromDate, toDate)
	if err != nil {
		log.Printf("ERROR: Failed to load archived classes: %v", err)
		if flashMessage == "" {
			flashMessage = "Couldn't load archived classes. Please refresh and try again."
			flashMessageType = "error"
		}
		archived = []*models.ArchivedOpsClass{}
	}
	if r.URL.Query().Get("unarchived") == "1" {
		flashMessage = "Class restored to Ops view"
		flashMessageType = "success"
	}

	data := map[string]interface{}{
		"Title":            "Archived Classes – Eighty Twenty",
		"ContentTemplate":  "classes_archived_content",
		"Archived":         archived,
		"FlashMessage":     flashMessage,
		"FlashMessageType": flashMessageType,
		"UserRole":         userRole,
		"IsModerator":      IsModerator(r),
		"ClassKey":         classKeyFilter,
		"From":             r.URL.Query().Get("from"),
		"To":               r.URL.Query().Get("to"),
	}
	renderTemplate(w, r, "classes_archived.html", data)
}
