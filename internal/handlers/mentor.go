package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

type MentorHandler struct {
	config *config.Config
}

func NewMentorHandler(cfg *config.Config) *MentorHandler {
	return &MentorHandler{config: cfg}
}

// Dashboard lists all classes assigned to the current mentor
func (h *MentorHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor or Admin access required", http.StatusForbidden)
		return
	}

	userIDStr := middleware.GetUserID(r)
	mentorUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	classes, err := models.GetMentorClasses(mentorUserID)
	if err != nil {
		log.Printf("ERROR: Failed to get mentor classes: %v", err)
		http.Error(w, "Failed to load classes", http.StatusInternalServerError)
		return
	}

	mentorEmail := middleware.GetUserEmail(r)
	classCount := len(classes)
	nextClassTime := nextUpcomingClassTime(classes)

	data := map[string]interface{}{
		"Title":         "My Classes – Eighty Twenty",
		"Classes":       classes,
		"MentorEmail":   mentorEmail,
		"ClassCount":    classCount,
		"NextClassTime": nextClassTime,
		"IsAdmin":       userRole == "admin",
		"IsModerator":   userRole == "moderator",
	}

	renderTemplate(w, r, "mentor.html", data)
}

// nextUpcomingClassTime returns the soonest scheduled session (date + time) across mentor's classes, or "—" if none.
func nextUpcomingClassTime(classes []*models.ClassGroupWorkflow) string {
	now := time.Now()
	var bestDate *time.Time
	var bestTimeStr string
	for _, c := range classes {
		sessions, err := models.GetClassSessions(c.ClassKey)
		if err != nil {
			continue
		}
		for _, s := range sessions {
			if s.Status != "scheduled" {
				continue
			}
			d := s.ScheduledDate
			if d.Before(now) {
				continue
			}
			tstr := ""
			if s.ScheduledTime.Valid {
				tstr = s.ScheduledTime.String
			}
			if bestDate == nil || d.Before(*bestDate) || (d.Equal(*bestDate) && tstr < bestTimeStr) {
				dcopy := d
				bestDate = &dcopy
				bestTimeStr = tstr
			}
		}
	}
	if bestDate == nil {
		return "—"
	}
	if bestTimeStr != "" {
		return bestDate.Format("Mon Jan 2") + ", " + bestTimeStr
	}
	return bestDate.Format("Mon Jan 2")
}

// ClassDetail shows class detail with 8 sessions
func (h *MentorHandler) ClassDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor or Admin access required", http.StatusForbidden)
		return
	}

	classKeyRaw := r.URL.Query().Get("class_key")
	classKey, err := url.QueryUnescape(classKeyRaw)
	if err != nil {
		classKey = classKeyRaw // Fallback to raw if decode fails
	}
	if classKey == "" {
		http.Error(w, "class_key is required", http.StatusBadRequest)
		return
	}

	// Verify mentor is assigned to this class
	userIDStr := middleware.GetUserID(r)
	mentorUserID, err := uuid.Parse(userIDStr)
	if err == nil && userRole != "admin" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			http.Error(w, "Forbidden: You are not assigned to this class", http.StatusForbidden)
			return
		}
	}

	// Get class info
	classGroup, err := models.GetClassGroupByKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get class group: %v", err)
		http.Error(w, "Class not found", http.StatusNotFound)
		return
	}
	if classGroup == nil {
		http.Error(w, "Class not found", http.StatusNotFound)
		return
	}

	// Get sessions
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get sessions: %v", err)
		http.Error(w, "Failed to load sessions", http.StatusInternalServerError)
		return
	}

	// Get students
	students, err := models.GetStudentsInClassGroup(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get students: %v", err)
		http.Error(w, "Failed to load students", http.StatusInternalServerError)
		return
	}

	// Get attendance for all sessions
	type StudentWithAttendance struct {
		*models.ClassStudent
		Attendance  map[uuid.UUID]string // session_id -> status
		Notes       []*models.StudentNote
		LastNote    *models.StudentNote // most recent note (notes[0] when ordered DESC)
		Grade       *models.Grade
		MissedCount int // sessions where status='ABSENT'
	}

	studentsWithData := make([]StudentWithAttendance, 0, len(students))
	for _, student := range students {
		swa := StudentWithAttendance{
			ClassStudent: student,
			Attendance:   make(map[uuid.UUID]string),
		}

		// Get attendance for each session
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

		// Get notes (NOT filtered by sessions - notes exist independently)
		notes, err := models.GetStudentNotes(student.LeadID)
		if err != nil {
			log.Printf("WARNING: Failed to get notes for lead_id=%s: %v", student.LeadID, err)
		}
		swa.Notes = notes
		if len(notes) > 0 {
			swa.LastNote = notes[0]
		}

		// Get grade
		grade, _ := models.GetGrade(student.LeadID, classKey)
		swa.Grade = grade

		studentsWithData = append(studentsWithData, swa)
	}

	// Session selection: ?session=N (1–8). Default: next not-completed, or 1.
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
		"IsMentorHeadView": false,
		"CurrentUserID":    middleware.GetUserID(r),
		"UserRole":         userRole,
	}

	renderTemplate(w, r, "mentor_class_detail.html", data)
}

