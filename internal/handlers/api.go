package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

type APIHandler struct {
	cfg *config.Config
}

func NewAPIHandler(cfg *config.Config) *APIHandler {
	return &APIHandler{cfg: cfg}
}

// JSON response helpers
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("ERROR: Failed to encode JSON response: %v", err)
	}
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

// GET /api/me - returns current user info
func (h *APIHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	userEmail := middleware.GetUserEmail(r)
	userRole := middleware.GetUserRole(r)

	if userID == "" {
		jsonError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	// Get user name (for now, use email as name)
	userName := userEmail
	user, err := models.GetUserByID(userID)
	if err == nil && user != nil {
		// If we have a name field later, use it; for now email is fine
		userName = userEmail
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":    userID,
		"email": userEmail,
		"name":  userName,
		"role":  userRole,
	})
}

// GET /api/mentor/classes - returns classes for current mentor
func (h *APIHandler) GetMentorClasses(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor or Admin access required")
		return
	}

	userIDStr := middleware.GetUserID(r)
	mentorUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	classes, err := models.GetMentorClasses(mentorUserID)
	if err != nil {
		log.Printf("ERROR: Failed to get mentor classes: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load classes")
		return
	}

	// Get student count for each class
	type ClassResponse struct {
		ClassKey     string `json:"class_key"`
		Level        int32  `json:"level"`
		Days         string `json:"days"`
		Time         string `json:"time"`
		ClassNumber  int32  `json:"class_number"`
		StudentCount int    `json:"student_count"`
	}

	response := make([]ClassResponse, 0, len(classes))
	for _, c := range classes {
		students, err := models.GetStudentsInClassGroup(c.ClassKey)
		if err != nil {
			log.Printf("WARNING: Failed to get students for class %s: %v", c.ClassKey, err)
		}

		response = append(response, ClassResponse{
			ClassKey:     c.ClassKey,
			Level:        c.Level,
			Days:         c.ClassDays,
			Time:         c.ClassTime,
			ClassNumber:  c.ClassNumber,
			StudentCount: len(students),
		})
	}

	jsonResponse(w, http.StatusOK, response)
}

// GET /api/mentor-head/mentors - returns all mentors
func (h *APIHandler) GetMentors(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	mentors, err := models.GetUsersByRole("mentor")
	if err != nil {
		log.Printf("ERROR: Failed to get mentors: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load mentors")
		return
	}

	type MentorResponse struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}

	response := make([]MentorResponse, 0, len(mentors))
	for _, m := range mentors {
		response = append(response, MentorResponse{
			ID:    m.ID.String(),
			Email: m.Email,
		})
	}

	jsonResponse(w, http.StatusOK, response)
}

// GET /api/mentor-head/classes - returns classes grouped by mentor
func (h *APIHandler) GetMentorHeadClasses(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	classes, err := models.GetClassGroupsSentToMentor()
	if err != nil {
		log.Printf("ERROR: Failed to get classes: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load classes")
		return
	}

	type ClassResponse struct {
		ClassKey     string `json:"class_key"`
		Level        int32  `json:"level"`
		Days         string `json:"days"`
		Time         string `json:"time"`
		ClassNumber  int32  `json:"class_number"`
		StudentCount int    `json:"student_count"`
	}

	type MentorGroupResponse struct {
		MentorID    *string         `json:"mentor_id,omitempty"`
		MentorEmail string          `json:"mentor_email,omitempty"`
		Classes     []ClassResponse `json:"classes"`
	}

	// Group classes by mentor
	mentorMap := make(map[string]*MentorGroupResponse)
	unassigned := &MentorGroupResponse{Classes: []ClassResponse{}}
	mentorMap[""] = unassigned

	for _, c := range classes {
		assignment, err := models.GetMentorAssignment(c.ClassKey)
		if err != nil || assignment == nil {
			// Unassigned class
			students, _ := models.GetStudentsInClassGroup(c.ClassKey)
			unassigned.Classes = append(unassigned.Classes, ClassResponse{
				ClassKey:     c.ClassKey,
				Level:        c.Level,
				Days:         c.ClassDays,
				Time:         c.ClassTime,
				ClassNumber:  c.ClassNumber,
				StudentCount: len(students),
			})
			continue
		}

		mentorIDStr := assignment.MentorUserID.String()
		if mentorMap[mentorIDStr] == nil {
			user, err := models.GetUserByID(mentorIDStr)
			mentorEmail := ""
			if err == nil && user != nil {
				mentorEmail = user.Email
			}
			mentorMap[mentorIDStr] = &MentorGroupResponse{
				MentorID:    &mentorIDStr,
				MentorEmail: mentorEmail,
				Classes:     []ClassResponse{},
			}
		}

		students, _ := models.GetStudentsInClassGroup(c.ClassKey)
		mentorMap[mentorIDStr].Classes = append(mentorMap[mentorIDStr].Classes, ClassResponse{
			ClassKey:     c.ClassKey,
			Level:        c.Level,
			Days:         c.ClassDays,
			Time:         c.ClassTime,
			ClassNumber:  c.ClassNumber,
			StudentCount: len(students),
		})
	}

	// Convert map to slice
	response := make([]MentorGroupResponse, 0, len(mentorMap))
	for _, group := range mentorMap {
		if len(group.Classes) > 0 {
			response = append(response, *group)
		}
	}

	jsonResponse(w, http.StatusOK, response)
}

