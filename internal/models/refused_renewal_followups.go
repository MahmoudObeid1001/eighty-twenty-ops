package models

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
)

const (
	RefusedRenewalReasonTimePressure = "time_pressure"
	RefusedRenewalReasonFinancial    = "financial"
	RefusedRenewalReasonNotSatisfied = "not_satisfied"
	RefusedRenewalReasonOther        = "other"

	RefusedRenewalBannerKey = "refused_renewal_follow_ups"
)

type ContactHistoryLogInput struct {
	LeadID          uuid.UUID
	Channel         string
	EventType       string
	Source          string
	TemplateKey     string
	MessageText     string
	Metadata        map[string]interface{}
	CreatedByUserID *uuid.UUID
}

func IsValidRefusedRenewalReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case RefusedRenewalReasonTimePressure, RefusedRenewalReasonFinancial, RefusedRenewalReasonNotSatisfied, RefusedRenewalReasonOther:
		return true
	default:
		return false
	}
}

func ComputeRefusedRenewalFollowUpState(refusedAt time.Time, lastStep int, lastSentAt sql.NullTime, now time.Time) (int, time.Time, bool, bool) {
	if refusedAt.IsZero() {
		return 0, time.Time{}, false, false
	}

	switch {
	case lastStep <= 0:
		dueAt := refusedAt.AddDate(0, 0, 21)
		return 1, dueAt, !now.Before(dueAt), false
	case lastStep == 1:
		if !lastSentAt.Valid {
			dueAt := refusedAt.AddDate(0, 0, 21)
			return 1, dueAt, !now.Before(dueAt), false
		}
		dueAt := lastSentAt.Time.AddDate(0, 1, 0)
		return 2, dueAt, !now.Before(dueAt), false
	default:
		return 3, time.Time{}, true, true
	}
}