// MarkAttendance marks attendance for a student in a session
func (h *MentorHandler) MarkAttendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor or Admin access required", http.StatusForbidden)
		return
	}

	sessionIDStr := r.FormValue("session_id")
	leadIDStr := r.FormValue("lead_id")
	attendedStr := r.FormValue("attended")
	notes := r.FormValue("notes")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		http.Error(w, "Invalid session_id", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		http.Error(w, "Invalid lead_id", http.StatusBadRequest)
		return
	}

	status := attendedStr
	if status == "true" {
		status = "PRESENT"
	} else if status == "false" {
		status = "ABSENT"
	}

	// Verify mentor is assigned to this session's class
	userIDStr := middleware.GetUserID(r)
	mentorUserID, _ := uuid.Parse(userIDStr)
	if userRole != "admin" {
		session, err := models.GetSessionByID(sessionID)
		if err != nil || session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		assignment, err := models.GetMentorAssignment(session.ClassKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			http.Error(w, "Forbidden: You are not assigned to this class", http.StatusForbidden)
			return
		}
	}

	markedByUserID, _ := uuid.Parse(userIDStr)
	if err := models.MarkAttendance(sessionID, leadID, status, notes, markedByUserID); err != nil {
		log.Printf("ERROR: Failed to mark attendance: %v", err)
		http.Error(w, "Failed to mark attendance", http.StatusInternalServerError)
		return
	}

	ck := r.FormValue("class_key")
	sess := r.FormValue("session")
	studentID := r.FormValue("student_id")
	u := fmt.Sprintf("/mentor/class?class_key=%s&attendance_saved=1", url.QueryEscape(ck))
	if sess != "" {
		u += "&session=" + url.QueryEscape(sess)
	}
	if studentID != "" {
		u += "&student_id=" + url.QueryEscape(studentID)
	}
	http.Redirect(w, r, u, http.StatusFound)
}

// EnterGrade enters a grade for a student at session 8
func (h *MentorHandler) EnterGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor or Admin access required", http.StatusForbidden)
		return
	}

	leadIDStr := r.FormValue("lead_id")
	classKey := r.FormValue("class_key")
	grade := r.FormValue("grade")
	notes := r.FormValue("notes")

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		http.Error(w, "Invalid lead_id", http.StatusBadRequest)
		return
	}

	// Verify grade is valid
	allowedGrades := map[string]bool{"A": true, "B": true, "C": true, "F": true}
	if !allowedGrades[grade] {
		http.Error(w, "Invalid grade. Must be A, B, C, or F", http.StatusBadRequest)
		return
	}

	// Verify session 8 is completed
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		http.Error(w, "Failed to verify session", http.StatusInternalServerError)
		return
	}
	var session8Completed bool
	for _, s := range sessions {
		if s.SessionNumber == 8 && s.Status == "completed" {
			session8Completed = true
			break
		}
	}
	if !session8Completed {
		http.Error(w, "Session 8 must be completed before entering grades", http.StatusBadRequest)
		return
	}

	// Verify mentor is assigned
	userIDStr := middleware.GetUserID(r)
	mentorUserID, _ := uuid.Parse(userIDStr)
	if userRole != "admin" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			http.Error(w, "Forbidden: You are not assigned to this class", http.StatusForbidden)
			return
		}
	}

	createdByUserID, _ := uuid.Parse(userIDStr)
	if err := models.EnterGrade(leadID, classKey, grade, notes, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to enter grade: %v", err)
		http.Error(w, "Failed to enter grade", http.StatusInternalServerError)
		return
	}

	sess := r.FormValue("session")
	studentID := r.FormValue("student_id")
	u := fmt.Sprintf("/mentor/class?class_key=%s&grade_saved=1", url.QueryEscape(classKey))
	if sess != "" {
		u += "&session=" + url.QueryEscape(sess)
	}
	if studentID != "" {
		u += "&student_id=" + url.QueryEscape(studentID)
	}
	http.Redirect(w, r, u, http.StatusFound)
}