// GET /api/class-workspace?class_key=... - returns class workspace data
func (h *APIHandler) GetClassWorkspace(w http.ResponseWriter, r *http.Request) {
	classKeyRaw := r.URL.Query().Get("class_key")
	classKey, err := url.QueryUnescape(classKeyRaw)
	if err != nil {
		classKey = classKeyRaw
	}
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)

	// Verify access: mentor can only access assigned classes, mentor_head/admin can access any
	if userRole == "mentor" {
		mentorUserID, err := uuid.Parse(userIDStr)
		if err == nil {
			assignment, err := models.GetMentorAssignment(classKey)
			if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
				jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
				return
			}
		}
	} else if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	// Get class info
	classGroup, err := models.GetClassGroupByKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get class group: %v", err)
		jsonError(w, http.StatusNotFound, "Class not found")
		return
	}

	// Get students
	students, err := models.GetStudentsInClassGroup(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get students: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load students")
		return
	}

	// Get sessions count
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Printf("WARNING: Failed to get sessions: %v", err)
		sessions = []*models.ClassSession{}
	}

	type StudentResponse struct {
		LeadID      string            `json:"lead_id"`
		FullName    string            `json:"full_name"`
		Phone       string            `json:"phone"`
		MissedCount int               `json:"missed_count"`
		Attendance  map[string]string `json:"attendance"` // session_id -> status
	}

	type SessionResponse struct {
		ID            string `json:"id"`
		SessionNumber int32  `json:"session_number"`
		ScheduledDate string `json:"scheduled_date"`
		ScheduledTime string `json:"scheduled_time"`
		Status        string `json:"status"`
	}

	type ClassWorkspaceResponse struct {
		Class         map[string]interface{} `json:"class"`
		SessionsCount int                    `json:"sessionsCount"`
		TotalSessions int                    `json:"totalSessions"`
		Students      []StudentResponse      `json:"students"`
		Sessions      []SessionResponse      `json:"sessions"`
	}

	sessionList := make([]SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		st := ""
		if s.ScheduledTime.Valid {
			st = s.ScheduledTime.String
		}
		sessionList = append(sessionList, SessionResponse{
			ID:            s.ID.String(),
			SessionNumber: s.SessionNumber,
			ScheduledDate: s.ScheduledDate.Format("2006-01-02"),
			ScheduledTime: st,
			Status:        s.Status,
		})
	}

	studentList := make([]StudentResponse, 0, len(students))
	for _, s := range students {
		swa := StudentResponse{
			LeadID:     s.LeadID.String(),
			FullName:   s.FullName,
			Phone:      s.Phone,
			Attendance: make(map[string]string),
		}

		// Get attendance for each session
		for _, session := range sessions {
			attendance, err := models.GetAttendanceForSession(session.ID)
			if err == nil {
				for _, att := range attendance {
					if att.LeadID == s.LeadID {
						swa.Attendance[session.ID.String()] = att.Status
						if att.Status == "ABSENT" {
							swa.MissedCount++
						}
						break
					}
				}
			}
		}
		studentList = append(studentList, swa)
		// Log calculated missed count for debugging
		log.Printf("GetClassWorkspace: Student %s (LeadID=%s) MissedCount=%d", swa.FullName, swa.LeadID, swa.MissedCount)
	}

	jsonResponse(w, http.StatusOK, ClassWorkspaceResponse{
		Class: map[string]interface{}{
			"class_key":    classGroup.ClassKey,
			"level":        classGroup.Level,
			"days":         classGroup.ClassDays,
			"time":         classGroup.ClassTime,
			"class_number": classGroup.ClassNumber,
			"round_status": classGroup.RoundStatus,
		},
		SessionsCount: len(sessions),
		TotalSessions: 8,
		Students:      studentList,
		Sessions:      sessionList,
	})
}

