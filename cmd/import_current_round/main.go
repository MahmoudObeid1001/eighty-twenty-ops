package main

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type mentorRow struct {
	Email    string
	FullName string
	Phone    string
	IsActive bool
}

type classRow struct {
	ClassKey             string
	Level                int
	ClassDays            string
	ClassTime            string
	ClassNumber          int
	MentorEmail          string
	CurrentSessionNumber int
}

type studentRow struct {
	ExternalID           string
	FullName             string
	Phone                string
	ClassKey             string
	Level                int
	ClassDays            string
	ClassTime            string
	ClassNumber          int
	RosterStartSession   int
	Status               string
	IsReturning          bool
	LevelsPurchasedTotal int
	LevelsConsumed       int
	Source               string
	Notes                string
}

type summary struct {
	NewMentors      int
	UpdatedMentors  int
	NewClasses      int
	UpdatedClasses  int
	NewAssignments  int
	UpdatedAssigns  int
	NewStudents     int
	UpdatedStudents int
	NewSessions     int
}

func main() {
	var (
		classesPath    = flag.String("classes", "data/import/current_round/classes_final.csv", "Path to classes CSV")
		mentorsPath    = flag.String("mentors", "data/import/current_round/mentors_final.csv", "Path to mentors CSV")
		studentsPath   = flag.String("students", "data/import/current_round/students_ready_for_import.csv", "Path to students CSV")
		apply          = flag.Bool("apply", false, "Write changes to the database")
		mentorTempPass = flag.String("mentor-temp-password", "", "Temporary password for newly created mentor accounts (required with --apply)")
		satStart       = flag.String("sat-start", "2026-04-04", "Start date for Sat/Tues classes (YYYY-MM-DD)")
		sunStart       = flag.String("sun-start", "2026-04-05", "Start date for Sun/Wed classes (YYYY-MM-DD)")
		monStart       = flag.String("mon-start", "2026-04-06", "Start date for Mon/Thu classes (YYYY-MM-DD)")
	)
	flag.Parse()

	if *apply && strings.TrimSpace(*mentorTempPass) == "" {
		log.Fatal("--mentor-temp-password is required with --apply")
	}

	satDate := mustParseDate("sat-start", *satStart)
	sunDate := mustParseDate("sun-start", *sunStart)
	monDate := mustParseDate("mon-start", *monStart)

	mentors := mustLoadMentors(*mentorsPath)
	classes := mustLoadClasses(*classesPath)
	students := mustLoadStudents(*studentsPath)

	validateCrossReferences(mentors, classes, students)

	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("warning: failed to close db: %v", err)
		}
	}()

	adminUserID, err := findCreatedByUserID(cfg.AdminEmail)
	if err != nil {
		log.Fatalf("Failed to resolve created_by user: %v", err)
	}

	currentRound, err := getCurrentRound()
	if err != nil {
		log.Fatalf("Failed to resolve current_round setting: %v", err)
	}

	params := importParams{
		Mentors:         mentors,
		Classes:         classes,
		Students:        students,
		Apply:           *apply,
		MentorTempPass:  *mentorTempPass,
		CurrentRound:    currentRound,
		CreatedByUserID: adminUserID,
		StartDateByDays: map[string]time.Time{
			"Sat/Tues": satDate,
			"Sun/Wed":  sunDate,
			"Mon/Thu":  monDate,
		},
	}

	result, err := runImport(params)
	if err != nil {
		log.Fatalf("Import failed: %v", err)
	}

	mode := "DRY RUN"
	if *apply {
		mode = "APPLY"
	}

	fmt.Printf("%s summary\n", mode)
	fmt.Printf("  mentors:    new=%d updated=%d\n", result.NewMentors, result.UpdatedMentors)
	fmt.Printf("  classes:    new=%d updated=%d\n", result.NewClasses, result.UpdatedClasses)
	fmt.Printf("  assignments:new=%d updated=%d\n", result.NewAssignments, result.UpdatedAssigns)
	fmt.Printf("  students:   new=%d updated=%d\n", result.NewStudents, result.UpdatedStudents)
	fmt.Printf("  sessions:   new=%d\n", result.NewSessions)
	if *apply {
		if result.NewMentors > 0 {
			fmt.Println("")
			fmt.Println("New mentor accounts were created with the supplied temporary password and must_change_password=true.")
		}
		fmt.Println("Import completed successfully.")
	} else {
		fmt.Println("")
		fmt.Println("No database changes were written. Re-run with --apply to commit.")
	}
}

