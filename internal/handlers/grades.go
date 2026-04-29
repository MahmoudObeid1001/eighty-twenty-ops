package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

type GradePreviewResponse struct {
	LeadID               string  `json:"lead_id"`
	Absences             int     `json:"absences"`
	CompletedTasks       int     `json:"completed_tasks"`
	AttendedSessions     int     `json:"attended_sessions"`
	AverageStars         float64 `json:"average_stars"`
	AttendanceScore      float64 `json:"attendance_score"`
	TaskScore            float64 `json:"task_score"`
	ParticipationScore   float64 `json:"participation_score"`
	TotalScore           float64 `json:"total_score"`
	CalculatedGrade      string  `json:"calculated_grade"`
	UsedLegacyTaskSafety bool    `json:"used_legacy_task_safety"`
}

func mapGradePreview(leadID uuid.UUID, p models.GradePreview) GradePreviewResponse {
	return GradePreviewResponse{
		LeadID:               leadID.String(),
		Absences:             p.Absences,
		CompletedTasks:       p.CompletedTasks,
		AttendedSessions:     p.AttendedSessions,
		AverageStars:         p.AverageParticipation,
		AttendanceScore:      p.AttendanceScore,
		TaskScore:            p.TaskScore,
		ParticipationScore:   p.ParticipationScore,
		TotalScore:           p.TotalScore,
		CalculatedGrade:      p.CalculatedGrade,
		UsedLegacyTaskSafety: p.UsedLegacyTaskFallback,
	}
}

// POST /api/mentor/grades - mentor creates grade for student in their class
func (h *APIHandler) CreateGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		LeadID   string `json:"lead_id"`
		ClassKey string `json:"class_key"`
		Grade    string `json:"grade"`
		Notes    string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.LeadID == "" || req.ClassKey == "" || req.Grade == "" {
		jsonError(w, http.StatusBadRequest, "lead_id, class_key, and grade are required")
		return
	}

	// Validate grade is A/B/C/F
	if req.Grade != "A" && req.Grade != "B" && req.Grade != "C" && req.Grade != "F" {
		jsonError(w, http.StatusBadRequest, "Grade must be A, B, C, or F")
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

	// Verify mentor has access to this class
	if userRole == "mentor" {
		assignment, err := models.GetMentorAssignment(req.ClassKey)
		if err != nil || assignment == nil || assignment.MentorUserID != userID {
			jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
			return
		}
	} else if userRole != "mentor_head" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Only mentors and mentor heads can create grades")
		return
	}

	// Validate mentor-submitted grade against calculated value (mentor head may override).
	previews, err := models.GetGradePreviewsByClass(req.ClassKey)
	if err != nil {
		log.Printf("ERROR: Failed to compute grade previews for class %s: %v", req.ClassKey, err)
		jsonError(w, http.StatusInternalServerError, "Failed to validate grade against calculated breakdown")
		return
	}
	preview, ok := previews[leadID]
	if !ok {
		jsonError(w, http.StatusBadRequest, "Student not found in class roster for grade calculation")
		return
	}
	if userRole != "mentor_head" && req.Grade != preview.CalculatedGrade {
		jsonError(w, http.StatusBadRequest, fmt.Sprintf("Submitted grade %s does not match calculated grade %s", req.Grade, preview.CalculatedGrade))
		return
	}

	// Create grade (session 8 is set automatically)
	gradeID, err := models.InsertGrade(leadID, req.ClassKey, req.Grade, req.Notes, userID)
	if err != nil {
		if strings.Contains(err.Error(), "at least 10 words") {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			jsonError(w, http.StatusConflict, "Grade already exists for this student in this class")
			return
		}
		log.Printf("ERROR: Failed to create grade: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to create grade")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"grade_id": gradeID.String(),
	})
}

// GET /api/grades/preview?class_key=X - calculated grading breakdown for class students
func (h *APIHandler) GetGradesPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	classKey := r.URL.Query().Get("class_key")
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key query parameter is required")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	if userRole == "mentor" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != userID {
			jsonError(w, http.StatusForbidden, "Forbidden: You can only view grade previews for your assigned classes")
			return
		}
	} else if userRole != "mentor_head" && userRole != "student_success" && userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	previews, err := models.GetGradePreviewsByClass(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to load grade previews: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load grade previews")
		return
	}

	resp := make([]GradePreviewResponse, 0, len(previews))
	for leadID, p := range previews {
		resp = append(resp, mapGradePreview(leadID, p))
	}
	jsonResponse(w, http.StatusOK, map[string]interface{}{"previews": resp})
}

