package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

type MentorHeadHandler struct {
	config *config.Config
}

func NewMentorHeadHandler(cfg *config.Config) *MentorHeadHandler {
	return &MentorHeadHandler{config: cfg}
}

// Dashboard lists all classes sent to mentor head
func (h *MentorHeadHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor Head or Admin access required", http.StatusForbidden)
		return
	}

	// Get all classes where sent_to_mentor = true
	classes, err := models.GetClassGroupsSentToMentor()
	if err != nil {
		log.Printf("ERROR: Failed to get classes: %v", err)
		http.Error(w, "Failed to load classes", http.StatusInternalServerError)
		return
	}

	// Get mentor assignments and users for each class
	type ClassWithMentor struct {
		*models.ClassGroupWorkflow
		MentorUserID    *uuid.UUID
		MentorUserIDStr string
		MentorEmail     string
		StudentCount    int
		Readiness       string
	}

	classesWithMentors := make([]ClassWithMentor, 0, len(classes))
	for _, c := range classes {
		cwm := ClassWithMentor{ClassGroupWorkflow: c}

		// Get mentor assignment
		assignment, err := models.GetMentorAssignment(c.ClassKey)
		if err != nil {
			log.Printf("WARNING: Failed to get mentor assignment for %s: %v", c.ClassKey, err)
		} else if assignment != nil {
			cwm.MentorUserID = &assignment.MentorUserID
			cwm.MentorUserIDStr = assignment.MentorUserID.String()
			// Get mentor email
			user, err := models.GetUserByID(assignment.MentorUserID.String())
			if err == nil && user != nil {
				cwm.MentorEmail = user.Email
			}
		}

		// Get student count and readiness
		students, err := models.GetStudentsInClassGroup(c.ClassKey)
		if err == nil {
			cwm.StudentCount = len(students)
			if cwm.StudentCount >= 6 {
				cwm.Readiness = "LOCKED"
			} else if cwm.StudentCount >= 4 {
				cwm.Readiness = "READY"
			} else {
				cwm.Readiness = "NOT READY"
			}
		}

		classesWithMentors = append(classesWithMentors, cwm)
	}

	// Get all mentors (users with role='mentor')
	mentors, err := models.GetUsersByRole("mentor")
	if err != nil {
		log.Printf("WARNING: Failed to get mentors: %v", err)
		mentors = []*models.User{}
	}

	data := map[string]interface{}{
		"Title":       "Mentor Head – Eighty Twenty",
		"Classes":     classesWithMentors,
		"Mentors":     mentors,
		"IsAdmin":     middleware.GetUserRole(r) == "admin",
		"IsModerator": userRole == "moderator",
		"UserRole":    userRole,
		"assigned":    r.URL.Query().Get("assigned"),
		"returned":    r.URL.Query().Get("returned"),
		"rescheduled": r.URL.Query().Get("rescheduled"),
		"closed":      r.URL.Query().Get("closed"),
	}

	renderTemplate(w, r, "mentor_head.html", data)
}