// MarkAttendance handles JSON POST to mark student attendance
func (h *APIHandler) MarkAttendance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		LeadID    string `json:"lead_id"`
		Status    string `json:"status"`
		Attended  *bool  `json:"attended"` // Legacy field, optional
		ClassKey  string `json:"class_key"`
		Notes     string `json:"notes"`
	}

	// Read body for logging
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	log.Printf("MarkAttendance RAW BODY: %s", string(bodyBytes))

	// Decode from the bytes we just read
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		log.Printf("MarkAttendance JSON DECODE ERROR: %v", err)
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Handle legacy "attended" boolean field
	// If status is empty but attended is provided, convert it
	if req.Status == "" && req.Attended != nil {
		if *req.Attended {
			req.Status = "PRESENT"
		} else {
			req.Status = "ABSENT"
		}
		log.Printf("MarkAttendance: Converted attended=%v to status=%s", *req.Attended, req.Status)
	}

	// Validate status
	if req.Status == "" {
		jsonError(w, http.StatusBadRequest, "Either 'status' or 'attended' field is required")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid session_id")
		return
	}

	leadID, err := uuid.Parse(req.LeadID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead_id")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	// Verify access
	if userRole == "mentor" {
		assignment, err := models.GetMentorAssignment(req.ClassKey)
		if err != nil || assignment == nil || assignment.MentorUserID != userID {
			jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
			return
		}
	} else if userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	// Log incoming payload for debugging
	log.Printf("MarkAttendance Payload: SessionID=%s, LeadID=%s, Status=%s, ClassKey=%s, UserID=%s",
		req.SessionID, req.LeadID, req.Status, req.ClassKey, userIDStr)

	if err := models.MarkAttendance(sessionID, leadID, req.Status, req.Notes, userID); err != nil {
		log.Printf("ERROR: Failed to mark attendance: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to mark attendance")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// CompleteSession handles JSON POST to mark a session as completed
func (h *APIHandler) CompleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		ClassKey  string `json:"class_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid session_id")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	// Verify access
	if userRole == "mentor" {
		assignment, err := models.GetMentorAssignment(req.ClassKey)
		if err != nil || assignment == nil || assignment.MentorUserID != userID {
			jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
			return
		}
	} else if userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	now := time.Now()
	if err := models.CompleteSession(sessionID, now, now.Format("15:04")); err != nil {
		log.Printf("ERROR: Failed to complete session: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to complete session")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/class?class_key=... - returns class details (kept for backward compatibility)
func (h *APIHandler) GetClass(w http.ResponseWriter, r *http.Request) {
	// Delegate to GetClassWorkspace for now
	h.GetClassWorkspace(w, r)
}

// GET /api/notes?student_id=...&class_key=... - returns notes for a student
func (h *APIHandler) GetNotes(w http.ResponseWriter, r *http.Request) {
	// Support both lead_id (legacy) and student_id (new)
	studentIDStr := r.URL.Query().Get("student_id")
	if studentIDStr == "" {
		studentIDStr = r.URL.Query().Get("lead_id") // Fallback for backward compatibility
	}
	classKeyRaw := r.URL.Query().Get("class_key")
	classKey, err := url.QueryUnescape(classKeyRaw)
	if err != nil {
		classKey = classKeyRaw
	}

	if studentIDStr == "" || classKey == "" {
		jsonError(w, http.StatusBadRequest, "student_id and class_key are required")
		return
	}

	leadID, err := uuid.Parse(studentIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid student_id")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)

	// Verify access: mentor can only access notes for classes they're assigned to
	if userRole == "mentor" {
		mentorUserID, err := uuid.Parse(userIDStr)
		if err == nil {
			assignment, err := models.GetMentorAssignment(classKey)
			if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
				jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
				return
			}
		}
	} else if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	notes, err := models.GetStudentNotes(leadID)
	if err != nil {
		log.Printf("ERROR: Failed to get notes: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load notes")
		return
	}

	type NoteResponse struct {
		ID             string `json:"id"`
		Text           string `json:"text"`
		CreatedAt      string `json:"created_at"`
		CreatedByEmail string `json:"created_by_email"`
	}

	response := make([]NoteResponse, 0, len(notes))
	for _, n := range notes {
		email := "System"
		if n.CreatedByEmail.Valid {
			email = n.CreatedByEmail.String
		}
		response = append(response, NoteResponse{
			ID:             n.ID.String(),
			Text:           n.NoteText,
			CreatedAt:      n.CreatedAt.Format(time.RFC3339),
			CreatedByEmail: email,
		})
	}

	jsonResponse(w, http.StatusOK, response)
}

// POST /api/notes - creates a new note
func (h *APIHandler) CreateNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		StudentID string `json:"student_id"`
		LeadID    string `json:"lead_id"` // Legacy support
		ClassKey  string `json:"class_key"`
		Text      string `json:"text"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Support both student_id (new) and lead_id (legacy)
	studentIDStr := req.StudentID
	if studentIDStr == "" {
		studentIDStr = req.LeadID
	}

	if studentIDStr == "" || req.ClassKey == "" || req.Text == "" {
		jsonError(w, http.StatusBadRequest, "student_id, class_key, and text are required")
		return
	}

	leadID, err := uuid.Parse(studentIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid student_id")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)

	// Verify access
	if userRole == "mentor" {
		mentorUserID, err := uuid.Parse(userIDStr)
		if err == nil {
			assignment, err := models.GetMentorAssignment(req.ClassKey)
			if err != nil || assignment == nil || assignment.MentorUserID != mentorUserID {
				jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
				return
			}
		}
	} else if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	createdByUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Create note (session_number is optional, can be null)
	var sessionNumber sql.NullInt32
	if err := models.AddStudentNote(leadID, req.ClassKey, sessionNumber, req.Text, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to add note: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to create note")
		return
	}

	// Fetch the created note to return it
	notes, err := models.GetStudentNotes(leadID)
	if err != nil || len(notes) == 0 {
		jsonError(w, http.StatusInternalServerError, "Note created but failed to retrieve")
		return
	}

	// Return the most recent note (first in DESC order)
	latestNote := notes[0]
	email := "System"
	if latestNote.CreatedByEmail.Valid {
		email = latestNote.CreatedByEmail.String
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":               latestNote.ID.String(),
		"text":             latestNote.NoteText,
		"created_at":       latestNote.CreatedAt.Format(time.RFC3339),
		"created_by_email": email,
	})
}

// DELETE /api/notes?id=... - deletes a note
func (h *APIHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	noteIDStr := r.URL.Query().Get("id")
	if noteIDStr == "" {
		jsonError(w, http.StatusBadRequest, "id is required")
		return
	}

	noteID, err := uuid.Parse(noteIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid note id")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)

	// Get note to check permissions
	note, err := models.GetStudentNoteByID(noteID)
	if err != nil {
		log.Printf("ERROR: Failed to get note: %v", err)
		jsonError(w, http.StatusNotFound, "Note not found")
		return
	}

	// Check permissions: mentor can only delete own notes, mentor_head/admin can delete any
	if userRole == "mentor" {
		if !note.CreatedByUserID.Valid || note.CreatedByUserID.String != userIDStr {
			jsonError(w, http.StatusForbidden, "Forbidden: You can only delete your own notes")
			return
		}
	} else if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	if err := models.DeleteStudentNote(noteID); err != nil {
		log.Printf("ERROR: Failed to delete note: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to delete note")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/student?student_id=... - returns student profile for ID card
func (h *APIHandler) GetStudent(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	studentIDStr := r.URL.Query().Get("student_id")
	if studentIDStr == "" {
		jsonError(w, http.StatusBadRequest, "student_id is required")
		return
	}

	studentID, err := uuid.Parse(studentIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid student_id")
		return
	}

	// Get lead/student info directly (we only need basic fields + levels)
	lead := &models.Lead{}
	var levelsPurchasedTotal, levelsConsumed sql.NullInt32
	err = db.DB.QueryRow(`
		SELECT id, full_name, phone, levels_purchased_total, levels_consumed
		FROM leads WHERE id = $1
	`, studentID).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &levelsPurchasedTotal, &levelsConsumed,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			jsonError(w, http.StatusNotFound, "Student not found")
			return
		}
		log.Printf("ERROR: Failed to get lead: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load student")
		return
	}

	// Calculate levels finished and left
	// Return safe defaults if data not available
	levelsFinished := int32(0)
	levelsLeft := int32(0)
	lastLevelGrade := ""

	if levelsConsumed.Valid {
		levelsFinished = levelsConsumed.Int32
	}
	if levelsPurchasedTotal.Valid {
		total := levelsPurchasedTotal.Int32
		if total > levelsFinished {
			levelsLeft = total - levelsFinished
		}
	}

	// Get last level grade (from current class if available)
	classKey := r.URL.Query().Get("class_key")
	if classKey != "" {
		grade, err := models.GetGrade(studentID, classKey)
		if err == nil && grade != nil {
			lastLevelGrade = grade.Grade
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":             studentID.String(),
		"name":           lead.FullName,
		"phone":          lead.Phone,
		"levelsFinished": levelsFinished,
		"levelsLeft":     levelsLeft,
		"lastLevelGrade": lastLevelGrade,
	})
}

// GET /api/mentor-head/dashboard - returns dashboard data (classes + mentors)
func (h *APIHandler) GetMentorHeadDashboard(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	// Get all classes where sent_to_mentor = true
	classes, err := models.GetClassGroupsSentToMentor()
	if err != nil {
		log.Printf("ERROR: Failed to get classes: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load classes")
		return
	}

	// Get mentor assignments and users for each class (same logic as SSR Dashboard)
	type ClassResponse struct {
		ClassKey     string  `json:"class_key"`
		Level        int32   `json:"level"`
		Days         string  `json:"days"`
		Time         string  `json:"time"`
		ClassNumber  int32   `json:"class_number"`
		StudentCount int     `json:"student_count"`
		Readiness    string  `json:"readiness"`
		MentorUserID *string `json:"mentor_user_id,omitempty"`
		MentorEmail  string  `json:"mentor_email,omitempty"`
		SentToMentor bool    `json:"sent_to_mentor"`
	}

	classesResponse := make([]ClassResponse, 0, len(classes))
	for _, c := range classes {
		cr := ClassResponse{
			ClassKey:     c.ClassKey,
			Level:        c.Level,
			Days:         c.ClassDays,
			Time:         c.ClassTime,
			ClassNumber:  c.ClassNumber,
			SentToMentor: c.SentToMentor,
		}

		// Get mentor assignment
		assignment, err := models.GetMentorAssignment(c.ClassKey)
		if err == nil && assignment != nil {
			mentorIDStr := assignment.MentorUserID.String()
			cr.MentorUserID = &mentorIDStr
			// Get mentor email
			user, err := models.GetUserByID(mentorIDStr)
			if err == nil && user != nil {
				cr.MentorEmail = user.Email
			}
		}

		// Get student count and readiness
		students, err := models.GetStudentsInClassGroup(c.ClassKey)
		if err == nil {
			cr.StudentCount = len(students)
			if cr.StudentCount >= 6 {
				cr.Readiness = "LOCKED"
			} else if cr.StudentCount >= 4 {
				cr.Readiness = "READY"
			} else {
				cr.Readiness = "NOT READY"
			}
		}

		classesResponse = append(classesResponse, cr)
	}

	// Get all mentors (users with role='mentor')
	mentors, err := models.GetUsersByRole("mentor")
	if err != nil {
		log.Printf("WARNING: Failed to get mentors: %v", err)
		mentors = []*models.User{}
	}

	type MentorResponse struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}

	mentorsResponse := make([]MentorResponse, 0, len(mentors))
	for _, m := range mentors {
		mentorsResponse = append(mentorsResponse, MentorResponse{
			ID:    m.ID.String(),
			Email: m.Email,
		})
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"classes": classesResponse,
		"mentors": mentorsResponse,
	})
}

