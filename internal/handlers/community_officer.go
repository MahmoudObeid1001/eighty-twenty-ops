package handlers

import (
	"log"
	"net/http"
	"strconv"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

type CommunityOfficerHandler struct {
	config *config.Config
}

func NewCommunityOfficerHandler(cfg *config.Config) *CommunityOfficerHandler {
	return &CommunityOfficerHandler{config: cfg}
}

// Dashboard shows pending feedback and absence follow-up tasks
func (h *CommunityOfficerHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "community_officer" && userRole != "admin" {
		http.Error(w, "Forbidden: Community Officer or Admin access required", http.StatusForbidden)
		return
	}

	// Get pending feedback for sessions 4 and 8
	pending4, err := models.GetPendingFeedback(4)
	if err != nil {
		log.Printf("WARNING: Failed to get pending feedback for session 4: %v", err)
		pending4 = []struct {
			LeadID   uuid.UUID
			FullName string
			Phone    string
			ClassKey string
		}{}
	}

	pending8, err := models.GetPendingFeedback(8)
	if err != nil {
		log.Printf("WARNING: Failed to get pending feedback for session 8: %v", err)
		pending8 = []struct {
			LeadID   uuid.UUID
			FullName string
			Phone    string
			ClassKey string
		}{}
	}

	data := map[string]interface{}{
		"Title":               "Community Officer – Eighty Twenty",
		"PendingFeedback4":    pending4,
		"PendingFeedback8":    pending8,
		"IsAdmin":             userRole == "admin",
		"IsModerator":         userRole == "moderator",
		"feedback_submitted": r.URL.Query().Get("feedback_submitted"),
		"follow_up_logged":   r.URL.Query().Get("follow_up_logged"),
	}

	renderTemplate(w, r, "community_officer.html", data)
}

// SubmitFeedback submits feedback for a student at session 4 or 8
func (h *CommunityOfficerHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "community_officer" && userRole != "admin" {
		http.Error(w, "Forbidden: Community Officer or Admin access required", http.StatusForbidden)
		return
	}

	leadIDStr := r.FormValue("lead_id")
	classKey := r.FormValue("class_key")
	sessionNumberStr := r.FormValue("session_number")
	feedbackText := r.FormValue("feedback_text")
	followUpRequiredStr := r.FormValue("follow_up_required")

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		http.Error(w, "Invalid lead_id", http.StatusBadRequest)
		return
	}

	sessionNumber, err := strconv.Atoi(sessionNumberStr)
	if err != nil || (sessionNumber != 4 && sessionNumber != 8) {
		http.Error(w, "Invalid session_number. Must be 4 or 8", http.StatusBadRequest)
		return
	}

	followUpRequired := followUpRequiredStr == "true" || followUpRequiredStr == "1"

	createdByUserID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.SubmitFeedback(leadID, classKey, int32(sessionNumber), feedbackText, followUpRequired, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to submit feedback: %v", err)
		http.Error(w, "Failed to submit feedback", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/community-officer?feedback_submitted=1", http.StatusFound)
}

// LogFollowUp logs an absence follow-up action
func (h *CommunityOfficerHandler) LogFollowUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "community_officer" && userRole != "admin" {
		http.Error(w, "Forbidden: Community Officer or Admin access required", http.StatusForbidden)
		return
	}

	leadIDStr := r.FormValue("lead_id")
	sessionIDStr := r.FormValue("session_id")
	messageSentStr := r.FormValue("message_sent")
	reason := r.FormValue("reason")
	studentReply := r.FormValue("student_reply")
	actionTaken := r.FormValue("action_taken")
	notes := r.FormValue("notes")

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		http.Error(w, "Invalid lead_id", http.StatusBadRequest)
		return
	}

	var sessionID uuid.UUID
	if sessionIDStr != "" {
		sessionID, err = uuid.Parse(sessionIDStr)
		if err != nil {
			http.Error(w, "Invalid session_id", http.StatusBadRequest)
			return
		}
	}

	messageSent := messageSentStr == "true" || messageSentStr == "1"

	createdByUserID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.LogAbsenceFollowUp(leadID, sessionID, messageSent, reason, studentReply, actionTaken, notes, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to log follow-up: %v", err)
		http.Error(w, "Failed to log follow-up", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/community-officer?follow_up_logged=1", http.StatusFound)
}
