package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func newPlacementBookingRequest(form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/pre-enrolment/test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestBuildBookedPlacementTestRequiresPaidAmount(t *testing.T) {
	form := url.Values{
		"test_date":                     {"2026-05-17"},
		"test_time":                     {"15:23"},
		"test_type":                     {"Online"},
		"placement_test_fee":            {"60"},
		"placement_test_fee_paid":       {"0"},
		"placement_test_payment_date":   {"2026-05-17"},
		"placement_test_payment_method": {"vodafone_cash"},
	}

	_, err := buildBookedPlacementTestFromRequest(uuid.New(), nil, newPlacementBookingRequest(form))
	if err == nil || !strings.Contains(err.Error(), "Paid amount is required") {
		t.Fatalf("expected paid amount validation error, got %v", err)
	}
}

func TestBuildBookedPlacementTestRequiresPaidAmountToEqualFinalFee(t *testing.T) {
	form := url.Values{
		"test_date":                     {"2026-05-17"},
		"test_time":                     {"15:23"},
		"test_type":                     {"Online"},
		"placement_test_fee":            {"60"},
		"placement_test_fee_paid":       {"30"},
		"placement_test_payment_date":   {"2026-05-17"},
		"placement_test_payment_method": {"vodafone_cash"},
	}

	_, err := buildBookedPlacementTestFromRequest(uuid.New(), nil, newPlacementBookingRequest(form))
	if err == nil || !strings.Contains(err.Error(), "must equal the final placement test fee") {
		t.Fatalf("expected exact paid amount validation error, got %v", err)
	}
}

func TestBuildBookedPlacementTestAcceptsFullPaidAmount(t *testing.T) {
	form := url.Values{
		"test_date":                     {"2026-05-17"},
		"test_time":                     {"15:23"},
		"test_type":                     {"Online"},
		"placement_test_fee":            {"60"},
		"placement_test_fee_paid":       {"60"},
		"placement_test_payment_date":   {"2026-05-17"},
		"placement_test_payment_method": {"vodafone_cash"},
	}

	pt, err := buildBookedPlacementTestFromRequest(uuid.New(), nil, newPlacementBookingRequest(form))
	if err != nil {
		t.Fatalf("expected valid booking request, got %v", err)
	}
	if !pt.PlacementTestFeePaid.Valid || pt.PlacementTestFeePaid.Int32 != 60 {
		t.Fatalf("expected paid amount 60, got %+v", pt.PlacementTestFeePaid)
	}
}