// POST /api/mentor-head/assign-mentor - assigns a mentor to a class
func (h *APIHandler) AssignMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	var req struct {
		ClassKey     string `json:"class_key"`
		MentorEmail  string `json:"mentor_email"`
		MentorUserID string `json:"mentor_user_id"` // Legacy support
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	// Support both mentor_email (new) and mentor_user_id (legacy)
	var mentorUserID uuid.UUID
	var err error
	if req.MentorEmail != "" {
		// Look up mentor by email
		user, err := models.GetUserByEmail(req.MentorEmail)
		if err != nil || user == nil || user.Role != "mentor" {
			jsonError(w, http.StatusBadRequest, "Invalid mentor email")
			return
		}
		mentorUserID = user.ID
	} else if req.MentorUserID != "" {
		mentorUserID, err = uuid.Parse(req.MentorUserID)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid mentor_user_id")
			return
		}
		// Verify user has role='mentor'
		user, err := models.GetUserByID(req.MentorUserID)
		if err != nil || user == nil || user.Role != "mentor" {
			jsonError(w, http.StatusBadRequest, "Invalid mentor user")
			return
		}
	} else {
		jsonError(w, http.StatusBadRequest, "mentor_email is required")
		return
	}

	// Get class (days + time) for double-book check
	classGroup, err := models.GetClassGroupByKey(req.ClassKey)
	if err != nil || classGroup == nil {
		jsonError(w, http.StatusNotFound, "Class not found")
		return
	}

	// Double-book check: same days_pattern + start_time
	hasDoubleBook, conflictDays, conflictTime, err := models.CheckMentorDoubleBookByDaysTime(
		mentorUserID, req.ClassKey, classGroup.ClassDays, classGroup.ClassTime)
	if err != nil {
		log.Printf("ERROR: Failed to check double-book: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to check conflicts")
		return
	}
	if hasDoubleBook {
		jsonError(w, http.StatusConflict, fmt.Sprintf("Mentor already assigned to another class at %s %s.", conflictDays, conflictTime))
		return
	}

	// Get class sessions to check for conflicts
	sessions, err := models.GetClassSessions(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to get sessions: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to check schedule conflicts")
		return
	}

	// Check for conflicts with each session (same logic as SSR AssignMentor)
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
			jsonError(w, http.StatusInternalServerError, "Failed to check schedule conflicts")
			return
		}
		if hasConflict {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("Mentor has conflicting session on %s at %s", session.ScheduledDate.Format("2006-01-02"), session.ScheduledTime.String))
			return
		}
	}

	// Assign mentor
	createdByUserID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.AssignMentorToClass(req.ClassKey, mentorUserID, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to assign mentor: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to assign mentor")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/mentor-head/return-to-ops - returns a class to Operations
func (h *APIHandler) ReturnToOps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	var req struct {
		ClassKey string `json:"class_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	if err := models.ReturnClassGroupFromMentor(req.ClassKey); err != nil {
		log.Printf("ERROR: Failed to return class: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to return class")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/mentor-head/unassign - removes mentor assignment from a class (body: { class_key })
func (h *APIHandler) UnassignMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head access required")
		return
	}

	var req struct {
		ClassKey string `json:"class_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	// Find class
	classGroup, err := models.GetClassGroupByKey(req.ClassKey)
	if err != nil || classGroup == nil {
		jsonError(w, http.StatusNotFound, "Class not found")
		return
	}

	// Block if sessions exist (round started)
	sessions, err := models.GetClassSessions(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to get sessions: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to check sessions")
		return
	}
	if len(sessions) > 0 {
		jsonError(w, http.StatusBadRequest, "Cannot unassign: round already started (sessions exist).")
		return
	}

	if err := models.UnassignMentorFromClass(req.ClassKey); err != nil {
		log.Printf("ERROR: Failed to unassign mentor: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to unassign mentor")
		return
	}

	// Return updated class (no mentor)
	students, _ := models.GetStudentsInClassGroup(req.ClassKey)
	readiness := "NOT READY"
	if len(students) >= 6 {
		readiness = "LOCKED"
	} else if len(students) >= 4 {
		readiness = "READY"
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"class_key":      classGroup.ClassKey,
		"level":          classGroup.Level,
		"days":           classGroup.ClassDays,
		"time":           classGroup.ClassTime,
		"class_number":   classGroup.ClassNumber,
		"student_count":  len(students),
		"readiness":      readiness,
		"mentor_user_id": nil,
		"mentor_email":   "",
	})
}

