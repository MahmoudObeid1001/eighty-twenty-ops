package handlers

import (
	"log"
	"net/http"
	"strings"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"golang.org/x/crypto/bcrypt"
)

// normalizeEmail trims and lowercases email for consistent lookup and storage.
func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// RoleHomePath returns the designated home path for a role. Fallback: /pre-enrolment.
func RoleHomePath(role string) string {
	switch role {
	case "admin", "moderator":
		return "/pre-enrolment"
	case "mentor_head":
		return "/mentor-head"
	case "mentor":
		return "/mentor"
	case "community_officer":
		return "/community-officer"
	case "hr":
		return "/hr/mentors"
	default:
		return "/pre-enrolment"
	}
}

// isSafeRedirectPath returns true if path is a relative app path (no open redirect).
func isSafeRedirectPath(path string) bool {
	if path == "" || !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.HasPrefix(path, "//") || strings.Contains(path, "\\") {
		return false
	}
	// Disallow redirect to login or logout
	if strings.HasPrefix(path, "/login") || strings.HasPrefix(path, "/logout") {
		return false
	}
	return true
}

// roleCanAccessPath returns true if the role is allowed to access the path.
func roleCanAccessPath(role, path string) bool {
	// Strip query string for permission check
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return false
	}
	switch role {
	case "admin":
		return true
	case "moderator":
		return path == "/pre-enrolment" || strings.HasPrefix(path, "/pre-enrolment/")
	case "mentor_head":
		return path == "/mentor-head" || strings.HasPrefix(path, "/mentor-head/") ||
			path == "/classes" || strings.HasPrefix(path, "/classes") ||
			path == "/learning"
	case "mentor":
		return path == "/mentor" || strings.HasPrefix(path, "/mentor/") || path == "/learning"
	case "community_officer":
		return path == "/community-officer" || strings.HasPrefix(path, "/community-officer/") || path == "/learning"
	case "hr":
		return path == "/hr/mentors" || strings.HasPrefix(path, "/hr/mentors") || path == "/learning"
	default:
		return false
	}
}

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
		_, _, userRole, err := middleware.ValidateSessionCookie(cookie, h.cfg.SessionSecret)
		if err == nil {
			home := RoleHomePath(userRole)
			h.cfg.Debugf("  → Valid session found, redirecting to %s", home)
			http.Redirect(w, r, home, http.StatusFound)
			return
		}
		h.cfg.Debugf("  → Session cookie exists but invalid: %v", err)
	}

	next := r.URL.Query().Get("next")
	h.cfg.Debugf("  → No valid session, rendering login.html with auth_layout")
	data := map[string]interface{}{
		"Title":       "Login - Eighty Twenty",
		"HideSidebar": true,
		"Next":        next,
	}
	renderTemplate(w, r, "login.html", data)
	h.cfg.Debugf("  → Template render complete")
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	emailRaw := r.FormValue("email")
	password := r.FormValue("password")
	email := normalizeEmail(emailRaw)

	next := r.FormValue("next")
	loginError := func(msg string) {
		data := map[string]interface{}{
			"Title": "Login - Eighty Twenty",
			"Error": msg,
			"Next":  next,
		}
		renderTemplate(w, r, "login.html", data)
	}

	if email == "" || password == "" {
		loginError("Email and password are required")
		return
	}

	user, err := models.GetUserByEmail(email)
	if err != nil {
		log.Printf("LOGIN: user not found or db error email=%q: %v", email, err)
		loginError("Invalid email or password")
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		log.Printf("LOGIN: password mismatch email=%q: %v", email, err)
		loginError("Invalid email or password")
		return
	}

	cookie, err := middleware.CreateSessionCookie(user.ID.String(), user.Email, user.Role, h.cfg.SessionSecret)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, cookie)

	if next != "" && isSafeRedirectPath(next) && roleCanAccessPath(user.Role, next) {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}
	http.Redirect(w, r, RoleHomePath(user.Role), http.StatusFound)
}

// LearningRedirect redirects authenticated users to their role-specific Learning home.
// GET /learning -> mentor: /mentor, mentor_head: /mentor-head, hr: /hr/mentors, community_officer: /community-officer, admin/moderator: /pre-enrolment.
func (h *AuthHandler) LearningRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	role := middleware.GetUserRole(r)
	home := RoleHomePath(role)
	http.Redirect(w, r, home, http.StatusFound)
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
