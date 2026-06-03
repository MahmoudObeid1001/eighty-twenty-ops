package tests

import (
	"bytes"
	"database/sql"
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
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	h := handlers.NewAPIHandler(cfg)
	nowSuffix := time.Now().UnixNano()

	admin := mustCreateUser(t, "admin", fmt.Sprintf("neg_admin_%d@eightytwenty.test", nowSuffix))
	manager := mustCreateUser(t, "manager", fmt.Sprintf("neg_manager_%d@eightytwenty.test", nowSuffix))
	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("neg_mh_%d@eightytwenty.test", nowSuffix))
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, manager.ID)
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

	t.Run("manager can late join beyond session 2 while admin stays blocked", func(t *testing.T) {
		leadID := createLeadWithStatus(t, "Late Join Override", uniquePhone(nowSuffix, 40), "ready_to_start", admin.ID)
		t.Cleanup(func() { cleanupLead(t, leadID) })

		classKey := createClassGroup(t, nowSuffix, 6, "Mon/Thu", "18:00:00", 95001, true, "active")
		t.Cleanup(func() { cleanupClass(t, classKey) })

		mustExec(t, `
			INSERT INTO placement_tests (lead_id, assigned_level, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (lead_id) DO UPDATE
			SET assigned_level = EXCLUDED.assigned_level, updated_at = NOW()
		`, leadID, 6)
		mustExec(t, `
			INSERT INTO scheduling (lead_id, class_days, class_time, class_group_index, updated_at)
			VALUES ($1, $2, $3::time, $4, NOW())
			ON CONFLICT (lead_id) DO UPDATE
			SET class_days = EXCLUDED.class_days,
			    class_time = EXCLUDED.class_time,
			    class_group_index = EXCLUDED.class_group_index,
			    updated_at = NOW()
		`, leadID, "Mon/Thu", "18:00:00", 95001+int32(nowSuffix%1000))

		for i := 1; i <= 3; i++ {
			mustExec(t, `
				INSERT INTO class_sessions (id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time, actual_date, actual_time, actual_end_time, status, completed_at, created_at, updated_at)
				VALUES (
					gen_random_uuid(), $1, $2, DATE '2026-06-01' + ($2 - 1), '18:00'::time, '20:00'::time,
					DATE '2026-06-01' + ($2 - 1), '18:00'::time, '20:00'::time, 'completed', NOW(), NOW(), NOW()
				)
			`, classKey, i)
		}

		adminEligibleReq := httptest.NewRequest(http.MethodGet, "/api/pre-enrolment/"+leadID.String()+"/late-join-eligible-classes", nil)
		adminEligibleReq = withUserContext(adminEligibleReq, admin.ID, admin.Email, "admin")
		adminEligibleRes := httptest.NewRecorder()
		h.GetEligibleClassesForLateJoin(adminEligibleRes, adminEligibleReq)
		if adminEligibleRes.Code != http.StatusOK {
			t.Fatalf("expected admin eligible classes call to succeed, got %d body=%s", adminEligibleRes.Code, adminEligibleRes.Body.String())
		}
		var adminEligiblePayload struct {
			Classes []models.EligibleClass `json:"classes"`
		}
		if err := json.Unmarshal(adminEligibleRes.Body.Bytes(), &adminEligiblePayload); err != nil {
			t.Fatalf("failed to decode admin eligible classes: %v", err)
		}
		for _, cls := range adminEligiblePayload.Classes {
			if cls.ClassKey == classKey {
				t.Fatalf("expected admin not to see late-join class %s beyond session 2, got %+v", classKey, adminEligiblePayload.Classes)
			}
		}

		managerEligibleReq := httptest.NewRequest(http.MethodGet, "/api/pre-enrolment/"+leadID.String()+"/late-join-eligible-classes", nil)
		managerEligibleReq = withUserContext(managerEligibleReq, manager.ID, manager.Email, "manager")
		managerEligibleRes := httptest.NewRecorder()
		h.GetEligibleClassesForLateJoin(managerEligibleRes, managerEligibleReq)
		if managerEligibleRes.Code != http.StatusOK {
			t.Fatalf("expected manager eligible classes call to succeed, got %d body=%s", managerEligibleRes.Code, managerEligibleRes.Body.String())
		}
		var managerEligiblePayload struct {
			Classes []models.EligibleClass `json:"classes"`
		}
		if err := json.Unmarshal(managerEligibleRes.Body.Bytes(), &managerEligiblePayload); err != nil {
			t.Fatalf("failed to decode manager eligible classes: %v", err)
		}
		foundManagerClass := false
		for _, cls := range managerEligiblePayload.Classes {
			if cls.ClassKey == classKey {
				foundManagerClass = true
				break
			}
		}
		if !foundManagerClass {
			t.Fatalf("expected manager to see class %s, got %+v", classKey, managerEligiblePayload.Classes)
		}

		payload := map[string]string{
			"class_key": classKey,
			"reason":    "Manager approved late join after current session two.",
		}
		body, _ := json.Marshal(payload)
		adminAddReq := httptest.NewRequest(http.MethodPost, "/api/pre-enrolment/"+leadID.String()+"/late-join?id="+leadID.String(), bytes.NewReader(body))
		adminAddReq = withUserContext(adminAddReq, admin.ID, admin.Email, "admin")
		adminAddRes := httptest.NewRecorder()
		h.AddLateJoiner(adminAddRes, adminAddReq)
		requireErrorResponse(t, adminAddRes, http.StatusBadRequest, "cannot join class: too late")

		managerAddReq := httptest.NewRequest(http.MethodPost, "/api/pre-enrolment/"+leadID.String()+"/late-join?id="+leadID.String(), bytes.NewReader(body))
		managerAddReq = withUserContext(managerAddReq, manager.ID, manager.Email, "manager")
		managerAddRes := httptest.NewRecorder()
		h.AddLateJoiner(managerAddRes, managerAddReq)
		if managerAddRes.Code != http.StatusOK {
			t.Fatalf("expected manager late join to succeed, got %d body=%s", managerAddRes.Code, managerAddRes.Body.String())
		}

		var joinedAt int32
		if err := mustQueryRow(t, `
			SELECT joined_at_session_number
			FROM class_memberships
			WHERE lead_id = $1 AND class_key = $2 AND join_reason = 'late_join'
		`, leadID, classKey).Scan(&joinedAt); err != nil {
			t.Fatalf("failed to load late join membership: %v", err)
		}
		if joinedAt != 4 {
			t.Fatalf("expected manager late join at session 4, got %d", joinedAt)
		}
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

	t.Run("send to class skips a closed group even when class time formats differ", func(t *testing.T) {
		leadID := createLeadWithStatus(t, "Waiting Returner", uniquePhone(nowSuffix, 3), "waiting_for_round", admin.ID)
		t.Cleanup(func() { cleanupLead(t, leadID) })

		level := int32(7)
		classDays := "Sat/Tues"
		classTimeText := "07:30"
		classTimeScheduling := "07:30:00"
		closedClassKey := models.GenerateClassKey(level, classDays, classTimeText, 1)
		cleanupClass(t, closedClassKey)

		mustExec(t, `
			INSERT INTO placement_tests (lead_id, assigned_level, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (lead_id) DO UPDATE
			SET assigned_level = EXCLUDED.assigned_level, updated_at = NOW()
		`, leadID, level)
		mustExec(t, `
			INSERT INTO scheduling (lead_id, class_days, class_time, class_group_index, updated_at)
			VALUES ($1, $2, $3::time, NULL, NOW())
			ON CONFLICT (lead_id) DO UPDATE
			SET class_days = EXCLUDED.class_days,
			    class_time = EXCLUDED.class_time,
			    class_group_index = EXCLUDED.class_group_index,
			    updated_at = NOW()
		`, leadID, classDays, classTimeScheduling)
		mustExec(t, `
			INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, round_status, updated_at)
			VALUES ($1, $2, $3, $4, 1, false, 'closed', NOW())
		`, closedClassKey, level, classDays, classTimeText)
		t.Cleanup(func() { cleanupClass(t, closedClassKey) })

		if err := models.SendLeadToClasses(leadID); err != nil {
			t.Fatalf("SendLeadToClasses failed: %v", err)
		}

		var sentToClasses bool
		var assignedGroup sql.NullInt32
		if err := mustQueryRow(t, `
				SELECT l.sent_to_classes, s.class_group_index
				FROM leads l
				JOIN scheduling s ON s.lead_id = l.id
				WHERE l.id = $1
			`, leadID).Scan(&sentToClasses, &assignedGroup); err != nil {
			t.Fatalf("failed to load assigned group: %v", err)
		}
		if !sentToClasses {
			t.Fatalf("expected lead to stay sent_to_classes")
		}
		if !assignedGroup.Valid {
			t.Fatalf("expected class_group_index to be assigned")
		}
		if assignedGroup.Int32 != 2 {
			t.Fatalf("expected closed group 1 to be skipped and group 2 assigned, got %d", assignedGroup.Int32)
		}
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
	mustExec(t, `DELETE FROM class_memberships WHERE class_key = $1`, classKey)
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
	mustExec(t, `DELETE FROM class_memberships WHERE lead_id = $1`, leadID)
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
