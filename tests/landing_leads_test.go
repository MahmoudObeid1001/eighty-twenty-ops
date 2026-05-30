package tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/handlers"
)

func TestCreateLandingLead(t *testing.T) {
	cfg := config.Load()
	cfg.LandingLeadToken = "test-landing-token"
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	h := handlers.NewAPIHandler(cfg)
	nowSuffix := time.Now().UnixNano()

	t.Run("rejects missing token", func(t *testing.T) {
		body := bytes.NewBufferString(`{"full_name":"Landing Missing Token","whatsapp_number":"01000000000"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/public/landing-leads", body)
		res := httptest.NewRecorder()

		h.CreateLandingLead(res, req)

		requireErrorResponse(t, res, http.StatusUnauthorized, "Unauthorized")
	})

	t.Run("creates normal lead from landing payload", func(t *testing.T) {
		phone := uniquePhone(nowSuffix, 90)
		payload := map[string]string{
			"full_name":       "Landing Lead",
			"whatsapp_number": phone,
		}
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/public/landing-leads", bytes.NewReader(raw))
		req.Header.Set("X-Landing-Lead-Token", cfg.LandingLeadToken)
		res := httptest.NewRecorder()

		h.CreateLandingLead(res, req)

		if res.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, res.Code, res.Body.String())
		}
		var leadID string
		var source, notes sql.NullString
		var status string
		if err := db.DB.QueryRow(`
			SELECT id, source, notes, status
			FROM leads
			WHERE phone = $1
		`, phone).Scan(&leadID, &source, &notes, &status); err != nil {
			t.Fatalf("failed to load created lead: %v", err)
		}
		t.Cleanup(func() {
			mustExec(t, `DELETE FROM leads WHERE id = $1`, leadID)
		})

		if !source.Valid || source.String != "Landing Page" {
			t.Fatalf("expected Landing Page source, got %q", source.String)
		}
		if !notes.Valid || notes.String != "Landing page signup" {
			t.Fatalf("expected landing signup note, got %q", notes.String)
		}
		if status != "lead_created" {
			t.Fatalf("expected lead_created status, got %s", status)
		}
	})

	t.Run("rejects invalid level", func(t *testing.T) {
		phone := uniquePhone(nowSuffix, 91)
		body := bytes.NewBufferString(fmt.Sprintf(`{"full_name":"Landing Invalid","whatsapp_number":%q,"english_level":"fluent"}`, phone))
		req := httptest.NewRequest(http.MethodPost, "/api/public/landing-leads", body)
		req.Header.Set("X-Landing-Lead-Token", cfg.LandingLeadToken)
		res := httptest.NewRecorder()

		h.CreateLandingLead(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "english_level must be beginner, intermediate, or advanced")
	})
}
