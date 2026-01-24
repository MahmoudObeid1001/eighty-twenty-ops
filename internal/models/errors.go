package models

import (
	"errors"
	"fmt"
	"strings"

	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// PhoneAlreadyExistsError represents a phone number uniqueness violation
type PhoneAlreadyExistsError struct {
	Phone          string
	ExistingLeadID *uuid.UUID
	Message        string
}

func (e *PhoneAlreadyExistsError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("Phone number %s already exists", e.Phone)
}

// IsPhoneConstraintError checks if an error is a PostgreSQL unique constraint violation on phone
// Returns the error wrapped as PhoneAlreadyExistsError if it matches, or nil otherwise
func IsPhoneConstraintError(err error) *PhoneAlreadyExistsError {
	if err == nil {
		return nil
	}

	// Check if it's a pgx constraint violation
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// SQLSTATE 23505 = unique_violation
		if pgErr.Code == "23505" {
			// Check if constraint name contains "phone"
			constraintName := strings.ToLower(pgErr.ConstraintName)
			if strings.Contains(constraintName, "phone") {
				return &PhoneAlreadyExistsError{
					Phone:   "", // Will be set by caller if available
					Message: "Phone number already exists",
				}
			}
		}
	}

	// Also check error message for phone constraint (fallback)
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "phone") && (strings.Contains(errMsg, "unique") || strings.Contains(errMsg, "duplicate")) {
		return &PhoneAlreadyExistsError{
			Phone:   "",
			Message: "Phone number already exists",
		}
	}

	return nil
}

// GetLeadByPhone finds a lead by phone number
func GetLeadByPhone(phone string) (*Lead, error) {
	lead := &Lead{}
	err := db.DB.QueryRow(`
		SELECT id, full_name, phone, source, notes, status, sent_to_classes, 
		       levels_purchased_total, levels_consumed, bundle_type, 
		       created_by_user_id, created_at, updated_at
		FROM leads
		WHERE phone = $1
		LIMIT 1
	`, phone).Scan(
		&lead.ID, &lead.FullName, &lead.Phone, &lead.Source, &lead.Notes, &lead.Status,
		&lead.SentToClasses, &lead.LevelsPurchasedTotal, &lead.LevelsConsumed, &lead.BundleType,
		&lead.CreatedByUserID, &lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return lead, nil
}
