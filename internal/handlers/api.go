package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

func userDisplayName(user *models.User) string {
	if user == nil {
		return ""
	}
	if user.FullName.Valid && strings.TrimSpace(user.FullName.String) != "" {
		return strings.TrimSpace(user.FullName.String)
	}
	return user.Email
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

	// Prefer stored full name, fallback to email.
	userName := userEmail
	user, err := models.GetUserByID(userID)
	if err == nil && user != nil {
		if name := userDisplayName(user); name != "" {
			userName = name
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":                   userID,
		"email":                userEmail,
		"name":                 userName,
		"role":                 userRole,
		"must_change_password": user != nil && user.MustChangePassword,
	})
}

// GET /api/manager/users - list users for manager
func (h *APIHandler) GetManagerUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	users, err := models.GetAllUsers()
	if err != nil {
		log.Printf("ERROR: failed to list users: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load users")
		return
	}

	type userRow struct {
		ID                 string `json:"id"`
		FullName           string `json:"full_name"`
		Email              string `json:"email"`
		Role               string `json:"role"`
		IsActive           bool   `json:"is_active"`
		MustChangePassword bool   `json:"must_change_password"`
		CreatedAt          string `json:"created_at"`
	}
	rows := make([]userRow, 0, len(users))
	for _, u := range users {
		name := strings.TrimSpace(u.FullName.String)
		if name == "" {
			name = u.Email
		}
		rows = append(rows, userRow{
			ID:                 u.ID.String(),
			FullName:           name,
			Email:              u.Email,
			Role:               u.Role,
			IsActive:           u.IsActive,
			MustChangePassword: u.MustChangePassword,
			CreatedAt:          u.CreatedAt.Format(time.RFC3339),
		})
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"users": rows})
}

// POST /api/manager/users - create user with temporary password
func (h *APIHandler) CreateManagerUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		FullName          string `json:"full_name"`
		Email             string `json:"email"`
		Role              string `json:"role"`
		TemporaryPassword string `json:"temporary_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.FullName = strings.TrimSpace(req.FullName)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Role = strings.TrimSpace(req.Role)
	req.TemporaryPassword = strings.TrimSpace(req.TemporaryPassword)

	allowedRoles := map[string]bool{
		"admin":           true,
		"mentor_head":     true,
		"mentor":          true,
		"hr":              true,
		"student_success": true,
		"moderator":       true,
	}

	if req.FullName == "" || req.Email == "" || req.Role == "" || req.TemporaryPassword == "" {
		jsonError(w, http.StatusBadRequest, "full_name, email, role, and temporary_password are required")
		return
	}
	if !allowedRoles[req.Role] {
		jsonError(w, http.StatusBadRequest, "Invalid role")
		return
	}

	if _, err := models.GetUserByEmail(req.Email); err == nil {
		jsonError(w, http.StatusConflict, "Email already exists")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.TemporaryPassword), bcrypt.DefaultCost)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	user, err := models.CreateUserWithMustChange(req.Email, string(hash), req.Role, req.FullName, "", true)
	if err != nil {
		log.Printf("ERROR: failed to create manager user: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":                   user.ID.String(),
		"email":                user.Email,
		"full_name":            req.FullName,
		"role":                 user.Role,
		"must_change_password": true,
	})
}

// DELETE /api/manager/users/:id - remove a user (manager only)
func (h *APIHandler) DeleteManagerUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentUserID := middleware.GetUserID(r)
	const routePrefix = "/api/manager/users/"
	if !strings.HasPrefix(r.URL.Path, routePrefix) {
		jsonError(w, http.StatusBadRequest, "Invalid user delete route")
		return
	}
	targetUserID := strings.Trim(strings.TrimPrefix(r.URL.Path, routePrefix), "/")
	if targetUserID == "" {
		jsonError(w, http.StatusBadRequest, "User id is required")
		return
	}
	if strings.Contains(targetUserID, "/") {
		jsonError(w, http.StatusBadRequest, "Invalid user id")
		return
	}
	if targetUserID == currentUserID {
		jsonError(w, http.StatusBadRequest, "Manager cannot remove their own account")
		return
	}

	if err := models.DeleteUserByID(targetUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, http.StatusNotFound, "User not found")
			return
		}
		// Foreign-key / dependency safety: prevent deletion of users linked to existing records.
		if strings.Contains(strings.ToLower(err.Error()), "violates foreign key constraint") {
			jsonError(w, http.StatusConflict, "User cannot be removed because they are linked to existing records")
			return
		}
		log.Printf("ERROR: failed to delete user %s: %v", targetUserID, err)
		jsonError(w, http.StatusInternalServerError, "Failed to remove user")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/manager/users/:id/deactivate - deactivate a user (manager only)
func (h *APIHandler) DeactivateManagerUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentUserID := middleware.GetUserID(r)
	const routePrefix = "/api/manager/users/"
	if !strings.HasPrefix(r.URL.Path, routePrefix) || !strings.HasSuffix(r.URL.Path, "/deactivate") {
		jsonError(w, http.StatusBadRequest, "Invalid user deactivate route")
		return
	}

	targetPath := strings.Trim(strings.TrimPrefix(r.URL.Path, routePrefix), "/")
	targetPath = strings.TrimSuffix(targetPath, "/deactivate")
	targetUserID := strings.Trim(targetPath, "/")
	if targetUserID == "" || strings.Contains(targetUserID, "/") {
		jsonError(w, http.StatusBadRequest, "Invalid user id")
		return
	}
	if targetUserID == currentUserID {
		jsonError(w, http.StatusBadRequest, "Manager cannot deactivate their own account")
		return
	}

	if err := models.DeactivateUserByID(targetUserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, http.StatusNotFound, "User not found")
			return
		}
		log.Printf("ERROR: failed to deactivate user %s: %v", targetUserID, err)
		jsonError(w, http.StatusInternalServerError, "Failed to deactivate user")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/mentor/classes - returns classes for current mentor
func (h *APIHandler) GetMentorClasses(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" && userRole != "manager" {
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
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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

// GET /api/mentors - mentor directory for mentor_head/admin
func (h *APIHandler) GetMentorDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	mentors, err := models.GetMentorDirectory()
	if err != nil {
		log.Printf("ERROR: Failed to load mentor directory: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load mentors")
		return
	}

	type MentorDirectoryRow struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		Email              string `json:"email"`
		Phone              string `json:"phone"`
		Status             string `json:"status"`
		TotalClassesTaught int    `json:"total_classes_taught"`
	}

	rows := make([]MentorDirectoryRow, 0, len(mentors))
	for _, m := range mentors {
		rows = append(rows, MentorDirectoryRow{
			ID:                 m.ID.String(),
			Name:               m.Name,
			Email:              m.Email,
			Phone:              m.Phone,
			Status:             m.Status,
			TotalClassesTaught: m.TotalClassesTaught,
		})
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"mentors": rows,
	})
}

// GET /api/mentors/:id/profile - mentor profile details for mentor_head/admin
func (h *APIHandler) GetMentorProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /api/mentors/:id/profile
	if len(parts) != 4 || parts[0] != "api" || parts[1] != "mentors" || parts[3] != "profile" {
		http.NotFound(w, r)
		return
	}

	mentorID, err := uuid.Parse(parts[2])
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid mentor ID")
		return
	}

	profile, err := models.GetMentorProfile(mentorID)
	if err != nil {
		log.Printf("ERROR: Failed to load mentor profile: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load mentor profile")
		return
	}
	if profile == nil {
		jsonError(w, http.StatusNotFound, "Mentor not found")
		return
	}

	formatTime := func(t sql.NullTime) interface{} {
		if !t.Valid {
			return nil
		}
		return t.Time.Format(time.RFC3339)
	}

	classHistory := make([]map[string]interface{}, 0, len(profile.ClassHistory))
	for _, ch := range profile.ClassHistory {
		classHistory = append(classHistory, map[string]interface{}{
			"class_key":        ch.ClassKey,
			"level":            ch.Level,
			"days":             ch.Days,
			"time":             ch.Time,
			"start_date":       formatTime(ch.StartDate),
			"end_date":         formatTime(ch.EndDate),
			"duration":         ch.Duration,
			"evaluation_score": ch.EvaluationScore,
			"compliance_score": ch.ComplianceScore,
		})
	}

	testimonials := make([]map[string]interface{}, 0, len(profile.Testimonials))
	for _, t := range profile.Testimonials {
		testimonials = append(testimonials, map[string]interface{}{
			"id":               t.ID.String(),
			"class_key":        t.ClassKey,
			"testimonial_text": t.TestimonialText,
			"created_by":       t.CreatedByEmail,
			"created_at":       t.CreatedAt.Format(time.RFC3339),
		})
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"mentor_details": map[string]interface{}{
			"id":     profile.MentorDetails.ID.String(),
			"name":   profile.MentorDetails.Name,
			"email":  profile.MentorDetails.Email,
			"phone":  profile.MentorDetails.Phone,
			"status": profile.MentorDetails.Status,
		},
		"stats": map[string]interface{}{
			"total_classes":    profile.Stats.TotalClasses,
			"first_class_date": formatTime(profile.Stats.FirstClassDate),
			"last_class_date":  formatTime(profile.Stats.LastClassDate),
			"avg_rating":       profile.Stats.AvgRating,
			"feedback_meter":   profile.Stats.FeedbackMeter,
			"compliance_score": profile.Stats.ComplianceScore,
		},
		"class_history": classHistory,
		"testimonials":  testimonials,
	})
}

// POST /api/mentor-head/mentors/:id/testimonials
func (h *APIHandler) CreateMentorTestimonial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /api/mentor-head/mentors/:id/testimonials
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "mentor-head" || parts[2] != "mentors" || parts[4] != "testimonials" {
		http.NotFound(w, r)
		return
	}

	mentorID, err := uuid.Parse(parts[3])
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid mentor ID")
		return
	}

	var req struct {
		ClassKey        string `json:"class_key"`
		TestimonialText string `json:"testimonial_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	createdByID, err := uuid.Parse(middleware.GetUserID(r))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	item, err := models.CreateMentorTestimonial(mentorID, req.ClassKey, req.TestimonialText, createdByID)
	if err != nil {
		log.Printf("ERROR: Failed to create mentor testimonial: %v", err)
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":               item.ID.String(),
		"class_key":        item.ClassKey,
		"testimonial_text": item.TestimonialText,
		"created_by":       item.CreatedByEmail,
		"created_at":       item.CreatedAt.Format(time.RFC3339),
	})
}