// DELETE /api/grades?lead_id=...&class_key=... - mentor/mentor head clears grade
func (h *APIHandler) DeleteGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	leadIDRaw := r.URL.Query().Get("lead_id")
	classKey := r.URL.Query().Get("class_key")
	if leadIDRaw == "" || classKey == "" {
		jsonError(w, http.StatusBadRequest, "lead_id and class_key are required")
		return
	}

	leadID, err := uuid.Parse(leadIDRaw)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid lead_id")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	if userRole == "mentor" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != userID {
			jsonError(w, http.StatusForbidden, "Forbidden: You are not assigned to this class")
			return
		}
	} else if userRole != "mentor_head" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Only mentors and mentor heads can delete grades")
		return
	}

	if err := models.DeleteGrade(leadID, classKey); err != nil {
		log.Printf("ERROR: Failed to delete grade: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to delete grade")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// PUT /api/mentor-head/grades/:id - mentor head edits existing grade
func (h *APIHandler) UpdateGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Extract grade ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/mentor-head/grades/"), "/")
	if len(pathParts) < 1 || pathParts[0] == "" {
		jsonError(w, http.StatusBadRequest, "grade_id is required in path")
		return
	}

	gradeID, err := uuid.Parse(pathParts[0])
	if err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid grade_id")
		return
	}

	var req struct {
		Grade string `json:"grade"`
		Notes string `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Grade == "" {
		jsonError(w, http.StatusBadRequest, "grade is required")
		return
	}

	// Validate grade is A/B/C/F
	if req.Grade != "A" && req.Grade != "B" && req.Grade != "C" && req.Grade != "F" {
		jsonError(w, http.StatusBadRequest, "Grade must be A, B, C, or F")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	err = models.UpdateGrade(gradeID, req.Grade, req.Notes, userID)
	if err != nil {
		if strings.Contains(err.Error(), "at least 10 words") {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("ERROR: Failed to update grade: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to update grade")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /api/grades?class_key=X - view grades for a class (role-based access)
func (h *APIHandler) GetGradesForClass(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	classKey := r.URL.Query().Get("class_key")
	if classKey == "" {
		jsonError(w, http.StatusBadRequest, "class_key query parameter is required")
		return
	}

	userRole := middleware.GetUserRole(r)
	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	// Check access: mentor (own class), mentor_head, student_success, admin
	if userRole == "mentor" {
		assignment, err := models.GetMentorAssignment(classKey)
		if err != nil || assignment == nil || assignment.MentorUserID != userID {
			jsonError(w, http.StatusForbidden, "Forbidden: You can only view grades for your assigned classes")
			return
		}
	} else if userRole != "mentor_head" && userRole != "student_success" && userRole != "admin" && userRole != "manager" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	grades, err := models.GetGradesByClassKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get grades: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load grades")
		return
	}

	type GradeResp struct {
		ID            string `json:"id"`
		LeadID        string `json:"lead_id"`
		ClassKey      string `json:"class_key"`
		SessionNumber int32  `json:"session_number"`
		Grade         string `json:"grade"`
		Notes         string `json:"notes"`
		CreatedByUser string `json:"created_by_user_id"`
		CreatedAt     string `json:"created_at"`
	}

	out := make([]GradeResp, 0, len(grades))
	for _, g := range grades {
		notes := ""
		if g.Notes.Valid {
			notes = g.Notes.String
		}
		createdBy := ""
		if g.CreatedByUserID.Valid {
			createdBy = g.CreatedByUserID.String
		}
		out = append(out, GradeResp{
			ID:            g.ID.String(),
			LeadID:        g.LeadID.String(),
			ClassKey:      g.ClassKey,
			SessionNumber: g.SessionNumber,
			Grade:         g.Grade,
			Notes:         notes,
			CreatedByUser: createdBy,
			CreatedAt:     g.CreatedAt.Format(time.RFC3339),
		})
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"grades": out})
}
