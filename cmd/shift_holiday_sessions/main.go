package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"
)

type classSchedule struct {
	ClassKey  string
	ClassDays string
	Sessions  []sessionSchedule
}

type sessionSchedule struct {
	ID            uuid.UUID
	Number        int
	ScheduledDate time.Time
	ScheduledTime string
	Status        string
}

type holidayChange struct {
	ClassKey      string
	SessionID     uuid.UUID
	SessionNumber int
	Status        string
	OldDate       time.Time
	NewDate       time.Time
	ScheduledTime string
	Skipped       bool
	SkipReason    string
}

func main() {
	var (
		apply           = flag.Bool("apply", false, "Write shifted session dates to the database")
		classKey        = flag.String("class-key", "", "Optional class_key to shift one class only")
		breakStartValue = flag.String("break-start", "2026-05-26", "Inclusive holiday break start date, YYYY-MM-DD")
		breakEndValue   = flag.String("break-end", "2026-05-31", "Inclusive holiday break end date, YYYY-MM-DD")
		resumeDateValue = flag.String("resume-date", "2026-06-01", "First available date after the break, YYYY-MM-DD")
		changedByValue  = flag.String("changed-by-user-id", "", "Optional user id to store in class_session_reschedules")
	)
	flag.Parse()

	breakStart, err := parseDateFlag("break-start", *breakStartValue)
	if err != nil {
		log.Fatal(err)
	}
	breakEnd, err := parseDateFlag("break-end", *breakEndValue)
	if err != nil {
		log.Fatal(err)
	}
	resumeDate, err := parseDateFlag("resume-date", *resumeDateValue)
	if err != nil {
		log.Fatal(err)
	}
	if breakEnd.Before(breakStart) {
		log.Fatal("break-end must be on or after break-start")
	}
	if !resumeDate.After(breakEnd) {
		log.Fatal("resume-date must be after break-end")
	}

	var changedBy uuid.NullUUID
	if strings.TrimSpace(*changedByValue) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*changedByValue))
		if err != nil {
			log.Fatalf("invalid changed-by-user-id: %v", err)
		}
		changedBy = uuid.NullUUID{UUID: parsed, Valid: true}
	}

	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("warning: failed to close db: %v", err)
		}
	}()

	classes, err := loadClassesWithBreakSessions(*classKey, breakStart, breakEnd)
	if err != nil {
		log.Fatalf("failed to load classes: %v", err)
	}

	changes, err := computeHolidayShiftChanges(classes, breakStart, breakEnd, resumeDate)
	if err != nil {
		log.Fatalf("failed to compute holiday shift: %v", err)
	}

	printSummary(*apply, breakStart, breakEnd, resumeDate, classes, changes)
	if !*apply {
		fmt.Println("\nNo database changes were written. Re-run with --apply to commit.")
		return
	}

	if err := applyChanges(changes, changedBy); err != nil {
		log.Fatalf("failed to apply holiday shift: %v", err)
	}
}

