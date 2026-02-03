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

	// Find Belal
	fmt.Println("=== SEARCHING FOR BELAL ===")
	rows, err := db.Query(`
		SELECT 
			l.id,
			l.full_name,
			l.phone,
			l.status,
			l.sent_to_classes,
			COALESCE(pt.assigned_level::text, 'NULL') as level,
			COALESCE(s.class_days, 'NULL') as days,
			COALESCE(s.class_time::text, 'NULL') as time,
			COALESCE(s.class_group_index::text, 'NULL') as group_idx
		FROM leads l
		LEFT JOIN placement_tests pt ON pt.lead_id = l.id
		LEFT JOIN scheduling s ON s.lead_id = l.id
		WHERE l.full_name ILIKE '%belal%'
		ORDER BY l.created_at DESC
		LIMIT 5
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, phone, status, level, days, time, groupIdx string
		var sentToClasses sql.NullBool
		err := rows.Scan(&id, &name, &phone, &status, &sentToClasses, &level, &days, &time, &groupIdx)
		if err != nil {
			log.Fatal(err)
		}
		sent := "false"
		if sentToClasses.Valid && sentToClasses.Bool {
			sent = "true"
		}
		fmt.Printf("ID: %s\nName: %s\nPhone: %s\nStatus: %s\nSent to classes: %s\nLevel: %s\nDays: %s\nTime: %s\nGroup: %s\n\n",
			id, name, phone, status, sent, level, days, time, groupIdx)
	}

	// Check late_joiners
	fmt.Println("=== LATE JOINER RECORDS ===")
	rows2, err := db.Query(`
		SELECT 
			lj.id,
			lj.class_key,
			lj.joined_at_session_number,
			lj.reason,
			l.full_name
		FROM late_joiners lj
		INNER JOIN leads l ON l.id = lj.lead_id
		WHERE l.full_name ILIKE '%belal%'
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var id, classKey, reason, name string
		var session int
		err := rows2.Scan(&id, &classKey, &session, &reason, &name)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Late Join ID: %s\nStudent: %s\nClass: %s\nSession: %d\nReason: %s\n\n",
			id, name, classKey, session, reason)
	}

	// Check class status
	fmt.Println("=== CLASS STATUS ===")
	rows3, err := db.Query(`
		SELECT 
			class_key,
			round_status,
			sent_to_mentor
		FROM class_groups
		WHERE level = 5 
		  AND class_days = 'Sat/Tues'
		  AND class_time = '07:30:00'
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows3.Close()

	for rows3.Next() {
		var classKey, roundStatus string
		var sentToMentor bool
		err := rows3.Scan(&classKey, &roundStatus, &sentToMentor)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Class: %s\nRound Status: %s\nSent to Mentor: %v\n\n",
			classKey, roundStatus, sentToMentor)
	}
}