// GET /api/mentor-head/classes - returns classes grouped by mentor
func (h *APIHandler) GetMentorHeadClasses(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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
			students, _ := models.GetStudentsForMentorHeadClass(c.ClassKey)
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

		students, _ := models.GetStudentsForMentorHeadClass(c.ClassKey)
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
	} else if userRole != "mentor_head" && userRole != "admin" && userRole != "student_success" && userRole != "manager" {
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

	// Get students.
	// Pre-start visibility includes ready_to_start roster for sent/not_started classes.
	// Mentors get the same visibility for their assigned class, but actions remain locked until round is active.
	var students []*models.ClassStudent
	if userRole == "mentor_head" || userRole == "admin" || userRole == "mentor" {
		students, err = models.GetStudentsForMentorHeadClass(classKey)
	} else {
		students, err = models.GetStudentsInClassGroup(classKey)
	}
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
		LeadID                string                            `json:"lead_id"`
		FullName              string                            `json:"full_name"`
		Phone                 string                            `json:"phone"`
		MissedCount           int                               `json:"missed_count"`
		JoinedAtSessionNumber *int32                            `json:"joined_at_session_number,omitempty"`
		Attendance            map[string]string                 `json:"attendance"`          // session_id -> status
		SessionPerformance    map[string]map[string]interface{} `json:"session_performance"` // session_id -> {task_completed, participation_score}
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

	attendanceBySession, err := models.GetAttendanceByClassKey(classKey)
	if err != nil {
		log.Printf("WARNING: Failed to get attendance map for class %s: %v", classKey, err)
		attendanceBySession = make(map[uuid.UUID]map[uuid.UUID]*models.Attendance)
	}
	performanceBySession, err := models.GetSessionPerformanceByClassKey(classKey)
	if err != nil {
		log.Printf("WARNING: Failed to get session performance map for class %s: %v", classKey, err)
		performanceBySession = make(map[uuid.UUID]map[uuid.UUID]*models.SessionPerformance)
	}

	studentList := make([]StudentResponse, 0, len(students))
	for _, s := range students {
		swa := StudentResponse{
			LeadID:             s.LeadID.String(),
			FullName:           s.FullName,
			Phone:              s.Phone,
			Attendance:         make(map[string]string),
			SessionPerformance: make(map[string]map[string]interface{}),
		}
		if s.JoinedAtSessionNumber.Valid {
			joined := s.JoinedAtSessionNumber.Int32
			swa.JoinedAtSessionNumber = &joined
		}

		// Attach attendance + performance for each session
		for _, session := range sessions {
			if byLead, ok := attendanceBySession[session.ID]; ok {
				if att, ok := byLead[s.LeadID]; ok && att != nil {
					swa.Attendance[session.ID.String()] = att.Status
					if strings.EqualFold(att.Status, "ABSENT") {
						swa.MissedCount++
					}
				}
			}
			if byLead, ok := performanceBySession[session.ID]; ok {
				if perf, ok := byLead[s.LeadID]; ok && perf != nil {
					swa.SessionPerformance[session.ID.String()] = map[string]interface{}{
						"task_completed":      perf.TaskCompleted,
						"participation_score": perf.ParticipationScore,
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
		SessionID          string `json:"session_id"`
		LeadID             string `json:"lead_id"`
		Status             string `json:"status"`
		Attended           *bool  `json:"attended"` // Legacy field, optional
		ClassKey           string `json:"class_key"`
		Notes              string `json:"notes"`
		TaskCompleted      *bool  `json:"task_completed"`
		ParticipationScore *int   `json:"participation_score"`
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
		classGroup, err := models.GetClassGroupByKey(req.ClassKey)
		if err != nil {
			jsonError(w, http.StatusNotFound, "Class not found")
			return
		}
		if strings.TrimSpace(classGroup.RoundStatus) != "active" {
			jsonError(w, http.StatusBadRequest, "Round has not started yet. Attendance is locked for mentors until Mentor Head starts the round.")
			return
		}
	} else if userRole != "admin" && userRole != "student_success" && userRole != "mentor_head" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	// Log incoming payload for debugging
	log.Printf("MarkAttendance Payload: SessionID=%s, LeadID=%s, Status=%s, ClassKey=%s, UserID=%s",
		req.SessionID, req.LeadID, req.Status, req.ClassKey, userIDStr)

	enforceDeadline := userRole == "mentor"
	if err := models.MarkAttendance(sessionID, leadID, req.Status, req.Notes, userID, enforceDeadline); err != nil {
		log.Printf("ERROR: Failed to mark attendance: %v", err)
		if errors.Is(err, models.ErrAttendanceDeadlinePassed) {
			jsonError(w, http.StatusBadRequest, "Attendance deadline has passed (24 hours after session end).")
			return
		}
		jsonError(w, http.StatusInternalServerError, "Failed to mark attendance")
		return
	}

	// Session performance upsert (grading automation inputs).
	// Missing task input should not silently mark homework as done.
	taskCompleted := false
	if req.TaskCompleted != nil {
		taskCompleted = *req.TaskCompleted
	}
	participationScore := 3
	if req.ParticipationScore != nil {
		participationScore = *req.ParticipationScore
	}
	if participationScore < 1 || participationScore > 5 {
		jsonError(w, http.StatusBadRequest, "participation_score must be between 1 and 5")
		return
	}
	if err := models.UpsertSessionPerformance(sessionID, leadID, taskCompleted, participationScore); err != nil {
		log.Printf("ERROR: Failed to upsert session performance: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to save session performance")
		return
	}

	// Trigger high priority check (3+ absences)
	if req.ClassKey != "" {
		if err := models.UpdateAbsencePriority(leadID, req.ClassKey); err != nil {
			log.Printf("WARNING: Failed to update absence priority for lead %s: %v", leadID, err)
		}
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
		classGroup, err := models.GetClassGroupByKey(req.ClassKey)
		if err != nil {
			jsonError(w, http.StatusNotFound, "Class not found")
			return
		}
		if strings.TrimSpace(classGroup.RoundStatus) != "active" {
			jsonError(w, http.StatusBadRequest, "Round has not started yet. Session completion is locked for mentors until Mentor Head starts the round.")
			return
		}
	} else if userRole != "admin" && userRole != "student_success" && userRole != "mentor_head" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	now := time.Now()
	if err := models.CompleteSession(sessionID, now, now.Format("15:04")); err != nil {
		log.Printf("ERROR: Failed to complete session: %v", err)
		if errors.Is(err, models.ErrAttendanceIncomplete) {
			jsonError(w, http.StatusBadRequest, "Please mark attendance for all students before completing the session.")
			return
		}
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
	} else if userRole != "mentor_head" && userRole != "admin" && userRole != "student_success" && userRole != "manager" {
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
		IsPrivate      bool   `json:"is_private"`
		CreatedAt      string `json:"created_at"`
		CreatedByEmail string `json:"created_by_email"`
	}

	role := middleware.GetUserRole(r)
	response := make([]NoteResponse, 0, len(notes))
	for _, n := range notes {
		if n.IsPrivate && role == "mentor" {
			continue
		}
		email := "System"
		if n.CreatedByEmail.Valid {
			email = n.CreatedByEmail.String
		}
		response = append(response, NoteResponse{
			ID:             n.ID.String(),
			Text:           n.NoteText,
			IsPrivate:      n.IsPrivate,
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
		IsPrivate bool   `json:"is_private"`
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
	} else if userRole != "mentor_head" && userRole != "admin" && userRole != "student_success" && userRole != "manager" {
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
	if err := models.AddStudentNote(leadID, req.ClassKey, sessionNumber, req.Text, req.IsPrivate, createdByUserID); err != nil {
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
	} else if userRole != "mentor_head" && userRole != "admin" && userRole != "student_success" && userRole != "manager" {
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

// GET /api/student?student_id=... or ?lead_id=... - returns student profile (+ report payload when class_key is provided)
func (h *APIHandler) GetStudent(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "mentor_head" && userRole != "admin" && userRole != "student_success" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	studentIDStr := r.URL.Query().Get("student_id")
	if strings.TrimSpace(studentIDStr) == "" {
		studentIDStr = r.URL.Query().Get("lead_id")
	}
	if studentIDStr == "" {
		jsonError(w, http.StatusBadRequest, "student_id or lead_id is required")
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
	var highPriority bool
	var highPriorityReason sql.NullString

	err = db.DB.QueryRow(`
		SELECT id, full_name, phone, levels_purchased_total, levels_consumed, high_priority, high_priority_reason
		FROM leads WHERE id = $1
	`, studentID).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &levelsPurchasedTotal, &levelsConsumed, &highPriority, &highPriorityReason,
	)
	lead.HighPriorityAbsence = highPriority
	lead.HighPriorityReason = highPriorityReason
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

	// Fetch and filter notes
	notes, err := models.GetStudentNotes(studentID)
	if err != nil {
		log.Printf("ERROR: Failed to get notes: %v", err)
		notes = []*models.StudentNote{}
	}

	var filteredNotes []*models.StudentNote
	role := middleware.GetUserRole(r)
	for _, n := range notes {
		if n.IsPrivate && role == "mentor" {
			continue
		}
		filteredNotes = append(filteredNotes, n)
	}

	response := map[string]interface{}{
		"id":                 studentID.String(),
		"name":               lead.FullName,
		"phone":              lead.Phone,
		"levelsFinished":     levelsFinished,
		"levelsLeft":         levelsLeft,
		"lastLevelGrade":     lastLevelGrade,
		"highPriority":       lead.HighPriorityAbsence,
		"highPriorityReason": lead.HighPriorityReason.String,
		"notes":              filteredNotes,
	}

	// Optional report-card payload (for printable grade justification paper).
	if classKey != "" {
		type SessionEvidence struct {
			SessionNumber       int32  `json:"session_number"`
			AttendanceStatus    string `json:"attendance_status"`
			TaskCompleted       *bool  `json:"task_completed,omitempty"`
			ParticipationScore  *int32 `json:"participation_score,omitempty"`
			ParticipationStars  string `json:"participation_stars"`
			TaskDisplay         string `json:"task_display"`
			AttendanceDisplay   string `json:"attendance_display"`
			ParticipationSymbol string `json:"participation_symbol"`
		}

		classGroup, _ := models.GetClassGroupByKey(classKey)
		sessions, _ := models.GetClassSessions(classKey)
		attendanceBySession, _ := models.GetAttendanceByClassKey(classKey)
		perfBySession, _ := models.GetSessionPerformanceByClassKey(classKey)
		previews, _ := models.GetGradePreviewsByClass(classKey)
		preview, hasPreview := previews[studentID]

		finalGrade := ""
		mentorComment := ""
		if grade, err := models.GetGrade(studentID, classKey); err == nil && grade != nil {
			finalGrade = grade.Grade
			if grade.Notes.Valid {
				mentorComment = grade.Notes.String
			}
		}
		if finalGrade == "" && hasPreview {
			finalGrade = preview.CalculatedGrade
		}

		evidence := make([]SessionEvidence, 0, len(sessions))
		for _, s := range sessions {
			if s == nil {
				continue
			}
			row := SessionEvidence{
				SessionNumber:       s.SessionNumber,
				AttendanceStatus:    "",
				ParticipationStars:  "—",
				TaskDisplay:         "—",
				AttendanceDisplay:   "—",
				ParticipationSymbol: "—",
			}
			if byLead, ok := attendanceBySession[s.ID]; ok {
				if att, ok := byLead[studentID]; ok && att != nil {
					row.AttendanceStatus = strings.ToUpper(strings.TrimSpace(att.Status))
					switch row.AttendanceStatus {
					case "ABSENT":
						row.AttendanceDisplay = "❌"
					case "PRESENT", "LATE":
						row.AttendanceDisplay = "✅"
					default:
						row.AttendanceDisplay = "—"
					}
				}
			}

			if s.SessionNumber == 1 {
				row.TaskDisplay = "➖"
			} else {
				taskCompleted := false
				if byLead, ok := perfBySession[s.ID]; ok {
					if perf, ok := byLead[studentID]; ok && perf != nil {
						taskCompleted = perf.TaskCompleted
						score := perf.ParticipationScore
						row.ParticipationScore = &score
					}
				}
				row.TaskCompleted = &taskCompleted
				if taskCompleted {
					row.TaskDisplay = "✅"
				} else {
					row.TaskDisplay = "❌"
				}
			}

			if row.ParticipationScore != nil && *row.ParticipationScore > 0 {
				row.ParticipationStars = strings.Repeat("⭐", int(*row.ParticipationScore))
				row.ParticipationSymbol = row.ParticipationStars
			}
			evidence = append(evidence, row)
		}

		response["report_card"] = map[string]interface{}{
			"class_key": classKey,
			"class_level": func() int32 {
				if classGroup != nil {
					return classGroup.Level
				}
				return 0
			}(),
			"student_name":     lead.FullName,
			"student_phone":    lead.Phone,
			"generated_at":     time.Now().Format(time.RFC3339),
			"final_grade":      finalGrade,
			"mentor_comment":   mentorComment,
			"session_evidence": evidence,
			"calculation": map[string]interface{}{
				"attendance_score": func() float64 {
					if hasPreview {
						return preview.AttendanceScore
					}
					return 0
				}(),
				"task_score": func() float64 {
					if hasPreview {
						return preview.TaskScore
					}
					return 0
				}(),
				"participation_score": func() float64 {
					if hasPreview {
						return preview.ParticipationScore
					}
					return 0
				}(),
				"total_score": func() float64 {
					if hasPreview {
						return preview.TotalScore
					}
					return 0
				}(),
				"absences": func() int {
					if hasPreview {
						return preview.Absences
					}
					return 0
				}(),
				"completed_tasks": func() int {
					if hasPreview {
						return preview.CompletedTasks
					}
					return 0
				}(),
				"missed_tasks": func() int {
					if hasPreview {
						return 7 - preview.CompletedTasks
					}
					return 0
				}(),
				"average_stars": func() float64 {
					if hasPreview {
						return preview.AverageParticipation
					}
					return 0
				}(),
				"calculated_grade": func() string {
					if hasPreview {
						return preview.CalculatedGrade
					}
					return finalGrade
				}(),
				"used_legacy_task_safe": func() bool {
					if hasPreview {
						return preview.UsedLegacyTaskFallback
					}
					return false
				}(),
			},
		}
	}

	jsonResponse(w, http.StatusOK, response)
}

// GET /api/mentor-head/dashboard - returns dashboard data (classes + mentors)
func (h *APIHandler) GetMentorHeadDashboard(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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
		AllGraded    bool    `json:"all_graded"`
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
		students, err := models.GetStudentsForMentorHeadClass(c.ClassKey)
		if err == nil {
			cr.StudentCount = len(students)
			if cr.StudentCount >= 6 {
				cr.Readiness = "LOCKED"
			} else if cr.StudentCount >= 4 {
				cr.Readiness = "READY"
			} else {
				cr.Readiness = "NOT READY"
			}

			grades, err := models.GetGradesByClassKey(c.ClassKey)
			if err == nil {
				gradedLeadIDs := make(map[uuid.UUID]bool)
				for _, g := range grades {
					if g.SessionNumber == 8 {
						gradedLeadIDs[g.LeadID] = true
					}
				}
				allGraded := true
				for _, s := range students {
					if !gradedLeadIDs[s.LeadID] {
						allGraded = false
						break
					}
				}
				cr.AllGraded = allGraded
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

// GET /api/mentor-head/archive - returns closed classes grouped by mentor
func (h *APIHandler) GetMentorHeadArchive(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Admin access required")
		return
	}

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "oldest"
	}

	var fromDate *time.Time
	var toDate *time.Time
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		parsed, err := time.Parse("2006-01-02", fromStr)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid from date. Use YYYY-MM-DD.")
			return
		}
		fromDate = &parsed
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		parsed, err := time.Parse("2006-01-02", toStr)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid to date. Use YYYY-MM-DD.")
			return
		}
		toDate = &parsed
	}

	classes, err := models.GetArchivedClassGroups(sort, fromDate, toDate)
	if err != nil {
		log.Printf("ERROR: Failed to get archived classes: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load archived classes")
		return
	}

	type ArchivedClass struct {
		ClassKey               string `json:"class_key"`
		Level                  int32  `json:"level"`
		Days                   string `json:"days"`
		Time                   string `json:"time"`
		ClassNumber            int32  `json:"class_number"`
		StudentCount           int    `json:"student_count"`
		ClosedAt               string `json:"closed_at"`
		CompletedSessionsCount int32  `json:"completed_sessions_count"`
	}

	type MentorArchiveGroup struct {
		Mentor struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"mentor"`
		Classes []ArchivedClass `json:"classes"`
	}

	// Group by mentor
	groupMap := make(map[string]*MentorArchiveGroup)
	var unknownGroup *MentorArchiveGroup

	for _, c := range classes {
		archived := ArchivedClass{
			ClassKey:               c.ClassKey,
			Level:                  c.Level,
			Days:                   c.ClassDays,
			Time:                   c.ClassTime,
			ClassNumber:            c.ClassNumber,
			CompletedSessionsCount: c.CompletedSessions,
		}
		if c.RoundClosedAt.Valid {
			archived.ClosedAt = c.RoundClosedAt.Time.Format(time.RFC3339)
		}

		// Get student count (use historical enrollments for closed classes)
		if count, err := models.CountClassEnrollments(c.ClassKey); err == nil {
			archived.StudentCount = count
		}

		mentorID := ""
		if c.ClosedMentorUserID.Valid {
			mentorID = c.ClosedMentorUserID.String
		}

		if mentorID == "" {
			if unknownGroup == nil {
				unknownGroup = &MentorArchiveGroup{
					Classes: []ArchivedClass{},
				}
				unknownGroup.Mentor.Email = "Unknown/Unassigned"
			}
			unknownGroup.Classes = append(unknownGroup.Classes, archived)
			continue
		}

		if _, ok := groupMap[mentorID]; !ok {
			group := &MentorArchiveGroup{
				Classes: []ArchivedClass{},
			}
			group.Mentor.ID = mentorID
			// Get mentor info
			user, err := models.GetUserByID(mentorID)
			if err == nil && user != nil {
				group.Mentor.Email = user.Email
				group.Mentor.Name = userDisplayName(user)
			}
			groupMap[mentorID] = group
		}
		groupMap[mentorID].Classes = append(groupMap[mentorID].Classes, archived)
	}

	// Prepare final response
	response := make([]*MentorArchiveGroup, 0, len(groupMap))
	for _, g := range groupMap {
		response = append(response, g)
	}
	if unknownGroup != nil {
		response = append(response, unknownGroup)
	}

	jsonResponse(w, http.StatusOK, response)
}

// POST /api/mentor-head/assign-mentor - assigns a mentor to a class
func (h *APIHandler) AssignMentor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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
		switch {
		case errors.Is(err, models.ErrClassRoundActive):
			jsonError(w, http.StatusConflict, "Cannot return class because the round has started.")
		case errors.Is(err, models.ErrClassAlreadyReturned):
			jsonError(w, http.StatusConflict, "Class is already returned to Operations.")
		case errors.Is(err, models.ErrClassNotFound):
			jsonError(w, http.StatusNotFound, "Class not found.")
		default:
			jsonError(w, http.StatusInternalServerError, "Failed to return class")
		}
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
	if userRole != "mentor_head" && userRole != "manager" {
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
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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
		if strings.Contains(err.Error(), "mentor not assigned") {
			jsonError(w, http.StatusBadRequest, "Assign a mentor before starting the round")
			return
		}
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

// POST /api/mentor-head/shift-start-date - shifts the full class schedule by changing session 1 date
func (h *APIHandler) ShiftRoundStartDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Manager access required")
		return
	}

	var req struct {
		ClassKey     string `json:"class_key"`
		NewStartDate string `json:"new_start_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.ClassKey = strings.TrimSpace(req.ClassKey)
	req.NewStartDate = strings.TrimSpace(req.NewStartDate)
	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}
	if req.NewStartDate == "" {
		jsonError(w, http.StatusBadRequest, "new_start_date is required")
		return
	}

	newStartDate, err := time.Parse("2006-01-02", req.NewStartDate)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "new_start_date must be in YYYY-MM-DD format")
		return
	}

	changedByID, err := uuid.Parse(middleware.GetUserID(r))
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Invalid authenticated user")
		return
	}

	if err := models.ShiftClassRoundStart(req.ClassKey, newStartDate, changedByID); err != nil {
		log.Printf("ERROR: Failed to shift class start date for %s: %v", req.ClassKey, err)
		msg := err.Error()
		switch {
		case strings.Contains(msg, "class not found"):
			jsonError(w, http.StatusNotFound, "Class not found")
		case strings.Contains(msg, "must be a"),
			strings.Contains(msg, "cannot change start date before the round is started"),
			strings.Contains(msg, "cannot change start date after session completion has begun"),
			strings.Contains(msg, "cannot change start date for a closed class"):
			jsonError(w, http.StatusBadRequest, msg)
		default:
			jsonError(w, http.StatusInternalServerError, msg)
		}
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"ok":             true,
		"class_key":      req.ClassKey,
		"new_start_date": newStartDate.Format("2006-01-02"),
	})
}

// POST /api/mentor-head/reschedule-session - changes one session's date/time
func (h *APIHandler) RescheduleClassSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head or Manager access required")
		return
	}

	var req struct {
		ClassKey  string `json:"class_key"`
		SessionID string `json:"session_id"`
		NewDate   string `json:"new_date"`
		NewTime   string `json:"new_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.ClassKey = strings.TrimSpace(req.ClassKey)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.NewDate = strings.TrimSpace(req.NewDate)
	req.NewTime = strings.TrimSpace(req.NewTime)
	if req.ClassKey == "" || req.SessionID == "" || req.NewDate == "" || req.NewTime == "" {
		jsonError(w, http.StatusBadRequest, "class_key, session_id, new_date, and new_time are required")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid session_id")
		return
	}

	newDate, err := time.Parse("2006-01-02", req.NewDate)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "new_date must be in YYYY-MM-DD format")
		return
	}
	if _, err := time.Parse("15:04", req.NewTime); err != nil {
		jsonError(w, http.StatusBadRequest, "new_time must be in HH:MM format")
		return
	}

	sessions, err := models.GetClassSessions(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to load sessions for %s: %v", req.ClassKey, err)
		jsonError(w, http.StatusInternalServerError, "Failed to load class sessions")
		return
	}

	var target *models.ClassSession
	for _, s := range sessions {
		if s != nil && s.ID == sessionID {
			target = s
			break
		}
	}
	if target == nil {
		jsonError(w, http.StatusNotFound, "Session not found in this class")
		return
	}
	if strings.EqualFold(strings.TrimSpace(target.Status), "completed") {
		jsonError(w, http.StatusBadRequest, "Completed sessions cannot be rescheduled")
		return
	}

	if err := models.CancelAndRescheduleSession(sessionID, newDate, req.NewTime); err != nil {
		log.Printf("ERROR: Failed to reschedule session %s for class %s: %v", req.SessionID, req.ClassKey, err)
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"class_key":  req.ClassKey,
		"session_id": req.SessionID,
		"new_date":   req.NewDate,
		"new_time":   req.NewTime,
	})
}

// POST /api/mentor-head/close-round - closes a round
func (h *APIHandler) CloseRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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

	// Safety check: ensure all students in this class have been graded
	students, err := models.GetStudentsInClassGroup(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to get students for class %s: %v", req.ClassKey, err)
		jsonError(w, http.StatusInternalServerError, "Failed to verify student grades")
		return
	}

	grades, err := models.GetGradesByClassKey(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to get grades for class %s: %v", req.ClassKey, err)
		jsonError(w, http.StatusInternalServerError, "Failed to verify student grades")
		return
	}

	// Session 8 is the final grade session
	gradedLeadIDs := make(map[uuid.UUID]bool)
	for _, g := range grades {
		if g.SessionNumber == 8 {
			gradedLeadIDs[g.LeadID] = true
		}
	}

	for _, s := range students {
		// Only check students who are currently in the class (scheduling table)
		if !gradedLeadIDs[s.LeadID] {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("Cannot close round: %s is missing final grade", s.FullName))
			return
		}
	}

	if err := models.CloseRound(req.ClassKey, closedByID); err != nil {
		log.Printf("ERROR: Failed to close round: %v", err)
		if strings.Contains(strings.ToLower(err.Error()), "cannot close round") {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		jsonError(w, http.StatusInternalServerError, "Failed to close round")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/mentor-head/reopen-round - reopens a closed round if incomplete
func (h *APIHandler) ReopenRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "admin" && userRole != "manager" {
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

	if err := models.ReopenClosedRound(req.ClassKey); err != nil {
		log.Printf("ERROR: Failed to reopen round: %v", err)
		if strings.Contains(err.Error(), "not closed") {
			jsonError(w, http.StatusBadRequest, "Class is not closed")
			return
		}
		if strings.Contains(err.Error(), "completed") {
			jsonError(w, http.StatusBadRequest, "Cannot reopen a class that completed all 8 sessions")
			return
		}
		jsonError(w, http.StatusInternalServerError, "Failed to reopen class")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/mentor-head/evaluations - returns mentors with class-scoped evaluation data by scope.
func (h *APIHandler) GetMentorEvaluations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head access required")
		return
	}

	scope := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("scope")))
	if scope == "" {
		scope = "active"
	}
	if scope != "active" && scope != "closed" {
		jsonError(w, http.StatusBadRequest, "scope must be active or closed")
		return
	}

	mentorQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	var fromDate *time.Time
	var toDate *time.Time
	if scope == "closed" {
		fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
		toRaw := strings.TrimSpace(r.URL.Query().Get("to"))
		if fromRaw != "" {
			parsed, err := time.Parse("2006-01-02", fromRaw)
			if err != nil {
				jsonError(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
				return
			}
			fromDate = &parsed
		}
		if toRaw != "" {
			parsed, err := time.Parse("2006-01-02", toRaw)
			if err != nil {
				jsonError(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
				return
			}
			toDate = &parsed
		}
		if fromDate != nil && toDate != nil && fromDate.After(*toDate) {
			jsonError(w, http.StatusBadRequest, "from date must be before or equal to to date")
			return
		}
	}

	mentorItems, err := models.GetMentorEvaluationsByRoundStatus(scope, mentorQuery, fromDate, toDate)
	if err != nil {
		log.Printf("ERROR: Failed to get mentor evaluations by class: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load evaluations")
		return
	}

	type ClassResponse struct {
		ClassKey    string `json:"classKey"`
		Level       int32  `json:"level"`
		Days        string `json:"days"`
		Time        string `json:"time"`
		ClassNumber int32  `json:"classNumber"`
		RoundStatus string `json:"roundStatus"`
		Manual      struct {
			SessionQuality      int    `json:"sessionQuality"`
			StudentsFeedback    int    `json:"studentsFeedback"`
			TrelloSessionChecks []bool `json:"trelloSessionChecks"`
			TrelloCompliancePct int    `json:"trelloCompliancePercent"`
		} `json:"manual"`
		Automatic struct {
			WhatsAppManagementPercent int      `json:"whatsAppManagementPercent"`
			AttendancePunctualityPct  int      `json:"attendancePunctualityPercent"`
			AttendanceStatuses        []string `json:"attendanceStatuses"`
		} `json:"automatic"`
	}

	type MentorResponse struct {
		ID               string          `json:"id"`
		Email            string          `json:"email"`
		Name             string          `json:"name"`
		ActiveClassCount int             `json:"activeClassCount"`
		Classes          []ClassResponse `json:"classes"`
	}

	mentorsResponse := make([]MentorResponse, 0, len(mentorItems))
	for _, item := range mentorItems {
		mentor := MentorResponse{
			ID:               item.User.ID.String(),
			Email:            item.User.Email,
			Name:             userDisplayName(item.User),
			ActiveClassCount: len(item.ActiveClasses),
			Classes:          make([]ClassResponse, 0, len(item.ActiveClasses)),
		}

		for _, classItem := range item.ActiveClasses {
			c := ClassResponse{
				ClassKey:    classItem.ClassKey,
				Level:       classItem.Level,
				Days:        classItem.ClassDays,
				Time:        classItem.ClassTime,
				ClassNumber: classItem.ClassNumber,
				RoundStatus: classItem.RoundStatus,
			}
			c.Manual.SessionQuality = classItem.KPISessionQuality
			c.Manual.StudentsFeedback = classItem.KPIStudentsFeedback
			c.Manual.TrelloSessionChecks = classItem.TrelloSessionChecks
			checked := 0
			for _, ok := range classItem.TrelloSessionChecks {
				if ok {
					checked++
				}
			}
			c.Manual.TrelloCompliancePct = (checked * 100) / 8

			c.Automatic.WhatsAppManagementPercent = classItem.AutoWhatsAppPercent
			c.Automatic.AttendancePunctualityPct = classItem.AttendancePercent
			c.Automatic.AttendanceStatuses = classItem.AttendanceStatuses
			mentor.Classes = append(mentor.Classes, c)
		}

		mentorsResponse = append(mentorsResponse, mentor)
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"scope": scope,
		"filters": map[string]interface{}{
			"q":    mentorQuery,
			"from": r.URL.Query().Get("from"),
			"to":   r.URL.Query().Get("to"),
		},
		"mentors": mentorsResponse,
	})
}

// PUT /api/mentor-head/evaluations/:mentorId - updates class-scoped manual mentor evaluation
func (h *APIHandler) UpdateMentorEvaluation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor_head" && userRole != "manager" {
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
		ClassKey string `json:"classKey"`
		Manual   struct {
			SessionQuality      int    `json:"sessionQuality"`
			StudentsFeedback    int    `json:"studentsFeedback"`
			TrelloSessionChecks []bool `json:"trelloSessionChecks"`
		} `json:"manual"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "classKey is required")
		return
	}

	// Upsert class-scoped manual evaluation.
	if err := models.UpsertMentorEvaluationByClass(
		mentorID,
		req.ClassKey,
		evaluatorID,
		req.Manual.SessionQuality,
		req.Manual.StudentsFeedback,
		req.Manual.TrelloSessionChecks,
	); err != nil {
		if strings.Contains(err.Error(), "must be between") || strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "active class") {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("ERROR: Failed to upsert mentor evaluation: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to save evaluation")
		return
	}

	trelloChecks := req.Manual.TrelloSessionChecks
	if len(trelloChecks) < 8 {
		fixed := make([]bool, 8)
		copy(fixed, trelloChecks)
		trelloChecks = fixed
	}
	checked := 0
	for _, ok := range trelloChecks {
		if ok {
			checked++
		}
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":       mentorID.String(),
		"classKey": req.ClassKey,
		"manual": map[string]interface{}{
			"sessionQuality":      req.Manual.SessionQuality,
			"studentsFeedback":    req.Manual.StudentsFeedback,
			"trelloSessionChecks": trelloChecks,
			"trelloCompliancePct": (checked * 100) / 8,
		},
	})
}

