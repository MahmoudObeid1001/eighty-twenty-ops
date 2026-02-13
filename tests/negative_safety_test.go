package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/handlers"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

func TestNegativeSafetyRails(t *testing.T) {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	h := handlers.NewAPIHandler(cfg)
	nowSuffix := time.Now().UnixNano()

	admin := mustCreateUser(t, "admin", fmt.Sprintf("neg_admin_%d@eightytwenty.test", nowSuffix))
	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("neg_mh_%d@eightytwenty.test", nowSuffix))
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, mentorHead.ID)
	})

	t.Run("reject late join eligibility for non-ready student", func(t *testing.T) {
		leadID := createLeadWithStatus(t, "Negative Renewal", uniquePhone(nowSuffix, 1), "renewal_pending", admin.ID)
		t.Cleanup(func() { cleanupLead(t, leadID) })

		req := httptest.NewRequest(http.MethodGet, "/api/pre-enrolment/"+leadID.String()+"/late-join-eligible-classes", nil)
		req = withUserContext(req, admin.ID, admin.Email, "admin")
		res := httptest.NewRecorder()

		h.GetEligibleClassesForLateJoin(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "late join is only available for ready-to-start students")
	})

	t.Run("reject late join request with short reason", func(t *testing.T) {
		leadID := uuid.New() // valid UUID shape; request should fail before DB-dependent checks.
		payload := map[string]string{
			"class_key": "L2-Sat/Tues-10:00:00-C1",
			"reason":    "too short",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/pre-enrolment/"+leadID.String()+"/late-join?id="+leadID.String(), bytes.NewReader(body))
		req = withUserContext(req, admin.ID, admin.Email, "admin")
		res := httptest.NewRecorder()

		h.AddLateJoiner(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "Reason must be at least 10 characters long")
	})

	t.Run("reject start round when mentor is not assigned", func(t *testing.T) {
		classKey := createClassGroup(t, nowSuffix, 2, "Sat/Tues", "10:00:00", 91001, true, "not_started")
		t.Cleanup(func() { cleanupClass(t, classKey) })

		payload := map[string]string{"class_key": classKey}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/mentor-head/start-round", bytes.NewReader(body))
		req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
		res := httptest.NewRecorder()

		h.StartRound(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "Assign a mentor before starting the round")
	})

	t.Run("reject close round when a student is missing final grade", func(t *testing.T) {
		classNumber := int32(92001)
		classKey := createClassGroup(t, nowSuffix, 3, "Sat/Tues", "07:30:00", classNumber, true, "active")
		t.Cleanup(func() { cleanupClass(t, classKey) })

		leadID := createLeadWithStatus(t, "No Final Grade", uniquePhone(nowSuffix, 2), "in_classes", admin.ID)
		t.Cleanup(func() { cleanupLead(t, leadID) })

		mustExec(t, `
			INSERT INTO placement_tests (lead_id, assigned_level, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (lead_id) DO UPDATE
			SET assigned_level = EXCLUDED.assigned_level, updated_at = NOW()
		`, leadID, 3)

		mustExec(t, `
			INSERT INTO scheduling (lead_id, class_days, class_time, class_group_index, updated_at)
			VALUES ($1, $2, $3::time, $4, NOW())
			ON CONFLICT (lead_id) DO UPDATE
			SET class_days = EXCLUDED.class_days,
			    class_time = EXCLUDED.class_time,
			    class_group_index = EXCLUDED.class_group_index,
			    updated_at = NOW()
		`, leadID, "Sat/Tues", "07:30:00", classNumber)

		payload := map[string]string{"class_key": classKey}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/mentor-head/close-round", bytes.NewReader(body))
		req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
		res := httptest.NewRecorder()

		h.CloseRound(res, req)

		requireErrorResponse(t, res, http.StatusBadRequest, "missing final grade")
	})
}

func createLeadWithStatus(t *testing.T, fullName, phone, status string, createdBy uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustExec(t, `
		INSERT INTO leads (id, full_name, phone, status, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
	`, id, fullName, phone, status, createdBy)
	return id
}

func createClassGroup(t *testing.T, nowSuffix int64, level int32, classDays, classTime string, classNumber int32, sentToMentor bool, roundStatus string) string {
	t.Helper()
	uniqueClassNumber := classNumber + int32(nowSuffix%1000)
	classKey := models.GenerateClassKey(level, classDays, classTime, uniqueClassNumber)
	mustExec(t, `
		INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, round_status, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`, classKey, level, classDays, classTime, uniqueClassNumber, sentToMentor, roundStatus)
	return classKey
}

func cleanupClass(t *testing.T, classKey string) {
	t.Helper()
	mustExec(t, `DELETE FROM late_joiner_notifications WHERE class_key = $1`, classKey)
	mustExec(t, `DELETE FROM late_joiners WHERE class_key = $1`, classKey)
	mustExec(t, `DELETE FROM attendance WHERE session_id IN (SELECT id FROM class_sessions WHERE class_key = $1)`, classKey)
	mustExec(t, `DELETE FROM class_sessions WHERE class_key = $1`, classKey)
	mustExec(t, `DELETE FROM grades WHERE class_key = $1`, classKey)
	mustExec(t, `DELETE FROM class_enrollments WHERE class_key = $1`, classKey)
	mustExec(t, `DELETE FROM mentor_assignments WHERE class_key = $1`, classKey)
	mustExec(t, `DELETE FROM class_groups WHERE class_key = $1`, classKey)
}

func cleanupLead(t *testing.T, leadID uuid.UUID) {
	t.Helper()
	mustExec(t, `DELETE FROM late_joiner_notifications WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM late_joiners WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM attendance WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM grades WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM class_enrollments WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM transactions WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM payment_cycles WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM lead_payments WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM shipping WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM bookings WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM offers WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM payments WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM scheduling WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM placement_tests WHERE lead_id = $1`, leadID)
	mustExec(t, `DELETE FROM leads WHERE id = $1`, leadID)
}

func requireErrorResponse(t *testing.T, res *httptest.ResponseRecorder, expectedStatus int, contains string) {
	t.Helper()
	if res.Code < 400 {
		t.Fatalf("expected API error status, got %d body=%s", res.Code, res.Body.String())
	}
	if expectedStatus > 0 && res.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d body=%s", expectedStatus, res.Code, res.Body.String())
	}
	if contains != "" && !strings.Contains(res.Body.String(), contains) {
		t.Fatalf("expected error body to contain %q, got: %s", contains, res.Body.String())
	}
}

func uniquePhone(seed int64, offset int) string {
	return fmt.Sprintf("09%09d", (seed+int64(offset))%1000000000)
}
