// Seeder command for populating demo leads for testing /classes board.
//
// SAFETY: This command ONLY runs when:
//   - APP_ENV=development
//   - --confirm flag is provided
//
// Usage:
//   APP_ENV=development go run cmd/seed/main.go --count 25 --confirm
//
// Default count is 25 if --count is not provided.
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

	"github.com/google/uuid"
)

func main() {
	// Parse flags
	count := flag.Int("count", 25, "Number of students to seed")
	confirm := flag.Bool("confirm", false, "Confirm seeding (required)")
	flag.Parse()

	// Safety check: APP_ENV must be development
	appEnv := os.Getenv("APP_ENV")
	if appEnv != "development" {
		log.Fatalf("ERROR: Seeder can only run in development environment.")
		log.Fatalf("       Set APP_ENV=development and try again.")
	}

	// Safety check: --confirm flag required
	if !*confirm {
		log.Fatalf("ERROR: --confirm flag is required to run seeder.")
		log.Fatalf("       Usage: APP_ENV=development go run cmd/seed/main.go --count %d --confirm", *count)
	}

	// Load config and connect to database
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Do NOT run migrations - assume DB is already set up

	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("SEEDER: Preparing to insert %d READY_TO_START students", *count)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Get admin user ID for created_by_user_id (optional, can be null)
	adminUser, err := models.GetUserByEmail(cfg.AdminEmail)
	var adminUserID string
	if err == nil {
		adminUserID = adminUser.ID.String()
	}

	// Data distribution arrays
	classDaysOptions := []string{"Sun/Wed", "Sat/Tues", "Mon/Thu"}
	classTimeOptions := []string{"07:30", "10:00"}
	levels := []int32{1, 2, 3, 4, 5, 6, 7, 8}

	// Track distribution for summary
	type groupKey struct {
		Level     int32
		ClassDays string
		ClassTime string
	}
	groupCounts := make(map[groupKey]int)

	// Plan distribution to create interesting scenarios:
	// - Some groups with 6 students (LOCKED)
	// - Some groups with 5 students (READY)
	// - Some groups with 4 students (READY)
	// - Some groups with 3 students (NOT READY)
	// We'll create groups in a pattern that demonstrates all states
	groupPlan := []struct {
		Level     int32
		ClassDays string
		ClassTime string
		Count     int
	}{
		// Level 1: Create LOCKED (6), READY (5), READY (4), NOT READY (3)
		{1, "Sun/Wed", "07:30", 6},  // LOCKED
		{1, "Sun/Wed", "10:00", 5},  // READY
		{1, "Sat/Tues", "07:30", 4}, // READY
		{1, "Sat/Tues", "10:00", 3}, // NOT READY
		// Level 2: Similar pattern
		{2, "Sun/Wed", "07:30", 6},  // LOCKED
		{2, "Sun/Wed", "10:00", 5},  // READY
		{2, "Mon/Thu", "07:30", 4},  // READY
		{2, "Mon/Thu", "10:00", 3},  // NOT READY
		// Level 3: Fill remaining slots
		{3, "Sun/Wed", "07:30", 2}, // NOT READY (if we have slots left)
	}

	// Calculate total planned
	totalPlanned := 0
	for _, plan := range groupPlan {
		totalPlanned += plan.Count
	}

	// If count is less than planned, adjust
	// If count is more than planned, fill remaining with round-robin
	inserted := 0
	now := time.Now()
	planIndex := 0
	planStudentIndex := 0

	for i := 1; i <= *count; i++ {
		// Generate name and phone
		name := fmt.Sprintf("Seed Student %02d", i)
		phone := fmt.Sprintf("010000000%02d", i)

		var level int32
		var classDays, classTime string

		// Use planned distribution if we haven't exhausted it
		if planIndex < len(groupPlan) && planStudentIndex < groupPlan[planIndex].Count {
			level = groupPlan[planIndex].Level
			classDays = groupPlan[planIndex].ClassDays
			classTime = groupPlan[planIndex].ClassTime
			planStudentIndex++
			if planStudentIndex >= groupPlan[planIndex].Count {
				planIndex++
				planStudentIndex = 0
			}
		} else {
			// Fallback: round-robin distribution
			level = levels[(i-1)%len(levels)]
			classDays = classDaysOptions[(i-1)%len(classDaysOptions)]
			classTime = classTimeOptions[(i-1)%len(classTimeOptions)]
		}

		// Create lead with status ready_to_start
		leadID := uuid.New()
		var createdByID sql.NullString
		if adminUserID != "" {
			createdByID = sql.NullString{String: adminUserID, Valid: true}
		}

		// Insert lead
		_, err := db.DB.Exec(`
			INSERT INTO leads (id, full_name, phone, source, notes, status, created_by_user_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, leadID, name, phone, sql.NullString{String: "seeder", Valid: true}, sql.NullString{String: "Demo data for testing classes board", Valid: true}, "ready_to_start", createdByID, now, now)
		if err != nil {
			log.Printf("ERROR: Failed to insert lead %d (%s): %v", i, name, err)
			continue
		}

		// Insert placement test with assigned_level
		_, err = db.DB.Exec(`
			INSERT INTO placement_tests (id, lead_id, assigned_level, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3)
		`, leadID, level, now)
		if err != nil {
			log.Printf("ERROR: Failed to insert placement test for lead %d: %v", i, err)
			// Continue anyway - we'll clean up orphaned leads manually if needed
		}

		// Insert scheduling with class_days and class_time
		// Note: class_group_index will be auto-assigned when viewing /classes
		_, err = db.DB.Exec(`
			INSERT INTO scheduling (id, lead_id, class_days, class_time, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, $4)
		`, leadID, classDays, classTime, now)
		if err != nil {
			log.Printf("ERROR: Failed to insert scheduling for lead %d: %v", i, err)
			// Continue anyway
		}

		inserted++
		key := groupKey{Level: level, ClassDays: classDays, ClassTime: classTime}
		groupCounts[key]++
	}

	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("SEEDER: Inserted %d READY_TO_START students", inserted)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("")
	log.Printf("Distribution by (Level, Days, Time):")
	log.Printf("")

	// Sort and print distribution
	type groupKeyWithCount struct {
		Key   groupKey
		Count int
	}
	var sortedGroups []groupKeyWithCount
	for key, count := range groupCounts {
		sortedGroups = append(sortedGroups, groupKeyWithCount{Key: key, Count: count})
	}

	// Simple sort by level, then days, then time
	for i := 0; i < len(sortedGroups)-1; i++ {
		for j := i + 1; j < len(sortedGroups); j++ {
			a, b := sortedGroups[i], sortedGroups[j]
			if a.Key.Level > b.Key.Level ||
				(a.Key.Level == b.Key.Level && a.Key.ClassDays > b.Key.ClassDays) ||
				(a.Key.Level == b.Key.Level && a.Key.ClassDays == b.Key.ClassDays && a.Key.ClassTime > b.Key.ClassTime) {
				sortedGroups[i], sortedGroups[j] = sortedGroups[j], sortedGroups[i]
			}
		}
	}

	for _, item := range sortedGroups {
		readiness := "NOT READY"
		if item.Count >= 6 {
			readiness = "LOCKED"
		} else if item.Count >= 4 {
			readiness = "READY"
		}
		log.Printf("  Level %d, %s @ %s: %d students (%s)", item.Key.Level, item.Key.ClassDays, item.Key.ClassTime, item.Count, readiness)
	}

	log.Printf("")
	log.Printf("✓ Seeding complete. Visit /classes to see the groups.")
	log.Printf("")
}
