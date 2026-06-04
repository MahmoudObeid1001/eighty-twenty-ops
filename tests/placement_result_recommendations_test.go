package tests

import (
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

type placementResultPayload struct {
	Name    string                         `json:"name"`
	Level   int32                          `json:"level"`
	Classes string                         `json:"classes"`
	Slots   []handlers.PlacementResultSlot `json:"slots"`
}

func TestPlacementResultRecommendations_PrioritizesUnderconstructionClass(t *testing.T) {
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
	admin := mustCreateUser(t, "admin", fmt.Sprintf("placement_result_admin_%d@eightytwenty.test", nowSuffix))
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
	})

	leadID := createLeadWithStatus(t, "Result Student", uniquePhone(nowSuffix, 1), "offer_sent", admin.ID)
	enrolledLeadID := createLeadWithStatus(t, "Existing Student", uniquePhone(nowSuffix, 2), "ready_to_start", admin.ID)
	t.Cleanup(func() {
		cleanupLead(t, enrolledLeadID)
		cleanupLead(t, leadID)
	})

	mustExec(t, `
		INSERT INTO placement_tests (lead_id, assigned_level, updated_at)
		VALUES ($1, 2, NOW()), ($2, 2, NOW())
		ON CONFLICT (lead_id) DO UPDATE
		SET assigned_level = EXCLUDED.assigned_level, updated_at = NOW()
	`, leadID, enrolledLeadID)

	classNumber := int32(nowSuffix%100000) + 1
	classKey := models.GenerateClassKey(2, "Sat/Tues", "10:00:00", classNumber)
	t.Cleanup(func() {
		cleanupClass(t, classKey)
	})
	mustExec(t, `
		INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, round_status, updated_at)
		VALUES ($1, 2, 'Sat/Tues', '10:00:00', $2, false, 'not_started', NOW())
	`, classKey, classNumber)
	mustExec(t, `
		INSERT INTO scheduling (lead_id, class_days, class_time, class_group_index, updated_at)
		VALUES ($1, 'Sat/Tues', '10:00:00'::time, $2, NOW())
		ON CONFLICT (lead_id) DO UPDATE
		SET class_days = EXCLUDED.class_days,
		    class_time = EXCLUDED.class_time,
		    class_group_index = EXCLUDED.class_group_index,
		    updated_at = NOW()
	`, enrolledLeadID, classNumber)

	req := httptest.NewRequest(http.MethodGet, "/api/pre-enrolment/"+leadID.String()+"/placement-result-data", nil)
	res := httptest.NewRecorder()

	h.GetPlacementResultData(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected success, got %d body=%s", res.Code, res.Body.String())
	}

	var payload placementResultPayload
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(payload.Slots) != 4 {
		t.Fatalf("expected 4 slots when populated underconstruction class exists, got %d payload=%+v", len(payload.Slots), payload)
	}
	if payload.Slots[0].Kind != "underconstruction" {
		t.Fatalf("expected first slot to be underconstruction, got %+v", payload.Slots[0])
	}
	if payload.Slots[0].Status != "not_started" {
		t.Fatalf("expected first slot status not_started, got %+v", payload.Slots[0])
	}
	if payload.Slots[0].Days != "Sat-Tue" || payload.Slots[0].Time != "10:00PM" {
		t.Fatalf("expected first slot to match underconstruction class, got %+v", payload.Slots[0])
	}
	for i, slot := range payload.Slots[1:] {
		if slot.Kind != "standard" {
			t.Fatalf("expected remaining slot %d to be standard, got %+v", i+1, slot)
		}
	}
}

func TestPlacementResultRecommendations_DoesNotUseActiveOrEmptyUnderconstructionForSpecialBranch(t *testing.T) {
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
	admin := mustCreateUser(t, "admin", fmt.Sprintf("placement_result_branch_admin_%d@eightytwenty.test", nowSuffix))
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM users WHERE id = $1`, admin.ID)
	})

	leadID := createLeadWithStatus(t, "Result Student Two", uniquePhone(nowSuffix, 11), "offer_sent", admin.ID)
	activeLeadID := createLeadWithStatus(t, "Active Student", uniquePhone(nowSuffix, 12), "in_classes", admin.ID)
	t.Cleanup(func() {
		cleanupLead(t, activeLeadID)
		cleanupLead(t, leadID)
	})

	mustExec(t, `
		INSERT INTO placement_tests (lead_id, assigned_level, updated_at)
		VALUES ($1, 3, NOW()), ($2, 3, NOW())
		ON CONFLICT (lead_id) DO UPDATE
		SET assigned_level = EXCLUDED.assigned_level, updated_at = NOW()
	`, leadID, activeLeadID)

	activeClassNumber := int32(nowSuffix%100000) + 20
	activeClassKey := models.GenerateClassKey(3, "Sun/Wed", "07:30:00", activeClassNumber)
	t.Cleanup(func() {
		cleanupClass(t, activeClassKey)
	})
	mustExec(t, `
		INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, round_status, updated_at)
		VALUES ($1, 3, 'Sun/Wed', '07:30:00', $2, false, 'active', NOW())
	`, activeClassKey, activeClassNumber)
	mustExec(t, `
		INSERT INTO scheduling (lead_id, class_days, class_time, class_group_index, updated_at)
		VALUES ($1, 'Sun/Wed', '07:30:00'::time, $2, NOW())
		ON CONFLICT (lead_id) DO UPDATE
		SET class_days = EXCLUDED.class_days,
		    class_time = EXCLUDED.class_time,
		    class_group_index = EXCLUDED.class_group_index,
		    updated_at = NOW()
	`, activeLeadID, activeClassNumber)

	emptyNotStartedClassNumber := int32(nowSuffix%100000) + 21
	emptyNotStartedClassKey := models.GenerateClassKey(3, "Mon/Thu", "10:00:00", emptyNotStartedClassNumber)
	t.Cleanup(func() {
		cleanupClass(t, emptyNotStartedClassKey)
	})
	mustExec(t, `
		INSERT INTO class_groups (class_key, level, class_days, class_time, class_number, sent_to_mentor, round_status, updated_at)
		VALUES ($1, 3, 'Mon/Thu', '10:00:00', $2, false, 'not_started', NOW())
	`, emptyNotStartedClassKey, emptyNotStartedClassNumber)

	req := httptest.NewRequest(http.MethodGet, "/api/pre-enrolment/"+leadID.String()+"/placement-result-data", nil)
	res := httptest.NewRecorder()

	h.GetPlacementResultData(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected success, got %d body=%s", res.Code, res.Body.String())
	}

	var payload placementResultPayload
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(payload.Slots) != 6 {
		t.Fatalf("expected 6 standard slots when no populated underconstruction class exists, got %d payload=%+v", len(payload.Slots), payload)
	}
	for i, slot := range payload.Slots {
		if slot.Kind != "standard" {
			t.Fatalf("expected slot %d to be standard, got %+v", i, slot)
		}
	}
}
