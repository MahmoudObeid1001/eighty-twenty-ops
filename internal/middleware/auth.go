package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"eighty-twenty-ops/internal/models"
)

type contextKey string

const UserIDKey contextKey = "userID"
const UserEmailKey contextKey = "userEmail"
const UserRoleKey contextKey = "userRole"

func CreateSessionCookie(userID, userEmail, userRole, secret string) (*http.Cookie, error) {
	value := fmt.Sprintf("%s|%s|%s|%d", userID, userEmail, userRole, time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	cookieValue := fmt.Sprintf("%s|%s", value, signature)

	cookie := &http.Cookie{
		Name:     "eighty_twenty_session",
		Value:    cookieValue,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 7, // 7 days
	}

	return cookie, nil
}

func ValidateSessionCookie(cookie *http.Cookie, secret string) (userID, userEmail, userRole string, err error) {
	if cookie == nil {
		return "", "", "", fmt.Errorf("no session cookie")
	}

	parts := strings.Split(cookie.Value, "|")
	if len(parts) != 5 {
		return "", "", "", fmt.Errorf("invalid session format")
	}

	value := strings.Join(parts[:4], "|")
	signature := parts[4]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return "", "", "", fmt.Errorf("invalid session signature")
	}

	userID = parts[0]
	userEmail = parts[1]
	userRole = parts[2]

	return userID, userEmail, userRole, nil
}

func redirectToLoginWithNext(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Authentication required"}`))
		return
	}

	next := r.URL.Path
	if r.URL.RawQuery != "" {
		next += "?" + r.URL.RawQuery
	}
	if next != "" && next != "/" && strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusFound)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func RequireAuth(next http.HandlerFunc, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("eighty_twenty_session")
		if err != nil {
			redirectToLoginWithNext(w, r)
			return
		}

		userID, userEmail, userRole, err := ValidateSessionCookie(cookie, secret)
		if err != nil {
			redirectToLoginWithNext(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, UserEmailKey, userEmail)
		ctx = context.WithValue(ctx, UserRoleKey, userRole)

		user, err := models.GetUserByID(userID)
		if err != nil || user == nil {
			redirectToLoginWithNext(w, r)
			return
		}
		if !user.IsActive {
			http.SetCookie(w, &http.Cookie{
				Name:     "eighty_twenty_session",
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				MaxAge:   -1,
			})
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"Account is deactivated"}`))
				return
			}
			redirectToLoginWithNext(w, r)
			return
		}

		if user.MustChangePassword && !passwordSetupAllowedPath(r.URL.Path) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"Password setup required","require_password_change":true}`))
				return
			}
			http.Redirect(w, r, "/app/setup-password", http.StatusFound)
			return
		}

		next(w, r.WithContext(ctx))
	}
}

func passwordSetupAllowedPath(path string) bool {
	return path == "/logout" ||
		path == "/api/me" ||
		path == "/api/auth/force-change-password" ||
		strings.HasPrefix(path, "/app/setup-password")
}

func GetUserID(r *http.Request) string {
	if val := r.Context().Value(UserIDKey); val != nil {
		return val.(string)
	}
	return ""
}

func GetUserEmail(r *http.Request) string {
	if val := r.Context().Value(UserEmailKey); val != nil {
		return val.(string)
	}
	return ""
}

func GetUserRole(r *http.Request) string {
	if val := r.Context().Value(UserRoleKey); val != nil {
		return val.(string)
	}
	return ""
}

// RequireRole ensures the user has one of the specified roles
func RequireRole(allowedRoles []string, secret string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return RequireAuth(func(w http.ResponseWriter, r *http.Request) {
			userRole := GetUserRole(r)
			if userRole == "manager" {
				next(w, r)
				return
			}
			allowed := false
			for _, role := range allowedRoles {
				if userRole == role {
					allowed = true
					break
				}
			}
			if !allowed {
				http.Error(w, "You don't have permission to access this page.", http.StatusForbidden)
				return
			}
			next(w, r)
		}, secret)
	}
}

// RequireAnyRole is an alias for RequireRole (for clarity)
func RequireAnyRole(allowedRoles []string, secret string) func(http.HandlerFunc) http.HandlerFunc {
	return RequireRole(allowedRoles, secret)
}
