package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	// ExpectedNextLevelConsumedOnClose mirrors class_enrollments.next_level_consumed_on_close.
	// True means the close-round pipeline reserved the next prepaid level on behalf of the student
	// (distinguishing remaining_credits=0-because-reserved from 0-because-renewal-needed).
	ExpectedNextLevelConsumedOnClose bool
	ExpectedLevelsConsumed           int
}

func TestAfterClassPipeline(t *testing.T) {
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
	classDays := "Sat/Tues"
	classTime := "10:00:00"
	level := int32(1)
	classNumber := int32(nowSuffix%100000) + 1
	classKey := models.GenerateClassKey(level, classDays, classTime, classNumber)

	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("mh_%d@eightytwenty.test", nowSuffix))
	mentor := mustCreateUser(t, "mentor", fmt.Sprintf("mentor_%d@eightytwenty.test", nowSuffix))

	cleanup := []func(){
		func() {
			mustExec(t, `DELETE FROM attendance WHERE session_id IN (SELECT id FROM class_sessions WHERE class_key = $1)`, classKey)
		},
		func() { mustExec(t, `DELETE FROM class_sessions WHERE class_key = $1`, classKey) },
		func() { mustExec(t, `DELETE FROM class_memberships WHERE class_key = $1`, classKey) },
		func() { mustExec(t, `DELETE FROM class_enrollments WHERE class_key = $1`, classKey) },
		func() { mustExec(t, `DELETE FROM grades WHERE class_key = $1`, classKey) },
		func() { mustExec(t, `DELETE FROM mentor_assignments WHERE class_key = $1`, classKey) },
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
	mustCreateCompletedSessions(t, classKey, classTime)

	leads := []testLead{
		{
			Name:               "The Star",
			Phone:              fmt.Sprintf("0100%07d", nowSuffix%10000000),
			LevelsPurchased:    1,
			LevelsConsumed:     0,
			ExpectedLevel:      2,
			ExpectedCredits:    0,
			ExpectedStatus:     "renewal_pending",
			ExpectedOutcome:    "promoted",
			ExpectedFinalGrade: "A",
			// purchased=1, consumed=0 → after consuming current level: remaining=0 → no next to reserve
			ExpectedNextLevelConsumedOnClose: false,
			ExpectedLevelsConsumed:           1,
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
			// purchased=0 → no credits at all → no next level to reserve
			ExpectedNextLevelConsumedOnClose: false,
			ExpectedLevelsConsumed:           1,
		},
		{
			Name:               "The Repeater",
			Phone:              fmt.Sprintf("0102%07d", nowSuffix%10000000),
			LevelsPurchased:    5,
			LevelsConsumed:     0,
			ExpectedLevel:      1,
			ExpectedCredits:    3,
			ExpectedStatus:     "waiting_for_round",
			ExpectedOutcome:    "repeated",
			ExpectedFinalGrade: "F",
			// purchased=5, consumed=0 → after consuming current: remaining=4 → next reserved
			ExpectedNextLevelConsumedOnClose: true,
			ExpectedLevelsConsumed:           2,
		},
		{
			// Bundle of 2: finishes Level 1, Level 2 is prepaid → reserved on close.
			// Expected: status = waiting_for_round, remaining_credits = 0 (Level 2 consumed/reserved).
			Name:               "The Bundle Waiter",
			Phone:              fmt.Sprintf("0103%07d", nowSuffix%10000000),
			LevelsPurchased:    2,
			LevelsConsumed:     0,
			ExpectedLevel:      2,
			ExpectedCredits:    0,
			ExpectedStatus:     "waiting_for_round",
			ExpectedOutcome:    "promoted",
			ExpectedFinalGrade: "B",
			// purchased=2, consumed=0 → after consuming current: remaining=1 → next reserved → then credits drop to 0
			ExpectedNextLevelConsumedOnClose: true,
			ExpectedLevelsConsumed:           2,
		},
	}

	for i := range leads {
		leads[i].ID = mustCreateLead(t, leads[i], mentorHead.ID)
		mustCreatePlacementTest(t, leads[i].ID, 1)
		mustCreateScheduling(t, leads[i].ID, classDays, classTime, classNumber)
	}
	defer func() {
		for _, lead := range leads {
			mustExec(t, `DELETE FROM class_memberships WHERE lead_id = $1`, lead.ID)
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
			"notes":     "This student completed the round well and showed strong improvement throughout sessions.",
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
		var levelsConsumed int

		if err := mustQueryRow(t, `
				SELECT l.status, COALESCE(l.remaining_credits, 0), l.is_returning, pt.assigned_level, l.levels_consumed
				FROM leads l
				JOIN placement_tests pt ON pt.lead_id = l.id
				WHERE l.id = $1
			`, lead.ID).Scan(&status, &remaining, &isReturning, &assignedLevel, &levelsConsumed); err != nil {
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
		if levelsConsumed != lead.ExpectedLevelsConsumed {
			t.Fatalf("%s: expected levels_consumed %d, got %d", lead.Name, lead.ExpectedLevelsConsumed, levelsConsumed)
		}
		if !isReturning {
			t.Fatalf("%s: expected is_returning true", lead.Name)
		}

		var outcome, finalGrade sql.NullString
		var nextLevelConsumedOnClose bool
		if err := mustQueryRow(t, `
				SELECT outcome, final_grade, COALESCE(next_level_consumed_on_close, false)
				FROM class_enrollments
				WHERE lead_id = $1 AND class_key = $2
			`, lead.ID, classKey).Scan(&outcome, &finalGrade, &nextLevelConsumedOnClose); err != nil {
			t.Fatalf("%s: failed to load class enrollment outcome: %v", lead.Name, err)
		}

		if !outcome.Valid || outcome.String != lead.ExpectedOutcome {
			t.Fatalf("%s: expected outcome %s, got %s", lead.Name, lead.ExpectedOutcome, outcome.String)
		}
		if !finalGrade.Valid || finalGrade.String != lead.ExpectedFinalGrade {
			t.Fatalf("%s: expected final_grade %s, got %s", lead.Name, lead.ExpectedFinalGrade, finalGrade.String)
		}
		if nextLevelConsumedOnClose != lead.ExpectedNextLevelConsumedOnClose {
			t.Fatalf("%s: expected next_level_consumed_on_close=%v, got %v",
				lead.Name, lead.ExpectedNextLevelConsumedOnClose, nextLevelConsumedOnClose)
		}

		// Invariant: a student whose bundle covered more than the current level
		// (i.e. paid credits remained after consuming the in-progress level)
		// must NEVER exit close-round in renewal_pending, and the enrollment
		// flag must confirm the reservation.
		// remaining = purchased - (consumed_before_close + 1_for_current_level)
		hadPrepaidNext := lead.LevelsPurchased-(lead.LevelsConsumed+1) > 0
		if hadPrepaidNext {
			if status == "renewal_pending" {
				t.Fatalf("%s: INVARIANT VIOLATED — student had prepaid next level but status is renewal_pending, expected waiting_for_round",
					lead.Name)
			}
			if !nextLevelConsumedOnClose {
				t.Fatalf("%s: INVARIANT VIOLATED — student had prepaid next level but next_level_consumed_on_close is false",
					lead.Name)
			}
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
	// remaining_credits must reflect any already-consumed levels so the lead's
	// snapshot matches what CompleteSession would have written mid-round.
	remainingCredits := lead.LevelsPurchased - lead.LevelsConsumed
	if remainingCredits < 0 {
		remainingCredits = 0
	}
	mustExec(t, `
		INSERT INTO leads (
			id, full_name, phone, status, created_by_user_id,
			levels_purchased_total, levels_consumed, remaining_credits, is_returning,
			high_priority_follow_up, created_at, updated_at
		) VALUES ($1, $2, $3, 'in_classes', $4, $5, $6, $7, false, false, NOW(), NOW())
	`, id, lead.Name, lead.Phone, createdBy, lead.LevelsPurchased, lead.LevelsConsumed, remainingCredits)
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

func mustCreateCompletedSessions(t *testing.T, classKey, timeStr string) {
	t.Helper()
	for i := 1; i <= 8; i++ {
		mustExec(t, `
			INSERT INTO class_sessions (
				id, class_key, session_number, scheduled_date, scheduled_time, scheduled_end_time,
				actual_date, actual_time, actual_end_time, status, completed_at, created_at, updated_at
			)
			VALUES (
				gen_random_uuid(), $1, $2, CURRENT_DATE + ($2 - 1), $3::time, ($3::time + INTERVAL '2 hour')::time,
				CURRENT_DATE + ($2 - 1), $3::time, ($3::time + INTERVAL '2 hour')::time, 'completed', NOW(), NOW(), NOW()
			)
			ON CONFLICT (class_key, session_number) DO UPDATE
			SET status = 'completed',
			    completed_at = NOW(),
			    updated_at = NOW()
		`, classKey, i, timeStr)
	}
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

// mustPreConsumeMembership inserts a class_membership that already has
// level_consumed_at_session_number set, exactly as CompleteSession would have
// written it when session `consumedAtSession` was marked completed mid-round.
// This must be called after the class_group exists and before CloseRound runs,
// so that ensureClassMembershipsTx's NOT EXISTS guard preserves it unchanged.
func mustPreConsumeMembership(t *testing.T, leadID uuid.UUID, classKey string, consumedAtSession int32) {
	t.Helper()
	mustExec(t, `
		INSERT INTO class_memberships (
			id, lead_id, class_key,
			joined_at_session_number, level_consumed_at_session_number,
			join_reason, created_at, updated_at
		) VALUES (gen_random_uuid(), $1, $2, 1, $3, 'round_start', NOW(), NOW())
	`, leadID, classKey, consumedAtSession)
}

// TestAfterClassPipelineMidRoundConsumption proves that close-round is correct
// when the current level was already consumed during sessions (the production path),
// not just during the close transaction itself.
//
// Specifically it guards against the "Dalia scenario": a student with a prepaid
// next level who was left in renewal_pending or offer_sent after close-round,
// forcing manual recovery.
func TestAfterClassPipelineMidRoundConsumption(t *testing.T) {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := handlers.NewAPIHandler(cfg)
	nowSuffix := time.Now().UnixNano()
	classDays := "Sun/Wed"
	classTime := "18:00:00"
	level := int32(1)
	classNumber := int32(nowSuffix%100000) + 1
	classKey := models.GenerateClassKey(level, classDays, classTime, classNumber)

	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("mh_mid_%d@eightytwenty.test", nowSuffix))
	mentor := mustCreateUser(t, "mentor", fmt.Sprintf("mentor_mid_%d@eightytwenty.test", nowSuffix))

	defer func() {
		mustExec(t, `DELETE FROM attendance WHERE session_id IN (SELECT id FROM class_sessions WHERE class_key = $1)`, classKey)
		mustExec(t, `DELETE FROM class_sessions WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM class_memberships WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM class_enrollments WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM grades WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM mentor_assignments WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM class_groups WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM users WHERE id = $1`, mentor.ID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, mentorHead.ID)
	}()

	mustExec(t, `
		INSERT INTO class_groups (
			class_key, level, class_days, class_time, class_number,
			sent_to_mentor, sent_at, updated_at, round_status, round_started_at, round_started_by
		) VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW(), 'active', NOW(), $6)
	`, classKey, level, classDays, classTime, classNumber, mentorHead.ID)
	mustExec(t, `INSERT INTO mentor_assignments (mentor_user_id, class_key, created_by_user_id) VALUES ($1, $2, $3)`,
		mentor.ID, classKey, mentorHead.ID)
	mustCreateCompletedSessions(t, classKey, classTime)

	// "Mid-round consumed": bundle=2, current level was consumed at session 2
	// (levels_consumed=1 already on the lead, membership flag already set).
	// close-round must NOT double-consume, must produce waiting_for_round, remaining_credits=0.
	midRound := testLead{
		Name:                             "Mid-Round Consumed",
		Phone:                            fmt.Sprintf("0200%07d", nowSuffix%10000000),
		LevelsPurchased:                  2,
		LevelsConsumed:                   1, // already consumed mid-round at session 2
		ExpectedLevel:                    2,
		ExpectedCredits:                  0, // Level 2 reserved on close
		ExpectedStatus:                   "waiting_for_round",
		ExpectedOutcome:                  "promoted",
		ExpectedFinalGrade:               "A",
		ExpectedNextLevelConsumedOnClose: true,
	}

	midRound.ID = mustCreateLead(t, midRound, mentorHead.ID)
	defer func() {
		mustExec(t, `DELETE FROM class_memberships WHERE lead_id = $1`, midRound.ID)
		mustExec(t, `DELETE FROM leads WHERE id = $1`, midRound.ID)
		mustExec(t, `DELETE FROM placement_tests WHERE lead_id = $1`, midRound.ID)
		mustExec(t, `DELETE FROM scheduling WHERE lead_id = $1`, midRound.ID)
	}()

	mustCreatePlacementTest(t, midRound.ID, 1)
	mustCreateScheduling(t, midRound.ID, classDays, classTime, classNumber)

	// Simulate what CompleteSession wrote at session 2:
	// membership exists with level_consumed_at_session_number=2.
	// ensureClassMembershipsTx will see the NOT EXISTS guard and skip insertion,
	// so ensureMembershipLevelConsumedTx will find LevelConsumedAtSession.Valid=true and be a no-op.
	mustPreConsumeMembership(t, midRound.ID, classKey, 2)

	// Post grade
	body, _ := json.Marshal(map[string]string{
		"lead_id":   midRound.ID.String(),
		"class_key": classKey,
		"grade":     midRound.ExpectedFinalGrade,
		"notes":     "Mid-round consumption regression test ensuring close-round does not double-consume the level.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/mentor/grades", bytes.NewReader(body))
	req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
	res := httptest.NewRecorder()
	h.CreateGrade(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("CreateGrade failed: status=%d body=%s", res.Code, res.Body.String())
	}

	// Close the round
	closeBody, _ := json.Marshal(map[string]string{"class_key": classKey})
	closeReq := httptest.NewRequest(http.MethodPost, "/api/mentor-head/close-round", bytes.NewReader(closeBody))
	closeReq = withUserContext(closeReq, mentorHead.ID, mentorHead.Email, "mentor_head")
	closeRes := httptest.NewRecorder()
	h.CloseRound(closeRes, closeReq)
	if closeRes.Code != http.StatusOK {
		t.Fatalf("CloseRound failed: status=%d body=%s", closeRes.Code, closeRes.Body.String())
	}

	// Assert outcome
	var status string
	var remaining int
	var assignedLevel int
	var levelsConsumed int
	if err := mustQueryRow(t, `
		SELECT l.status, COALESCE(l.remaining_credits, 0), pt.assigned_level, l.levels_consumed
		FROM leads l JOIN placement_tests pt ON pt.lead_id = l.id
		WHERE l.id = $1
	`, midRound.ID).Scan(&status, &remaining, &assignedLevel, &levelsConsumed); err != nil {
		t.Fatalf("failed to load lead: %v", err)
	}

	if status != midRound.ExpectedStatus {
		t.Fatalf("MidRound: expected status %s, got %s", midRound.ExpectedStatus, status)
	}
	if remaining != midRound.ExpectedCredits {
		t.Fatalf("MidRound: expected remaining_credits %d, got %d", midRound.ExpectedCredits, remaining)
	}
	if assignedLevel != midRound.ExpectedLevel {
		t.Fatalf("MidRound: expected assigned_level %d, got %d", midRound.ExpectedLevel, assignedLevel)
	}
	if levelsConsumed != 2 {
		t.Fatalf("MidRound: expected levels_consumed 2, got %d", levelsConsumed)
	}

	var outcome, finalGrade sql.NullString
	var nextLevelConsumedOnClose bool
	if err := mustQueryRow(t, `
		SELECT outcome, final_grade, COALESCE(next_level_consumed_on_close, false)
		FROM class_enrollments WHERE lead_id = $1 AND class_key = $2
	`, midRound.ID, classKey).Scan(&outcome, &finalGrade, &nextLevelConsumedOnClose); err != nil {
		t.Fatalf("MidRound: failed to load enrollment: %v", err)
	}
	if !outcome.Valid || outcome.String != midRound.ExpectedOutcome {
		t.Fatalf("MidRound: expected outcome %s, got %s", midRound.ExpectedOutcome, outcome.String)
	}
	if nextLevelConsumedOnClose != midRound.ExpectedNextLevelConsumedOnClose {
		t.Fatalf("MidRound: expected next_level_consumed_on_close=%v, got %v",
			midRound.ExpectedNextLevelConsumedOnClose, nextLevelConsumedOnClose)
	}

	// Dalia guard: a student with a prepaid next level must NEVER exit close-round
	// in a renewal/offer state. These are the statuses that put them on the ops
	// renewal feed, which is wrong — they already paid.
	daliaBadStatuses := []string{"renewal_pending", "offer_sent"}
	for _, bad := range daliaBadStatuses {
		if status == bad {
			t.Fatalf(
				"DALIA GUARD VIOLATED: student with prepaid next level landed in %q after close-round. "+
					"They should be in waiting_for_round. This is the exact failure mode that required manual recovery.",
				bad,
			)
		}
	}
}

func TestPrepaidContinuationBlocker(t *testing.T) {
	cfg := config.Load()
	if err := db.Connect(cfg.DatabaseURL); err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := handlers.NewAPIHandler(cfg)
	nowSuffix := time.Now().UnixNano()
	classDays := "Mon/Thu"
	classTime := "07:30:00"
	level := int32(1)
	classNumber := int32(nowSuffix%100000) + 1
	classKey := models.GenerateClassKey(level, classDays, classTime, classNumber)

	mentorHead := mustCreateUser(t, "mentor_head", fmt.Sprintf("mh_block_%d@eightytwenty.test", nowSuffix))
	mentor := mustCreateUser(t, "mentor", fmt.Sprintf("mentor_block_%d@eightytwenty.test", nowSuffix))

	defer func() {
		mustExec(t, `DELETE FROM attendance WHERE session_id IN (SELECT id FROM class_sessions WHERE class_key = $1)`, classKey)
		mustExec(t, `DELETE FROM class_sessions WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM class_memberships WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM class_enrollments WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM grades WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM mentor_assignments WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM class_groups WHERE class_key = $1`, classKey)
		mustExec(t, `DELETE FROM users WHERE id = $1`, mentor.ID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, mentorHead.ID)
	}()

	mustExec(t, `
		INSERT INTO class_groups (
			class_key, level, class_days, class_time, class_number,
			sent_to_mentor, sent_at, updated_at, round_status, round_started_at, round_started_by
		) VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW(), 'active', NOW(), $6)
	`, classKey, level, classDays, classTime, classNumber, mentorHead.ID)
	mustExec(t, `INSERT INTO mentor_assignments (mentor_user_id, class_key, created_by_user_id) VALUES ($1, $2, $3)`,
		mentor.ID, classKey, mentorHead.ID)
	mustCreateCompletedSessions(t, classKey, classTime)

	lead := testLead{
		Name:                             "Blocked Bundle Waiter",
		Phone:                            fmt.Sprintf("0300%07d", nowSuffix%10000000),
		LevelsPurchased:                  2,
		LevelsConsumed:                   0,
		ExpectedLevel:                    2,
		ExpectedCredits:                  0,
		ExpectedStatus:                   "waiting_for_round",
		ExpectedOutcome:                  "promoted",
		ExpectedFinalGrade:               "A",
		ExpectedNextLevelConsumedOnClose: true,
		ExpectedLevelsConsumed:           2,
	}
	lead.ID = mustCreateLead(t, lead, mentorHead.ID)
	defer func() {
		mustExec(t, `DELETE FROM class_memberships WHERE lead_id = $1`, lead.ID)
		mustExec(t, `DELETE FROM leads WHERE id = $1`, lead.ID)
		mustExec(t, `DELETE FROM placement_tests WHERE lead_id = $1`, lead.ID)
		mustExec(t, `DELETE FROM scheduling WHERE lead_id = $1`, lead.ID)
	}()

	mustCreatePlacementTest(t, lead.ID, 1)
	mustCreateScheduling(t, lead.ID, classDays, classTime, classNumber)

	body, _ := json.Marshal(map[string]string{
		"lead_id":   lead.ID.String(),
		"class_key": classKey,
		"grade":     lead.ExpectedFinalGrade,
		"notes":     "Prepaid continuation blocker regression test confirms the student should stay waiting.",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/mentor/grades", bytes.NewReader(body))
	req = withUserContext(req, mentorHead.ID, mentorHead.Email, "mentor_head")
	res := httptest.NewRecorder()
	h.CreateGrade(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("CreateGrade failed: status=%d body=%s", res.Code, res.Body.String())
	}

	closeBody, _ := json.Marshal(map[string]string{"class_key": classKey})
	closeReq := httptest.NewRequest(http.MethodPost, "/api/mentor-head/close-round", bytes.NewReader(closeBody))
	closeReq = withUserContext(closeReq, mentorHead.ID, mentorHead.Email, "mentor_head")
	closeRes := httptest.NewRecorder()
	h.CloseRound(closeRes, closeReq)
	if closeRes.Code != http.StatusOK {
		t.Fatalf("CloseRound failed: status=%d body=%s", closeRes.Code, closeRes.Body.String())
	}

	var status string
	if err := mustQueryRow(t, `SELECT status FROM leads WHERE id = $1`, lead.ID).Scan(&status); err != nil {
		t.Fatalf("failed to load post-close status: %v", err)
	}
	if status != "waiting_for_round" {
		t.Fatalf("expected post-close status waiting_for_round, got %s", status)
	}

	err := models.UpdateLeadStatus(lead.ID, "offer_sent")
	if err == nil {
		t.Fatalf("expected prepaid continuation blocker error when moving lead to offer_sent")
	}
	var blockedErr *models.PrepaidContinuationBlockedError
	if !errors.As(err, &blockedErr) {
		t.Fatalf("expected PrepaidContinuationBlockedError, got %T: %v", err, err)
	}

	if err := mustQueryRow(t, `SELECT status FROM leads WHERE id = $1`, lead.ID).Scan(&status); err != nil {
		t.Fatalf("failed to reload status after blocker: %v", err)
	}
	if status != "waiting_for_round" {
		t.Fatalf("expected status to remain waiting_for_round after blocker, got %s", status)
	}
}

func TestMain(m *testing.M) {
	// Ensure a DB URL is available for local runs.
	if os.Getenv("DATABASE_URL") == "" {
		_ = os.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/eighty_twenty_ops?sslmode=disable")
	}
	os.Exit(m.Run())
}
