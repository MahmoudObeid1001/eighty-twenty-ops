package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"eighty-twenty-ops/internal/models"
)

type landingLeadRequest struct {
	FullName        string `json:"full_name"`
	WhatsAppNumber  string `json:"whatsapp_number"`
	EnglishLevel    string `json:"english_level"`
	LearningGoal    string `json:"learning_goal"`
	Source          string `json:"source"`
	CurrentJob      string `json:"current_job"`
	CurrentLevel    string `json:"current_level"`
	EnglishNeed     string `json:"english_need"`
	SelectedPackage string `json:"selected_package"`
}

func (h *APIHandler) CreateLandingLead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if strings.TrimSpace(h.cfg.LandingLeadToken) == "" {
		jsonError(w, http.StatusServiceUnavailable, "Landing lead intake is not configured")
		return
	}

	token := r.Header.Get("X-Landing-Lead-Token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(h.cfg.LandingLeadToken)) != 1 {
		jsonError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()

	var req landingLeadRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	fullName := strings.TrimSpace(req.FullName)
	phone := strings.TrimSpace(req.WhatsAppNumber)
	level := normalizeLandingEnglishLevel(req.EnglishLevel)
	learningGoal := normalizeLandingLearningGoal(req.LearningGoal)
	if fullName == "" || phone == "" || strings.TrimSpace(req.LearningGoal) == "" {
		jsonError(w, http.StatusBadRequest, "full_name, whatsapp_number, and learning_goal are required")
		return
	}
	if strings.TrimSpace(req.EnglishLevel) != "" && level == "" {
		jsonError(w, http.StatusBadRequest, "english_level must be beginner, intermediate, or advanced")
		return
	}
	if strings.TrimSpace(req.LearningGoal) != "" && learningGoal == "" {
		jsonError(w, http.StatusBadRequest, "learning_goal must be one of the supported landing page options")
		return
	}

	notes := "Landing page signup"
	lead, err := models.CreateLandingLeadWithMetadata(
		fullName,
		phone,
		notes,
		learningGoal,
		level,
		strings.TrimSpace(req.Source),
		strings.TrimSpace(req.CurrentJob),
		strings.TrimSpace(req.CurrentLevel),
		strings.TrimSpace(req.EnglishNeed),
		strings.TrimSpace(req.SelectedPackage),
	)
	if err != nil {
		var phoneErr *models.PhoneAlreadyExistsError
		if errors.As(err, &phoneErr) || models.IsPhoneConstraintError(err) != nil {
			jsonError(w, http.StatusConflict, "Phone number already exists")
			return
		}
		jsonError(w, http.StatusInternalServerError, "Failed to create lead")
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]interface{}{
		"id":     lead.ID.String(),
		"status": lead.Status,
	})
}

func normalizeLandingEnglishLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "beginner":
		return "beginner"
	case "intermediate":
		return "intermediate"
	case "advanced":
		return "advanced"
	default:
		return ""
	}
}

func normalizeLandingLearningGoal(raw string) string {
	switch strings.TrimSpace(raw) {
	case "الشغل والترقي":
		return "الشغل والترقي"
	case "السفر والهجرة":
		return "السفر والهجرة"
	case "الدراسة والمنح":
		return "الدراسة والمنح"
	case "ازاكر للأولاد":
		return "ازاكر للأولاد"
	case "غير كده":
		return "غير كده"
	default:
		return ""
	}
}
