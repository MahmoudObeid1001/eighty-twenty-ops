package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/handlers"
	"eighty-twenty-ops/internal/middleware"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

type testLead struct {
	ID                 uuid.UUID
	Name               string
	Phone              string
	LevelsPurchased    int
	LevelsConsumed     int
	ExpectedLevel      int
	ExpectedCredits    int
	ExpectedStatus     string
	ExpectedOutcome    string
	ExpectedFinalGrade string
}

func TestAfterClassPipeline(t *testing.T) {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	h := handlers.NewAPIHandler(cfg)

	nowSuffix := time.Now().UnixNano()
	classDays := "Sat/Tues"
	classTime := "10:00:00"
	level := int32(1)
	classNumber := int32(nowSuffix%100000) + 1
	classKey := models.GenerateClassKey(level, classDays, classTime, classNumber)

	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("mh_%d@eightytwenty.test", nowSuffix))
	mentor := mustCreateUser(t, "mentor", fmt.Sprintf("mentor_%d@eightytwenty.test", nowSuffix))

	cleanup := []func(){
		func() { mustExec(t, `DELETE FROM class_groups WHERE class_key = $1`, classKey) },
		func() { mustExec(t, `DELETE FROM users WHERE id = $1`, mentor.ID) },
		func() { mustExec(t, `DELETE FROM users WHERE id = $1`, mentorHead.ID) },
	}
	defer func() {
		for _, fn := range cleanup {
			fn()
		}
	}()

	mustExec(t, `
		INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, sent_at, updated_at, round_status, round_started_at, round_started_by)
		VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW(), 'active', NOW(), $6)
	`, classKey, level, classDays, classTime, classNumber, mentorHead.ID)

	mustExec(t, `
		INSERT INTO mentor_assignments (mentor_user_id, class_key, created_by_user_id)
		VALUES ($1, $2, $3)
	`, mentor.ID, classKey, mentorHead.ID)

	leads := []testLead{
		{
			Name:               "The Star",
			Phone:              fmt.Sprintf("0100%07d", nowSuffix%10000000),
			LevelsPurchased:    1,
			LevelsConsumed:     0,
			ExpectedLevel:      2,
			ExpectedCredits:    0,
			ExpectedStatus:     "waiting_for_round",
			ExpectedOutcome:    "promoted",
			ExpectedFinalGrade: "A",
		},
		{
			Name:               "The Payer",
			Phone:              fmt.Sprintf("0101%07d", nowSuffix%10000000),
			LevelsPurchased:    0,
			LevelsConsumed:     0,
			ExpectedLevel:      2,
			ExpectedCredits:    0,
			ExpectedStatus:     "renewal_pending",
			ExpectedOutcome:    "promoted",
			ExpectedFinalGrade: "B",
		},
		{
			Name:               "The Repeater",
			Phone:              fmt.Sprintf("0102%07d", nowSuffix%10000000),
			LevelsPurchased:    5,
			LevelsConsumed:     0,
			ExpectedLevel:      1,
			ExpectedCredits:    5,
			ExpectedStatus:     "waiting_for_round",
			ExpectedOutcome:    "repeated",
			ExpectedFinalGrade: "F",
		},
	}

	for i := range leads {
		leads[i].ID = mustCreateLead(t, leads[i], mentorHead.ID)
		mustCreatePlacementTest(t, leads[i].ID, 1)
		mustCreateScheduling(t, leads[i].ID, classDays, classTime, classNumber)
	}
	defer func() {
		for _, lead := range leads {
			mustExec(t, `DELETE FROM leads WHERE id = $1`, lead.ID)
		}
	}()

	preCount := mustCount(t, `SELECT COUNT(*) FROM class_enrollments WHERE class_key = $1`, classKey)
	if preCount != 0 {
		t.Fatalf("expected no class_enrollments before close, got %d", preCount)
	}

	for _, lead := range leads {
		payload := map[string]string{
			"lead_id":   lead.ID.String(),
			"class_key": classKey,
			"grade":     lead.ExpectedFinalGrade,
			"notes":     "test grade",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/mentor/grades", bytes.NewReader(body))
		req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
		res := httptest.NewRecorder()
		h.CreateGrade(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("CreateGrade failed for %s: status=%d body=%s", lead.Name, res.Code, res.Body.String())
		}
	}

	closeBody, _ := json.Marshal(map[string]string{"class_key": classKey})
	closeReq := httptest.NewRequest(http.MethodPost, "/api/mentor-head/close-round", bytes.NewReader(closeBody))
	closeReq = withUserContext(closeReq, mentorHead.ID, mentorHead.Email, "mentor_head")
	closeRes := httptest.NewRecorder()
	h.CloseRound(closeRes, closeReq)
	if closeRes.Code != http.StatusOK {
		t.Fatalf("CloseRound failed: status=%d body=%s", closeRes.Code, closeRes.Body.String())
	}

	enrollmentCount := mustCount(t, `SELECT COUNT(*) FROM class_enrollments WHERE class_key = $1`, classKey)
	if enrollmentCount != len(leads) {
		t.Fatalf("expected %d class_enrollments, got %d", len(leads), enrollmentCount)
	}

	for _, lead := range leads {
		var status string
		var remaining int
		var isReturning bool
		var assignedLevel int

		if err := mustQueryRow(t, `
				SELECT l.status, COALESCE(l.remaining_credits, 0), l.is_returning, pt.assigned_level
				FROM leads l
				JOIN placement_tests pt ON pt.lead_id = l.id
				WHERE l.id = $1
			`, lead.ID).Scan(&status, &remaining, &isReturning, &assignedLevel); err != nil {
			t.Fatalf("%s: failed to load lead status snapshot: %v", lead.Name, err)
		}

		if status != lead.ExpectedStatus {
			t.Fatalf("%s: expected status %s, got %s", lead.Name, lead.ExpectedStatus, status)
		}
		if remaining != lead.ExpectedCredits {
			t.Fatalf("%s: expected remaining_credits %d, got %d", lead.Name, lead.ExpectedCredits, remaining)
		}
		if assignedLevel != lead.ExpectedLevel {
			t.Fatalf("%s: expected assigned_level %d, got %d", lead.Name, lead.ExpectedLevel, assignedLevel)
		}
		if !isReturning {
			t.Fatalf("%s: expected is_returning true", lead.Name)
		}

		var outcome, finalGrade sql.NullString
		if err := mustQueryRow(t, `
				SELECT outcome, final_grade FROM class_enrollments
				WHERE lead_id = $1 AND class_key = $2
			`, lead.ID, classKey).Scan(&outcome, &finalGrade); err != nil {
			t.Fatalf("%s: failed to load class enrollment outcome: %v", lead.Name, err)
		}

		if !outcome.Valid || outcome.String != lead.ExpectedOutcome {
			t.Fatalf("%s: expected outcome %s, got %s", lead.Name, lead.ExpectedOutcome, outcome.String)
		}
		if !finalGrade.Valid || finalGrade.String != lead.ExpectedFinalGrade {
			t.Fatalf("%s: expected final_grade %s, got %s", lead.Name, lead.ExpectedFinalGrade, finalGrade.String)
		}

		var classDaysVal, classTimeVal sql.NullString
		var classGroupIndex sql.NullInt32
		if err := mustQueryRow(t, `
				SELECT class_days, class_time::text, class_group_index
				FROM scheduling WHERE lead_id = $1
			`, lead.ID).Scan(&classDaysVal, &classTimeVal, &classGroupIndex); err != nil {
			t.Fatalf("%s: failed to load scheduling row: %v", lead.Name, err)
		}

		if !classDaysVal.Valid || classDaysVal.String != classDays {
			t.Fatalf("%s: expected class_days preference %q to be preserved, got %v", lead.Name, classDays, classDaysVal)
		}
		if !classTimeVal.Valid || classTimeVal.String != classTime {
			t.Fatalf("%s: expected class_time preference %q to be preserved, got %v", lead.Name, classTime, classTimeVal)
		}
		if classGroupIndex.Valid {
			t.Fatalf("%s: expected class_group_index to be cleared, got %v", lead.Name, classGroupIndex)
		}
	}
}

