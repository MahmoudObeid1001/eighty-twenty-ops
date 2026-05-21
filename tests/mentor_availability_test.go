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
	"eighty-twenty-ops/internal/models"
)

func TestMentorAvailabilityWarnings(t *testing.T) {
	cfg := config.Load()
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
	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("avail_mh_%d@eightytwenty.test", nowSuffix))
	mentor := mustCreateUser(t, "mentor", fmt.Sprintf("avail_mentor_%d@eightytwenty.test", nowSuffix))
	otherMentor := mustCreateUser(t, "mentor", fmt.Sprintf("avail_other_%d@eightytwenty.test", nowSuffix))
	hr := mustCreateUser(t, "hr", fmt.Sprintf("avail_hr_%d@eightytwenty.test", nowSuffix))
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM mentor_availability_windows WHERE mentor_user_id IN ($1, $2, $3)`, mentor.ID, otherMentor.ID, hr.ID)
		mustExec(t, `DELETE FROM users WHERE id IN ($1, $2, $3, $4)`, mentorHead.ID, mentor.ID, otherMentor.ID, hr.ID)
	})

	startDate := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	classKey := createClassGroup(t, nowSuffix, 4, "Mon/Thu", "18:00:00", 93001, true, "not_started")
	t.Cleanup(func() { cleanupClass(t, classKey) })
	mustExec(t, `UPDATE class_groups SET suggested_start_date = $1 WHERE class_key = $2`, startDate, classKey)

	sessionDates, err := models.BuildClassSessionDates("Mon/Thu", startDate, 8)
	if err != nil {
		t.Fatalf("failed to build session dates: %v", err)
	}
	fullWindows := make([]models.MentorAvailabilityWindow, 0, len(sessionDates))
	for _, sessionDate := range sessionDates {
		fullWindows = append(fullWindows, models.MentorAvailabilityWindow{
			MentorUserID:  mentor.ID,
			AvailableDate: sessionDate,
			StartTime:     "17:30",
			EndTime:       "20:30",
			Note:          sql.NullString{String: "available", Valid: true},
		})
	}
	if _, err := models.ReplaceMentorAvailabilityWindows(mentor.ID, startDate, fullWindows); err != nil {
		t.Fatalf("failed to seed mentor availability: %v", err)
	}

	t.Run("mentor self edit succeeds", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"windows": []map[string]string{{
				"available_date": "2026-06-01",
				"start_time":     "18:00",
				"end_time":       "20:30",
				"note":           "updated by mentor",
			}},
		})
		req := httptest.NewRequest(http.MethodPut, "/api/mentor/availability?month=2026-06", bytes.NewReader(body))
		req = withUserContext(req, otherMentor.ID, otherMentor.Email, "mentor")
		res := httptest.NewRecorder()

		h.MentorAvailability(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("expected mentor availability update to succeed, got %d body=%s", res.Code, res.Body.String())
		}
	})

	t.Run("mentor head cannot edit mentor availability", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{"windows": []map[string]string{}})
		req := httptest.NewRequest(http.MethodPut, "/api/mentor/availability?month=2026-06", bytes.NewReader(body))
		req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
		res := httptest.NewRecorder()

		h.MentorAvailability(res, req)

		requireErrorResponse(t, res, http.StatusForbidden, "Only mentors")
	})

	t.Run("hr can view mentor availability", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/mentors/"+mentor.ID.String()+"/availability?month=2026-06", nil)
		req = withUserContext(req, hr.ID, hr.Email, "hr")
		res := httptest.NewRecorder()

		h.GetMentorAvailability(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("expected hr availability view to succeed, got %d body=%s", res.Code, res.Body.String())
		}
	})

	t.Run("all sessions covered returns no warnings", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_key": classKey, "mentor_user_id": mentor.ID.String()})
		req := httptest.NewRequest(http.MethodPost, "/api/mentor-head/availability-check", bytes.NewReader(body))
		req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
		res := httptest.NewRecorder()

		h.CheckMentorAvailability(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("availability check failed: status=%d body=%s", res.Code, res.Body.String())
		}
		var payload struct {
			Warnings []models.MentorAvailabilityWarning `json:"availability_warnings"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode availability response: %v", err)
		}
		if len(payload.Warnings) != 0 {
			t.Fatalf("expected no warnings, got %+v", payload.Warnings)
		}
	})

	t.Run("missing month returns warnings and assignment still succeeds", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"class_key": classKey, "mentor_user_id": otherMentor.ID.String()})
		req := httptest.NewRequest(http.MethodPost, "/api/mentor-head/assign-mentor", bytes.NewReader(body))
		req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
		res := httptest.NewRecorder()

		h.AssignMentor(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("expected assignment with availability warnings to succeed, got %d body=%s", res.Code, res.Body.String())
		}
		var payload struct {
			OK       bool                               `json:"ok"`
			Warnings []models.MentorAvailabilityWarning `json:"availability_warnings"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode assign response: %v", err)
		}
		if !payload.OK {
			t.Fatalf("expected ok=true")
		}
		if len(payload.Warnings) == 0 {
			t.Fatalf("expected missing availability warnings")
		}
	})
}

func TestAvailabilityReminder(t *testing.T) {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	nowSuffix := time.Now().UnixNano()
	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("reminder_mh_%d@eightytwenty.test", nowSuffix))
	mentor := mustCreateUser(t, "mentor", fmt.Sprintf("reminder_mentor_%d@eightytwenty.test", nowSuffix))
	coveredMentor := mustCreateUser(t, "mentor", fmt.Sprintf("reminder_covered_%d@eightytwenty.test", nowSuffix))
	monthStart := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	reminderDate := time.Date(2026, time.July, 2, 9, 0, 0, 0, time.UTC)

	t.Cleanup(func() {
		mustExec(t, `DELETE FROM availability_banner_dismissals WHERE user_id IN ($1, $2, $3)`, mentorHead.ID, mentor.ID, coveredMentor.ID)
		mustExec(t, `DELETE FROM mentor_availability_windows WHERE mentor_user_id IN ($1, $2, $3)`, mentorHead.ID, mentor.ID, coveredMentor.ID)
		mustExec(t, `DELETE FROM users WHERE id IN ($1, $2, $3)`, mentorHead.ID, mentor.ID, coveredMentor.ID)
	})

	if _, err := models.ReplaceMentorAvailabilityWindows(coveredMentor.ID, monthStart, []models.MentorAvailabilityWindow{{
		MentorUserID:  coveredMentor.ID,
		AvailableDate: monthStart.AddDate(0, 0, 9),
		StartTime:     "17:00",
		EndTime:       "21:00",
		Note:          sql.NullString{String: "covered", Valid: true},
	}}); err != nil {
		t.Fatalf("failed to seed covered mentor availability: %v", err)
	}

	mentorReminder, err := models.GetAvailabilityReminder(mentor.ID, "mentor", reminderDate)
	if err != nil {
		t.Fatalf("failed to load mentor reminder: %v", err)
	}
	if mentorReminder == nil || mentorReminder.Month != "2026-07" {
		t.Fatalf("expected mentor reminder for 2026-07, got %+v", mentorReminder)
	}

	mhReminder, err := models.GetAvailabilityReminder(mentorHead.ID, "mentor_head", reminderDate)
	if err != nil {
		t.Fatalf("failed to load mentor head reminder: %v", err)
	}
	if mhReminder == nil || mhReminder.MissingCount < 1 {
		t.Fatalf("expected mentor head reminder with missing mentors, got %+v", mhReminder)
	}

	if err := models.DismissAvailabilityReminder(mentor.ID, monthStart); err != nil {
		t.Fatalf("failed to dismiss mentor reminder: %v", err)
	}
	dismissedReminder, err := models.GetAvailabilityReminder(mentor.ID, "mentor", reminderDate)
	if err != nil {
		t.Fatalf("failed to reload mentor reminder after dismissal: %v", err)
	}
	if dismissedReminder != nil {
		t.Fatalf("expected dismissed mentor reminder to stay hidden, got %+v", dismissedReminder)
	}
}

func TestMentorAvailabilityLocking(t *testing.T) {
	cfg := config.Load()
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
	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("lock_mh_%d@eightytwenty.test", nowSuffix))
	mentor := mustCreateUser(t, "mentor", fmt.Sprintf("lock_mentor_%d@eightytwenty.test", nowSuffix))
	classKey := createClassGroup(t, nowSuffix, 5, "Mon/Thu", "18:00:00", 94001, true, "active")
	t.Cleanup(func() {
		cleanupClass(t, classKey)
		mustExec(t, `DELETE FROM mentor_availability_windows WHERE mentor_user_id = $1`, mentor.ID)
		mustExec(t, `DELETE FROM users WHERE id IN ($1, $2)`, mentorHead.ID, mentor.ID)
	})

	mustExec(t, `
		INSERT INTO mentor_assignments (mentor_user_id, class_key, created_by_user_id)
		VALUES ($1, $2, $3)
	`, mentor.ID, classKey, mentorHead.ID)
	mustExec(t, `
		INSERT INTO class_sessions (
			id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time, status, created_at, updated_at
		)
		VALUES
			(gen_random_uuid(), $1, 1, DATE '2026-06-10', '18:00'::time, '20:00'::time, 'scheduled', NOW(), NOW()),
			(gen_random_uuid(), $1, 2, DATE '2026-06-12', '18:00'::time, '20:00'::time, 'scheduled', NOW(), NOW())
	`, classKey)

	lockedDates, err := models.GetMentorLockedDates(mentor.ID)
	if err != nil {
		t.Fatalf("failed to get lock date: %v", err)
	}
	if len(lockedDates) != 2 || lockedDates[1] != "2026-06-12" {
		t.Fatalf("expected locked dates ending at 2026-06-12, got %v", lockedDates)
	}

	if _, err := models.ReplaceMentorAvailabilityWindows(mentor.ID, time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC), []models.MentorAvailabilityWindow{{
		MentorUserID:  mentor.ID,
		AvailableDate: time.Date(2026, time.June, 11, 0, 0, 0, 0, time.UTC),
		StartTime:     "18:00",
		EndTime:       "20:00",
	}}); err == nil {
		t.Fatalf("expected locked date update to fail")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/mentor/availability?month=2026-06", nil)
	req = withUserContext(req, mentor.ID, mentor.Email, "mentor")
	res := httptest.NewRecorder()

	h.MentorAvailability(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected mentor availability fetch to succeed, got %d body=%s", res.Code, res.Body.String())
	}
	var payload struct {
		BlockedUntil string `json:"blocked_until"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode mentor availability payload: %v", err)
	}
	if payload.BlockedUntil != "2026-06-12" {
		t.Fatalf("expected blocked_until=2026-06-12, got %q", payload.BlockedUntil)
	}
}