// AssignMentor assigns a mentor to a class
func (h *MentorHeadHandler) AssignMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor Head or Admin access required", http.StatusForbidden)
		return
	}

	classKey := r.FormValue("class_key")
	mentorUserIDStr := r.FormValue("mentor_user_id")

	if classKey == "" || mentorUserIDStr == "" {
		http.Error(w, "class_key and mentor_user_id are required", http.StatusBadRequest)
		return
	}

	mentorUserID, err := uuid.Parse(mentorUserIDStr)
	if err != nil {
		http.Error(w, "Invalid mentor_user_id", http.StatusBadRequest)
		return
	}

	// Verify user has role='mentor'
	user, err := models.GetUserByID(mentorUserIDStr)
	if err != nil || user == nil || user.Role != "mentor" {
		http.Error(w, "Invalid mentor user", http.StatusBadRequest)
		return
	}

	// Get class sessions to check for conflicts
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get sessions: %v", err)
		http.Error(w, "Failed to check schedule conflicts", http.StatusInternalServerError)
		return
	}

	// Check for conflicts with each session
	for _, session := range sessions {
		if session.Status == "cancelled" {
			continue
		}
		if !session.ScheduledTime.Valid || !session.ScheduledEndTime.Valid {
			continue
		}

		hasConflict, err := models.CheckMentorScheduleConflict(
			mentorUserID,
			session.ScheduledDate,
			session.ScheduledTime.String,
			session.ScheduledEndTime.String,
		)
		if err != nil {
			log.Printf("ERROR: Failed to check conflict: %v", err)
			http.Error(w, "Failed to check schedule conflicts", http.StatusInternalServerError)
			return
		}
		if hasConflict {
			http.Error(w, fmt.Sprintf("Mentor has conflicting session on %s at %s", session.ScheduledDate.Format("2006-01-02"), session.ScheduledTime.String), http.StatusBadRequest)
			return
		}
	}

	// Assign mentor
	createdByUserID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.AssignMentorToClass(classKey, mentorUserID, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to assign mentor: %v", err)
		http.Error(w, "Failed to assign mentor", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/mentor-head?assigned=1", http.StatusFound)
}

// ReturnClass returns a class from mentor head back to Operations.
// Uses POST /mentor-head/return with form field class_key (not path) because classKey
// can contain "/" (e.g. "Sun/Wed") and breaks path-based routing.
func (h *MentorHeadHandler) ReturnClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor Head or Admin access required", http.StatusForbidden)
		return
	}

	classKey := r.FormValue("class_key")
	if classKey == "" {
		http.Error(w, "class_key is required", http.StatusBadRequest)
		return
	}

	log.Printf("[Return] classKey=%q (from form), fields: sent_to_mentor=false, returned_at=now, delete mentor_assignments", classKey)
	if err := models.ReturnClassGroupFromMentor(classKey); err != nil {
		log.Printf("ERROR: Failed to return class classKey=%q: %v", classKey, err)
		http.Error(w, "Failed to return class", http.StatusInternalServerError)
		return
	}
	log.Printf("[Return] classKey=%q OK, removed from mentor-head list", classKey)

	http.Redirect(w, r, "/mentor-head?returned=1", http.StatusFound)
}

// StartRound starts a round for a class by creating 8 sessions
func (h *MentorHeadHandler) StartRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor Head or Admin access required", http.StatusForbidden)
		return
	}

	classKey := r.FormValue("class_key")
	if classKey == "" {
		http.Error(w, "class_key is required", http.StatusBadRequest)
		return
	}

	// Get class info to get start date/time
	classGroup, err := models.GetClassGroupByKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get class group: %v", err)
		http.Error(w, "Class not found", http.StatusNotFound)
		return
	}

	// Check if sessions already exist
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to check sessions: %v", err)
		http.Error(w, "Failed to check sessions", http.StatusInternalServerError)
		return
	}
	if len(sessions) > 0 {
		http.Redirect(w, r, fmt.Sprintf("/mentor-head?error=round_already_started"), http.StatusFound)
		return
	}

	// Use today as start date and class time from class_groups
	startDate := time.Now()
	startTime := classGroup.ClassTime

	// Get user ID
	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("ERROR: Invalid user ID: %v", err)
		http.Error(w, "Invalid user", http.StatusInternalServerError)
		return
	}

	// Start round (set status='active' + create 8 sessions)
	if err := models.StartClassRound(classKey, userID, startDate, startTime); err != nil {
		log.Printf("ERROR: Failed to start round: %v", err)
		http.Error(w, "Failed to start round", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/mentor-head?round_started=1"), http.StatusFound)
}

// CancelSession cancels a session and reschedules it
func (h *MentorHeadHandler) CancelSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor Head or Admin access required", http.StatusForbidden)
		return
	}

	sessionIDStr := r.FormValue("session_id")
	compensationDateStr := r.FormValue("compensation_date")
	compensationTimeStr := r.FormValue("compensation_time")

	if sessionIDStr == "" || compensationDateStr == "" || compensationTimeStr == "" {
		http.Error(w, "session_id, compensation_date, and compensation_time are required", http.StatusBadRequest)
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		http.Error(w, "Invalid session_id", http.StatusBadRequest)
		return
	}

	compensationDate, err := time.Parse("2006-01-02", compensationDateStr)
	if err != nil {
		http.Error(w, "Invalid compensation_date format (use YYYY-MM-DD)", http.StatusBadRequest)
		return
	}

	// Reschedule the same session
	if err := models.CancelAndRescheduleSession(sessionID, compensationDate, compensationTimeStr); err != nil {
		log.Printf("ERROR: Failed to reschedule session: %v", err)
		http.Error(w, "Failed to reschedule session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/mentor-head?rescheduled=1", http.StatusFound)
}

