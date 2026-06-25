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
		body := bytes.NewBufferString(`{"full_name":"Landing Missing Token","whatsapp_number":"01000000000","learning_goal":"الشغل والترقي"}`)
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
			"learning_goal":   "السفر والهجرة",
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
		if !notes.Valid || notes.String != "Landing page signup\nLearning goal: السفر والهجرة" {
			t.Fatalf("expected landing signup note, got %q", notes.String)
		}
		if status != "lead_created" {
			t.Fatalf("expected lead_created status, got %s", status)
		}
	})

	t.Run("stores private course metadata in notes", func(t *testing.T) {
		phone := uniquePhone(nowSuffix, 94)
		payload := map[string]string{
			"full_name":        "Private Courses Lead",
			"whatsapp_number":  phone,
			"learning_goal":    "الشغل والترقي",
			"source":           "private_courses",
			"current_job":      "Marketing Specialist",
			"current_level":    "B1",
			"english_need":     "Interview prep",
			"selected_package": "2",
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
		var source, notes, landingSource, currentJob, currentLevel, englishNeed, selectedPackage sql.NullString
		if err := db.DB.QueryRow(`
			SELECT id, source, notes, landing_source, current_job, current_level, english_need, selected_package
			FROM leads
			WHERE phone = $1
		`, phone).Scan(&leadID, &source, &notes, &landingSource, &currentJob, &currentLevel, &englishNeed, &selectedPackage); err != nil {
			t.Fatalf("failed to load created lead: %v", err)
		}
		t.Cleanup(func() {
			mustExec(t, `DELETE FROM leads WHERE id = $1`, leadID)
		})

		if !source.Valid || source.String != "Landing Page" {
			t.Fatalf("expected Landing Page source, got %q", source.String)
		}
		wantNotes := "Landing page signup\nLearning goal: الشغل والترقي\nLanding source: private_courses\nCurrent job: Marketing Specialist\nCurrent level: B1\nEnglish need: Interview prep\nSelected package: 2"
		if !notes.Valid || notes.String != wantNotes {
			t.Fatalf("expected private course notes %q, got %q", wantNotes, notes.String)
		}
		if !landingSource.Valid || landingSource.String != "private_courses" {
			t.Fatalf("expected landing_source private_courses, got %q", landingSource.String)
		}
		if !currentJob.Valid || currentJob.String != "Marketing Specialist" {
			t.Fatalf("expected current_job Marketing Specialist, got %q", currentJob.String)
		}
		if !currentLevel.Valid || currentLevel.String != "B1" {
			t.Fatalf("expected current_level B1, got %q", currentLevel.String)
		}
		if !englishNeed.Valid || englishNeed.String != "Interview prep" {
			t.Fatalf("expected english_need Interview prep, got %q", englishNeed.String)
		}
		if !selectedPackage.Valid || selectedPackage.String != "2" {
			t.Fatalf("expected selected_package 2, got %q", selectedPackage.String)
		}
	})

	t.Run("rejects missing learning goal", func(t *testing.T) {
		phone := uniquePhone(nowSuffix, 91)
		body := bytes.NewBufferString(fmt.Sprintf(`{"full_name":"Landing Missing Goal","whatsapp_number":%q}`, phone))
		req := httptest.NewRequest(http.MethodPost, "/api/public/landing-leads", body)
		req.Header.Set("X-Landing-Lead-Token", cfg.LandingLeadToken)
		res := httptest.NewRecorder()

		h.CreateLandingLead(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "full_name, whatsapp_number, and learning_goal are required")
	})

	t.Run("rejects invalid level", func(t *testing.T) {
		phone := uniquePhone(nowSuffix, 92)
		body := bytes.NewBufferString(fmt.Sprintf(`{"full_name":"Landing Invalid","whatsapp_number":%q,"learning_goal":"الشغل والترقي","english_level":"fluent"}`, phone))
		req := httptest.NewRequest(http.MethodPost, "/api/public/landing-leads", body)
		req.Header.Set("X-Landing-Lead-Token", cfg.LandingLeadToken)
		res := httptest.NewRecorder()

		h.CreateLandingLead(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "english_level must be beginner, intermediate, or advanced")
	})

	t.Run("rejects invalid learning goal", func(t *testing.T) {
		phone := uniquePhone(nowSuffix, 93)
		body := bytes.NewBufferString(fmt.Sprintf(`{"full_name":"Landing Invalid Goal","whatsapp_number":%q,"learning_goal":"anything else"}`, phone))
		req := httptest.NewRequest(http.MethodPost, "/api/public/landing-leads", body)
		req.Header.Set("X-Landing-Lead-Token", cfg.LandingLeadToken)
		res := httptest.NewRecorder()

		h.CreateLandingLead(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "learning_goal must be one of the supported landing page options")
	})
}
