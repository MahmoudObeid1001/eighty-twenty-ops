package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

// Student Profile Handlers (Milestone 4)

// SearchStudents handles GET /api/students/search?q=<query>
func SearchStudents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	results, err := models.SearchStudents(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
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

	profile, err := models.GetStudentProfile(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// GetStudentHistory handles GET /api/students/:id/history
func GetStudentHistory(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	history, err := models.GetAcademicHistory(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// GetCurrentStatus handles GET /api/students/:id/current-status
func GetCurrentStatus(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	status, err := models.GetCurrentClassStatus(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if status == nil {
		json.NewEncoder(w).Encode(nil)
	} else {
		json.NewEncoder(w).Encode(status)
	}
}

// GetStudentNotes handles GET /api/students/:id/notes
func GetStudentNotes(w http.ResponseWriter, r *http.Request) {
	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	notes, err := models.GetStudentNotesTimeline(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notes)
}
