package db

import (
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func RunMigrations() error {
	if DB == nil {
		return fmt.Errorf("database connection not initialized")
	}

	// Create migrations table if it doesn't exist
	_, err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Read all migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Sort migration files
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrationFiles = append(migrationFiles, entry.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Apply each migration
	for _, filename := range migrationFiles {
		// Check if already applied
		var count int
		err := DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = $1", filename).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}

		if count > 0 {
			log.Printf("Migration %s already applied, skipping", filename)
			continue
		}

		// Read migration file
		content, err := migrationsFS.ReadFile(filepath.Join("migrations", filename))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", filename, err)
		}

		// Execute migration
		log.Printf("Applying migration: %s", filename)
		_, err = DB.Exec(string(content))
		if err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", filename, err)
		}

		// Record migration
		_, err = DB.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", filename)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", filename, err)
		}

		log.Printf("Migration %s applied successfully", filename)
	}

	return nil
}
