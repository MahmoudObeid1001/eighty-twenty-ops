package handlers

import (
	"net/http"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	cfg *config.Config
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

// LoginForm renders the login page for unauthenticated users.
// FIX: Added explicit logging to track session state and ensure auth_layout is used.
// The route is correctly registered before protected routes in main.go, so it's not
// caught by RequireAuth middleware. The template system already maps login.html to
// use auth_layout (not the main app layout), so unauthenticated users see the proper
// login form without the app sidebar.
func (h *AuthHandler) LoginForm(w http.ResponseWriter, r *http.Request) {
	h.cfg.Debugf("📝 LoginForm() called - rendering login.html template")
	h.cfg.Debugf("  → Request path: %s, Method: %s", r.URL.Path, r.Method)
	
	// Check if already logged in
	cookie, err := r.Cookie("eighty_twenty_session")
	sessionExists := err == nil
	h.cfg.Debugf("  → Session cookie exists: %v", sessionExists)
	
	if sessionExists {
		// Validate the session
		_, _, _, err := middleware.ValidateSessionCookie(cookie, h.cfg.SessionSecret)
		if err == nil {
			// Valid session, redirect to pre-enrolment
			h.cfg.Debugf("  → Valid session found, redirecting to /pre-enrolment")
			http.Redirect(w, r, "/pre-enrolment", http.StatusFound)
			return
		}
		h.cfg.Debugf("  → Session cookie exists but invalid: %v", err)
	}

	// No valid session - render login form with auth_layout
	h.cfg.Debugf("  → No valid session, rendering login.html with auth_layout")
	data := map[string]interface{}{
		"Title":       "Login - Eighty Twenty",
		"HideSidebar": true,
	}
	renderTemplate(w, "login.html", data)
	h.cfg.Debugf("  → Template render complete")
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	if email == "" || password == "" {
		data := map[string]interface{}{
			"Title": "Login - Eighty Twenty",
			"Error": "Email and password are required",
		}
		renderTemplate(w, "login.html", data)
		return
	}

	user, err := models.GetUserByEmail(email)
	if err != nil {
		data := map[string]interface{}{
			"Title": "Login - Eighty Twenty",
			"Error": "Invalid email or password",
		}
		renderTemplate(w, "login.html", data)
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		data := map[string]interface{}{
			"Title": "Login - Eighty Twenty",
			"Error": "Invalid email or password",
		}
		renderTemplate(w, "login.html", data)
		return
	}

	cookie, err := middleware.CreateSessionCookie(user.ID.String(), user.Email, user.Role, h.cfg.SessionSecret)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/pre-enrolment", http.StatusFound)
}

// Logout clears the session cookie and redirects to login.
// FIX: Added logging to track logout requests and ensure cookie is cleared properly.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.cfg.Debugf("🚪 Logout() called - clearing session cookie")
	
	// Clear the session cookie
	cookie := &http.Cookie{
		Name:     "eighty_twenty_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Match CreateSessionCookie settings
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Immediately expire
	}
	http.SetCookie(w, cookie)
	h.cfg.Debugf("  → Session cookie cleared, redirecting to /login")
	http.Redirect(w, r, "/login", http.StatusFound)
}
