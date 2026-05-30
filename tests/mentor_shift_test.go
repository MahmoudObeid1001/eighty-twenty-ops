package tests

import (
	"bytes"
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

func TestShiftClassMentorCreatesSessionWindows(t *testing.T) {
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
	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("shift_mh_%d@eightytwenty.test", nowSuffix))
	oldMentor := mustCreateUser(t, "mentor", fmt.Sprintf("shift_old_%d@eightytwenty.test", nowSuffix))
	newMentor := mustCreateUser(t, "mentor", fmt.Sprintf("shift_new_%d@eightytwenty.test", nowSuffix))
	classKey := createClassGroup(t, nowSuffix, 3, "Mon/Thu", "18:00:00", 94001, true, "active")
	t.Cleanup(func() {
		cleanupClass(t, classKey)
		mustExec(t, `DELETE FROM users WHERE id IN ($1, $2, $3)`, mentorHead.ID, oldMentor.ID, newMentor.ID)
	})

	mustExec(t, `
		INSERT INTO mentor_assignments (mentor_user_id, class_key, created_by_user_id)
		VALUES ($1, $2, $3)
	`, oldMentor.ID, classKey, mentorHead.ID)
	mustExec(t, `
		INSERT INTO class_mentor_assignment_windows (class_key, mentor_user_id, effective_from_session, assigned_by_user_id, reason)
		VALUES ($1, $2, 1, $3, 'test seed')
	`, classKey, oldMentor.ID, mentorHead.ID)
	mustCreateShiftSessions(t, classKey, "18:00:00", 3)

	effective := int32(4)
	result, err := models.ShiftClassMentor(classKey, newMentor.ID, &effective, "mentor unavailable", mentorHead.ID)
	if err != nil {
		t.Fatalf("ShiftClassMentor returned error: %v", err)
	}
	if result.PreviousMentorUserID != oldMentor.ID || result.NewMentorUserID != newMentor.ID || result.EffectiveSessionNumber != 4 {
		t.Fatalf("unexpected shift result: %+v", result)
	}

	var currentMentor string
	if err := mustQueryRow(t, `SELECT mentor_user_id::text FROM mentor_assignments WHERE class_key = $1`, classKey).Scan(&currentMentor); err != nil {
		t.Fatalf("failed to load current mentor: %v", err)
	}
	if currentMentor != newMentor.ID.String() {
		t.Fatalf("expected current mentor %s, got %s", newMentor.ID, currentMentor)
	}

	windows, err := models.GetMentorAssignmentWindowsForClass(classKey)
	if err != nil {
		t.Fatalf("failed to load windows: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %+v", windows)
	}
	if windows[0].MentorUserID != oldMentor.ID || windows[0].EffectiveFromSession != 1 || !windows[0].EffectiveToSession.Valid || windows[0].EffectiveToSession.Int32 != 3 {
		t.Fatalf("unexpected old mentor window: %+v", windows[0])
	}
	if windows[1].MentorUserID != newMentor.ID || windows[1].EffectiveFromSession != 4 || windows[1].EffectiveToSession.Valid {
		t.Fatalf("unexpected new mentor window: %+v", windows[1])
	}

	result, err = models.ShiftClassMentor(classKey, oldMentor.ID, &effective, "shifted back before session 4", mentorHead.ID)
	if err != nil {
		t.Fatalf("ShiftClassMentor back to original mentor returned error: %v", err)
	}
	if result.PreviousMentorUserID != newMentor.ID || result.NewMentorUserID != oldMentor.ID || result.EffectiveSessionNumber != 4 {
		t.Fatalf("unexpected shift-back result: %+v", result)
	}
	windows, err = models.GetMentorAssignmentWindowsForClass(classKey)
	if err != nil {
		t.Fatalf("failed to reload windows after shift back: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected shift-back before session 4 to merge back to 1 open window, got %+v", windows)
	}
	if windows[0].MentorUserID != oldMentor.ID || windows[0].EffectiveFromSession != 1 || windows[0].EffectiveToSession.Valid {
		t.Fatalf("unexpected merged mentor window: %+v", windows[0])
	}
}

func TestShiftMentorAPIAllowsManagerAndRejectsClosedClass(t *testing.T) {
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
	manager := mustCreateUser(t, "manager", fmt.Sprintf("shift_mgr_%d@eightytwenty.test", nowSuffix))
	oldMentor := mustCreateUser(t, "mentor", fmt.Sprintf("shift_api_old_%d@eightytwenty.test", nowSuffix))
	newMentor := mustCreateUser(t, "mentor", fmt.Sprintf("shift_api_new_%d@eightytwenty.test", nowSuffix))
	classKey := createClassGroup(t, nowSuffix, 4, "Sun/Wed", "19:00:00", 95001, true, "active")
	t.Cleanup(func() {
		cleanupClass(t, classKey)
		mustExec(t, `DELETE FROM users WHERE id IN ($1, $2, $3)`, manager.ID, oldMentor.ID, newMentor.ID)
	})

	mustExec(t, `
		INSERT INTO mentor_assignments (mentor_user_id, class_key, created_by_user_id)
		VALUES ($1, $2, $3)
	`, oldMentor.ID, classKey, manager.ID)
	mustExec(t, `
		INSERT INTO class_mentor_assignment_windows (class_key, mentor_user_id, effective_from_session, assigned_by_user_id, reason)
		VALUES ($1, $2, 1, $3, 'test seed')
	`, classKey, oldMentor.ID, manager.ID)
	mustCreateShiftSessions(t, classKey, "19:00:00", 1)

	body, _ := json.Marshal(map[string]interface{}{
		"class_key":                classKey,
		"mentor_user_id":           newMentor.ID.String(),
		"effective_session_number": 2,
		"reason":                   "coverage change",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/mentor-head/shift-mentor", bytes.NewReader(body))
	req = withUserContext(req, manager.ID, manager.Email, "manager")
	res := httptest.NewRecorder()
	h.ShiftMentor(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected manager shift to succeed, got %d body=%s", res.Code, res.Body.String())
	}

	mustExec(t, `UPDATE class_groups SET round_status = 'closed' WHERE class_key = $1`, classKey)
	body, _ = json.Marshal(map[string]interface{}{
		"class_key":      classKey,
		"mentor_user_id": oldMentor.ID.String(),
		"reason":         "closed class attempt",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/mentor-head/shift-mentor", bytes.NewReader(body))
	req = withUserContext(req, manager.ID, manager.Email, "manager")
	res = httptest.NewRecorder()
	h.ShiftMentor(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected closed class shift to fail with 400, got %d body=%s", res.Code, res.Body.String())
	}
}

func mustCreateShiftSessions(t *testing.T, classKey, timeStr string, completedThrough int) {
	t.Helper()
	for i := 1; i <= 8; i++ {
		status := "scheduled"
		var completedAt interface{}
		if i <= completedThrough {
			status = "completed"
			completedAt = time.Now()
		}
		mustExec(t, `
			INSERT INTO class_sessions (
				id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time,
				status, completed_at, created_at, updated_at
			)
			VALUES (
				gen_random_uuid(), $1, $2, CURRENT_DATE + ($2 - 1), $3::time, ($3::time + INTERVAL '2 hour')::time,
				$4, $5, NOW(), NOW()
			)
		`, classKey, i, timeStr, status, completedAt)
	}
}
