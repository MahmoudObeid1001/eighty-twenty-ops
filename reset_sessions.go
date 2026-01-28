package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:postgres@localhost:5432/eightytwenty?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	res, err := db.Exec("UPDATE class_sessions SET status = 'scheduled'")
	if err != nil {
		log.Fatal(err)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("Reset %d sessions to scheduled\n", n)
}
