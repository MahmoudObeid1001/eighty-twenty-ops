package models

import (
	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
)

// UpdateFeedbackStatus updates the status of a feedback record (sent -> received or removed)
func UpdateFeedbackStatus(leadID uuid.UUID, classKey string, sessionNumber int32, status string) (int64, error) {
	res, err := db.DB.Exec(`
		UPDATE community_officer_feedback
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE lead_id = $2 AND class_key = $3 AND session_number = $4
	`, status, leadID, classKey, sessionNumber)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
