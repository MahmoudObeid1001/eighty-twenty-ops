package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

// Student Profile Handlers (Milestone 4)

func ensureMentorStudentAccess(r *http.Request, leadID uuid.UUID) (bool, error) {
	if middleware.GetUserRole(r) != "mentor" {
		return true, nil
	}
	mentorUserID, err := uuid.Parse(middleware.GetUserID(r))
	if err != nil {
		return false, err
	}
	return models.MentorHasStudentAccess(mentorUserID, leadID)
}

// SearchStudents handles GET /api/students/search?q=<query>
func SearchStudents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	role := middleware.GetUserRole(r)
	var (
		results []*models.StudentSearchResult
		err     error
	)
	if role == "mentor" {
		mentorUserID, parseErr := uuid.Parse(middleware.GetUserID(r))
		if parseErr != nil {
			http.Error(w, "Invalid mentor session", http.StatusUnauthorized)
			return
		}
		results, err = models.SearchStudentsForMentor(query, mentorUserID)
	} else {
		results, err = models.SearchStudents(query)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		log.Printf("ERROR: Failed to encode search students response: %v", err)
	}
}

// parseStudentID extracts the student ID from the URL path
func parseStudentID(path string) (uuid.UUID, error) {
	// Path format: /api/students/:id or /api/students/:id/...
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		return uuid.Nil, http.ErrNotSupported
	}
	return uuid.Parse(parts[2])
}

// GetStudentProfile handles GET /api/students/:id/profile
func GetStudentProfile(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}
	allowed, err := ensureMentorStudentAccess(r, leadID)
	if err != nil {
		http.Error(w, "Failed to verify access", http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	profile, err := models.GetStudentProfile(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(profile); err != nil {
		log.Printf("ERROR: Failed to encode student profile response: %v", err)
	}
}

// UpdateStudentBasicInfo handles PUT /api/students/:id/basic-info
func UpdateStudentBasicInfo(w http.ResponseWriter, r *http.Request) {
	userRole := middleware.GetUserRole(r)
	if userRole != "admin" && userRole != "mentor_head" && userRole != "manager" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	var req struct {
		FullName string `json:"full_name"`
		Phone    string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.FullName = strings.TrimSpace(req.FullName)
	req.Phone = strings.TrimSpace(req.Phone)
	if req.FullName == "" || req.Phone == "" {
		http.Error(w, "full_name and phone are required", http.StatusBadRequest)
		return
	}

	if err := models.UpdateStudentBasicInfo(leadID, req.FullName, req.Phone); err != nil {
		if phoneErr := models.IsPhoneConstraintError(err); phoneErr != nil {
			http.Error(w, phoneErr.Message, http.StatusConflict)
			return
		}
		if err == sql.ErrNoRows {
			http.Error(w, "Student not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to update student basic info", http.StatusInternalServerError)
		return
	}

	profile, err := models.GetStudentProfile(leadID)
	if err != nil {
		http.Error(w, "Student updated, but profile reload failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(profile); err != nil {
		log.Printf("ERROR: Failed to encode updated student profile response: %v", err)
	}
}

// GetStudentHistory handles GET /api/students/:id/history
func GetStudentHistory(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}
	allowed, err := ensureMentorStudentAccess(r, leadID)
	if err != nil {
		http.Error(w, "Failed to verify access", http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	history, err := models.GetAcademicHistory(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(history); err != nil {
		log.Printf("ERROR: Failed to encode student history response: %v", err)
	}
}

// GetCurrentStatus handles GET /api/students/:id/current-status
func GetCurrentStatus(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}
	allowed, err := ensureMentorStudentAccess(r, leadID)
	if err != nil {
		http.Error(w, "Failed to verify access", http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	status, err := models.GetCurrentClassStatus(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if status == nil {
		if err := json.NewEncoder(w).Encode(nil); err != nil {
			log.Printf("ERROR: Failed to encode nil current status response: %v", err)
		}
	} else {
		if err := json.NewEncoder(w).Encode(status); err != nil {
			log.Printf("ERROR: Failed to encode current status response: %v", err)
		}
	}
}

// GetStudentNotes handles GET /api/students/:id/notes
func GetStudentNotes(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}
	allowed, err := ensureMentorStudentAccess(r, leadID)
	if err != nil {
		http.Error(w, "Failed to verify access", http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	notes, err := models.GetStudentNotesTimeline(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(notes); err != nil {
		log.Printf("ERROR: Failed to encode student notes response: %v", err)
	}
}
