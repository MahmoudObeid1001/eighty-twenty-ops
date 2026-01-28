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

	// Get a class with students
	var classKey string
	err := db.DB.QueryRow(`
		SELECT cg.class_key 
		FROM class_groups cg
		JOIN scheduling s ON s.class_days = cg.class_days 
			AND s.class_time::text = cg.class_time 
			AND COALESCE(s.class_group_index, 1) = COALESCE(cg.class_number, 1)
		LIMIT 1
	`).Scan(&classKey)
	if err != nil {
		log.Fatalf("No class found: %v", err)
	}
	fmt.Printf("Testing ClassKey: %s\n\n", classKey)

	// Get students
	students, err := models.GetStudentsInClassGroup(classKey)
	if err != nil || len(students) == 0 {
		log.Fatal("No students in class")
	}

	fmt.Println("=== STUDENTS ORDER ===")
	for i, s := range students {
		fmt.Printf("[%d] %s (ID: %s)\n", i, s.FullName, s.LeadID)
	}

	// Get sessions
	sessions, err := models.GetClassSessions(classKey)
	if err != nil || len(sessions) == 0 {
		log.Fatal("No sessions in class")
	}

	fmt.Println("\n=== SESSIONS ===")
	for _, s := range sessions {
		fmt.Printf("Session %d (ID: %s) - Status: %s\n", s.SessionNumber, s.ID, s.Status)
	}

	// Get attendance for each session
	fmt.Println("\n=== ATTENDANCE RECORDS ===")
	for _, session := range sessions {
		attendance, err := models.GetAttendanceForSession(session.ID)
		if err != nil {
			fmt.Printf("Error getting attendance for session %d: %v\n", session.SessionNumber, err)
			continue
		}

		if len(attendance) == 0 {
			fmt.Printf("Session %d: No attendance records\n", session.SessionNumber)
			continue
		}

		fmt.Printf("Session %d:\n", session.SessionNumber)
		for _, att := range attendance {
			// Find student name
			var studentName string
			for _, s := range students {
				if s.LeadID == att.LeadID {
					studentName = s.FullName
					break
				}
			}
			if studentName == "" {
				studentName = "UNKNOWN"
			}
			fmt.Printf("  - %s (ID: %s): %s\n", studentName, att.LeadID, att.Status)
		}
	}

	// Simulate GetClassWorkspace logic
	fmt.Println("\n=== SIMULATED WORKSPACE RESPONSE ===")
	type StudentResponse struct {
		FullName    string
		LeadID      string
		MissedCount int
		Attendance  map[string]string
	}

	studentList := make([]StudentResponse, 0, len(students))
	for _, s := range students {
		swa := StudentResponse{
			LeadID:     s.LeadID.String(),
			FullName:   s.FullName,
			Attendance: make(map[string]string),
		}

		// Get attendance for each session
		for _, session := range sessions {
			attendance, err := models.GetAttendanceForSession(session.ID)
			if err == nil {
				for _, att := range attendance {
					if att.LeadID == s.LeadID {
						swa.Attendance[session.ID.String()] = att.Status
						if att.Status == "ABSENT" {
							swa.MissedCount++
						}
						break
					}
				}
			}
		}
		studentList = append(studentList, swa)
	}

	for i, s := range studentList {
		fmt.Printf("[%d] %s (ID: %s...): MissedCount=%d\n",
			i, s.FullName, s.LeadID[:8], s.MissedCount)
	}
}
