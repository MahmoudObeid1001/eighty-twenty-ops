package tests

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"eighty-twenty-ops/internal/config"
	"eighty-twenty-ops/internal/db"
	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

func TestPlacementTestSchedulingRules(t *testing.T) {
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
	admin := mustCreateUser(t, "admin", fmt.Sprintf("pt_admin_%d@eightytwenty.test", nowSuffix))
	ssOne := mustCreateUser(t, "student_success", fmt.Sprintf("pt_ss_one_%d@eightytwenty.test", nowSuffix))
	ssTwo := mustCreateUser(t, "student_success", fmt.Sprintf("pt_ss_two_%d@eightytwenty.test", nowSuffix))
	leadOne := createLeadWithStatus(t, "Scheduled One", uniquePhone(nowSuffix, 501), "lead_created", admin.ID)
	leadTwo := createLeadWithStatus(t, "Scheduled Two", uniquePhone(nowSuffix, 502), "lead_created", admin.ID)
	leadThree := createLeadWithStatus(t, "Scheduled Three", uniquePhone(nowSuffix, 503), "lead_created", admin.ID)
	t.Cleanup(func() {
		cleanupLead(t, leadOne)
		cleanupLead(t, leadTwo)
		cleanupLead(t, leadThree)
		mustExec(t, `DELETE FROM student_success_availability_windows WHERE student_success_user_id IN ($1, $2)`, ssOne.ID, ssTwo.ID)
		mustExec(t, `DELETE FROM users WHERE id IN ($1, $2, $3)`, admin.ID, ssOne.ID, ssTwo.ID)
	})

	testDate := nextFutureWeekday(time.Monday)
	if _, err := models.ReplaceStudentSuccessAvailabilityWindows(ssOne.ID, testDate, []models.StudentSuccessAvailabilityWindow{{
		StudentSuccessUserID: ssOne.ID,
		AvailableDate:        testDate,
		StartTime:            "15:00",
		EndTime:              "17:00",
		Note:                 sql.NullString{String: "placement tests", Valid: true},
	}}); err != nil {
		t.Fatalf("failed to seed first SS availability: %v", err)
	}
	if _, err := models.ReplaceStudentSuccessAvailabilityWindows(ssTwo.ID, testDate, []models.StudentSuccessAvailabilityWindow{{
		StudentSuccessUserID: ssTwo.ID,
		AvailableDate:        testDate,
		StartTime:            "15:00",
		EndTime:              "17:00",
	}}); err != nil {
		t.Fatalf("failed to seed second SS availability: %v", err)
	}

	if err := models.BookPlacementTest(leadOne, placementTestBooking(testDate, "15:30", ssOne.ID)); err != nil {
		t.Fatalf("expected booking inside availability to succeed: %v", err)
	}

	err := models.BookPlacementTest(leadTwo, placementTestBooking(testDate, "15:30", ssOne.ID))
	if err == nil || !models.IsPlacementTestSlotConflict(err) {
		t.Fatalf("expected double booking to fail with slot conflict, got %v", err)
	}

	err = models.BookPlacementTest(leadTwo, placementTestBooking(testDate, "18:00", ssOne.ID))
	if err == nil || !strings.Contains(err.Error(), "outside this Student Success availability") {
		t.Fatalf("expected outside availability booking to fail, got %v", err)
	}

	slots, err := models.GetPlacementTestSlots(ssOne.ID, testDate, testDate.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("failed to load placement test slots: %v", err)
	}
	if state := slotState(slots, "15:30"); state != "booked" {
		t.Fatalf("expected 15:30 to be booked, got %q", state)
	}
	if state := slotState(slots, "17:00"); state != "outside_availability" {
		t.Fatalf("expected 17:00 to be outside availability, got %q", state)
	}

	if err := models.BookPlacementTest(leadThree, placementTestBooking(testDate, "15:30", ssTwo.ID)); err != nil {
		t.Fatalf("expected same slot with different SS to succeed: %v", err)
	}
	ssOneQueue, err := models.GetPlacementTestsForStudentSuccess(ssOne.ID, false)
	if err != nil {
		t.Fatalf("failed to load first SS queue: %v", err)
	}
	if len(ssOneQueue) != 1 || ssOneQueue[0].LeadID != leadOne {
		t.Fatalf("expected first SS queue to contain only lead one, got %+v", ssOneQueue)
	}
	ssTwoQueue, err := models.GetPlacementTestsForStudentSuccess(ssTwo.ID, false)
	if err != nil {
		t.Fatalf("failed to load second SS queue: %v", err)
	}
	if len(ssTwoQueue) != 1 || ssTwoQueue[0].LeadID != leadThree {
		t.Fatalf("expected second SS queue to contain only lead three, got %+v", ssTwoQueue)
	}
}

