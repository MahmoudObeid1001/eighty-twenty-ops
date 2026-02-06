package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

func main() {
	days := flag.Int("days", 0, "Limit to class_enrollments completed within last N days (0 = all)")
	phonesRaw := flag.String("phones", "", "Comma-separated phone numbers to limit backfill (optional)")
	dryRun := flag.Bool("dry-run", false, "Print counts only, do not write updates")
	dbURL := flag.String("db", "postgres://postgres:postgres@localhost:5432/eightytwenty?sslmode=disable", "Postgres connection string")
	flag.Parse()

	db, err := sql.Open("postgres", *dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	whereDate := ""
	var args []interface{}
	if *days > 0 {
		whereDate = "AND ce.completed_at >= $1"
		args = append(args, time.Now().AddDate(0, 0, -*days))
	}

	phoneFilter := ""
	if strings.TrimSpace(*phonesRaw) != "" {
		phones := strings.Split(*phonesRaw, ",")
		for i := range phones {
			phones[i] = strings.TrimSpace(phones[i])
		}
		args = append(args, pq.Array(phones))
		phoneFilter = fmt.Sprintf("AND l.phone = ANY($%d)", len(args))
	}

	query := fmt.Sprintf(`
		WITH latest_outcome AS (
			SELECT DISTINCT ON (ce.lead_id)
				ce.lead_id,
				ce.outcome,
				ce.final_grade,
				COALESCE(ce.completed_at, ce.enrolled_at) AS completed_at
			FROM class_enrollments ce
			WHERE ce.completed_at IS NOT NULL
			%s
			ORDER BY ce.lead_id, COALESCE(ce.completed_at, ce.enrolled_at) DESC
		)
		SELECT COUNT(*) FROM latest_outcome lo
		INNER JOIN leads l ON l.id = lo.lead_id
		WHERE l.status NOT IN ('in_classes', 'cancelled')
		%s
	`, whereDate, phoneFilter)

	var total int
	if err := db.QueryRow(query, args...).Scan(&total); err != nil {
		log.Fatal(err)
	}
	log.Printf("Eligible leads to backfill: %d", total)

	if *dryRun {
		log.Println("Dry run enabled; no updates performed.")
		return
	}

	updateQuery := fmt.Sprintf(`
		WITH latest_outcome AS (
			SELECT DISTINCT ON (ce.lead_id)
				ce.lead_id,
				ce.outcome,
				ce.final_grade,
				COALESCE(ce.completed_at, ce.enrolled_at) AS completed_at
			FROM class_enrollments ce
			WHERE ce.completed_at IS NOT NULL
			%s
			ORDER BY ce.lead_id, COALESCE(ce.completed_at, ce.enrolled_at) DESC
		),
		credits AS (
			SELECT
				l.id,
				GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) AS remaining_credits
			FROM leads l
		),
		targets AS (
			SELECT l.id, c.remaining_credits
			FROM latest_outcome lo
			INNER JOIN leads l ON l.id = lo.lead_id
			INNER JOIN credits c ON c.id = l.id
			WHERE l.status NOT IN ('in_classes', 'cancelled')
			%s
		)
		UPDATE leads l
		SET
			remaining_credits = t.remaining_credits,
			status = CASE WHEN t.remaining_credits > 0 THEN 'waiting_for_round' ELSE 'renewal_pending' END,
			is_returning = true,
			high_priority_follow_up = CASE WHEN t.remaining_credits <= 0 THEN true ELSE l.high_priority_follow_up END,
			updated_at = NOW()
		FROM targets t
		WHERE l.id = t.id
	`, whereDate, phoneFilter)

	res, err := db.Exec(updateQuery, args...)
	if err != nil {
		log.Fatal(err)
	}
	affected, _ := res.RowsAffected()
	log.Printf("Backfill complete. Updated %d lead(s).", affected)
}
