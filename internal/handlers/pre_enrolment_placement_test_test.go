package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"eighty-twenty-ops/internal/models"

	"github.com/google/uuid"
)

func newPlacementBookingRequest(form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/pre-enrolment/test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func placementBookingBaseForm() url.Values {
	return url.Values{
		"test_date":                         {"2026-05-17"},
		"test_time":                         {"15:30"},
		"test_type":                         {"online"},
		"scheduled_student_success_user_id": {uuid.New().String()},
		"placement_test_fee":                {"60"},
		"placement_test_payment_date":       {"2026-05-17"},
		"placement_test_payment_method":     {"vodafone_cash"},
	}
}

func TestBuildBookedPlacementTestRequiresPaidAmount(t *testing.T) {
	form := placementBookingBaseForm()
	form.Set("placement_test_fee_paid", "0")

	_, err := buildBookedPlacementTestFromRequest(uuid.New(), nil, newPlacementBookingRequest(form))
	if err == nil || !strings.Contains(err.Error(), "Paid amount is required") {
		t.Fatalf("expected paid amount validation error, got %v", err)
	}
}

func TestBuildBookedPlacementTestRequiresPaidAmountToEqualFinalFee(t *testing.T) {
	form := placementBookingBaseForm()
	form.Set("placement_test_fee_paid", "30")

	_, err := buildBookedPlacementTestFromRequest(uuid.New(), nil, newPlacementBookingRequest(form))
	if err == nil || !strings.Contains(err.Error(), "must equal the final placement test fee") {
		t.Fatalf("expected exact paid amount validation error, got %v", err)
	}
}

func TestBuildBookedPlacementTestAcceptsFullPaidAmount(t *testing.T) {
	form := placementBookingBaseForm()
	form.Set("placement_test_fee_paid", "60")

	pt, err := buildBookedPlacementTestFromRequest(uuid.New(), nil, newPlacementBookingRequest(form))
	if err != nil {
		t.Fatalf("expected valid booking request, got %v", err)
	}
	if !pt.PlacementTestFeePaid.Valid || pt.PlacementTestFeePaid.Int32 != 60 {
		t.Fatalf("expected paid amount 60, got %+v", pt.PlacementTestFeePaid)
	}
}

func TestBuildBookedPlacementTestPreservesExistingNotesOnReschedule(t *testing.T) {
	form := placementBookingBaseForm()
	form.Set("placement_test_fee_paid", "60")
	existing := &models.PlacementTest{
		TestNotes: sql.NullString{String: "Placement test no-show.", Valid: true},
	}

	pt, err := buildBookedPlacementTestFromRequest(uuid.New(), existing, newPlacementBookingRequest(form))
	if err != nil {
		t.Fatalf("expected valid reschedule request, got %v", err)
	}
	if !pt.TestNotes.Valid || pt.TestNotes.String != existing.TestNotes.String {
		t.Fatalf("expected existing test notes to be preserved, got %+v", pt.TestNotes)
	}
}
