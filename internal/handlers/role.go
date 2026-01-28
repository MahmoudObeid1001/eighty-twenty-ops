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

// IsMentorHead returns true if the current user has the mentor_head role.
func IsMentorHead(r *http.Request) bool {
	return middleware.GetUserRole(r) == "mentor_head"
}

// IsMentor returns true if the current user has the mentor role.
func IsMentor(r *http.Request) bool {
	return middleware.GetUserRole(r) == "mentor"
}

// IsCommunityOfficer returns true if the current user has the community_officer role.
func IsCommunityOfficer(r *http.Request) bool {
	return middleware.GetUserRole(r) == "community_officer"
}

// IsHR returns true if the current user has the hr role.
func IsHR(r *http.Request) bool {
	return middleware.GetUserRole(r) == "hr"
}