// POST /api/mentor-head/return-class - legacy alias for return-to-ops
func (h *APIHandler) ReturnClass(w http.ResponseWriter, r *http.Request) {
	h.ReturnToOps(w, r)
}

// POST /api/mentor-head/start-round - starts a round for a class
func (h *APIHandler) StartRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	var req struct {
		ClassKey string `json:"class_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	// Get class info to get start date/time
	classGroup, err := models.GetClassGroupByKey(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to get class group: %v", err)
		jsonError(w, http.StatusNotFound, "Class not found")
		return
	}

	// Check if sessions already exist
	sessions, err := models.GetClassSessions(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to check sessions: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to check sessions")
		return
	}
	if len(sessions) > 0 {
		jsonError(w, http.StatusBadRequest, "Round already started")
		return
	}

	// Use today as start date and class time from class_groups
	startDate := time.Now()
	startTime := classGroup.ClassTime

	// Start round (set status='active' + create 8 sessions)
	startedByID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.StartClassRound(req.ClassKey, startedByID, startDate, startTime); err != nil {
		log.Printf("ERROR: Failed to start round: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to start round")
		return
	}

	// Return updated class summary
	updated, _ := models.GetClassGroupByKey(req.ClassKey)
	students, _ := models.GetStudentsInClassGroup(req.ClassKey)
	readiness := "NOT READY"
	if len(students) >= 6 {
		readiness = "LOCKED"
	} else if len(students) >= 4 {
		readiness = "READY"
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"class_key":     updated.ClassKey,
		"level":         updated.Level,
		"days":          updated.ClassDays,
		"time":          updated.ClassTime,
		"class_number":  updated.ClassNumber,
		"round_status":  updated.RoundStatus,
		"student_count": len(students),
		"readiness":     readiness,
	})
}

// POST /api/mentor-head/close-round - closes a round
func (h *APIHandler) CloseRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	var req struct {
		ClassKey string `json:"class_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	closedByID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.CloseRound(req.ClassKey, closedByID); err != nil {
		log.Printf("ERROR: Failed to close round: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to close round")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/mentor-head/evaluations - returns mentors assigned to classes with KPI data
func (h *APIHandler) GetMentorEvaluations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head access required")
		return
	}

	// Get all mentors assigned to classes
	assignedMentors, err := models.GetAssignedMentors()
	if err != nil {
		log.Printf("ERROR: Failed to get assigned mentors: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load evaluations")
		return
	}

	type MentorKPIResponse struct {
		ID                 string `json:"id"`
		Email              string `json:"email"`
		Name               string `json:"name"`
		AssignedClassCount int    `json:"assignedClassCount"`
		KPIs               struct {
			SessionQuality     int `json:"sessionQuality"`
			TrelloCompliance   int `json:"trelloCompliance"`
			WhatsappManagement int `json:"whatsappManagement"`
			StudentsFeedback   int `json:"studentsFeedback"`
		} `json:"kpis"`
		Attendance struct {
			SessionsTotal int      `json:"sessionsTotal"`
			Statuses      []string `json:"statuses"`
			OnTimePercent int      `json:"onTimePercent"`
		} `json:"attendance"`
	}

	mentorsResponse := make([]MentorKPIResponse, 0, len(assignedMentors))
	for _, am := range assignedMentors {
		mentor := MentorKPIResponse{
			ID:                 am.User.ID.String(),
			Email:              am.User.Email,
			Name:               am.User.Email, // Use email as name (no name field in User model)
			AssignedClassCount: am.AssignedClassCount,
		}

		// Use evaluation data if exists, otherwise defaults
		if am.Evaluation != nil {
			mentor.KPIs.SessionQuality = am.Evaluation.KPISessionQuality
			mentor.KPIs.TrelloCompliance = am.Evaluation.KPITrello
			mentor.KPIs.WhatsappManagement = am.Evaluation.KPIWhatsapp
			mentor.KPIs.StudentsFeedback = am.Evaluation.KPIStudentsFeedback
			mentor.Attendance.Statuses = am.Evaluation.AttendanceStatuses
		} else {
			// Defaults
			mentor.KPIs.SessionQuality = 0
			mentor.KPIs.TrelloCompliance = 0
			mentor.KPIs.WhatsappManagement = 0
			mentor.KPIs.StudentsFeedback = 0
			mentor.Attendance.Statuses = []string{"unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown", "unknown"}
		}

		// Compute on-time percent from attendance statuses
		mentor.Attendance.SessionsTotal = 8
		onTimeCount := 0
		for _, status := range mentor.Attendance.Statuses {
			if status == "on-time" {
				onTimeCount++
			}
		}
		mentor.Attendance.OnTimePercent = (onTimeCount * 100) / 8

		mentorsResponse = append(mentorsResponse, mentor)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"mentors": mentorsResponse,
	})
}