// GET /api/student-success/classes - returns active classes only (round_status='active')
func (h *APIHandler) GetStudentSuccessClasses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "manager" {
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
		ClassKey           string `json:"class_key"`
		Level              int    `json:"level"`
		Days               string `json:"days"`
		Time               string `json:"time"`
		ClassNumber        int    `json:"class_number"`
		MentorEmail        string `json:"mentor_email"`
		MentorName         string `json:"mentor_name"`
		MentorUserID       string `json:"mentor_user_id,omitempty"`
		StudentCount       int    `json:"student_count"`
		HasHighPriority    bool   `json:"has_high_priority"`
		HighPriorityReason string `json:"high_priority_reason,omitempty"`
		MidRoundRequired   bool   `json:"mid_round_required"`
		EndRoundRequired   bool   `json:"end_round_required"`
		ComplianceRequired bool   `json:"compliance_required"`
		ComplianceDone     int    `json:"compliance_done"`
		ComplianceTotal    int    `json:"compliance_total"`
	}
	classes := make([]ClassResp, 0, len(rows))
	for _, row := range rows {
		cr := ClassResp{
			ClassKey:           row.ClassKey,
			Level:              row.Level,
			Days:               row.ClassDays,
			Time:               row.ClassTime,
			ClassNumber:        row.ClassNumber,
			MentorEmail:        row.MentorEmail,
			MentorName:         row.MentorName,
			MentorUserID:       row.MentorUserID,
			StudentCount:       row.StudentCount,
			HasHighPriority:    row.HasHighPriority,
			HighPriorityReason: row.HighPriorityReason,
		}

		students, err := models.GetStudentsInClassGroup(row.ClassKey)
		if err == nil {
			studentIDs := make(map[uuid.UUID]bool, len(students))
			for _, s := range students {
				studentIDs[s.LeadID] = true
			}

			feedbackRecords, _ := models.GetClassFeedbackRecords(row.ClassKey)
			hasS4 := make(map[uuid.UUID]bool)
			hasS8 := make(map[uuid.UUID]bool)
			for _, f := range feedbackRecords {
				switch f.SessionNumber {
				case 4:
					hasS4[f.LeadID] = true
				case 8:
					hasS8[f.LeadID] = true
				}
			}

			allS4 := true
			allS8 := true
			for leadID := range studentIDs {
				if !hasS4[leadID] {
					allS4 = false
				}
				if !hasS8[leadID] {
					allS8 = false
				}
			}

			sessions, err := models.GetClassSessions(row.ClassKey)
			if err == nil {
				completedCount := 0
				totalSessions := 0
				for _, s := range sessions {
					totalSessions++
					if s.Status == "completed" {
						completedCount++
					}
				}
				cr.MidRoundRequired = completedCount >= 4 && !allS4
				cr.EndRoundRequired = completedCount >= 8 && !allS8
				if completedCount >= 8 {
					complianceRows, compErr := models.GetComplianceByClassKey(row.ClassKey)
					if compErr == nil {
						done := 0
						total := 0
						for _, item := range complianceRows {
							// count only sessions that exist for this class
							if item.ClassSessionID == nil {
								continue
							}
							total++
							if item.Check != nil {
								done++
							}
						}
						if total == 0 {
							total = totalSessions
						}
						cr.ComplianceDone = done
						cr.ComplianceTotal = total
						cr.ComplianceRequired = done < total
					}
				}
			}
		}

		classes = append(classes, cr)
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"classes": classes})
}

