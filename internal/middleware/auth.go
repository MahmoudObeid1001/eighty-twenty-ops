package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
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

func RequireAuth(next http.HandlerFunc, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("eighty_twenty_session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		userID, userEmail, userRole, err := ValidateSessionCookie(cookie, secret)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, UserEmailKey, userEmail)
		ctx = context.WithValue(ctx, UserRoleKey, userRole)

		next(w, r.WithContext(ctx))
	}
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
			allowed := false
			for _, role := range allowedRoles {
				if userRole == role {
					allowed = true
					break
				}
			}
			if !allowed {
				http.Error(w, "Forbidden: Insufficient permissions", http.StatusForbidden)
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
