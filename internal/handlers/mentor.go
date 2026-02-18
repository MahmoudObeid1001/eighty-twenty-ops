package handlers

import (
	"database/sql"
	"errors"
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

func mentorClassURL(basePath, classKey string, r *http.Request) string {
	u := fmt.Sprintf("%s?class_key=%s", basePath, url.QueryEscape(classKey))
	if r != nil {
		if sess := r.FormValue("session"); sess != "" {
			u += "&session=" + url.QueryEscape(sess)
		}
		if studentID := r.FormValue("student_id"); studentID != "" {
			u += "&student_id=" + url.QueryEscape(studentID)
		}
	}
	return u
}

// Dashboard lists all classes assigned to the current mentor
func (h *MentorHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		redirectWithError(w, r, "/mentor", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		redirectWithError(w, r, "/mentor", "You don't have permission to do this.")
		return
	}

	userIDStr := middleware.GetUserID(r)
	mentorUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		redirectWithError(w, r, "/mentor", "We couldn't verify your account. Please refresh and try again.")
		return
	}

	flashMessage, flashMessageType := flashFromQuery(r)
	classes, err := models.GetMentorClasses(mentorUserID)
	if err != nil {
		log.Printf("ERROR: Failed to get mentor classes: %v", err)
		if flashMessage == "" {
			flashMessage = "Couldn't load classes. Please refresh and try again."
			flashMessageType = "error"
		}
		classes = []*models.ClassGroupWorkflow{}
	}

	mentorEmail := middleware.GetUserEmail(r)
	classCount := len(classes)
	nextClassTime := nextUpcomingClassTime(classes)

	data := map[string]interface{}{
		"Title":            "My Classes – Eighty Twenty",
		"Classes":          classes,
		"MentorEmail":      mentorEmail,
		"ClassCount":       classCount,
		"NextClassTime":    nextClassTime,
		"IsAdmin":          userRole == "admin",
		"IsModerator":      userRole == "moderator",
		"FlashMessage":     flashMessage,
		"FlashMessageType": flashMessageType,
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
		redirectWithError(w, r, "/mentor", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		redirectWithError(w, r, "/mentor", "You don't have permission to do this.")
		return
	}

	classKeyRaw := r.URL.Query().Get("class_key")
	classKey, err := url.QueryUnescape(classKeyRaw)
	if err != nil {
		classKey = classKeyRaw // Fallback to raw if decode fails
	}
	if classKey == "" {
		redirectWithError(w, r, "/mentor", "Missing class reference. Please refresh and try again.")
		return
	}

	// Verify mentor is assigned to this class
	userIDStr := middleware.GetUserID(r)
	mentorUserID, err := uuid.Parse(userIDStr)
	if err == nil && userRole != "admin" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			redirectWithError(w, r, "/mentor", "You aren't assigned to this class.")
			return
		}
	}

	// Get class info
	classGroup, err := models.GetClassGroupByKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get class group: %v", err)
		redirectWithError(w, r, "/mentor", "Class not found. Please refresh and try again.")
		return
	}
	if classGroup == nil {
		redirectWithError(w, r, "/mentor", "Class not found. Please refresh and try again.")
		return
	}

	// Get sessions
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get sessions: %v", err)
		redirectWithError(w, r, "/mentor", "Couldn't load sessions. Please refresh and try again.")
		return
	}

	// Get students
	students, err := models.GetStudentsInClassGroup(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get students: %v", err)
		redirectWithError(w, r, "/mentor", "Couldn't load students. Please refresh and try again.")
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

	// Attendance deadline banner: sessions older than 24h with missing attendance
	overdueSessions := []int32{}
	now := time.Now()
	for _, session := range sessions {
		if session.Status == "completed" || session.Status == "cancelled" {
			continue
		}
		endTime, err := models.ComputeSessionEndTime(session)
		if err != nil {
			continue
		}
		if now.After(endTime.Add(24 * time.Hour)) {
			hasMissing := false
			for _, student := range studentsWithData {
				if _, ok := student.Attendance[session.ID]; !ok {
					hasMissing = true
					break
				}
			}
			if hasMissing {
				overdueSessions = append(overdueSessions, session.SessionNumber)
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
		"OverdueSessions":  overdueSessions,
		"IsAdmin":          userRole == "admin",
		"IsModerator":      userRole == "moderator",
		"IsMentorHeadView": false,
		"CurrentUserID":    middleware.GetUserID(r),
		"UserRole":         userRole,
		"FlashMessage":     "",
		"FlashMessageType": "",
	}
	if msg, kind := flashFromQuery(r); msg != "" {
		data["FlashMessage"] = msg
		data["FlashMessageType"] = kind
	} else if r.URL.Query().Get("attendance_saved") == "1" {
		data["FlashMessage"] = "Attendance saved."
		data["FlashMessageType"] = "success"
	} else if r.URL.Query().Get("grade_saved") == "1" {
		data["FlashMessage"] = "Grade saved."
		data["FlashMessageType"] = "success"
	} else if r.URL.Query().Get("note_saved") == "1" {
		data["FlashMessage"] = "Note saved."
		data["FlashMessageType"] = "success"
	} else if r.URL.Query().Get("session_completed") == "1" {
		data["FlashMessage"] = "Session completed."
		data["FlashMessageType"] = "success"
	}

	renderTemplate(w, r, "mentor_class_detail.html", data)
}

// MarkAttendance marks attendance for a student in a session
func (h *MentorHandler) MarkAttendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/mentor", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		redirectWithError(w, r, "/mentor", "You don't have permission to do this.")
		return
	}

	classKey := r.FormValue("class_key")
	sessionIDStr := r.FormValue("session_id")
	leadIDStr := r.FormValue("lead_id")
	attendedStr := r.FormValue("attended")
	notes := r.FormValue("notes")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Please choose a valid session.")
		return
	}

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "We couldn't find that student. Please refresh and try again.")
		return
	}

	status := attendedStr
	switch status {
	case "true":
		status = "PRESENT"
	case "false":
		status = "ABSENT"
	}

	// Verify mentor is assigned to this session's class
	userIDStr := middleware.GetUserID(r)
	mentorUserID, _ := uuid.Parse(userIDStr)
	if userRole != "admin" {
		session, err := models.GetSessionByID(sessionID)
		if err != nil || session == nil {
			redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Session not found. Please refresh and try again.")
			return
		}
		assignment, err := models.GetMentorAssignment(session.ClassKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "You aren't assigned to this class.")
			return
		}
	}

	markedByUserID, _ := uuid.Parse(userIDStr)
	enforceDeadline := userRole == "mentor"
	if err := models.MarkAttendance(sessionID, leadID, status, notes, markedByUserID, enforceDeadline); err != nil {
		log.Printf("ERROR: Failed to mark attendance: %v", err)
		if errors.Is(err, models.ErrAttendanceDeadlinePassed) {
			redirectWithError(w, r, mentorClassURL("/mentor/class", r.FormValue("class_key"), r), "Attendance deadline has passed (24 hours after session end). Please contact an admin.")
			return
		}
		redirectWithError(w, r, mentorClassURL("/mentor/class", r.FormValue("class_key"), r), "Couldn't save attendance. Please try again.")
		return
	}

	sess := r.FormValue("session")
	studentID := r.FormValue("student_id")
	u := fmt.Sprintf("/mentor/class?class_key=%s&attendance_saved=1", url.QueryEscape(classKey))
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
		redirectWithError(w, r, "/mentor", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		redirectWithError(w, r, "/mentor", "You don't have permission to do this.")
		return
	}

	leadIDStr := r.FormValue("lead_id")
	classKey := r.FormValue("class_key")
	grade := r.FormValue("grade")
	notes := r.FormValue("notes")

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "We couldn't find that student. Please refresh and try again.")
		return
	}

	// Verify grade is valid
	allowedGrades := map[string]bool{"A": true, "B": true, "C": true, "F": true}
	if !allowedGrades[grade] {
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Invalid grade. Must be A, B, C, or F.")
		return
	}

	// Verify session 8 is completed
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Couldn't verify the session. Please try again.")
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
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Session 8 must be completed before entering grades.")
		return
	}

	// Verify mentor is assigned
	userIDStr := middleware.GetUserID(r)
	mentorUserID, _ := uuid.Parse(userIDStr)
	if userRole != "admin" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "You aren't assigned to this class.")
			return
		}
	}

	createdByUserID, _ := uuid.Parse(userIDStr)
	if err := models.EnterGrade(leadID, classKey, grade, notes, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to enter grade: %v", err)
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Couldn't save the grade. Please try again.")
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
		redirectWithError(w, r, "/mentor", "This action isn't available.")
		return
	}

	// Explicitly parse form to ensure data is available
	if err := r.ParseForm(); err != nil {
		log.Printf("ERROR: Failed to parse form: %v", err)
		redirectWithError(w, r, "/mentor", "We couldn't read the form data. Please refresh and try again.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" && userRole != "mentor_head" {
		redirectWithError(w, r, "/mentor", "You don't have permission to do this.")
		return
	}

	leadIDStr := r.FormValue("lead_id")
	classKey := r.FormValue("class_key")
	sessionNumberStr := r.FormValue("session_number")
	noteText := strings.TrimSpace(r.FormValue("note_text"))
	basePath := "/mentor/class"
	if userRole == "mentor_head" {
		basePath = "/mentor-head/class"
	}

	// Validate required fields
	if leadIDStr == "" {
		log.Printf("ERROR: AddNote - lead_id is empty")
		redirectWithError(w, r, mentorClassURL(basePath, classKey, r), "Please choose a student.")
		return
	}
	if noteText == "" {
		log.Printf("ERROR: AddNote - note_text is empty")
		redirectWithError(w, r, mentorClassURL(basePath, classKey, r), "Please enter a note.")
		return
	}

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		log.Printf("ERROR: AddNote - Invalid lead_id: %q, error: %v", leadIDStr, err)
		redirectWithError(w, r, mentorClassURL(basePath, classKey, r), "We couldn't find that student. Please refresh and try again.")
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
		redirectWithError(w, r, mentorClassURL(basePath, classKey, r), "Please log in to continue.")
		return
	}

	createdByUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("ERROR: AddNote - Invalid user ID: %q, error: %v", userIDStr, err)
		redirectWithError(w, r, mentorClassURL(basePath, classKey, r), "We couldn't verify your account. Please refresh and try again.")
		return
	}

	if err := models.AddStudentNote(leadID, classKey, sessionNumber, noteText, false, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to add note: lead_id=%s, error: %v", leadID, err)
		redirectWithError(w, r, mentorClassURL(basePath, classKey, r), "Couldn't save the note. Please try again.")
		return
	}

	sess := r.FormValue("session")
	studentID := r.FormValue("student_id")
	// Redirect to appropriate route based on role
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
		redirectWithError(w, r, "/mentor", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		redirectWithError(w, r, "/mentor", "You don't have permission to do this.")
		return
	}

	classKey := r.FormValue("class_key")
	sessionIDStr := r.FormValue("session_id")
	actualDateStr := r.FormValue("actual_date")
	actualTimeStr := r.FormValue("actual_time")

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Please choose a valid session.")
		return
	}

	// Verify mentor is assigned
	userIDStr := middleware.GetUserID(r)
	mentorUserID, _ := uuid.Parse(userIDStr)
	if userRole != "admin" {
		session, err := models.GetSessionByID(sessionID)
		if err != nil || session == nil {
			redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "Session not found. Please refresh and try again.")
			return
		}
		assignment, err := models.GetMentorAssignment(session.ClassKey)
		if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
			redirectWithError(w, r, mentorClassURL("/mentor/class", classKey, r), "You aren't assigned to this class.")
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
		if errors.Is(err, models.ErrAttendanceIncomplete) {
			redirectWithError(w, r, mentorClassURL("/mentor/class", r.FormValue("class_key"), r), "Please mark attendance for all students before completing the session.")
			return
		}
		redirectWithError(w, r, mentorClassURL("/mentor/class", r.FormValue("class_key"), r), "Couldn't complete the session. Please try again.")
		return
	}

	sess := r.FormValue("session")
	studentID := r.FormValue("student_id")
	u := fmt.Sprintf("/mentor/class?class_key=%s&session_completed=1", url.QueryEscape(classKey))
	if sess != "" {
		u += "&session=" + url.QueryEscape(sess)
	}
	if studentID != "" {
		u += "&student_id=" + url.QueryEscape(studentID)
	}
	http.Redirect(w, r, u, http.StatusFound)
}
