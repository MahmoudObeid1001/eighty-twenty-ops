package handlers

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

// DeleteNote deletes a note (mentors can only delete their own, mentor_head can delete any)
func (h *MentorHandler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	// Accept both DELETE and POST (HTML forms only support POST)
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "mentor" && userRole != "admin" && userRole != "mentor_head" {
		http.Error(w, "Forbidden: Mentor, Mentor Head, or Admin access required", http.StatusForbidden)
		return
	}

	// Get note_id from query (DELETE) or form (POST)
	noteIDStr := r.URL.Query().Get("note_id")
	if noteIDStr == "" {
		noteIDStr = r.FormValue("note_id")
	}
	if noteIDStr == "" {
		http.Error(w, "note_id is required", http.StatusBadRequest)
		return
	}

	noteID, err := uuid.Parse(noteIDStr)
	if err != nil {
		http.Error(w, "Invalid note_id", http.StatusBadRequest)
		return
	}

	// Get the note to check creator
	note, err := models.GetStudentNoteByID(noteID)
	if err != nil {
		log.Printf("ERROR: Failed to get note: %v", err)
		http.Error(w, "Failed to load note", http.StatusInternalServerError)
		return
	}
	if note == nil {
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}

	// Permission check: mentors can only delete their own notes, mentor_head/admin can delete any
	currentUserID := middleware.GetUserID(r)
	if userRole != "mentor_head" && userRole != "admin" {
		// Regular mentor: must be the creator
		if !note.CreatedByUserID.Valid || note.CreatedByUserID.String != currentUserID {
			http.Error(w, "Forbidden: You can only delete your own notes", http.StatusForbidden)
			return
		}
	}

	// Delete the note
	if err := models.DeleteStudentNote(noteID); err != nil {
		log.Printf("ERROR: Failed to delete note: %v", err)
		http.Error(w, "Failed to delete note", http.StatusInternalServerError)
		return
	}

	// Redirect back to class page (get from query or form)
	classKey := r.URL.Query().Get("class_key")
	if classKey == "" {
		classKey = r.FormValue("class_key")
	}
	sess := r.URL.Query().Get("session")
	if sess == "" {
		sess = r.FormValue("session")
	}
	studentID := r.URL.Query().Get("student_id")
	if studentID == "" {
		studentID = r.FormValue("student_id")
	}
	
	var basePath string
	if userRole == "mentor_head" {
		basePath = "/mentor-head/class"
	} else {
		basePath = "/mentor/class"
	}
	u := fmt.Sprintf("%s?class_key=%s&note_deleted=1", basePath, url.QueryEscape(classKey))
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
