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

type StudentSuccessHandler struct {
	config *config.Config
}

func NewStudentSuccessHandler(cfg *config.Config) *StudentSuccessHandler {
	return &StudentSuccessHandler{config: cfg}
}

// Dashboard shows pending feedback and absence follow-up tasks
func (h *StudentSuccessHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		redirectWithError(w, r, "/student-success", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "student_success" && userRole != "admin" && userRole != "manager" {
		redirectWithError(w, r, "/student-success", "You don't have permission to do this.")
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

	flashMessage, flashMessageType := flashFromQuery(r)
	if flashMessage == "" {
		if r.URL.Query().Get("feedback_submitted") == "1" {
			flashMessage = "Feedback submitted."
			flashMessageType = "success"
		} else if r.URL.Query().Get("follow_up_logged") == "1" {
			flashMessage = "Follow-up logged."
			flashMessageType = "success"
		}
	}

	data := map[string]interface{}{
		"Title":            "Community Officer – Eighty Twenty",
		"PendingFeedback4": pending4,
		"PendingFeedback8": pending8,
		"IsAdmin":          userRole == "admin",
		"IsModerator":      userRole == "moderator",
		"FlashMessage":     flashMessage,
		"FlashMessageType": flashMessageType,
	}

	renderTemplate(w, r, "student_success.html", data)
}

// SubmitFeedback submits feedback for a student at session 4 or 8
func (h *StudentSuccessHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/student-success", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "student_success" && userRole != "admin" && userRole != "manager" {
		redirectWithError(w, r, "/student-success", "You don't have permission to do this.")
		return
	}

	leadIDStr := r.FormValue("lead_id")
	classKey := r.FormValue("class_key")
	sessionNumberStr := r.FormValue("session_number")
	feedbackText := r.FormValue("feedback_text")
	followUpRequiredStr := r.FormValue("follow_up_required")

	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		redirectWithError(w, r, "/student-success", "We couldn't find that student. Please refresh and try again.")
		return
	}

	sessionNumber, err := strconv.Atoi(sessionNumberStr)
	if err != nil || (sessionNumber != 4 && sessionNumber != 8) {
		redirectWithError(w, r, "/student-success", "Please choose session 4 or session 8.")
		return
	}

	followUpRequired := followUpRequiredStr == "true" || followUpRequiredStr == "1"

	createdByUserID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.SubmitFeedback(leadID, classKey, int32(sessionNumber), feedbackText, followUpRequired, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to submit feedback: %v", err)
		redirectWithError(w, r, "/student-success", "Couldn't submit feedback. Please try again.")
		return
	}

	http.Redirect(w, r, "/student-success?feedback_submitted=1", http.StatusFound)
}

// LogFollowUp logs an absence follow-up action
func (h *StudentSuccessHandler) LogFollowUp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		redirectWithError(w, r, "/student-success", "This action isn't available.")
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "student_success" && userRole != "admin" && userRole != "manager" {
		redirectWithError(w, r, "/student-success", "You don't have permission to do this.")
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
		redirectWithError(w, r, "/student-success", "We couldn't find that student. Please refresh and try again.")
		return
	}

	var sessionID uuid.UUID
	if sessionIDStr != "" {
		sessionID, err = uuid.Parse(sessionIDStr)
		if err != nil {
			redirectWithError(w, r, "/student-success", "Please choose a valid session.")
			return
		}
	}

	messageSent := messageSentStr == "true" || messageSentStr == "1"

	createdByUserID, _ := uuid.Parse(middleware.GetUserID(r))
	if err := models.LogAbsenceFollowUp(leadID, sessionID, messageSent, reason, studentReply, actionTaken, notes, createdByUserID); err != nil {
		log.Printf("ERROR: Failed to log follow-up: %v", err)
		redirectWithError(w, r, "/student-success", "Couldn't save this follow-up. Please try again.")
		return
	}

	http.Redirect(w, r, "/student-success?follow_up_logged=1", http.StatusFound)
}