func loadClassesWithBreakSessions(classKey string, breakStart, breakEnd time.Time) ([]classSchedule, error) {
	rows, err := db.DB.Query(`
		SELECT DISTINCT cg.class_key, cg.class_days
		FROM class_groups cg
		JOIN class_sessions cs ON cs.class_key = cg.class_key
		WHERE COALESCE(cg.round_status, '') <> 'closed'
		  AND cs.status <> 'completed'
		  AND cs.scheduled_date BETWEEN $1 AND $2
		  AND ($3 = '' OR cg.class_key = $3)
		ORDER BY cg.class_key
	`, breakStart, breakEnd, classKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var classes []classSchedule
	for rows.Next() {
		var class classSchedule
		if err := rows.Scan(&class.ClassKey, &class.ClassDays); err != nil {
			return nil, err
		}
		sessions, err := loadSessions(class.ClassKey)
		if err != nil {
			return nil, err
		}
		class.Sessions = sessions
		classes = append(classes, class)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if classKey != "" && len(classes) == 0 {
		return nil, sql.ErrNoRows
	}
	return classes, nil
}

func loadSessions(classKey string) ([]sessionSchedule, error) {
	rows, err := db.DB.Query(`
		SELECT id, session_number, scheduled_date, COALESCE(scheduled_time::TEXT, ''), status
		FROM class_sessions
		WHERE class_key = $1
		ORDER BY session_number
	`, classKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var sessions []sessionSchedule
	for rows.Next() {
		var session sessionSchedule
		if err := rows.Scan(&session.ID, &session.Number, &session.ScheduledDate, &session.ScheduledTime, &session.Status); err != nil {
			return nil, err
		}
		session.ScheduledTime = strings.TrimSpace(session.ScheduledTime)
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func computeHolidayShiftChanges(classes []classSchedule, breakStart, breakEnd, resumeDate time.Time) ([]holidayChange, error) {
	var changes []holidayChange
	for _, class := range classes {
		firstAffected := -1
		for idx, session := range class.Sessions {
			if session.Status == "completed" {
				continue
			}
			if inDateRange(session.ScheduledDate, breakStart, breakEnd) {
				firstAffected = idx
				break
			}
		}
		if firstAffected == -1 {
			continue
		}

		newStart, err := firstClassDateOnOrAfter(class.ClassDays, resumeDate)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", class.ClassKey, err)
		}
		newDates, err := models.BuildClassSessionDates(class.ClassDays, newStart, len(class.Sessions)-firstAffected)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", class.ClassKey, err)
		}

		for idx := firstAffected; idx < len(class.Sessions); idx++ {
			session := class.Sessions[idx]
			targetDate := newDates[idx-firstAffected]
			change := holidayChange{
				ClassKey:      class.ClassKey,
				SessionID:     session.ID,
				SessionNumber: session.Number,
				Status:        session.Status,
				OldDate:       session.ScheduledDate,
				NewDate:       targetDate,
				ScheduledTime: session.ScheduledTime,
			}
			if session.Status == "completed" {
				change.Skipped = true
				change.SkipReason = "completed"
			}
			if sameDate(session.ScheduledDate, targetDate) {
				change.Skipped = true
				change.SkipReason = "already on target date"
			}
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func applyChanges(changes []holidayChange, changedBy uuid.NullUUID) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	for _, change := range changes {
		if change.Skipped {
			continue
		}
		_, err := tx.Exec(`
			INSERT INTO class_session_reschedules (
				id,
				class_session_id,
				class_key,
				session_number,
				old_scheduled_date,
				old_scheduled_time,
				new_scheduled_date,
				new_scheduled_time,
				changed_by_user_id,
				created_at
			)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, $5::time, $6, $5::time, $7, $8)
		`, change.SessionID, change.ClassKey, change.SessionNumber, change.OldDate, change.ScheduledTime, change.NewDate, nullableUUIDValue(changedBy), now)
		if err != nil {
			return fmt.Errorf("%s S%d: failed to write reschedule audit: %w", change.ClassKey, change.SessionNumber, err)
		}

		_, err = tx.Exec(`
			UPDATE class_sessions
			SET scheduled_date = $1,
			    actual_date = NULL,
			    actual_time = NULL,
			    actual_end_time = NULL,
			    completed_at = NULL,
			    status = 'scheduled',
			    updated_at = $2
			WHERE id = $3
			  AND status <> 'completed'
		`, change.NewDate, now, change.SessionID)
		if err != nil {
			return fmt.Errorf("%s S%d: failed to update session: %w", change.ClassKey, change.SessionNumber, err)
		}
	}

	return tx.Commit()
}

func firstClassDateOnOrAfter(classDays string, resumeDate time.Time) (time.Time, error) {
	for offset := 0; offset < 14; offset++ {
		candidate := dateOnly(resumeDate).AddDate(0, 0, offset)
		if _, err := models.BuildClassSessionDates(classDays, candidate, 1); err == nil {
			return candidate, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not find a valid class day on or after %s", formatDate(resumeDate))
}

func printSummary(apply bool, breakStart, breakEnd, resumeDate time.Time, classes []classSchedule, changes []holidayChange) {
	verb := "DRY RUN"
	if apply {
		verb = "APPLY"
	}

	updateCount := 0
	skippedCount := 0
	for _, change := range changes {
		if change.Skipped {
			skippedCount++
		} else {
			updateCount++
		}
	}

	fmt.Printf("%s holiday session shift\n", verb)
	fmt.Printf("  break: %s through %s\n", formatDate(breakStart), formatDate(breakEnd))
	fmt.Printf("  resume date: %s\n", formatDate(resumeDate))
	fmt.Printf("  classes affected: %d\n", len(classes))
	fmt.Printf("  sessions to update: %d\n", updateCount)
	fmt.Printf("  sessions skipped: %d\n", skippedCount)

	for _, change := range changes {
		action := "update"
		if change.Skipped {
			action = "skip " + change.SkipReason
		}
		fmt.Printf("  %s: %s S%d %s -> %s (%s @ %s)\n",
			action,
			change.ClassKey,
			change.SessionNumber,
			formatDate(change.OldDate),
			formatDate(change.NewDate),
			change.Status,
			change.ScheduledTime,
		)
	}
}

func parseDateFlag(name, value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s date %q: %w", name, value, err)
	}
	return parsed, nil
}

func inDateRange(value, start, end time.Time) bool {
	day := dateOnly(value)
	return !day.Before(dateOnly(start)) && !day.After(dateOnly(end))
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func dateOnly(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func formatDate(value time.Time) string {
	return value.Format("2006-01-02")
}

func nullableUUIDValue(value uuid.NullUUID) interface{} {
	if !value.Valid {
		return nil
	}
	return value.UUID
}

func init() {
	log.SetOutput(os.Stderr)
}
