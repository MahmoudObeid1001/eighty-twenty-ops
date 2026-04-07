package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"
)

type classSchedule struct {
	ClassKey  string
	ClassDays string
	StartDate time.Time
	Sessions  []sessionSchedule
}

type sessionSchedule struct {
	Number int
	Date   time.Time
	Status string
}

type repairChange struct {
	ClassKey string
	Session  int
	From     time.Time
	To       time.Time
	Status   string
	Skipped  bool
}

func main() {
	var (
		apply    = flag.Bool("apply", false, "Write repaired session dates to the database")
		classKey = flag.String("class-key", "", "Optional class_key to repair one class only")
	)
	flag.Parse()

	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("warning: failed to close db: %v", err)
		}
	}()

	classes, err := loadActiveClassSchedules(*classKey)
	if err != nil {
		log.Fatalf("Failed to load class schedules: %v", err)
	}

	changes, err := computeChanges(classes)
	if err != nil {
		log.Fatalf("Failed to compute repairs: %v", err)
	}

	printSummary(*apply, classes, changes)
	if !*apply {
		fmt.Println("\nNo database changes were written. Re-run with --apply to commit.")
		return
	}

	if err := applyChanges(changes); err != nil {
		log.Fatalf("Failed to apply repairs: %v", err)
	}
}

func loadActiveClassSchedules(classKey string) ([]classSchedule, error) {
	rows, err := db.DB.Query(`
		SELECT cg.class_key, cg.class_days, cs.scheduled_date
		FROM class_groups cg
		JOIN class_sessions cs ON cs.class_key = cg.class_key AND cs.session_number = 1
		WHERE COALESCE(cg.round_status, '') = 'active'
		  AND ($1 = '' OR cg.class_key = $1)
		ORDER BY cg.class_key
	`, classKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var classes []classSchedule
	for rows.Next() {
		var class classSchedule
		if err := rows.Scan(&class.ClassKey, &class.ClassDays, &class.StartDate); err != nil {
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
		SELECT session_number, scheduled_date, status
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
		if err := rows.Scan(&session.Number, &session.Date, &session.Status); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func computeChanges(classes []classSchedule) ([]repairChange, error) {
	var changes []repairChange
	for _, class := range classes {
		expectedDates, err := models.BuildClassSessionDates(class.ClassDays, class.StartDate, len(class.Sessions))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", class.ClassKey, err)
		}
		for _, session := range class.Sessions {
			if session.Number < 1 || session.Number > len(expectedDates) {
				return nil, fmt.Errorf("%s: unexpected session number %d", class.ClassKey, session.Number)
			}
			expected := expectedDates[session.Number-1]
			if sameDate(session.Date, expected) {
				continue
			}
			change := repairChange{
				ClassKey: class.ClassKey,
				Session:  session.Number,
				From:     session.Date,
				To:       expected,
				Status:   session.Status,
				Skipped:  session.Status == "completed",
			}
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func applyChanges(changes []repairChange) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, change := range changes {
		if change.Skipped {
			continue
		}
		_, err := tx.Exec(`
			UPDATE class_sessions
			SET scheduled_date = $1,
			    updated_at = NOW()
			WHERE class_key = $2
			  AND session_number = $3
			  AND status <> 'completed'
		`, change.To, change.ClassKey, change.Session)
		if err != nil {
			return fmt.Errorf("%s S%d: %w", change.ClassKey, change.Session, err)
		}
	}

	return tx.Commit()
}

func printSummary(apply bool, classes []classSchedule, changes []repairChange) {
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

	fmt.Printf("%s class session date repair\n", verb)
	fmt.Printf("  classes inspected: %d\n", len(classes))
	fmt.Printf("  sessions to update: %d\n", updateCount)
	fmt.Printf("  completed sessions skipped: %d\n", skippedCount)

	for _, change := range changes {
		action := "update"
		if change.Skipped {
			action = "skip completed"
		}
		fmt.Printf("  %s: %s S%d %s -> %s (%s)\n",
			action,
			change.ClassKey,
			change.Session,
			formatDate(change.From),
			formatDate(change.To),
			change.Status,
		)
	}
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func formatDate(value time.Time) string {
	return value.Format("2006-01-02")
}

func init() {
	log.SetOutput(os.Stderr)
}
