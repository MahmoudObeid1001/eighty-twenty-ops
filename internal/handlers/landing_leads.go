package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"eighty-twenty-ops/internal/models"
)

type landingLeadRequest struct {
	FullName       string `json:"full_name"`
	WhatsAppNumber string `json:"whatsapp_number"`
	EnglishLevel   string `json:"english_level"`
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
	if fullName == "" || phone == "" {
		jsonError(w, http.StatusBadRequest, "full_name and whatsapp_number are required")
		return
	}
	if strings.TrimSpace(req.EnglishLevel) != "" && level == "" {
		jsonError(w, http.StatusBadRequest, "english_level must be beginner, intermediate, or advanced")
		return
	}

	notes := "Landing page signup"
	if level != "" {
		notes = fmt.Sprintf("%s\nEnglish level: %s", notes, level)
	}
	lead, err := models.CreateLead(fullName, phone, "Landing Page", notes, "")
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
