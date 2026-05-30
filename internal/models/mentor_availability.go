package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
)

const defaultClassSessionDuration = 2 * time.Hour
const AvailabilityReminderBannerKey = "mentor_availability_monthly"

type availabilitySession struct {
	SessionNumber int32
	Date          time.Time
	StartTime     string
	EndTime       string
}

func ParseAvailabilityMonth(month string) (time.Time, time.Time, error) {
	trimmed := strings.TrimSpace(month)
	if trimmed == "" {
		now := time.Now()
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), nil
	}
	// Automatically normalize YYYY-M to YYYY-MM (e.g. 2026-6 -> 2026-06)
	parts := strings.Split(trimmed, "-")
	if len(parts) == 2 {
		if len(parts[1]) == 1 {
			trimmed = parts[0] + "-0" + parts[1]
		}
	}
	start, err := time.Parse("2006-01", trimmed)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("month must be in YYYY-MM format")
	}
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0), nil
}

func GetMentorAvailabilityWindows(mentorUserID uuid.UUID, monthStart time.Time) ([]MentorAvailabilityWindow, error) {
	monthStart = monthStartDate(monthStart)
	monthEnd := monthStart.AddDate(0, 1, 0)
	rows, err := db.DB.Query(`
		SELECT id, mentor_user_id, available_date, start_time::TEXT, end_time::TEXT, note, created_at, updated_at
		FROM mentor_availability_windows
		WHERE mentor_user_id = $1
		  AND available_date >= $2
		  AND available_date < $3
		ORDER BY available_date, start_time, end_time
	`, mentorUserID, monthStart, monthEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	windows := make([]MentorAvailabilityWindow, 0)
	for rows.Next() {
		var item MentorAvailabilityWindow
		if err := rows.Scan(
			&item.ID,
			&item.MentorUserID,
			&item.AvailableDate,
			&item.StartTime,
			&item.EndTime,
			&item.Note,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor availability: %w", err)
		}
		item.StartTime = formatClockText(item.StartTime)
		item.EndTime = formatClockText(item.EndTime)
		windows = append(windows, item)
	}
	return windows, rows.Err()
}

// GetMentorLockedDates returns the specific scheduled dates (YYYY-MM-DD) of
// active, non-cancelled class sessions the mentor is assigned to.
// Only those exact dates are locked — not the entire range between them.
func GetMentorLockedDates(mentorUserID uuid.UUID) ([]string, error) {
	rows, err := db.DB.Query(`
		SELECT DISTINCT cs.scheduled_date
		FROM mentor_assignments ma
		INNER JOIN class_groups cg ON cg.class_key = ma.class_key
		INNER JOIN class_sessions cs ON cs.class_key = ma.class_key
		WHERE ma.mentor_user_id = $1
		  AND COALESCE(cg.round_status, 'not_started') = 'active'
		  AND cs.status != 'cancelled'
		ORDER BY cs.scheduled_date
	`, mentorUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to load mentor locked dates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var locked []string
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("failed to scan locked date: %w", err)
		}
		locked = append(locked, dateOnly(t).Format("2006-01-02"))
	}
	if locked == nil {
		locked = []string{}
	}
	return locked, rows.Err()
}

// MentorAvailabilityForMonth holds a mentor's windows for use in the MH calendar.
type MentorAvailabilityForMonth struct {
	MentorUserID string                     `json:"mentor_user_id"`
	Name         string                     `json:"name"`
	Windows      []MentorAvailabilityWindow `json:"windows"`
}

// GetAllMentorsAvailabilityForMonth returns all active mentors with their
// availability windows for a given month. Used by the MH read-only calendar.
func GetAllMentorsAvailabilityForMonth(monthStart time.Time) ([]MentorAvailabilityForMonth, error) {
	monthStart = monthStartDate(monthStart)
	mentors, err := GetUsersByRole("mentor")
	if err != nil {
		return nil, fmt.Errorf("failed to load mentors: %w", err)
	}

	result := make([]MentorAvailabilityForMonth, 0, len(mentors))
	for _, m := range mentors {
		windows, err := GetMentorAvailabilityWindows(m.ID, monthStart)
		if err != nil {
			return nil, fmt.Errorf("failed to load availability for mentor %s: %w", m.ID, err)
		}
		name := m.FullName.String
		if !m.FullName.Valid || name == "" {
			name = m.Email
		}
		result = append(result, MentorAvailabilityForMonth{
			MentorUserID: m.ID.String(),
			Name:         name,
			Windows:      windows,
		})
	}
	return result, nil
}