// GET /api/mentor/reminders - returns active reminders for mentor
func (h *APIHandler) GetMentorReminders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor or Admin access required")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	reminders, err := models.GetMentorReminders(userID)
	if err != nil {
		log.Printf("ERROR: Failed to get mentor reminders: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to get reminders")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"reminders": reminders,
	})
}

// GET /api/student-success/placement-tests - returns placement tests scheduled and awaiting results
func (h *APIHandler) GetStudentSuccessPlacementTests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if middleware.GetUserRole(r) != "student_success" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success access required")
		return
	}

	showCompleted := r.URL.Query().Get("show_completed") == "1" || r.URL.Query().Get("show_completed") == "true"
	rows, err := models.GetPlacementTestsForStudentSuccess(showCompleted)
	if err != nil {
		log.Printf("ERROR: Failed to get placement tests: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load placement tests")
		return
	}

	type PlacementTestResp struct {
		LeadID        string `json:"lead_id"`
		FullName      string `json:"full_name"`
		Phone         string `json:"phone"`
		Status        string `json:"status"`
		TestDate      string `json:"test_date"`
		TestTime      string `json:"test_time"`
		TestType      string `json:"test_type"`
		AssignedLevel *int32 `json:"assigned_level,omitempty"`
		TestNotes     string `json:"test_notes,omitempty"`
	}

	out := make([]PlacementTestResp, 0, len(rows))
	for _, row := range rows {
		testDate := ""
		if row.TestDate.Valid {
			testDate = row.TestDate.Time.Format("2006-01-02")
		}
		testTime := ""
		if row.TestTime.Valid {
			testTime = row.TestTime.String
		}
		testType := ""
		if row.TestType.Valid {
			testType = row.TestType.String
		}
		var assignedLevel *int32
		if row.AssignedLevel.Valid {
			val := row.AssignedLevel.Int32
			assignedLevel = &val
		}
		testNotes := ""
		if row.TestNotes.Valid {
			testNotes = row.TestNotes.String
		}
		out = append(out, PlacementTestResp{
			LeadID:        row.LeadID.String(),
			FullName:      row.FullName,
			Phone:         row.Phone,
			Status:        row.Status,
			TestDate:      testDate,
			TestTime:      testTime,
			TestType:      testType,
			AssignedLevel: assignedLevel,
			TestNotes:     testNotes,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"placement_tests": out})
}

