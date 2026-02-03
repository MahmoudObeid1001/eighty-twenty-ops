package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Check late joiner notifications
	fmt.Println("=== LATE JOINER NOTIFICATIONS ===")
	rows, err := db.Query(`
		SELECT 
			n.id,
			n.lead_id,
			l.full_name,
			n.class_key,
			n.user_id,
			u.email,
			u.role,
			n.joined_at_session_number,
			n.acknowledged_at
		FROM late_joiner_notifications n
		INNER JOIN leads l ON l.id = n.lead_id
		INNER JOIN users u ON u.id = n.user_id
		WHERE l.full_name ILIKE '%belal%'
		ORDER BY n.created_at DESC
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, leadID, fullName, classKey, userID, email, role string
		var sessionNum int
		var ackAt sql.NullTime
		err := rows.Scan(&id, &leadID, &fullName, &classKey, &userID, &email, &role, &sessionNum, &ackAt)
		if err != nil {
			log.Fatal(err)
		}
		ack := "NO"
		if ackAt.Valid {
			ack = "YES"
		}
		fmt.Printf("Notification ID: %s\nStudent: %s\nClass: %s\nUser: %s (%s)\nSession: %d\nAcknowledged: %s\n\n",
			id, fullName, classKey, email, role, sessionNum, ack)
		count++
	}

	if count == 0 {
		fmt.Println("❌ NO NOTIFICATIONS FOUND FOR BELAL!")
		fmt.Println("\nThis is the problem - notifications were not created during late join.")
	} else {
		fmt.Printf("✅ Found %d notification(s)\n", count)
	}
}