func withUserContext(req *http.Request, userID uuid.UUID, email, role string) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, userID.String())
	ctx = context.WithValue(ctx, middleware.UserEmailKey, email)
	ctx = context.WithValue(ctx, middleware.UserRoleKey, role)
	return req.WithContext(ctx)
}

type testUser struct {
	ID    uuid.UUID
	Email string
}

func mustCreateUser(t *testing.T, role, email string) testUser {
	t.Helper()
	id := uuid.New()
	fullName := fmt.Sprintf("%s Test User", role)
	phoneSeed := uint32(id[0])<<24 | uint32(id[1])<<16 | uint32(id[2])<<8 | uint32(id[3])
	phone := fmt.Sprintf("01%09d", int(phoneSeed)%1000000000)
	mustExec(t, `
		INSERT INTO users (id, email, password_hash, role, full_name, phone, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`, id, email, "hash", role, fullName, phone)
	return testUser{ID: id, Email: email}
}

func mustCreateLead(t *testing.T, lead testLead, createdBy uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustExec(t, `
		INSERT INTO leads (
			id, full_name, phone, status, created_by_user_id,
			levels_purchased_total, levels_consumed, remaining_credits, is_returning,
			high_priority_follow_up, created_at, updated_at
		) VALUES ($1, $2, $3, 'in_classes', $4, $5, $6, 0, false, false, NOW(), NOW())
	`, id, lead.Name, lead.Phone, createdBy, lead.LevelsPurchased, lead.LevelsConsumed)
	return id
}

func mustCreatePlacementTest(t *testing.T, leadID uuid.UUID, level int) {
	t.Helper()
	mustExec(t, `
		INSERT INTO placement_tests (lead_id, assigned_level, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (lead_id) DO UPDATE SET assigned_level = EXCLUDED.assigned_level, updated_at = NOW()
	`, leadID, level)
}

func mustCreateScheduling(t *testing.T, leadID uuid.UUID, days, timeStr string, groupIndex int32) {
	t.Helper()
	mustExec(t, `
		INSERT INTO scheduling (lead_id, class_days, class_time, class_group_index, updated_at)
		VALUES ($1, $2, $3::time, $4, NOW())
		ON CONFLICT (lead_id) DO UPDATE SET class_days = EXCLUDED.class_days, class_time = EXCLUDED.class_time, class_group_index = EXCLUDED.class_group_index, updated_at = NOW()
	`, leadID, days, timeStr, groupIndex)
}

func mustExec(t *testing.T, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.DB.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
}

func mustCount(t *testing.T, query string, args ...interface{}) int {
	t.Helper()
	var count int
	if err := db.DB.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	return count
}

func mustQueryRow(t *testing.T, query string, args ...interface{}) *sql.Row {
	t.Helper()
	return db.DB.QueryRow(query, args...)
}

func TestMain(m *testing.M) {
	// Ensure a DB URL is available for local runs.
	if os.Getenv("DATABASE_URL") == "" {
		_ = os.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable")
	}
	os.Exit(m.Run())
}
