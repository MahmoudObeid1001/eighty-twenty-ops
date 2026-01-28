package main

import (
	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Find an active class
	var classKey string
	err := db.DB.QueryRow("SELECT class_key FROM class_groups WHERE round_status = 'active' LIMIT 1").Scan(&classKey)
	if err != nil {
		fmt.Println("No active class found in class_groups, searching anyway...")
		// fallback to any class
		err = db.DB.QueryRow("SELECT class_key FROM class_sessions LIMIT 1").Scan(&classKey)
		if err != nil {
			log.Fatal("No sessions found")
		}
	}
	fmt.Printf("Verifying with ClassKey: %s\n", classKey)

	cg, students, sessions, missedSessions, feedbackRecords, completedCount, err := models.GetStudentSuccessClassDetail(classKey)
	if err != nil {
		log.Fatalf("Error getting detail: %v", err)
	}

	fmt.Printf("\nClass: %s, Sessions Completed: %d/%d, Feedbacks: %d\n", cg.ClassKey, completedCount, len(sessions), len(feedbackRecords))
	fmt.Printf("Current Session (if active): %d\n", completedCount+1)

	fmt.Println("\nStudents Attendance Details:")
	for _, s := range students {
		ms := missedSessions[s.LeadID]
		fmt.Printf("- %s: MissedCount=%d, Sessions=%v\n", s.FullName, len(ms), ms)
	}

	fmt.Println("\nAbsence Feed Sample (First 5):")
	feed, err := models.GetAbsenceFeed(classKey, "all", "")
	if err != nil {
		log.Fatalf("Error getting feed: %v", err)
	}
	for i, item := range feed {
		if i >= 5 {
			break
		}
		fmt.Printf("- Session %d: %s (%s) - %s\n", item.SessionNumber, item.StudentName, item.Status, item.MarkedAt)
	}
}
