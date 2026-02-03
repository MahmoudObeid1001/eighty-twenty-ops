package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

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

	// Validate grade is A/B/C
	if req.Grade != "A" && req.Grade != "B" && req.Grade != "C" {
		jsonError(w, http.StatusBadRequest, "Grade must be A, B, or C")
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
	} else if userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Only mentors and admins can create grades")
		return
	}

	// Create grade (session 8 is set automatically)
	gradeID, err := models.InsertGrade(leadID, req.ClassKey, req.Grade, req.Notes, userID)
	if err != nil {
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

	// Validate grade is A/B/C
	if req.Grade != "A" && req.Grade != "B" && req.Grade != "C" {
		jsonError(w, http.StatusBadRequest, "Grade must be A, B, or C")
		return
	}

	userIDStr := middleware.GetUserID(r)
	userID, _ := uuid.Parse(userIDStr)

	err = models.UpdateGrade(gradeID, req.Grade, req.Notes, userID)
	if err != nil {
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
	} else if userRole != "mentor_head" && userRole != "student_success" && userRole != "admin" {
		jsonError(w, http.StatusForbidden, "Forbidden: Insufficient permissions")
		return
	}

	grades, err := models.GetGradesByClassKey(classKey)
	if err != nil {
		log.Printf("ERROR: Failed to get grades: %v", err)
		jsonError(w, http.StatusInternalServerError, "Failed to load grades")
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{"grades": grades})
}