// CloseRound closes the round, computes outcomes, and returns class to Operations
func (h *MentorHeadHandler) CloseRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor Head or Admin access required", http.StatusForbidden)
		return
	}

	classKey := r.FormValue("class_key")
	if classKey == "" {
		http.Error(w, "class_key is required", http.StatusBadRequest)
		return
	}

	closedByID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.CloseRound(classKey, closedByID); err != nil {
		log.Printf("ERROR: Failed to close round: %v", err)
		http.Error(w, "Failed to close round", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/mentor-head?closed=1", http.StatusFound)
}

// ClassDetail shows read-only class detail (sessions, students, attendance, grades, notes) for mentor_head.
// Uses GET /mentor-head/class?class_key=... (query param) because classKey can contain "/" and breaks path params.
func (h *MentorHeadHandler) ClassDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor Head or Admin access required", http.StatusForbidden)
		return
	}

	classKey := r.URL.Query().Get("class_key")
	if classKey == "" {
		http.Error(w, "class_key is required", http.StatusBadRequest)
		return
	}

	wf, err := models.GetClassGroupWorkflow(classKey)
	if err != nil || wf == nil {
		http.Error(w, "Class not found", http.StatusNotFound)
		return
	}
	assignment, _ := models.GetMentorAssignment(classKey)
	if !wf.SentToMentor && assignment == nil {
		http.Error(w, "Class not sent to Mentor Head or assigned to a mentor", http.StatusForbidden)
		return
	}

	classGroup, err := models.GetClassGroupByKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get class group: %v", err)
		http.Error(w, "Class not found", http.StatusNotFound)
		return
	}

	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get sessions: %v", err)
		http.Error(w, "Failed to load sessions", http.StatusInternalServerError)
		return
	}

	students, err := models.GetStudentsInClassGroup(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get students: %v", err)
		http.Error(w, "Failed to load students", http.StatusInternalServerError)
		return
	}

	type StudentWithAttendance struct {
		*models.ClassStudent
		Attendance  map[uuid.UUID]string
		Notes       []*models.StudentNote
		LastNote    *models.StudentNote
		Grade       *models.Grade
		MissedCount int
	}

	studentsWithData := make([]StudentWithAttendance, 0, len(students))
	for _, student := range students {
		swa := StudentWithAttendance{
			ClassStudent: student,
			Attendance:   make(map[uuid.UUID]string),
		}
		for _, session := range sessions {
			attendance, err := models.GetAttendanceForSession(session.ID)
			if err == nil {
				for _, att := range attendance {
					if att.LeadID == student.LeadID {
						swa.Attendance[session.ID] = att.Status
						if att.Status == "ABSENT" {
							swa.MissedCount++
						}
						break
					}
				}
			}
		}
		notes, err := models.GetStudentNotes(student.LeadID)
		if err != nil {
			log.Printf("WARNING: Failed to get notes for lead_id=%s: %v", student.LeadID, err)
		}
		swa.Notes = notes
		if len(notes) > 0 {
			swa.LastNote = notes[0]
		}
		grade, _ := models.GetGrade(student.LeadID, classKey)
		swa.Grade = grade
		studentsWithData = append(studentsWithData, swa)
	}

	completedCount := 0
	var nextNotCompleted int32 = 1
	for _, s := range sessions {
		if s.Status == "completed" {
			completedCount++
		} else {
			nextNotCompleted = s.SessionNumber
			break
		}
	}
	selectedSession := nextNotCompleted
	if n, err := strconv.Atoi(r.URL.Query().Get("session")); err == nil && n >= 1 && n <= 8 {
		selectedSession = int32(n)
	}

	// Check if student_id is in query (for student panel view)
	studentIDStr := r.URL.Query().Get("student_id")
	var selectedStudent *StudentWithAttendance
	if studentIDStr != "" {
		studentID, err := uuid.Parse(studentIDStr)
		if err == nil {
			for i := range studentsWithData {
				if studentsWithData[i].LeadID == studentID {
					selectedStudent = &studentsWithData[i]
					break
				}
			}
		}
	}

	data := map[string]interface{}{
		"Title":            "Class – Eighty Twenty",
		"Class":            classGroup,
		"Sessions":         sessions,
		"Students":         studentsWithData,
		"SelectedSession":  selectedSession,
		"CompletedCount":   completedCount,
		"SelectedStudent":  selectedStudent,
		"IsAdmin":          userRole == "admin",
		"IsModerator":      userRole == "moderator",
		"UserRole":         userRole,
		"IsMentorHeadView": true,
		"CurrentUserID":    middleware.GetUserID(r),
	}

	renderTemplate(w, r, "mentor_class_detail.html", data)
}