// AddNote adds a note for a student
func (h *MentorHandler) AddNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Explicitly parse form to ensure data is available
	if err := r.ParseForm(); err != nil {
		log.Printf("ERROR: Failed to parse form: %v", err)
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" && userRole != "mentor_head" {
		http.Error(w, "Forbidden: Mentor, Mentor Head, or Admin access required", http.StatusForbidden)
		return
	}

	leadIDStr := r.FormValue("lead_id")
	classKey := r.FormValue("class_key")
	sessionNumberStr := r.FormValue("session_number")
	noteText := strings.TrimSpace(r.FormValue("note_text"))

	// Validate required fields
	if leadIDStr == "" {
		log.Printf("ERROR: AddNote - lead_id is empty")
		http.Error(w, "lead_id is required", http.StatusBadRequest)
		return
	}
	if noteText == "" {
		log.Printf("ERROR: AddNote - note_text is empty")
		http.Error(w, "Note text cannot be empty", http.StatusBadRequest)
		return
	}

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		log.Printf("ERROR: AddNote - Invalid lead_id: %q, error: %v", leadIDStr, err)
		http.Error(w, "Invalid lead_id", http.StatusBadRequest)
		return
	}

	var sessionNumber sql.NullInt32
	if sessionNumberStr != "" {
		sn, err := strconv.Atoi(sessionNumberStr)
		if err == nil {
			sessionNumber = sql.NullInt32{Int32: int32(sn), Valid: true}
		}
	}

	userIDStr := middleware.GetUserID(r)
	if userIDStr == "" {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	createdByUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("ERROR: AddNote - Invalid user ID: %q, error: %v", userIDStr, err)
		http.Error(w, "Invalid user ID", http.StatusInternalServerError)
		return
	}

	if err := models.AddStudentNote(leadID, classKey, sessionNumber, noteText, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to add note: lead_id=%s, error: %v", leadID, err)
		http.Error(w, fmt.Sprintf("Failed to add note: %v", err), http.StatusInternalServerError)
		return
	}

	sess := r.FormValue("session")
	studentID := r.FormValue("student_id")
	// Redirect to appropriate route based on role
	var basePath string
	if userRole == "mentor_head" {
		basePath = "/mentor-head/class"
	} else {
		basePath = "/mentor/class"
	}
	u := fmt.Sprintf("%s?class_key=%s&note_saved=1", basePath, url.QueryEscape(classKey))
	if sess != "" {
		u += "&session=" + url.QueryEscape(sess)
	}
	if studentID != "" {
		u += "&student_id=" + url.QueryEscape(studentID)
		// Add fragment anchor to prevent scroll jump - browser will scroll to this element
		u += "#student-" + url.QueryEscape(studentID)
	}
	http.Redirect(w, r, u, http.StatusFound)
}

// CompleteSession marks a session as completed
func (h *MentorHandler) CompleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		http.Error(w, "Forbidden: Mentor or Admin access required", http.StatusForbidden)
		return
	}

	sessionIDStr := r.FormValue("session_id")
	actualDateStr := r.FormValue("actual_date")
	actualTimeStr := r.FormValue("actual_time")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		http.Error(w, "Invalid session_id", http.StatusBadRequest)
		return
	}

	// Verify mentor is assigned
	userIDStr := middleware.GetUserID(r)
	mentorUserID, _ := uuid.Parse(userIDStr)
	if userRole != "admin" {
		session, err := models.GetSessionByID(sessionID)
		if err != nil || session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		assignment, err := models.GetMentorAssignment(session.ClassKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			http.Error(w, "Forbidden: You are not assigned to this class", http.StatusForbidden)
			return
		}
	}

	actualDate := time.Now()
	if actualDateStr != "" {
		if d, err := time.Parse("2006-01-02", actualDateStr); err == nil {
			actualDate = d
		}
	}

	actualTime := time.Now().Format("15:04")
	if actualTimeStr != "" {
		actualTime = actualTimeStr
	}

	if err := models.CompleteSession(sessionID, actualDate, actualTime); err != nil {
		log.Printf("ERROR: Failed to complete session: %v", err)
		http.Error(w, "Failed to complete session", http.StatusInternalServerError)
		return
	}

	ck := r.FormValue("class_key")
	sess := r.FormValue("session")
	studentID := r.FormValue("student_id")
	u := fmt.Sprintf("/mentor/class?class_key=%s&session_completed=1", url.QueryEscape(ck))
	if sess != "" {
		u += "&session=" + url.QueryEscape(sess)
	}
	if studentID != "" {
		u += "&student_id=" + url.QueryEscape(studentID)
	}
	http.Redirect(w, r, u, http.StatusFound)
}