type importParams struct {
	Mentors         []mentorRow
	Classes         []classRow
	Students        []studentRow
	Apply           bool
	MentorTempPass  string
	CurrentRound    string
	CreatedByUserID sql.NullString
	StartDateByDays map[string]time.Time
}

func runImport(params importParams) (summary, error) {
	tx, err := db.DB.Begin()
	if err != nil {
		return summary{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if !params.Apply {
		return importIntoTx(tx, params, true)
	}

	s, err := importIntoTx(tx, params, false)
	if err != nil {
		return summary{}, err
	}
	if err := tx.Commit(); err != nil {
		return summary{}, fmt.Errorf("commit transaction: %w", err)
	}
	return s, nil
}

func importIntoTx(tx *sql.Tx, params importParams, dryRun bool) (summary, error) {
	var out summary
	mentorIDs := make(map[string]string, len(params.Mentors))

	var mentorHash string
	if !dryRun {
		hash, err := bcrypt.GenerateFromPassword([]byte(params.MentorTempPass), bcrypt.DefaultCost)
		if err != nil {
			return out, fmt.Errorf("hash mentor temporary password: %w", err)
		}
		mentorHash = string(hash)
	}

	for _, row := range params.Mentors {
		userID, exists, changed, err := upsertMentor(tx, row, mentorHash, dryRun)
		if err != nil {
			return out, fmt.Errorf("mentor %s: %w", row.Email, err)
		}
		mentorIDs[row.Email] = userID
		if exists {
			if changed {
				out.UpdatedMentors++
			}
		} else {
			out.NewMentors++
		}
	}

	for _, row := range params.Classes {
		startDate, ok := params.StartDateByDays[row.ClassDays]
		if !ok {
			return out, fmt.Errorf("class %s: unsupported class_days %q", row.ClassKey, row.ClassDays)
		}
		if row.CurrentSessionNumber != 1 {
			return out, fmt.Errorf("class %s: current_session_number=%d is not supported by this importer; only 1 is supported", row.ClassKey, row.CurrentSessionNumber)
		}
		exists, changed, err := upsertClassGroup(tx, row, startDate, params.CreatedByUserID, dryRun)
		if err != nil {
			return out, fmt.Errorf("class %s: %w", row.ClassKey, err)
		}
		if exists {
			if changed {
				out.UpdatedClasses++
			}
		} else {
			out.NewClasses++
		}

		newSessions, err := ensureClassSessions(tx, row, startDate, dryRun)
		if err != nil {
			return out, fmt.Errorf("class %s sessions: %w", row.ClassKey, err)
		}
		out.NewSessions += newSessions

		mentorID, ok := mentorIDs[row.MentorEmail]
		if !ok || mentorID == "" {
			return out, fmt.Errorf("class %s: mentor email %s was not resolved", row.ClassKey, row.MentorEmail)
		}
		exists, changed, err = upsertMentorAssignment(tx, row.ClassKey, mentorID, params.CreatedByUserID, dryRun)
		if err != nil {
			return out, fmt.Errorf("assignment %s -> %s: %w", row.ClassKey, row.MentorEmail, err)
		}
		if exists {
			if changed {
				out.UpdatedAssigns++
			}
		} else {
			out.NewAssignments++
		}
	}

	classMap := make(map[string]classRow, len(params.Classes))
	for _, row := range params.Classes {
		classMap[row.ClassKey] = row
	}

	for _, row := range params.Students {
		classInfo, ok := classMap[row.ClassKey]
		if !ok {
			return out, fmt.Errorf("student %s: class_key %s not found in classes CSV", row.ExternalID, row.ClassKey)
		}
		startDate := params.StartDateByDays[classInfo.ClassDays]
		exists, err := upsertStudent(tx, row, params.CurrentRound, startDate, dryRun)
		if err != nil {
			return out, fmt.Errorf("student %s (%s): %w", row.ExternalID, row.FullName, err)
		}
		if exists {
			out.UpdatedStudents++
		} else {
			out.NewStudents++
		}
	}

	return out, nil
}

func upsertMentor(tx *sql.Tx, row mentorRow, passwordHash string, dryRun bool) (userID string, exists bool, changed bool, err error) {
	var currentID, currentRole, currentName, currentPhone string
	var currentActive bool
	err = tx.QueryRow(`
		SELECT id::text, role, COALESCE(full_name, ''), COALESCE(phone, ''), COALESCE(is_active, true)
		FROM users
		WHERE LOWER(TRIM(email)) = LOWER(TRIM($1))
	`, row.Email).Scan(&currentID, &currentRole, &currentName, &currentPhone, &currentActive)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, false, err
	}
	if err == nil {
		if currentRole != "mentor" {
			return "", true, false, fmt.Errorf("existing user %s has role %q, expected mentor", row.Email, currentRole)
		}
		changed = currentName != row.FullName || currentPhone != row.Phone || currentActive != row.IsActive
		if !dryRun && changed {
			_, err = tx.Exec(`
				UPDATE users
				SET full_name = $1, phone = $2, is_active = $3
				WHERE id = $4::uuid
			`, row.FullName, row.Phone, row.IsActive, currentID)
			if err != nil {
				return "", true, false, err
			}
		}
		return currentID, true, changed, nil
	}

	newID := uuid.New().String()
	if !dryRun {
		_, err = tx.Exec(`
			INSERT INTO users (id, email, full_name, phone, password_hash, role, is_active, must_change_password, created_at)
			VALUES ($1::uuid, $2, $3, $4, $5, 'mentor', $6, true, NOW())
		`, newID, row.Email, row.FullName, row.Phone, passwordHash, row.IsActive)
		if err != nil {
			return "", false, false, err
		}
	}
	return newID, false, false, nil
}

func upsertClassGroup(tx *sql.Tx, row classRow, startDate time.Time, createdByUserID sql.NullString, dryRun bool) (exists bool, changed bool, err error) {
	var currentLevel, currentNumber int
	var currentDays, currentTime, currentRoundStatus string
	var currentSent bool
	err = tx.QueryRow(`
		SELECT level, class_days, class_time, class_number, sent_to_mentor, COALESCE(round_status, 'not_started')
		FROM class_groups
		WHERE class_key = $1
	`, row.ClassKey).Scan(&currentLevel, &currentDays, &currentTime, &currentNumber, &currentSent, &currentRoundStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, false, err
	}

	startedAt := combineDateAndClock(startDate, row.ClassTime)
	exists = err == nil
	changed = !exists ||
		currentLevel != row.Level ||
		currentDays != row.ClassDays ||
		currentTime != row.ClassTime ||
		currentNumber != row.ClassNumber ||
		!currentSent ||
		currentRoundStatus != "active"

	if dryRun {
		return exists, changed && exists, nil
	}

	_, err = tx.Exec(`
		INSERT INTO class_groups (
			class_key, level, class_days, class_time, class_number,
			sent_to_mentor, sent_at, updated_at,
			hidden_in_ops, hidden_at, hidden_by,
			round_status, round_started_at, round_started_by
		)
		VALUES (
			$1, $2, $3, $4, $5,
			true, NOW(), NOW(),
			false, NULL, NULL,
			'active', $6, $7::uuid
		)
		ON CONFLICT (class_key) DO UPDATE SET
			level = EXCLUDED.level,
			class_days = EXCLUDED.class_days,
			class_time = EXCLUDED.class_time,
			class_number = EXCLUDED.class_number,
			sent_to_mentor = true,
			sent_at = COALESCE(class_groups.sent_at, EXCLUDED.sent_at),
			hidden_in_ops = false,
			hidden_at = NULL,
			hidden_by = NULL,
			round_status = 'active',
			round_started_at = COALESCE(class_groups.round_started_at, EXCLUDED.round_started_at),
			round_started_by = COALESCE(class_groups.round_started_by, EXCLUDED.round_started_by),
			updated_at = NOW()
	`, row.ClassKey, row.Level, row.ClassDays, row.ClassTime, row.ClassNumber, startedAt, nullUUIDParam(createdByUserID))
	if err != nil {
		return false, false, err
	}
	return exists, exists && changed, nil
}

func ensureClassSessions(tx *sql.Tx, row classRow, startDate time.Time, dryRun bool) (int, error) {
	sessionDates, err := models.BuildClassSessionDates(row.ClassDays, startDate, 8)
	if err != nil {
		return 0, err
	}

	if dryRun {
		var existing int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM class_sessions WHERE class_key = $1`, row.ClassKey).Scan(&existing); err != nil {
			return 0, err
		}
		if existing >= 8 {
			return 0, nil
		}
		return 8 - existing, nil
	}

	classClock, err := parseClock(row.ClassTime)
	if err != nil {
		return 0, err
	}
	endClock := classClock.Add(2 * time.Hour).Format("15:04")

	inserted := 0
	for i := 1; i <= 8; i++ {
		sessionDate := sessionDates[i-1]
		res, err := tx.Exec(`
			INSERT INTO class_sessions (
				id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time, status, created_at, updated_at
			)
			VALUES (gen_random_uuid(), $1, $2, $3, $4::time, $5::time, 'scheduled', NOW(), NOW())
			ON CONFLICT (class_key, session_number) DO NOTHING
		`, row.ClassKey, i, sessionDate, row.ClassTime, endClock)
		if err != nil {
			return 0, err
		}
		affected, _ := res.RowsAffected()
		inserted += int(affected)
	}
	return inserted, nil
}

func upsertMentorAssignment(tx *sql.Tx, classKey, mentorUserID string, createdByUserID sql.NullString, dryRun bool) (exists bool, changed bool, err error) {
	var currentMentorID string
	err = tx.QueryRow(`SELECT mentor_user_id::text FROM mentor_assignments WHERE class_key = $1`, classKey).Scan(&currentMentorID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, false, err
	}
	exists = err == nil
	changed = !exists || currentMentorID != mentorUserID
	if dryRun {
		return exists, exists && changed, nil
	}

	_, err = tx.Exec(`
		INSERT INTO mentor_assignments (id, mentor_user_id, class_key, assigned_at, created_by_user_id)
		VALUES (gen_random_uuid(), $1::uuid, $2, NOW(), $3::uuid)
		ON CONFLICT (class_key) DO UPDATE SET
			mentor_user_id = EXCLUDED.mentor_user_id,
			assigned_at = EXCLUDED.assigned_at,
			created_by_user_id = EXCLUDED.created_by_user_id
	`, mentorUserID, classKey, nullUUIDParam(createdByUserID))
	if err != nil {
		return false, false, err
	}
	return exists, exists && changed, nil
}

func upsertStudent(tx *sql.Tx, row studentRow, currentRound string, startDate time.Time, dryRun bool) (exists bool, err error) {
	if row.RosterStartSession != 1 {
		return false, fmt.Errorf("roster_start_session=%d is unsupported in this importer; use the in-app late-join flow", row.RosterStartSession)
	}
	if strings.TrimSpace(row.Status) == "" {
		row.Status = "in_classes"
	}

	var leadID string
	err = tx.QueryRow(`SELECT id::text FROM leads WHERE phone = $1`, row.Phone).Scan(&leadID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	exists = err == nil
	if !exists {
		leadID = uuid.New().String()
	}

	remainingCredits := row.LevelsPurchasedTotal - row.LevelsConsumed
	if remainingCredits < 0 {
		remainingCredits = 0
	}

	if !dryRun {
		if exists {
			_, err = tx.Exec(`
				UPDATE leads
				SET full_name = $1,
				    source = NULLIF($2, ''),
				    notes = NULLIF($3, ''),
				    status = $4,
				    sent_to_classes = true,
				    is_returning = $5,
				    levels_purchased_total = $6,
				    levels_consumed = $7,
				    remaining_credits = $8,
				    updated_at = NOW()
				WHERE id = $9::uuid
			`, row.FullName, row.Source, row.Notes, row.Status, row.IsReturning, row.LevelsPurchasedTotal, row.LevelsConsumed, remainingCredits, leadID)
		} else {
			_, err = tx.Exec(`
				INSERT INTO leads (
					id, full_name, phone, source, notes, status, sent_to_classes,
					is_returning, levels_purchased_total, levels_consumed, remaining_credits,
					created_at, updated_at
				)
				VALUES (
					$1::uuid, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, true,
					$7, $8, $9, $10,
					NOW(), NOW()
				)
			`, leadID, row.FullName, row.Phone, row.Source, row.Notes, row.Status, row.IsReturning, row.LevelsPurchasedTotal, row.LevelsConsumed, remainingCredits)
		}
		if err != nil {
			return false, err
		}

		_, err = tx.Exec(`
			INSERT INTO placement_tests (id, lead_id, assigned_level, updated_at)
			VALUES (gen_random_uuid(), $1::uuid, $2, NOW())
			ON CONFLICT (lead_id) DO UPDATE SET
				assigned_level = EXCLUDED.assigned_level,
				updated_at = EXCLUDED.updated_at
		`, leadID, row.Level)
		if err != nil {
			return false, err
		}

		_, err = tx.Exec(`
			INSERT INTO scheduling (
				id, lead_id, expected_round, class_days, class_time, start_date, start_time, class_group_index, updated_at
			)
			VALUES (
				gen_random_uuid(), $1::uuid, $2, $3, $4::time, $5, $6::time, $7, NOW()
			)
			ON CONFLICT (lead_id) DO UPDATE SET
				expected_round = EXCLUDED.expected_round,
				class_days = EXCLUDED.class_days,
				class_time = EXCLUDED.class_time,
				start_date = EXCLUDED.start_date,
				start_time = EXCLUDED.start_time,
				class_group_index = EXCLUDED.class_group_index,
				updated_at = EXCLUDED.updated_at
		`, leadID, currentRound, row.ClassDays, row.ClassTime, startDate, row.ClassTime, row.ClassNumber)
		if err != nil {
			return false, err
		}
	}

	return exists, nil
}

func findCreatedByUserID(adminEmail string) (sql.NullString, error) {
	if strings.TrimSpace(adminEmail) != "" {
		var id string
		err := db.DB.QueryRow(`
			SELECT id::text
			FROM users
			WHERE LOWER(TRIM(email)) = LOWER(TRIM($1))
			LIMIT 1
		`, adminEmail).Scan(&id)
		if err == nil {
			return sql.NullString{String: id, Valid: true}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return sql.NullString{}, err
		}
	}

	var fallbackID string
	err := db.DB.QueryRow(`
		SELECT id::text
		FROM users
		WHERE role IN ('admin', 'manager')
		ORDER BY created_at
		LIMIT 1
	`).Scan(&fallbackID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, nil
	}
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: fallbackID, Valid: true}, nil
}

func getCurrentRound() (string, error) {
	var currentRound string
	err := db.DB.QueryRow(`SELECT value FROM settings WHERE key = 'current_round'`).Scan(&currentRound)
	if errors.Is(err, sql.ErrNoRows) {
		return "1", nil
	}
	return currentRound, err
}

func mustLoadMentors(path string) []mentorRow {
	records := mustReadCSV(path)
	var out []mentorRow
	for _, rec := range records {
		out = append(out, mentorRow{
			Email:    normalizeEmail(rec["email"]),
			FullName: strings.TrimSpace(rec["full_name"]),
			Phone:    digitsOnly(rec["phone"]),
			IsActive: mustParseBool(rec["is_active"], path, "is_active"),
		})
	}
	return out
}

func mustLoadClasses(path string) []classRow {
	records := mustReadCSV(path)
	var out []classRow
	for _, rec := range records {
		out = append(out, classRow{
			ClassKey:             strings.TrimSpace(rec["class_key"]),
			Level:                mustParseInt(rec["level"], path, "level"),
			ClassDays:            strings.TrimSpace(rec["class_days"]),
			ClassTime:            normalizeClock(rec["class_time"]),
			ClassNumber:          mustParseInt(rec["class_number"], path, "class_number"),
			MentorEmail:          normalizeEmail(rec["mentor_email"]),
			CurrentSessionNumber: mustParseInt(rec["current_session_number"], path, "current_session_number"),
		})
	}
	return out
}

func mustLoadStudents(path string) []studentRow {
	records := mustReadCSV(path)
	var out []studentRow
	for _, rec := range records {
		purchased := mustParseInt(rec["levels_purchased_total"], path, "levels_purchased_total")
		consumed := mustParseInt(rec["levels_consumed"], path, "levels_consumed")
		out = append(out, studentRow{
			ExternalID:           strings.TrimSpace(rec["student_external_id"]),
			FullName:             strings.TrimSpace(rec["full_name"]),
			Phone:                digitsOnly(rec["phone"]),
			ClassKey:             strings.TrimSpace(rec["class_key"]),
			Level:                mustParseInt(rec["level"], path, "level"),
			ClassDays:            strings.TrimSpace(rec["class_days"]),
			ClassTime:            normalizeClock(rec["class_time"]),
			ClassNumber:          mustParseInt(rec["class_number"], path, "class_number"),
			RosterStartSession:   mustParseInt(rec["roster_start_session"], path, "roster_start_session"),
			Status:               strings.TrimSpace(rec["status"]),
			IsReturning:          mustParseBool(rec["is_returning"], path, "is_returning"),
			LevelsPurchasedTotal: purchased,
			LevelsConsumed:       consumed,
			Source:               strings.TrimSpace(rec["source"]),
			Notes:                strings.TrimSpace(rec["notes"]),
		})
	}
	return out
}

func validateCrossReferences(mentors []mentorRow, classes []classRow, students []studentRow) {
	mentorEmails := make(map[string]struct{}, len(mentors))
	for _, row := range mentors {
		if row.Email == "" || row.FullName == "" || row.Phone == "" {
			log.Fatalf("invalid mentor row: email/full_name/phone are required: %+v", row)
		}
		if _, exists := mentorEmails[row.Email]; exists {
			log.Fatalf("duplicate mentor email in CSV: %s", row.Email)
		}
		mentorEmails[row.Email] = struct{}{}
	}

	classKeys := make(map[string]classRow, len(classes))
	for _, row := range classes {
		if row.ClassKey == "" {
			log.Fatal("class row missing class_key")
		}
		if row.ClassKey != fmt.Sprintf("L%d|%s|%s|%d", row.Level, row.ClassDays, row.ClassTime, row.ClassNumber) {
			log.Fatalf("class row %s does not match canonical key", row.ClassKey)
		}
		if _, ok := mentorEmails[row.MentorEmail]; !ok {
			log.Fatalf("class %s references mentor email %s not present in mentors CSV", row.ClassKey, row.MentorEmail)
		}
		if _, exists := classKeys[row.ClassKey]; exists {
			log.Fatalf("duplicate class_key in CSV: %s", row.ClassKey)
		}
		classKeys[row.ClassKey] = row
	}

	phones := make(map[string]struct{}, len(students))
	for _, row := range students {
		if row.FullName == "" || row.Phone == "" || row.ClassKey == "" {
			log.Fatalf("invalid student row: name/phone/class_key are required: %+v", row)
		}
		if _, ok := classKeys[row.ClassKey]; !ok {
			log.Fatalf("student %s references unknown class_key %s", row.ExternalID, row.ClassKey)
		}
		if _, exists := phones[row.Phone]; exists {
			log.Fatalf("duplicate student phone in CSV: %s", row.Phone)
		}
		phones[row.Phone] = struct{}{}
	}
}

func mustReadCSV(path string) []map[string]string {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("read csv %s: %v", path, err)
	}
	if len(rows) < 1 {
		log.Fatalf("csv %s is empty", path)
	}
	headers := rows[0]
	var out []map[string]string
	for i, row := range rows[1:] {
		if isEmptyCSVRow(row) {
			continue
		}
		if len(row) != len(headers) {
			log.Fatalf("csv %s row %d has %d columns, expected %d", path, i+2, len(row), len(headers))
		}
		rec := make(map[string]string, len(headers))
		for j, h := range headers {
			rec[strings.TrimSpace(h)] = strings.TrimSpace(row[j])
		}
		out = append(out, rec)
	}
	return out
}

func mustParseDate(flagName, value string) time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		log.Fatalf("invalid %s value %q: %v", flagName, value, err)
	}
	return t
}

func mustParseInt(value, path, field string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		log.Fatalf("invalid integer in %s field %s: %q", path, field, value)
	}
	return n
}

func mustParseBool(value, path, field string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default:
		log.Fatalf("invalid boolean in %s field %s: %q", path, field, value)
		return false
	}
}

func parseClock(value string) (time.Time, error) {
	value = normalizeClock(value)
	if t, err := time.Parse("15:04", value); err == nil {
		return t, nil
	}
	return time.Parse("15:04:05", value)
}

func normalizeClock(value string) string {
	value = strings.TrimSpace(value)
	if t, err := time.Parse("15:04:05", value); err == nil {
		return t.Format("15:04")
	}
	if t, err := time.Parse("15:04", value); err == nil {
		return t.Format("15:04")
	}
	return value
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isEmptyCSVRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func combineDateAndClock(date time.Time, clock string) time.Time {
	t, err := parseClock(clock)
	if err != nil {
		return date
	}
	return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
}

func nullUUIDParam(v sql.NullString) interface{} {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	return v.String
}
