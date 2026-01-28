package handlers

import (
	"log"
	"net/http"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type HRHandler struct {
	config *config.Config
}

func NewHRHandler(cfg *config.Config) *HRHandler {
	return &HRHandler{config: cfg}
}

// MentorsList renders the HR mentors page (form to create mentor users).
func (h *HRHandler) MentorsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "hr" && userRole != "admin" {
		http.Error(w, "Forbidden: HR or Admin access required", http.StatusForbidden)
		return
	}

	data := map[string]interface{}{
		"Title":       "HR · Mentors – Eighty Twenty",
		"IsAdmin":     userRole == "admin",
		"IsModerator": userRole == "moderator",
		"UserRole":    userRole,
		"created":     r.URL.Query().Get("created"),
		"error":       r.URL.Query().Get("error"),
	}
	renderTemplate(w, r, "hr_mentors.html", data)
}

// MentorsCreate creates a new mentor user (POST).
func (h *HRHandler) MentorsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userRole := middleware.GetUserRole(r)
	if userRole != "hr" && userRole != "admin" {
		http.Error(w, "Forbidden: HR or Admin access required", http.StatusForbidden)
		return
	}

	emailRaw := r.FormValue("email")
	password := r.FormValue("password")
	email := normalizeEmail(emailRaw)
	if email == "" || password == "" {
		http.Redirect(w, r, "/hr/mentors?error=email_and_password_required", http.StatusFound)
		return
	}

	if len(password) < 6 {
		http.Redirect(w, r, "/hr/mentors?error=password_too_short", http.StatusFound)
		return
	}

	_, err := models.GetUserByEmail(email)
	if err == nil {
		http.Redirect(w, r, "/hr/mentors?error=email_exists", http.StatusFound)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("ERROR: HR create mentor: hash failed: %v", err)
		http.Redirect(w, r, "/hr/mentors?error=create_failed", http.StatusFound)
		return
	}

	_, err = models.CreateUser(email, string(hashed), "mentor")
	if err != nil {
		log.Printf("ERROR: HR create mentor: db insert failed email=%q: %v", email, err)
		http.Redirect(w, r, "/hr/mentors?error=create_failed", http.StatusFound)
		return
	}

	http.Redirect(w, r, "/hr/mentors?created=1", http.StatusFound)
}
