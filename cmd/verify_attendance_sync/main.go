package main

import (
	"database/sql"
	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"
	"fmt"
	"log"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func main() {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 1. Get a user for marked_by
	var userID string
	err := db.DB.QueryRow("SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		log.Fatalf("No users found: %v. Please seed database first.", err)
	}
	uID := uuid.MustParse(userID)

	// 2. Get a class with students
	var classKey string
	// Find a class that has students
	err = db.DB.QueryRow(`
		SELECT cg.class_key 
		FROM class_groups cg
		JOIN scheduling s ON s.class_days = cg.class_days 
			AND s.class_time::text = cg.class_time 
			AND COALESCE(s.class_group_index, 1) = COALESCE(cg.class_number, 1)
		LIMIT 1
	`).Scan(&classKey)
	if err != nil {
		log.Fatalf("No class with students found: %v. Please ensure data exists.", err)
	}
	fmt.Printf("Testing with ClassKey: %s\n", classKey)

	// 3. Get a student in that class
	students, err := models.GetStudentsInClassGroup(classKey)
	if err != nil || len(students) == 0 {
		log.Fatal("No students in class")
	}
	student := students[0]
	fmt.Printf("Testing with Student: %s (%s)\n", student.FullName, student.LeadID)

	// 4. Get a session
	sessions, err := models.GetClassSessions(classKey)
	if err != nil || len(sessions) == 0 {
		log.Fatal("No sessions in class")
	}
	session := sessions[0]
	fmt.Printf("Testing with Session: %d (%s)\n", session.SessionNumber, session.ID)

	// 5. Check initial state
	initialStatus := printAttendance(session.ID, student.LeadID)

	// 6. Mark Absent (toggle if already absent)
	newStatus := "ABSENT"
	if initialStatus == "ABSENT" {
		newStatus = "PRESENT"
	}

	fmt.Printf("\nMarking %s...\n", newStatus)
	err = models.MarkAttendance(session.ID, student.LeadID, newStatus, "Verify script", uID)
	if err != nil {
		log.Fatal("MarkAttendance failed:", err)
	}

	// 7. Check state
	printAttendance(session.ID, student.LeadID)

	// 8. Verify Class Workspace Logic
	fmt.Println("\nVerifying Workspace Logic:")
	verifyWorkspaceLogic(classKey, student.LeadID)
}

func printAttendance(sessionID, leadID uuid.UUID) string {
	var status string
	err := db.DB.QueryRow("SELECT status FROM attendance WHERE session_id = $1 AND lead_id = $2", sessionID, leadID).Scan(&status)
	if err == sql.ErrNoRows {
		fmt.Println("DB Status: <No Record>")
		return ""
	} else if err != nil {
		fmt.Printf("DB Error: %v\n", err)
		return ""
	} else {
		fmt.Printf("DB Status: %s\n", status)
		return status
	}
}

func verifyWorkspaceLogic(classKey string, leadID uuid.UUID) {
	sessions, err := models.GetClassSessions(classKey)
	if err != nil {
		log.Fatalf("Failed to get sessions: %v", err)
	}

	missed := 0
	foundAttendance := false

	for _, s := range sessions {
		atts, err := models.GetAttendanceForSession(s.ID)
		if err != nil {
			fmt.Printf("Error getting attendance for session %d: %v\n", s.SessionNumber, err)
			continue
		}

		for _, a := range atts {
			if a.LeadID == leadID {
				fmt.Printf("Session %d: %s\n", s.SessionNumber, a.Status)
				if a.Status == "ABSENT" {
					missed++
				}
				foundAttendance = true
			}
		}
	}

	if !foundAttendance {
		fmt.Println("No attendance records found for student in any session.")
	}
	fmt.Printf("Workspace Logic Missed Count: %d\n", missed)
}
