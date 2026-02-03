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

	// Fix Belal's status
	fmt.Println("=== FIXING BELAL'S STATUS ===")
	result, err := db.Exec(`
		UPDATE leads
		SET status = 'in_classes', updated_at = NOW()
		WHERE full_name ILIKE '%belal%'
		  AND status != 'in_classes'
	`)
	if err != nil {
		log.Fatal(err)
	}

	rows, _ := result.RowsAffected()
	fmt.Printf("Updated %d lead(s) to status 'in_classes'\n\n", rows)

	// Verify
	fmt.Println("=== VERIFICATION ===")
	var id, name, status string
	err = db.QueryRow(`
		SELECT id, full_name, status
		FROM leads
		WHERE full_name ILIKE '%belal%'
	`).Scan(&id, &name, &status)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("ID: %s\nName: %s\nStatus: %s\n", id, name, status)
	fmt.Println("\n✅ Belal should now appear in the class roster!")
}