func GetRefusedRenewalMessageTemplates() ([]*RefusedRenewalMessageTemplate, error) {
	rows, err := db.DB.Query(`
		SELECT id, refusal_reason, sequence_step, template_key, title, body, is_active, created_at, updated_at
		FROM refused_renewal_message_templates
		WHERE is_active = true
		ORDER BY sequence_step ASC, refusal_reason ASC, title ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query refused renewal message templates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*RefusedRenewalMessageTemplate
	for rows.Next() {
		item := &RefusedRenewalMessageTemplate{}
		if err := rows.Scan(
			&item.ID,
			&item.RefusalReason,
			&item.SequenceStep,
			&item.TemplateKey,
			&item.Title,
			&item.Body,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan refused renewal message template: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating refused renewal message templates: %w", err)
	}
	return items, nil
}

func GetRefusedRenewalMessageTemplateByID(templateID uuid.UUID) (*RefusedRenewalMessageTemplate, error) {
	item := &RefusedRenewalMessageTemplate{}
	err := db.DB.QueryRow(`
		SELECT id, refusal_reason, sequence_step, template_key, title, body, is_active, created_at, updated_at
		FROM refused_renewal_message_templates
		WHERE id = $1
	`, templateID).Scan(
		&item.ID,
		&item.RefusalReason,
		&item.SequenceStep,
		&item.TemplateKey,
		&item.Title,
		&item.Body,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load refused renewal message template: %w", err)
	}
	return item, nil
}

func GetLatestRefusedRenewalFollowUp(leadID uuid.UUID) (int, sql.NullTime, error) {
	var messageNumber int
	var sentAt sql.NullTime
	err := db.DB.QueryRow(`
		SELECT message_number, sent_at
		FROM refused_renewal_follow_ups
		WHERE lead_id = $1
		ORDER BY message_number DESC, sent_at DESC
		LIMIT 1
	`, leadID).Scan(&messageNumber, &sentAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, sql.NullTime{}, nil
		}
		return 0, sql.NullTime{}, fmt.Errorf("failed to load latest refused renewal follow-up: %w", err)
	}
	return messageNumber, sentAt, nil
}

func RecordRefusedRenewalFollowUp(leadID uuid.UUID, messageNumber int, templateID *uuid.UUID, messageText string, sentByUserID uuid.UUID) error {
	cleanText := strings.TrimSpace(messageText)
	if cleanText == "" {
		return fmt.Errorf("message text is required")
	}

	var templateValue interface{}
	if templateID != nil {
		templateValue = *templateID
	}
	if _, err := db.DB.Exec(`
		INSERT INTO refused_renewal_follow_ups (lead_id, message_number, template_id, message_text, sent_by_user_id, sent_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
	`, leadID, messageNumber, templateValue, cleanText, sentByUserID); err != nil {
		return fmt.Errorf("failed to record refused renewal follow-up: %w", err)
	}
	return nil
}

func RecordPreEnrolmentContactHistory(input ContactHistoryLogInput) error {
	channel := strings.TrimSpace(input.Channel)
	if channel == "" {
		channel = "whatsapp"
	}

	payload := []byte("{}")
	if len(input.Metadata) > 0 {
		raw, err := json.Marshal(input.Metadata)
		if err != nil {
			return fmt.Errorf("failed to encode contact history metadata: %w", err)
		}
		payload = raw
	}

	var actor interface{}
	if input.CreatedByUserID != nil {
		actor = *input.CreatedByUserID
	}

	templateKey := strings.TrimSpace(input.TemplateKey)
	messageText := strings.TrimSpace(input.MessageText)
	if _, err := db.DB.Exec(`
		INSERT INTO pre_enrolment_contact_history (
			lead_id, channel, event_type, source, template_key, message_text, metadata, created_by_user_id, created_at
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7::jsonb, $8, CURRENT_TIMESTAMP)
	`, input.LeadID, channel, strings.TrimSpace(input.EventType), strings.TrimSpace(input.Source), templateKey, messageText, string(payload), actor); err != nil {
		return fmt.Errorf("failed to record pre-enrolment contact history: %w", err)
	}
	return nil
}

func GetPreEnrolmentContactHistory(leadID uuid.UUID) ([]*ContactHistoryItem, error) {
	rows, err := db.DB.Query(`
		SELECT
			ch.id,
			ch.lead_id,
			ch.channel,
			ch.event_type,
			ch.source,
			COALESCE(ch.template_key, '') AS template_key,
			COALESCE(ch.message_text, '') AS message_text,
			COALESCE(NULLIF(TRIM(u.full_name), ''), u.email, '') AS created_by_name,
			ch.created_at,
			ch.metadata
		FROM pre_enrolment_contact_history ch
		LEFT JOIN users u ON u.id = ch.created_by_user_id
		WHERE ch.lead_id = $1
		ORDER BY ch.created_at DESC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pre-enrolment contact history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []*ContactHistoryItem
	for rows.Next() {
		item := &ContactHistoryItem{}
		var templateKey string
		var messageText string
		if err := rows.Scan(
			&item.ID,
			&item.LeadID,
			&item.Channel,
			&item.EventType,
			&item.Source,
			&templateKey,
			&messageText,
			&item.CreatedByName,
			&item.CreatedAt,
			&item.Metadata,
		); err != nil {
			return nil, fmt.Errorf("failed to scan pre-enrolment contact history: %w", err)
		}
		if templateKey != "" {
			item.TemplateKey = sql.NullString{String: templateKey, Valid: true}
		}
		if messageText != "" {
			item.MessageText = sql.NullString{String: messageText, Valid: true}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed iterating pre-enrolment contact history: %w", err)
	}
	return items, nil
}

func DismissGlobalBannerForDate(bannerKey string, bannerDate time.Time, dismissedByUserID *uuid.UUID) error {
	var actor interface{}
	if dismissedByUserID != nil {
		actor = *dismissedByUserID
	}
	if _, err := db.DB.Exec(`
		INSERT INTO global_banner_dismissals (banner_key, banner_date, dismissed_at, dismissed_by_user_id)
		VALUES ($1, $2, CURRENT_TIMESTAMP, $3)
		ON CONFLICT (banner_key, banner_date) DO UPDATE
		SET dismissed_at = EXCLUDED.dismissed_at,
		    dismissed_by_user_id = EXCLUDED.dismissed_by_user_id
	`, strings.TrimSpace(bannerKey), bannerDate.Format("2006-01-02"), actor); err != nil {
		return fmt.Errorf("failed to dismiss global banner: %w", err)
	}
	return nil
}

func IsGlobalBannerDismissedForDate(bannerKey string, bannerDate time.Time) (bool, error) {
	var exists bool
	if err := db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM global_banner_dismissals
			WHERE banner_key = $1 AND banner_date = $2
		)
	`, strings.TrimSpace(bannerKey), bannerDate.Format("2006-01-02")).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check global banner dismissal: %w", err)
	}
	return exists, nil
}