// PUT /api/mentor-head/evaluations/:mentorId - updates mentor evaluation
func (h *APIHandler) UpdateMentorEvaluation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head access required")
		return
	}

	// Extract mentor ID from URL path
	path := r.URL.Path
	prefix := "/api/mentor-head/evaluations/"
	if !strings.HasPrefix(path, prefix) {
		jsonError(w, http.StatusBadRequest, "Invalid URL format")
		return
	}
	mentorIDStr := path[len(prefix):]
	mentorID, err := uuid.Parse(mentorIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid mentor ID")
		return
	}

	// Get evaluator ID (current user)
	evaluatorIDStr := middleware.GetUserID(r)
	if evaluatorIDStr == "" {
		jsonError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}
	evaluatorID, err := uuid.Parse(evaluatorIDStr)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Invalid evaluator ID")
		return
	}

	// Parse request body
	var req struct {
		KPIs struct {
			SessionQuality     int `json:"sessionQuality"`
			TrelloCompliance   int `json:"trelloCompliance"`
			WhatsappManagement int `json:"whatsappManagement"`
			StudentsFeedback   int `json:"studentsFeedback"`
		} `json:"kpis"`
		Attendance struct {
			Statuses []string `json:"statuses"`
		} `json:"attendance"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate attendance statuses
	if len(req.Attendance.Statuses) != 8 {
		jsonError(w, http.StatusBadRequest, "attendance.statuses must have exactly 8 elements")
		return
	}

	// Upsert evaluation
	if err := models.UpsertMentorEvaluation(
		mentorID,
		evaluatorID,
		req.KPIs.SessionQuality,
		req.KPIs.TrelloCompliance,
		req.KPIs.WhatsappManagement,
		req.KPIs.StudentsFeedback,
		req.Attendance.Statuses,
	); err != nil {
		if strings.Contains(err.Error(), "must be between") || strings.Contains(err.Error(), "invalid attendance status") {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("ERROR: Failed to upsert mentor evaluation: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to save evaluation")
		return
	}

	// Return updated evaluation
	onTimeCount := 0
	for _, status := range req.Attendance.Statuses {
		if status == "on-time" {
			onTimeCount++
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id": mentorID.String(),
		"kpis": map[string]interface{}{
			"sessionQuality":     req.KPIs.SessionQuality,
			"trelloCompliance":   req.KPIs.TrelloCompliance,
			"whatsappManagement": req.KPIs.WhatsappManagement,
			"studentsFeedback":   req.KPIs.StudentsFeedback,
		},
		"attendance": map[string]interface{}{
			"sessionsTotal": 8,
			"statuses":      req.Attendance.Statuses,
			"onTimePercent": (onTimeCount * 100) / 8,
		},
	})
}

// GET /api/student-success/classes - returns active classes only (round_status='active')
func (h *APIHandler) GetStudentSuccessClasses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if middleware.GetUserRole(r) != "student_success" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success access required")
		return
	}

	rows, err := models.GetActiveClassesForStudentSuccess()
	if err != nil {
		log.Printf("ERROR: Failed to get active classes: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load classes")
		return
	}

	type ClassResp struct {
		ClassKey     string `json:"class_key"`
		Level        int32  `json:"level"`
		Days         string `json:"days"`
		Time         string `json:"time"`
		ClassNumber  int32  `json:"class_number"`
		MentorEmail  string `json:"mentor_email"`
		MentorName   string `json:"mentor_name"`
		MentorUserID string `json:"mentor_user_id,omitempty"`
		StudentCount int    `json:"student_count"`
	}
	classes := make([]ClassResp, 0, len(rows))
	for _, row := range rows {
		classes = append(classes, ClassResp{
			ClassKey:     row.ClassKey,
			Level:        row.Level,
			Days:         row.ClassDays,
			Time:         row.ClassTime,
			ClassNumber:  row.ClassNumber,
			MentorEmail:  row.MentorEmail,
			MentorName:   row.MentorName,
			MentorUserID: row.MentorUserID,
			StudentCount: row.StudentCount,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"classes": classes})
}

// GET /api/student-success/class?class_key=... - returns class details + students + sessions + attendance
func (h *APIHandler) GetStudentSuccessClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if middleware.GetUserRole(r) != "student_success" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success access required")
		return
	}

	classKeyRaw := r.URL.Query().Get("class_key")
	classKey, err := url.QueryUnescape(classKeyRaw)
	if err != nil {
		classKey = classKeyRaw
	}
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	cg, students, sessions, missedSessions, feedbackRecords, completedCount, err := models.GetStudentSuccessClassDetail(classKey)
	if err != nil {
		if strings.Contains(err.Error(), "not active") {
			jsonError(w, http.StatusBadRequest, "Class is not active")
			return
		}
		log.Printf("ERROR: Failed to get class detail: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load class")
		return
	}
	if cg == nil {
		jsonError(w, http.StatusNotFound, "Class not found")
		return
	}

	type StudentResp struct {
		LeadID         string  `json:"lead_id"`
		FullName       string  `json:"full_name"`
		Phone          string  `json:"phone"`
		MissedCount    int     `json:"missed_count"`
		MissedSessions []int32 `json:"missed_sessions"`
	}
	studentList := make([]StudentResp, 0, len(students))
	for _, s := range students {
		ms := []int32{}
		if sessionsList, ok := missedSessions[s.LeadID]; ok {
			ms = sessionsList
		}
		studentList = append(studentList, StudentResp{
			LeadID:         s.LeadID.String(),
			FullName:       s.FullName,
			Phone:          s.Phone,
			MissedCount:    len(ms),
			MissedSessions: ms,
		})
	}

	type SessionResp struct {
		ID            string `json:"id"`
		SessionNumber int32  `json:"session_number"`
		ScheduledDate string `json:"scheduled_date"`
		ScheduledTime string `json:"scheduled_time"`
		ScheduledEnd  string `json:"scheduled_end_time"`
		Status        string `json:"status"`
	}
	sessionList := make([]SessionResp, 0, len(sessions))
	for _, s := range sessions {
		st, se := "", ""
		if s.ScheduledTime.Valid {
			st = s.ScheduledTime.String
		}
		if s.ScheduledEndTime.Valid {
			se = s.ScheduledEndTime.String
		}
		sessionList = append(sessionList, SessionResp{
			ID:            s.ID.String(),
			SessionNumber: s.SessionNumber,
			ScheduledDate: s.ScheduledDate.Format("2006-01-02"),
			ScheduledTime: st,
			ScheduledEnd:  se,
			Status:        s.Status,
		})
	}

	// Feedback Checkpoints
	type FeedbackEntry struct {
		SessionNumber    int32  `json:"session_number"`
		Status           string `json:"status"` // sent | missing
		FeedbackText     string `json:"feedback_text,omitempty"`
		FollowUpRequired bool   `json:"follow_up_required"`
	}
	type StudentFeedback struct {
		LeadID   string         `json:"lead_id"`
		FullName string         `json:"full_name"`
		S4       *FeedbackEntry `json:"s4,omitempty"`
		S8       *FeedbackEntry `json:"s8,omitempty"`
	}

	feedbackMap := make(map[string]*StudentFeedback)
	for _, s := range students {
		feedbackMap[s.LeadID.String()] = &StudentFeedback{
			LeadID:   s.LeadID.String(),
			FullName: s.FullName,
		}
	}

	for _, f := range feedbackRecords {
		sf, ok := feedbackMap[f.LeadID.String()]
		if !ok {
			continue
		}
		entry := &FeedbackEntry{
			SessionNumber:    f.SessionNumber,
			Status:           "sent",
			FeedbackText:     f.FeedbackText,
			FollowUpRequired: f.FollowUpRequired,
		}
		if f.SessionNumber == 4 {
			sf.S4 = entry
		} else if f.SessionNumber == 8 {
			sf.S8 = entry
		}
	}

	feedbackList := make([]*StudentFeedback, 0, len(students))
	for _, s := range students {
		feedbackList = append(feedbackList, feedbackMap[s.LeadID.String()])
	}

	allS4 := true
	allS8 := true
	for _, sf := range feedbackList {
		if sf.S4 == nil {
			allS4 = false
		}
		if sf.S8 == nil {
			allS8 = false
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"class": map[string]interface{}{
			"class_key":    cg.ClassKey,
			"level":        cg.Level,
			"days":         cg.ClassDays,
			"time":         cg.ClassTime,
			"class_number": cg.ClassNumber,
			"round_status": cg.RoundStatus,
		},
		"students":               studentList,
		"sessions":               sessionList,
		"sessionsCount":          len(sessions),
		"completedSessionsCount": completedCount,
		"totalSessions":          8,
		"feedback":               feedbackList,
		"milestones": map[string]interface{}{
			"midRound": map[string]interface{}{
				"reached":  completedCount >= 4,
				"complete": allS4,
			},
			"endRound": map[string]interface{}{
				"reached":  completedCount >= 8,
				"complete": allS8,
			},
		},
	})
}

// GET /api/student-success/class/absence-feed?class_key=...
func (h *APIHandler) GetAbsenceFeed(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	classKeyRaw := r.URL.Query().Get("class_key")
	classKey, err := url.QueryUnescape(classKeyRaw)
	if err != nil {
		classKey = classKeyRaw
	}
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	filter := r.URL.Query().Get("filter")
	search := r.URL.Query().Get("search")

	feed, err := models.GetAbsenceFeed(classKey, filter, search)
	if err != nil {
		log.Printf("ERROR: Failed to get absence feed: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load absence feed")
		return
	}

	jsonResponse(w, http.StatusOK, feed)
}

// POST /api/student-success/followups
func (h *APIHandler) CreateFollowUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	var req struct {
		ClassKey      string `json:"class_key"`
		LeadID        string `json:"lead_id"`
		SessionNumber int    `json:"session_number"`
		Note          string `json:"note"`
		Status        string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	leadID, err := uuid.Parse(req.LeadID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead_id")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	if err := models.CreateFollowUp(req.ClassKey, leadID, req.SessionNumber, req.Note, req.Status, userID); err != nil {
		log.Printf("ERROR: Failed to create follow-up: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to save follow-up")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// PATCH /api/student-success/followups/:id (using query param or just simple PATCH)
// Let's use POST or PATCH /api/student-success/followups/resolve
// POST /api/absence-cases/:id/follow-up
func (h *APIHandler) PostFollowUpUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		jsonError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	idStr := pathParts[2]
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid id")
		return
	}

	var req struct {
		Status   string `json:"status"`
		Note     string `json:"note"`
		Resolved bool   `json:"resolved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	if err := models.UpdateFollowUp(id, req.Status, req.Note, req.Resolved, userID); err != nil {
		log.Printf("ERROR: Failed to update follow-up: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to update follow-up")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/absence-cases/:id/resolve
func (h *APIHandler) ResolveFollowUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		jsonError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	idStr := pathParts[2]
	id, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid id")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	if err := models.ResolveFollowUp(id, userID); err != nil {
		log.Printf("ERROR: Failed to resolve follow-up: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to resolve follow-up")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListClassSessions handles GET /api/classes/:id/sessions
func (h *APIHandler) ListClassSessions(w http.ResponseWriter, r *http.Request) {
	// Parse /api/classes/{classKey}/sessions
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/classes/") || !strings.HasSuffix(path, "/sessions") {
		jsonError(w, http.StatusBadRequest, "Invalid path")
		return
	}
	classKey := strings.TrimPrefix(path, "/api/classes/")
	classKey = strings.TrimSuffix(classKey, "/sessions")

	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get sessions for %s: %v", classKey, err)
		jsonError(w, http.StatusNotFound, "Class not found or no sessions")
		return
	}

	jsonResponse(w, http.StatusOK, sessions)
}

