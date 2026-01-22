package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var DB *sql.DB

func Connect(databaseURL string) error {
	var err error
	DB, err = sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	ctx := context.Background()
	if err := DB.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Database connection established")
	return nil
}

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