func TestStudentSuccessAvailabilityRejectsOverlappingWindows(t *testing.T) {
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
	ss := mustCreateUser(t, "student_success", fmt.Sprintf("pt_ss_overlap_%d@eightytwenty.test", nowSuffix))
	t.Cleanup(func() {
		mustExec(t, `DELETE FROM student_success_availability_windows WHERE student_success_user_id = $1`, ss.ID)
		mustExec(t, `DELETE FROM users WHERE id = $1`, ss.ID)
	})

	testDate := nextFutureWeekday(time.Tuesday)
	_, err := models.ReplaceStudentSuccessAvailabilityWindows(ss.ID, testDate, []models.StudentSuccessAvailabilityWindow{
		{
			StudentSuccessUserID: ss.ID,
			AvailableDate:        testDate,
			StartTime:            "14:00",
			EndTime:              "14:30",
		},
		{
			StudentSuccessUserID: ss.ID,
			AvailableDate:        testDate,
			StartTime:            "14:00",
			EndTime:              "14:30",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected duplicate slot windows to be rejected, got %v", err)
	}

	_, err = models.ReplaceStudentSuccessAvailabilityWindows(ss.ID, testDate, []models.StudentSuccessAvailabilityWindow{
		{
			StudentSuccessUserID: ss.ID,
			AvailableDate:        testDate,
			StartTime:            "14:00",
			EndTime:              "15:00",
		},
		{
			StudentSuccessUserID: ss.ID,
			AvailableDate:        testDate,
			StartTime:            "14:30",
			EndTime:              "15:30",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("expected overlapping slot windows to be rejected, got %v", err)
	}

	_, err = models.ReplaceStudentSuccessAvailabilityWindows(ss.ID, testDate, []models.StudentSuccessAvailabilityWindow{
		{
			StudentSuccessUserID: ss.ID,
			AvailableDate:        testDate,
			StartTime:            "14:00",
			EndTime:              "15:00",
		},
		{
			StudentSuccessUserID: ss.ID,
			AvailableDate:        testDate,
			StartTime:            "15:00",
			EndTime:              "15:30",
		},
	})
	if err != nil {
		t.Fatalf("expected adjacent slot windows to remain valid, got %v", err)
	}
}

func placementTestBooking(testDate time.Time, testTime string, studentSuccessID uuid.UUID) *models.PlacementTest {
	return &models.PlacementTest{
		TestDate:                  sql.NullTime{Time: testDate, Valid: true},
		TestTime:                  sql.NullString{String: testTime, Valid: true},
		TestType:                  sql.NullString{String: "online", Valid: true},
		ScheduledStudentSuccessID: sql.NullString{String: studentSuccessID.String(), Valid: true},
	}
}

func slotState(slots []models.PlacementTestSlot, testTime string) string {
	for _, slot := range slots {
		if slot.Time == testTime {
			return slot.State
		}
	}
	return ""
}

func nextFutureWeekday(weekday time.Weekday) time.Time {
	date := time.Now().UTC().AddDate(0, 0, 14)
	for date.Weekday() != weekday {
		date = date.AddDate(0, 0, 1)
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
}
