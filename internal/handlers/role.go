package handlers

import (
	"net/http"

	"eighty-twenty-ops/internal/middleware"
)

// IsModerator returns true if the current user has the moderator role.
func IsModerator(r *http.Request) bool {
	return middleware.GetUserRole(r) == "moderator"
}

// IsAdmin returns true if the current user has the admin role.
func IsAdmin(r *http.Request) bool {
	return middleware.GetUserRole(r) == "admin"
}