// POST /api/student-success/placement-tests/complete - record placement test results and mark lead tested
func (h *APIHandler) CompletePlacementTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if middleware.GetUserRole(r) != "student_success" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success access required")
		return
	}

	var req struct {
		LeadID        string `json:"lead_id"`
		AssignedLevel int    `json:"assigned_level"`
		TestNotes     string `json:"test_notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.LeadID == "" {
		jsonError(w, http.StatusBadRequest, "lead_id is required")
		return
	}
	if req.AssignedLevel < 1 || req.AssignedLevel > 10 {
		jsonError(w, http.StatusBadRequest, "assigned_level must be between 1 and 10")
		return
	}

	leadID, err := uuid.Parse(req.LeadID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead_id")
		return
	}

	detail, err := models.GetLeadByID(leadID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "Lead not found")
		return
	}
	if detail.PlacementTest == nil || !detail.PlacementTest.TestDate.Valid || !detail.PlacementTest.TestTime.Valid {
		jsonError(w, http.StatusBadRequest, "Placement test is not scheduled yet")
		return
	}

	pt := &models.PlacementTest{
		LeadID:        leadID,
		AssignedLevel: sql.NullInt32{Int32: int32(req.AssignedLevel), Valid: true},
	}
	if strings.TrimSpace(req.TestNotes) != "" {
		pt.TestNotes = sql.NullString{String: req.TestNotes, Valid: true}
	}

	if err := models.UpdatePlacementTest(pt); err != nil {
		log.Printf("ERROR: Failed to update placement test: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to save placement test")
		return
	}

	// Only promote to tested if lead is still before tested stage
	if detail.Lead.Status == "lead_created" || detail.Lead.Status == "test_booked" {
		if err := models.UpdateLeadStatus(leadID, "tested"); err != nil {
			log.Printf("ERROR: Failed to update lead status: %v", err)
			jsonError(w, http.StatusInternalServerError, "Failed to update lead status")
			return
		}
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/student-success/class?class_key=... - returns class details + students + sessions + attendance
func (h *APIHandler) GetStudentSuccessClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	role := middleware.GetUserRole(r)
	allowClosed := false
	switch role {
	case "student_success":
		allowClosed = false
	case "mentor_head", "admin":
		allowClosed = true
	default:
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success or Mentor Head access required")
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

	cg, students, sessions, missedSessions, feedbackRecords, completedCount, err := models.GetStudentSuccessClassDetail(classKey, allowClosed)
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
		Phone    string         `json:"phone"`
		S4       *FeedbackEntry `json:"s4,omitempty"`
		S8       *FeedbackEntry `json:"s8,omitempty"`
	}

	feedbackMap := make(map[string]*StudentFeedback)
	for _, s := range students {
		feedbackMap[s.LeadID.String()] = &StudentFeedback{
			LeadID:   s.LeadID.String(),
			FullName: s.FullName,
			Phone:    s.Phone,
		}
	}

	for _, f := range feedbackRecords {
		sf, ok := feedbackMap[f.LeadID.String()]
		if !ok {
			continue
		}
		entry := &FeedbackEntry{
			SessionNumber:    f.SessionNumber,
			Status:           f.Status,
			FeedbackText:     f.FeedbackText,
			FollowUpRequired: f.FollowUpRequired,
		}
		switch f.SessionNumber {
		case 4:
			sf.S4 = entry
		case 8:
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
	if role != "student_success" && role != "mentor_head" && role != "admin" && role != "manager" {
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

	// Trigger automatic progression of follow-up statuses
	if err := models.AutoProgressFollowupStatuses(); err != nil {
		log.Printf("WARNING: AutoProgressFollowupStatuses failed: %v", err)
	}

	feed, err := models.GetAbsenceFeed(classKey, filter, search)
	if err != nil {
		log.Printf("ERROR: Failed to get absence feed: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load absence feed")
		return
	}

	jsonResponse(w, http.StatusOK, feed)
}

// GET /api/student-success/followups
func (h *APIHandler) GetFollowUps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	classKey := r.URL.Query().Get("class_key")
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	resolvedStr := r.URL.Query().Get("resolved")
	resolved := resolvedStr == "true"

	log.Printf("DEBUG: GetFollowUps called with class_key: %q, resolved: %v", classKey, resolved)

	// Trigger automatic progression of follow-up statuses
	if err := models.AutoProgressFollowupStatuses(); err != nil {
		log.Printf("WARNING: AutoProgressFollowupStatuses failed: %v", err)
	}

	followUps, err := models.GetFollowUps(classKey, resolved)
	if err != nil {
		log.Printf("ERROR: Failed to get follow-ups: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load follow-ups")
		return
	}

	jsonResponse(w, http.StatusOK, followUps)
}

// POST /api/student-success/resolve-absence
func (h *APIHandler) ResolveAbsence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	var req struct {
		ClassKey      string `json:"class_key"`
		LeadID        string `json:"lead_id"`
		SessionNumber int    `json:"session_number"`
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

	if err := models.ResolveAbsence(req.ClassKey, leadID, req.SessionNumber, userID); err != nil {
		log.Printf("ERROR: Failed to resolve absence: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to resolve absence")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// POST /api/student-success/followups
func (h *APIHandler) CreateFollowUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" && role != "manager" {
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

	log.Printf("DEBUG: CreateFollowUp called with class_key: %q, lead_id: %s, session: %d", req.ClassKey, req.LeadID, req.SessionNumber)

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
	if role != "student_success" && role != "mentor_head" && role != "admin" && role != "manager" {
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

	log.Printf("WARNING: Deprecated endpoint used: /api/classes/:id/sessions/:n/complete. Prefer /api/session/complete")

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
		classGroup, err := models.GetClassGroupByKey(classKey)
		if err != nil {
			jsonError(w, http.StatusNotFound, "Class not found")
			return
		}
		if strings.TrimSpace(classGroup.RoundStatus) != "active" {
			jsonError(w, http.StatusBadRequest, "Round has not started yet. Session completion is locked for mentors until Mentor Head starts the round.")
			return
		}
	} else if userRole != "mentor_head" && userRole != "admin" && userRole != "student_success" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	now := time.Now()
	if err := models.CompleteSession(targetSession.ID, now, now.Format("15:04")); err != nil {
		log.Printf("ERROR: Failed to complete session %v: %v", targetSession.ID, err)
		if errors.Is(err, models.ErrAttendanceIncomplete) {
			jsonError(w, http.StatusBadRequest, "Please mark attendance for all students before completing the session.")
			return
		}
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
	if role != "student_success" && role != "admin" && role != "manager" {
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

// UpdateFeedbackStatus updates the status of sent feedback (received/removed)
func (h *APIHandler) UpdateFeedbackStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "admin" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success or Admin access required")
		return
	}

	var req struct {
		LeadID        string `json:"lead_id"`
		ClassKey      string `json:"class_key"`
		SessionNumber int32  `json:"session_number"`
		Status        string `json:"status"` // "received" or "removed"
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

	if req.Status != "received" && req.Status != "removed" {
		jsonError(w, http.StatusBadRequest, "Invalid status. Must be 'received' or 'removed'")
		return
	}

	rowsAffected, err := models.UpdateFeedbackStatus(leadID, req.ClassKey, req.SessionNumber, req.Status)
	if err != nil {
		log.Printf("ERROR: Failed to update feedback status: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to update feedback status")
		return
	}
	if rowsAffected == 0 {
		log.Printf("WARNING: UpdateFeedbackStatus affected 0 rows for lead_id=%s class_key=%s session=%d", leadID, req.ClassKey, req.SessionNumber)
		jsonError(w, http.StatusNotFound, "No feedback row updated for this lead/class/session combo")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "success"})
}

// GET /api/student-success/feedback-collected?class_key=...
func (h *APIHandler) GetFeedbackCollected(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden")
		return
	}
	classKey := r.URL.Query().Get("class_key")
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}
	uploads, err := models.GetFeedbackCollectedUploadsByClass(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to load feedback uploads: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load feedback uploads")
		return
	}

	type UploadResp struct {
		ID            string  `json:"id"`
		LeadID        string  `json:"lead_id"`
		ClassKey      string  `json:"class_key"`
		SessionNumber *int32  `json:"session_number,omitempty"`
		FileName      string  `json:"file_name"`
		FileURL       string  `json:"file_url"`
		MimeType      *string `json:"mime_type,omitempty"`
		SizeBytes     *int32  `json:"size_bytes,omitempty"`
		Note          *string `json:"note,omitempty"`
		UploadedBy    *string `json:"uploaded_by,omitempty"`
		UploadedAt    string  `json:"uploaded_at"`
	}

	out := make([]UploadResp, 0, len(uploads))
	for _, u := range uploads {
		var sn *int32
		if u.SessionNumber.Valid {
			val := u.SessionNumber.Int32
			sn = &val
		}
		var mt *string
		if u.MimeType.Valid {
			val := u.MimeType.String
			mt = &val
		}
		var sz *int32
		if u.SizeBytes.Valid {
			val := u.SizeBytes.Int32
			sz = &val
		}
		var note *string
		if u.Note.Valid {
			val := u.Note.String
			note = &val
		}
		var uploadedBy *string
		if u.UploadedByUser.Valid {
			val := u.UploadedByUser.String
			uploadedBy = &val
		}
		out = append(out, UploadResp{
			ID:            u.ID.String(),
			LeadID:        u.LeadID.String(),
			ClassKey:      u.ClassKey,
			SessionNumber: sn,
			FileName:      u.FileName,
			FileURL:       u.FileURL,
			MimeType:      mt,
			SizeBytes:     sz,
			Note:          note,
			UploadedBy:    uploadedBy,
			UploadedAt:    u.UploadedAt.Format(time.RFC3339),
		})
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"uploads": out})
}

// POST /api/student-success/feedback-collected (multipart form)
func (h *APIHandler) UploadFeedbackCollected(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden")
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid multipart form")
		return
	}

	classKey := r.FormValue("class_key")
	leadIDStr := r.FormValue("lead_id")
	if classKey == "" || leadIDStr == "" {
		jsonError(w, http.StatusBadRequest, "class_key and lead_id are required")
		return
	}
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead_id")
		return
	}
	var sessionNumber *int32
	if snStr := r.FormValue("session_number"); snStr != "" {
		if snInt, err := strconv.Atoi(snStr); err == nil {
			snVal := int32(snInt)
			sessionNumber = &snVal
		}
	}
	note := r.FormValue("note")

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer func() {
		_ = file.Close()
	}()

	// Save file under web/static/uploads/feedback_collected
	workDir, _ := os.Getwd()
	uploadDir := filepath.Join(workDir, "web", "static", "uploads", "feedback_collected")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}
	ext := filepath.Ext(header.Filename)
	fileID := uuid.New().String()
	fileName := fileID + ext
	dstPath := filepath.Join(uploadDir, fileName)

	dst, err := os.Create(dstPath)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}
	defer func() {
		_ = dst.Close()
	}()
	size, err := io.Copy(dst, file)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "Failed to write file")
		return
	}

	fileURL := "/static/uploads/feedback_collected/" + fileName
	uploadedByID, _ := uuid.Parse(middleware.GetUserID(r))

	record, err := models.CreateFeedbackCollectedUpload(
		leadID, classKey, sessionNumber, header.Filename, fileURL, header.Header.Get("Content-Type"), size, note, uploadedByID,
	)
	if err != nil {
		log.Printf("ERROR: Failed to save feedback upload: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to save feedback upload")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"id":        record.ID.String(),
		"file_url":  record.FileURL,
		"file_name": record.FileName,
	})
}

// DELETE /api/student-success/feedback-collected/:id
func (h *APIHandler) DeleteFeedbackCollected(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "mentor_head" && role != "admin" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden")
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		jsonError(w, http.StatusBadRequest, "Invalid upload id")
		return
	}
	uploadID, err := uuid.Parse(pathParts[3])
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid upload id")
		return
	}

	record, err := models.GetFeedbackCollectedUploadByID(uploadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, http.StatusNotFound, "Upload not found")
			return
		}
		log.Printf("ERROR: Failed to load feedback upload: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load upload")
		return
	}

	if record.FileURL != "" {
		workDir, _ := os.Getwd()
		if strings.HasPrefix(record.FileURL, "/static/") {
			relPath := strings.TrimPrefix(record.FileURL, "/static/")
			targetPath := filepath.Clean(filepath.Join(workDir, "web", "static", relPath))
			allowedRoot := filepath.Clean(filepath.Join(workDir, "web", "static", "uploads", "feedback_collected"))
			if strings.HasPrefix(targetPath, allowedRoot) {
				if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
					log.Printf("WARNING: Failed to delete feedback file %s: %v", targetPath, err)
				}
			} else {
				log.Printf("WARNING: Refused to delete feedback file outside uploads: %s", targetPath)
			}
		}
	}

	if err := models.DeleteFeedbackCollectedUpload(uploadID); err != nil {
		log.Printf("ERROR: Failed to delete feedback upload: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to delete upload")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// ========== COMPLAINTS API ENDPOINTS ==========

// POST /api/student-success/complaints - Create complaint
func (h *APIHandler) CreateComplaint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "admin" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success access required")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req struct {
		ClassKey      string `json:"class_key"`
		StudentPhone  string `json:"student_phone"`
		Category      string `json:"category"`
		ComplaintText string `json:"complaint_text"`
		Urgency       string `json:"urgency"` // low, medium, high
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ClassKey == "" || req.StudentPhone == "" || req.Category == "" || req.ComplaintText == "" {
		jsonError(w, http.StatusBadRequest, "Missing required fields: class_key, student_phone, category, complaint_text")
		return
	}

	// Default urgency to medium if not specified
	if req.Urgency == "" {
		req.Urgency = "medium"
	}

	complaint, err := models.CreateComplaint(req.ClassKey, req.StudentPhone, req.Category, req.ComplaintText, req.Urgency, userID)
	if err != nil {
		log.Printf("ERROR: Failed to create complaint: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to create complaint")
		return
	}

	jsonResponse(w, http.StatusCreated, complaint)
}

// GET /api/student-success/followups?class_key=...&show_resolved=0|1
// Modified to include complaints alongside absence escalations
func (h *APIHandler) GetFollowUpsWithComplaints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "student_success" && role != "admin" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Student Success access required")
		return
	}

	classKey := r.URL.Query().Get("class_key")
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	showResolved := r.URL.Query().Get("show_resolved") == "1"

	followUps, err := models.GetFollowUpsWithComplaints(classKey, showResolved)
	if err != nil {
		log.Printf("ERROR: Failed to get follow-ups with complaints: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load follow-ups")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"followups": followUps,
	})
}

// GET /api/mentor-head/complaints?show_resolved=0|1 - List all complaints
func (h *APIHandler) GetMentorHeadComplaints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "mentor_head" && role != "manager" && role != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head access required")
		return
	}

	showResolved := r.URL.Query().Get("show_resolved") == "1"

	complaints, err := models.GetComplaintsForMentorHead(showResolved)
	if err != nil {
		log.Printf("ERROR: Failed to get complaints: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load complaints")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"complaints": complaints,
	})
}

// POST /api/mentor-head/complaints/:id/update - Update complaint status and add note
func (h *APIHandler) UpdateComplaintStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "mentor_head" && role != "manager" && role != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head access required")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	// Extract complaint ID from URL path: /api/mentor-head/complaints/{id}/update
	path := r.URL.Path
	prefix := "/api/mentor-head/complaints/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, "/update") {
		jsonError(w, http.StatusBadRequest, "Invalid URL format")
		return
	}
	idPart := strings.TrimPrefix(path, prefix)
	idPart = strings.TrimSuffix(idPart, "/update")

	complaintID, err := uuid.Parse(idPart)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid complaint ID")
		return
	}

	var req struct {
		Status string `json:"status"` // contacted, investigating
		Note   string `json:"note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Status == "" {
		jsonError(w, http.StatusBadRequest, "Status is required")
		return
	}

	if err := models.UpdateComplaintStatus(complaintID, req.Status, req.Note, userID); err != nil {
		log.Printf("ERROR: Failed to update complaint status: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to update complaint")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

// POST /api/mentor-head/complaints/:id/resolve - Resolve complaint with required note
func (h *APIHandler) ResolveComplaintHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "mentor_head" && role != "manager" && role != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Mentor Head access required")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	// Extract complaint ID from URL path: /api/mentor-head/complaints/{id}/resolve
	path := r.URL.Path
	prefix := "/api/mentor-head/complaints/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, "/resolve") {
		jsonError(w, http.StatusBadRequest, "Invalid URL format")
		return
	}
	idPart := strings.TrimPrefix(path, prefix)
	idPart = strings.TrimSuffix(idPart, "/resolve")

	complaintID, err := uuid.Parse(idPart)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid complaint ID")
		return
	}

	var req struct {
		ResolutionNote string `json:"resolution_note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ResolutionNote == "" {
		jsonError(w, http.StatusBadRequest, "Resolution note is required")
		return
	}

	if err := models.ResolveComplaint(complaintID, req.ResolutionNote, userID); err != nil {
		log.Printf("ERROR: Failed to resolve complaint: %v", err)
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

// GetOpsNotifications returns Manager/Mentor Head operational notification banners.
func (h *APIHandler) GetOpsNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "mentor_head" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden")
		return
	}

	userID, err := uuid.Parse(middleware.GetUserID(r))
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	summary, err := models.GetOpsNotificationSummary(userID, time.Now())
	if err != nil {
		log.Printf("ERROR: Failed to load ops notifications: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load notifications")
		return
	}

	jsonResponse(w, http.StatusOK, summary)
}

// MarkComplaintNotificationRead marks one complaint notification as read for the current user.
func (h *APIHandler) MarkComplaintNotificationRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "mentor_head" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden")
		return
	}

	userID, err := uuid.Parse(middleware.GetUserID(r))
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	prefix := "/api/notifications/complaints/"
	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/read") {
		jsonError(w, http.StatusBadRequest, "Invalid request path")
		return
	}
	idPart := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/read")
	complaintID, err := uuid.Parse(idPart)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid complaint ID")
		return
	}

	if err := models.MarkComplaintRead(userID, complaintID); err != nil {
		log.Printf("ERROR: Failed to mark complaint notification read: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to mark complaint read")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GetEligibleClassesForLateJoin returns classes a student is eligible to join.
func (h *APIHandler) GetEligibleClassesForLateJoin(w http.ResponseWriter, r *http.Request) {
	// Canonical path: /api/pre-enrolment/:leadId/late-join-eligible-classes
	parts := strings.Split(r.URL.Path, "/")
	var leadIDStr string
	if len(parts) >= 4 && parts[1] == "api" && parts[2] == "pre-enrolment" {
		leadIDStr = parts[3]
	}

	if leadIDStr == "" {
		leadIDStr = r.URL.Query().Get("id") // Fallback
	}
	if leadIDStr == "" {
		jsonError(w, http.StatusBadRequest, "leadId is required")
		return
	}
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead ID")
		return
	}

	eligible, err := models.GetEligibleClassesForLateJoin(leadID)
	if err != nil {
		if strings.Contains(err.Error(), "late join is only available for ready-to-start students") ||
			strings.Contains(err.Error(), "lead not found") ||
			strings.Contains(err.Error(), "student has no") {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("ERROR: Failed to get eligible classes: %v", err)
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"classes": eligible,
	})
}

// AddLateJoiner adds a student to an active class.
func (h *APIHandler) AddLateJoiner(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Admin access required")
		return
	}

	leadIDStr := r.URL.Query().Get("id")
	if leadIDStr == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 3 && parts[1] == "pre-enrolment" {
			leadIDStr = parts[2]
		}
	}
	if leadIDStr == "" {
		jsonError(w, http.StatusBadRequest, "leadId is required")
		return
	}
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead ID")
		return
	}

	var req struct {
		ClassKey string `json:"class_key"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.ClassKey == "" {
		jsonError(w, http.StatusBadRequest, "Class key is required")
		return
	}
	if len(req.Reason) < 10 {
		jsonError(w, http.StatusBadRequest, "Reason must be at least 10 characters long")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := models.AddLateJoiner(leadID, req.ClassKey, req.Reason, userID); err != nil {
		log.Printf("ERROR: Failed to add late joiner: %v", err)
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

// UndoLateJoiner reverts a late join action.
func (h *APIHandler) UndoLateJoiner(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Admin access required")
		return
	}

	leadIDStr := r.URL.Query().Get("id")
	if leadIDStr == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 3 && parts[1] == "pre-enrolment" {
			leadIDStr = parts[2]
		}
	}
	if leadIDStr == "" {
		jsonError(w, http.StatusBadRequest, "leadId is required")
		return
	}
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead ID")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if err := models.UndoLateJoiner(leadID, userID); err != nil {
		log.Printf("ERROR: Failed to undo late joiner: %v", err)
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *APIHandler) GetLateJoinNotifications(w http.ResponseWriter, r *http.Request) {
	userIDStr := middleware.GetUserID(r)
	if userIDStr == "" {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	userID, _ := uuid.Parse(userIDStr)

	notifications, err := models.GetPendingLateJoinerNotifications(userID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(notifications); err != nil {
		log.Printf("ERROR: Failed to encode late-join notifications response: %v", err)
	}
}

func (h *APIHandler) AcknowledgeLateJoinNotification(w http.ResponseWriter, r *http.Request) {
	userIDStr := middleware.GetUserID(r)
	if userIDStr == "" {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	userID, _ := uuid.Parse(userIDStr)

	// Parse notification ID from path parts: /api/notifications/late-join/:id/acknowledge
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 {
		jsonError(w, http.StatusBadRequest, "Invalid request path")
		return
	}
	idStr := parts[3]
	notificationID, err := uuid.Parse(idStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid notification ID")
		return
	}

	err = models.AcknowledgeLateJoinerNotification(notificationID, userID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

// POST /api/compliance/check - upsert compliance check for a class session
func (h *APIHandler) UpsertComplianceCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		ClassSessionID string `json:"class_session_id"`
		Reminder1D     bool   `json:"reminder_1d"`
		Reminder1H     bool   `json:"reminder_1h"`
		ReminderTasks  bool   `json:"reminder_tasks"`
		DelayMinutes   int    `json:"delay_minutes"`
		IsAbsent       bool   `json:"is_absent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if strings.TrimSpace(req.ClassSessionID) == "" {
		jsonError(w, http.StatusBadRequest, "class_session_id is required")
		return
	}
	classSessionID, err := uuid.Parse(req.ClassSessionID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid class_session_id")
		return
	}
	if req.DelayMinutes < 0 {
		jsonError(w, http.StatusBadRequest, "delay_minutes must be >= 0")
		return
	}

	userIDStr := middleware.GetUserID(r)
	checkedByUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	row, err := models.UpsertMentorSessionCheck(
		classSessionID,
		checkedByUserID,
		req.Reminder1D,
		req.Reminder1H,
		req.ReminderTasks,
		req.DelayMinutes,
		req.IsAbsent,
	)
	if err != nil {
		log.Printf("ERROR: Failed to upsert compliance check: %v", err)
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			jsonError(w, http.StatusBadRequest, "Invalid class_session_id")
			return
		}
		jsonError(w, http.StatusInternalServerError, "Failed to save compliance check")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"check":   row,
	})
}

// GET /api/compliance/class/:class_key - returns compliance data for all 8 sessions of class
func (h *APIHandler) GetComplianceByClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	prefix := "/api/compliance/class/"
	if !strings.HasPrefix(r.URL.Path, prefix) || len(r.URL.Path) <= len(prefix) {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	encoded := strings.TrimPrefix(r.URL.Path, prefix)
	classKey, err := url.PathUnescape(encoded)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid class_key")
		return
	}
	classKey = strings.TrimSpace(classKey)
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key is required")
		return
	}

	sessions, err := models.GetComplianceByClassKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to load class compliance: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load class compliance")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"class_key": classKey,
		"sessions":  sessions,
	})
}

// GET /api/reports/mentors?round_status=active|closed&mentor_id=<uuid>
func (h *APIHandler) GetMentorReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	roundStatus := strings.TrimSpace(r.URL.Query().Get("round_status"))
	if roundStatus == "" {
		// Unified report is now the live/running view.
		roundStatus = "active"
	}
	if roundStatus != "" && roundStatus != "active" && roundStatus != "closed" {
		jsonError(w, http.StatusBadRequest, "round_status must be active or closed")
		return
	}

	var mentorID *uuid.UUID
	mentorIDStr := strings.TrimSpace(r.URL.Query().Get("mentor_id"))
	if mentorIDStr != "" {
		id, err := uuid.Parse(mentorIDStr)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid mentor_id")
			return
		}
		mentorID = &id
	}

	rows, err := models.GetMentorComplianceReports(roundStatus, mentorID)
	if err != nil {
		log.Printf("ERROR: Failed to load mentor reports: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load mentor reports")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"items": rows,
		"filters": map[string]interface{}{
			"round_status": roundStatus,
			"mentor_id":    mentorIDStr,
		},
	})
}

// POST /api/reports/mentors/exclude
func (h *APIHandler) ExcludeMentorReportRow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		MentorID    string `json:"mentor_id"`
		RoundStatus string `json:"round_status"`
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if strings.TrimSpace(req.MentorID) == "" {
		jsonError(w, http.StatusBadRequest, "mentor_id is required")
		return
	}
	if req.RoundStatus != "" && req.RoundStatus != "all" && req.RoundStatus != "active" && req.RoundStatus != "closed" {
		jsonError(w, http.StatusBadRequest, "round_status must be all, active, or closed")
		return
	}

	mentorID, err := uuid.Parse(req.MentorID)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid mentor_id")
		return
	}
	userID, err := uuid.Parse(middleware.GetUserID(r))
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var opErr error
	if req.RoundStatus == "" || req.RoundStatus == "all" {
		opErr = models.ExcludeMentorFromReportsAll(mentorID, userID, req.Reason)
	} else {
		opErr = models.ExcludeMentorFromReports(mentorID, req.RoundStatus, userID, req.Reason)
	}
	if opErr != nil {
		log.Printf("ERROR: Failed to exclude mentor row: %v", opErr)
		jsonError(w, http.StatusInternalServerError, "Failed to remove mentor row")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"success": true})
}

// GET /api/reports/mentors/checklist?mentor_id=<uuid>&round_status=active|closed(optional)
func (h *APIHandler) GetMentorReportChecklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	mentorIDStr := strings.TrimSpace(r.URL.Query().Get("mentor_id"))
	if mentorIDStr == "" {
		jsonError(w, http.StatusBadRequest, "mentor_id is required")
		return
	}
	mentorID, err := uuid.Parse(mentorIDStr)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid mentor_id")
		return
	}

	roundStatus := strings.TrimSpace(r.URL.Query().Get("round_status"))
	if roundStatus == "" {
		// Keep checklist aligned with the unified report scope.
		roundStatus = "active"
	}
	if roundStatus != "" && roundStatus != "active" && roundStatus != "closed" {
		jsonError(w, http.StatusBadRequest, "round_status must be active or closed")
		return
	}

	rows, err := models.GetMentorComplianceChecklist(mentorID, roundStatus)
	if err != nil {
		log.Printf("ERROR: Failed to load mentor checklist details: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load mentor checklist details")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"items": rows,
	})
}

// GET /api/reports/mentors/classes?round_status=active|closed(optional)&mentor_id=<uuid>(optional)
func (h *APIHandler) GetMentorClassReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	roundStatus := strings.TrimSpace(r.URL.Query().Get("round_status"))
	if roundStatus == "" {
		roundStatus = "active"
	}
	if roundStatus != "active" && roundStatus != "closed" {
		jsonError(w, http.StatusBadRequest, "round_status must be active or closed")
		return
	}

	var mentorID *uuid.UUID
	mentorIDStr := strings.TrimSpace(r.URL.Query().Get("mentor_id"))
	if mentorIDStr != "" {
		id, err := uuid.Parse(mentorIDStr)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid mentor_id")
			return
		}
		mentorID = &id
	}

	rows, err := models.GetMentorClassComplianceReports(roundStatus, mentorID)
	if err != nil {
		log.Printf("ERROR: Failed to load mentor class reports: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load mentor class reports")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"items": rows,
		"filters": map[string]interface{}{
			"round_status": roundStatus,
			"mentor_id":    mentorIDStr,
		},
	})
}

// GET /api/reports/daily?date=YYYY-MM-DD
func (h *APIHandler) GetDailyReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "mentor_head" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden")
		return
	}

	reportDate, _ := models.LatestReadyDailyReportWindow(time.Now())
	if raw := strings.TrimSpace(r.URL.Query().Get("date")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid date format (expected YYYY-MM-DD)")
			return
		}
		reportDate = parsed
	}

	report, err := models.GetDailyReportPayload(reportDate)
	if err != nil {
		log.Printf("ERROR: Failed to load daily report: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load daily report")
		return
	}

	jsonResponse(w, http.StatusOK, report)
}

// POST /api/reports/daily/read
func (h *APIHandler) MarkDailyReportRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	role := middleware.GetUserRole(r)
	if role != "mentor_head" && role != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden")
		return
	}

	userID, err := uuid.Parse(middleware.GetUserID(r))
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		ReportDate string `json:"report_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	reportDate, err := time.Parse("2006-01-02", strings.TrimSpace(req.ReportDate))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid report_date format (expected YYYY-MM-DD)")
		return
	}

	if err := models.MarkDailyReportRead(userID, reportDate); err != nil {
		log.Printf("ERROR: Failed to mark daily report read: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to mark daily report read")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/reports/bi?from=YYYY-MM-DD&to=YYYY-MM-DD
func (h *APIHandler) GetBIReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	now := time.Now()
	defaultFrom := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -5, 0)
	defaultTo := now

	fromDate := defaultFrom
	toDate := defaultTo

	if fromRaw := strings.TrimSpace(r.URL.Query().Get("from")); fromRaw != "" {
		parsed, err := time.Parse("2006-01-02", fromRaw)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid from date format (expected YYYY-MM-DD)")
			return
		}
		fromDate = parsed
	}

	if toRaw := strings.TrimSpace(r.URL.Query().Get("to")); toRaw != "" {
		parsed, err := time.Parse("2006-01-02", toRaw)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "Invalid to date format (expected YYYY-MM-DD)")
			return
		}
		toDate = parsed
	}

	if toDate.Before(fromDate) {
		jsonError(w, http.StatusBadRequest, "to date must be on or after from date")
		return
	}

	report, err := models.GetBIReportPayload(fromDate, toDate)
	if err != nil {
		log.Printf("ERROR: Failed to load BI reports: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load BI reports")
		return
	}

	jsonResponse(w, http.StatusOK, report)
}