// CompleteSessionByNumber handles POST /api/classes/:id/sessions/:n/complete
func (h *APIHandler) CompleteSessionByNumber(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/classes/") || !strings.HasSuffix(path, "/complete") {
		jsonError(w, http.StatusBadRequest, "Invalid path")
		return
	}

	// Format: /api/classes/{classKey}/sessions/{n}/complete
	trimmed := strings.TrimPrefix(path, "/api/classes/")
	trimmed = strings.TrimSuffix(trimmed, "/complete")
	parts := strings.Split(trimmed, "/sessions/")
	if len(parts) != 2 {
		jsonError(w, http.StatusBadRequest, "Invalid path format")
		return
	}
	classKey := parts[0]
	sessionNumStr := parts[1]

	sessionNum, err := strconv.Atoi(sessionNumStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid session number")
		return
	}

	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Class not found")
		return
	}

	var targetSession *models.ClassSession
	for _, s := range sessions {
		if s.SessionNumber == int32(sessionNum) {
			targetSession = s
			break
		}
	}

	if targetSession == nil {
		jsonError(w, http.StatusNotFound, "Session not found")
		return
	}

	// Re-use CompleteSession logic
	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	if userRole == "mentor" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != userID {
			jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
			return
		}
	} else if userRole != "mentor_head" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	now := time.Now()
	if err := models.CompleteSession(targetSession.ID, now, now.Format("15:04")); err != nil {
		log.Printf("ERROR: Failed to complete session %v: %v", targetSession.ID, err)
		jsonError(w, http.StatusInternalServerError, "Failed to complete session")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/student-success/feedback - AJAX feedback submission
func (h *APIHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success or Admin access required")
		return
	}

	var req struct {
		LeadID           string `json:"lead_id"`
		ClassKey         string `json:"class_key"`
		SessionNumber    int32  `json:"session_number"`
		FeedbackText     string `json:"feedback_text"`
		FollowUpRequired bool   `json:"follow_up_required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	leadID, err := uuid.Parse(req.LeadID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead_id")
		return
	}

	if req.SessionNumber != 4 && req.SessionNumber != 8 {
		jsonError(w, http.StatusBadRequest, "Invalid session_number. Must be 4 or 8")
		return
	}

	userID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.SubmitFeedback(leadID, req.ClassKey, req.SessionNumber, req.FeedbackText, req.FollowUpRequired, userID); err != nil {
		log.Printf("ERROR: Failed to submit feedback: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to submit feedback")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "success"})
}
