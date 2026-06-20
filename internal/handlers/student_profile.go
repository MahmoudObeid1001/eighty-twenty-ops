package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
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

func containsArabicText(text string) bool {
	for _, r := range text {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) || (r >= 0x08A0 && r <= 0x08FF) {
			return true
		}
	}
	return false
}

func looksEnglishGradeNote(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || containsArabicText(trimmed) {
		return false
	}

	latinLetters := 0
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			latinLetters++
		}
	}

	return latinLetters >= 8
}

func translateFinalGradeTextsToArabic(texts []string) []string {
	cfg := config.Load()
	if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
		return nil
	}

	inputJSON, _ := json.Marshal(map[string][]string{"notes": texts})
	requestBody := map[string]interface{}{
		"model": cfg.OpenAIModel,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "Translate mentor final grade notes into clear Egyptian Arabic for admins. Preserve meaning. Return JSON only: {\"notes_ar\":[...]} with the same order and count.",
			},
			{
				"role":    "user",
				"content": string(inputJSON),
			},
		},
		"temperature": 0.1,
	}

	rawBody, _ := json.Marshal(requestBody)
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(rawBody))
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAIAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil
	}
	if len(completion.Choices) == 0 {
		return nil
	}

	var parsed struct {
		NotesAR []string `json:"notes_ar"`
	}
	content := sanitizeJSONText(completion.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil
	}
	if len(parsed.NotesAR) != len(texts) {
		return nil
	}

	out := make([]string, len(parsed.NotesAR))
	for i, translated := range parsed.NotesAR {
		out[i] = strings.TrimSpace(translated)
	}
	return out
}

func translateSingleFinalGradeTextToArabic(text string) string {
	trimmed := strings.TrimSpace(text)
	if !looksEnglishGradeNote(trimmed) {
		return ""
	}

	translated := translateFinalGradeTextsToArabic([]string{trimmed})
	if len(translated) != 1 {
		return ""
	}
	if translated[0] == "" || translated[0] == trimmed {
		return ""
	}
	return translated[0]
}

func translateGradeNotesToArabic(notes []*models.TimelineItem) {
	targets := make([]*models.TimelineItem, 0, len(notes))
	inputNotes := make([]string, 0, len(notes))
	for _, item := range notes {
		if item == nil || item.Type != "grade_note" || !looksEnglishGradeNote(item.Text) {
			continue
		}
		targets = append(targets, item)
		inputNotes = append(inputNotes, strings.TrimSpace(item.Text))
	}
	if len(targets) == 0 {
		return
	}

	translated := translateFinalGradeTextsToArabic(inputNotes)
	if len(translated) != len(targets) {
		return
	}

	for i, text := range translated {
		if text == "" || text == targets[i].Text {
			continue
		}
		targets[i].TranslatedText = text
	}
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
		Gender   string `json:"gender"`
		Phone    string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.FullName = strings.TrimSpace(req.FullName)
	req.Gender = strings.TrimSpace(req.Gender)
	req.Phone = strings.TrimSpace(req.Phone)
	if req.FullName == "" || req.Phone == "" {
		http.Error(w, "full_name and phone are required", http.StatusBadRequest)
		return
	}

	if err := models.UpdateStudentBasicInfo(leadID, req.FullName, req.Gender, req.Phone); err != nil {
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
	translateGradeNotesToArabic(notes)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(notes); err != nil {
		log.Printf("ERROR: Failed to encode student notes response: %v", err)
	}
}

// GetStudentPaymentHistory handles GET /api/students/:id/payment-history.
// Payment history is finance-sensitive and is restricted to admin and manager.
func GetStudentPaymentHistory(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetUserRole(r)
	if role != "admin" && role != "manager" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	leadID, err := parseStudentID(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid student ID", http.StatusBadRequest)
		return
	}

	history, err := models.GetStudentPaymentHistory(leadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(history); err != nil {
		log.Printf("ERROR: Failed to encode student payment history response: %v", err)
	}
}