func ReplaceMentorAvailabilityWindows(mentorUserID uuid.UUID, monthStart time.Time, windows []MentorAvailabilityWindow) ([]MentorAvailabilityWindow, error) {
	monthStart = monthStartDate(monthStart)
	monthEnd := monthStart.AddDate(0, 1, 0)
	lockedDates, err := GetMentorLockedDates(mentorUserID)
	if err != nil {
		return nil, err
	}
	// Build a set for O(1) lookup
	lockedSet := make(map[string]bool, len(lockedDates))
	for _, d := range lockedDates {
		lockedSet[d] = true
	}
	for i, item := range windows {
		availableDate := dateOnly(item.AvailableDate)
		if availableDate.Before(monthStart) || !availableDate.Before(monthEnd) {
			return nil, fmt.Errorf("availability date %s is outside the selected month", availableDate.Format("2006-01-02"))
		}
		if availableDate.Weekday() == time.Friday {
			return nil, fmt.Errorf("availability cannot be submitted for Fridays (%s is a Friday)", availableDate.Format("2006-01-02"))
		}

		today := time.Now().UTC().Truncate(24 * time.Hour)
		if availableDate.Before(today) {
			return nil, fmt.Errorf("availability cannot be submitted for past dates (%s)", availableDate.Format("2006-01-02"))
		}

		dateStr := availableDate.Format("2006-01-02")
		if lockedSet[dateStr] {
			return nil, fmt.Errorf("date %s is locked — you have an active class session on this day", dateStr)
		}
		startClock, err := parseSessionClock(item.StartTime)
		if err != nil {
			return nil, fmt.Errorf("window %d start_time is invalid: %w", i+1, err)
		}
		endClock, err := parseSessionClock(item.EndTime)
		if err != nil {
			return nil, fmt.Errorf("window %d end_time is invalid: %w", i+1, err)
		}
		if !clockAfter(endClock, startClock) {
			return nil, fmt.Errorf("window %d end_time must be after start_time", i+1)
		}
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin availability update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete future dates that are NOT locked (past dates and locked dates are preserved)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	deleteStart := monthStart
	if today.After(deleteStart) {
		deleteStart = today
	}

	if deleteStart.Before(monthEnd) {
		// Build the list of locked dates within the editable range to exclude from DELETE
		excludes := make([]string, 0)
		for _, d := range lockedDates {
			t, err := time.Parse("2006-01-02", d)
			if err != nil {
				continue
			}
			if !t.Before(deleteStart) && t.Before(monthEnd) {
				excludes = append(excludes, d)
			}
		}

		if len(excludes) == 0 {
			if _, err := tx.Exec(`
			DELETE FROM mentor_availability_windows
			WHERE mentor_user_id = $1
			  AND available_date >= $2
			  AND available_date < $3
		`, mentorUserID, deleteStart, monthEnd); err != nil {
				return nil, fmt.Errorf("failed to clear mentor availability: %w", err)
			}
		} else {
			// Delete all except locked dates
			// Build a parameterised IN list
			args := []interface{}{mentorUserID, deleteStart, monthEnd}
			placeholders := make([]string, len(excludes))
			for i, d := range excludes {
				args = append(args, d)
				placeholders[i] = fmt.Sprintf("$%d", len(args))
			}
			query := fmt.Sprintf(`
			DELETE FROM mentor_availability_windows
			WHERE mentor_user_id = $1
			  AND available_date >= $2
			  AND available_date < $3
			  AND available_date::TEXT NOT IN (%s)
			`, strings.Join(placeholders, ","))
			if _, err := tx.Exec(query, args...); err != nil {
				return nil, fmt.Errorf("failed to clear mentor availability: %w", err)
			}
		}
	}

	now := time.Now()
	for _, item := range windows {
		note := sql.NullString{String: strings.TrimSpace(item.Note.String), Valid: item.Note.Valid && strings.TrimSpace(item.Note.String) != ""}
		if !note.Valid {
			note.String = ""
		}
		if _, err := tx.Exec(`
			INSERT INTO mentor_availability_windows (
				id,
				mentor_user_id,
				available_date,
				start_time,
				end_time,
				note,
				created_at,
				updated_at
			)
			VALUES (gen_random_uuid(), $1, $2, $3::TIME, $4::TIME, $5, $6, $6)
			ON CONFLICT DO NOTHING
		`, mentorUserID, dateOnly(item.AvailableDate), formatClockText(item.StartTime), formatClockText(item.EndTime), note, now); err != nil {
			return nil, fmt.Errorf("failed to insert mentor availability: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit availability update: %w", err)
	}
	return GetMentorAvailabilityWindows(mentorUserID, monthStart)
}

func GetAvailabilityReminder(userID uuid.UUID, role string, now time.Time) (*AvailabilityReminderNotification, error) {
	role = strings.TrimSpace(role)
	if role != "mentor" && role != "mentor_head" {
		return nil, nil
	}
	cairoNow := now
	if cairoNow.IsZero() {
		cairoNow = time.Now()
	}
	if cairoNow.Day() > 7 {
		return nil, nil
	}
	monthStart := monthStartDate(cairoNow)
	dismissed, err := isAvailabilityBannerDismissed(userID, monthStart)
	if err != nil {
		return nil, err
	}
	if dismissed {
		return nil, nil
	}

	monthLabel := monthStart.Format("2006-01")
	if role == "mentor" {
		windows, err := GetMentorAvailabilityWindows(userID, monthStart)
		if err != nil {
			return nil, err
		}
		if len(windows) > 0 {
			return nil, nil
		}
		return &AvailabilityReminderNotification{
			BannerKey:   AvailabilityReminderBannerKey,
			Month:       monthLabel,
			Title:       "Update your monthly availability",
			Message:     "Add the days and time windows you can teach this month.",
			ActionPath:  "/mentor/availability",
			ActionLabel: "Open availability",
		}, nil
	}

	missingCount, err := countMentorsMissingAvailability(monthStart)
	if err != nil {
		return nil, err
	}
	if missingCount == 0 {
		return nil, nil
	}
	return &AvailabilityReminderNotification{
		BannerKey:    AvailabilityReminderBannerKey,
		Month:        monthLabel,
		Title:        "Collect mentor availability",
		Message:      fmt.Sprintf("%d mentor(s) still have no availability submitted for this month.", missingCount),
		ActionPath:   "/mentors",
		ActionLabel:  "Open mentor directory",
		MissingCount: missingCount,
	}, nil
}

func DismissAvailabilityReminder(userID uuid.UUID, monthStart time.Time) error {
	_, err := db.DB.Exec(`
		INSERT INTO availability_banner_dismissals (user_id, banner_key, banner_month, dismissed_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, banner_key, banner_month) DO UPDATE
		SET dismissed_at = EXCLUDED.dismissed_at
	`, userID, AvailabilityReminderBannerKey, monthStartDate(monthStart))
	if err != nil {
		return fmt.Errorf("failed to dismiss availability reminder: %w", err)
	}
	return nil
}

func CheckMentorAvailabilityForClass(classKey string, mentorUserID uuid.UUID) ([]MentorAvailabilityWarning, error) {
	sessions, err := buildAvailabilitySessionsForClass(classKey)
	if err != nil {
		return nil, err
	}
	return CheckMentorAvailabilityForSessions(mentorUserID, sessions)
}

func CheckMentorAvailabilityForClassFromSession(classKey string, mentorUserID uuid.UUID, fromSessionNumber int32) ([]MentorAvailabilityWarning, error) {
	sessions, err := buildAvailabilitySessionsForClass(classKey)
	if err != nil {
		return nil, err
	}
	filtered := make([]availabilitySession, 0, len(sessions))
	for _, session := range sessions {
		if session.SessionNumber >= fromSessionNumber {
			filtered = append(filtered, session)
		}
	}
	return CheckMentorAvailabilityForSessions(mentorUserID, filtered)
}

func CheckMentorAvailabilityForSessions(mentorUserID uuid.UUID, sessions []availabilitySession) ([]MentorAvailabilityWarning, error) {
	if len(sessions) == 0 {
		return nil, nil
	}

	minDate := dateOnly(sessions[0].Date)
	maxDate := minDate
	for _, session := range sessions {
		d := dateOnly(session.Date)
		if d.Before(minDate) {
			minDate = d
		}
		if d.After(maxDate) {
			maxDate = d
		}
	}

	windows, err := getMentorAvailabilityWindowsBetween(mentorUserID, minDate, maxDate.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}

	byDate := make(map[string][]MentorAvailabilityWindow)
	byMonth := make(map[string]int)
	for _, window := range windows {
		dateKey := window.AvailableDate.Format("2006-01-02")
		monthKey := window.AvailableDate.Format("2006-01")
		byDate[dateKey] = append(byDate[dateKey], window)
		byMonth[monthKey]++
	}

	warnings := make([]MentorAvailabilityWarning, 0)
	for _, session := range sessions {
		dateKey := dateOnly(session.Date).Format("2006-01-02")
		monthKey := dateOnly(session.Date).Format("2006-01")
		sessionStart, err := parseSessionClock(session.StartTime)
		if err != nil {
			return nil, fmt.Errorf("session %d has invalid start time: %w", session.SessionNumber, err)
		}
		sessionEnd, err := parseSessionClock(session.EndTime)
		if err != nil {
			return nil, fmt.Errorf("session %d has invalid end time: %w", session.SessionNumber, err)
		}

		dateWindows := byDate[dateKey]
		if len(dateWindows) == 0 {
			message := fmt.Sprintf("Session %d on %s has no submitted availability for this mentor.", session.SessionNumber, dateKey)
			if byMonth[monthKey] == 0 {
				message = fmt.Sprintf("Session %d on %s is in a month with no submitted availability for this mentor.", session.SessionNumber, dateKey)
			}
			warnings = append(warnings, MentorAvailabilityWarning{
				Code:          "missing_availability",
				Message:       message,
				SessionNumber: session.SessionNumber,
				ScheduledDate: dateKey,
				StartTime:     formatClockText(session.StartTime),
				EndTime:       formatClockText(session.EndTime),
			})
			continue
		}

		covered := false
		for _, window := range dateWindows {
			windowStart, err := parseSessionClock(window.StartTime)
			if err != nil {
				return nil, fmt.Errorf("availability window has invalid start time: %w", err)
			}
			windowEnd, err := parseSessionClock(window.EndTime)
			if err != nil {
				return nil, fmt.Errorf("availability window has invalid end time: %w", err)
			}
			if !clockBefore(sessionStart, windowStart) && !clockAfter(sessionEnd, windowEnd) {
				covered = true
				break
			}
		}
		if !covered {
			warnings = append(warnings, MentorAvailabilityWarning{
				Code:          "outside_availability",
				Message:       fmt.Sprintf("Session %d on %s from %s to %s is outside this mentor's submitted availability.", session.SessionNumber, dateKey, formatClockText(session.StartTime), formatClockText(session.EndTime)),
				SessionNumber: session.SessionNumber,
				ScheduledDate: dateKey,
				StartTime:     formatClockText(session.StartTime),
				EndTime:       formatClockText(session.EndTime),
			})
		}
	}

	return warnings, nil
}

func buildAvailabilitySessionsForClass(classKey string) ([]availabilitySession, error) {
	existingSessions, err := GetClassSessions(classKey)
	if err != nil {
		return nil, err
	}
	if len(existingSessions) > 0 {
		sessions := make([]availabilitySession, 0, len(existingSessions))
		for _, session := range existingSessions {
			if session == nil || strings.EqualFold(session.Status, "cancelled") {
				continue
			}
			startTime := ""
			if session.ScheduledTime.Valid {
				startTime = session.ScheduledTime.String
			}
			endTime := ""
			if session.ScheduledEndTime.Valid {
				endTime = session.ScheduledEndTime.String
			}
			item, err := buildAvailabilitySession(session.SessionNumber, session.ScheduledDate, startTime, endTime)
			if err != nil {
				return nil, err
			}
			sessions = append(sessions, item)
		}
		return sessions, nil
	}

	classGroup, err := GetClassGroupByKey(classKey)
	if err != nil {
		return nil, err
	}
	if classGroup == nil {
		return nil, fmt.Errorf("class not found")
	}

	startDate := time.Now()
	if classGroup.SuggestedStartDate.Valid {
		startDate = classGroup.SuggestedStartDate.Time
	} else if allowedWeekdays, ok := allowedRoundStartWeekdays(classGroup.ClassDays); ok {
		normalizedStart := dateOnly(startDate)
		for !containsWeekday(allowedWeekdays, normalizedStart.Weekday()) {
			normalizedStart = normalizedStart.AddDate(0, 0, 1)
		}
		startDate = normalizedStart
	}

	sessionDates, err := BuildClassSessionDates(classGroup.ClassDays, startDate, 8)
	if err != nil {
		return nil, err
	}

	sessions := make([]availabilitySession, 0, len(sessionDates))
	for i, sessionDate := range sessionDates {
		item, err := buildAvailabilitySession(int32(i+1), sessionDate, classGroup.ClassTime, "")
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, item)
	}
	return sessions, nil
}

func getMentorAvailabilityWindowsBetween(mentorUserID uuid.UUID, startDate, endDate time.Time) ([]MentorAvailabilityWindow, error) {
	rows, err := db.DB.Query(`
		SELECT id, mentor_user_id, available_date, start_time::TEXT, end_time::TEXT, note, created_at, updated_at
		FROM mentor_availability_windows
		WHERE mentor_user_id = $1
		  AND available_date >= $2
		  AND available_date < $3
		ORDER BY available_date, start_time, end_time
	`, mentorUserID, dateOnly(startDate), dateOnly(endDate))
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	windows := make([]MentorAvailabilityWindow, 0)
	for rows.Next() {
		var item MentorAvailabilityWindow
		if err := rows.Scan(
			&item.ID,
			&item.MentorUserID,
			&item.AvailableDate,
			&item.StartTime,
			&item.EndTime,
			&item.Note,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor availability: %w", err)
		}
		item.StartTime = formatClockText(item.StartTime)
		item.EndTime = formatClockText(item.EndTime)
		windows = append(windows, item)
	}
	return windows, rows.Err()
}

func buildAvailabilitySession(sessionNumber int32, date time.Time, startTime, endTime string) (availabilitySession, error) {
	parsedStart, err := parseSessionClock(startTime)
	if err != nil {
		return availabilitySession{}, err
	}
	if strings.TrimSpace(endTime) == "" {
		endTime = parsedStart.Add(defaultClassSessionDuration).Format("15:04")
	} else if _, err := parseSessionClock(endTime); err != nil {
		return availabilitySession{}, err
	}
	return availabilitySession{
		SessionNumber: sessionNumber,
		Date:          dateOnly(date),
		StartTime:     formatClockText(startTime),
		EndTime:       formatClockText(endTime),
	}, nil
}

func monthStartDate(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func dateOnly(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

func formatClockText(value string) string {
	parsed, err := parseSessionClock(value)
	if err != nil {
		return strings.TrimSpace(value)
	}
	return parsed.Format("15:04")
}

func clockBefore(left, right time.Time) bool {
	return left.Hour()*60+left.Minute() < right.Hour()*60+right.Minute()
}

func clockAfter(left, right time.Time) bool {
	return left.Hour()*60+left.Minute() > right.Hour()*60+right.Minute()
}

func isAvailabilityBannerDismissed(userID uuid.UUID, monthStart time.Time) (bool, error) {
	var exists bool
	if err := db.DB.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM availability_banner_dismissals
			WHERE user_id = $1
			  AND banner_key = $2
			  AND banner_month = $3
		)
	`, userID, AvailabilityReminderBannerKey, monthStartDate(monthStart)).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check availability reminder dismissal: %w", err)
	}
	return exists, nil
}

func countMentorsMissingAvailability(monthStart time.Time) (int, error) {
	var count int
	if err := db.DB.QueryRow(`
		SELECT COUNT(*)
		FROM users u
		WHERE u.role = 'mentor'
		  AND COALESCE(u.is_active, true) = true
		  AND NOT EXISTS (
			SELECT 1
			FROM mentor_availability_windows maw
			WHERE maw.mentor_user_id = u.id
			  AND maw.available_date >= $1
			  AND maw.available_date < $2
		  )
	`, monthStartDate(monthStart), monthStartDate(monthStart).AddDate(0, 1, 0)).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count mentors missing availability: %w", err)
	}
	return count, nil
}
