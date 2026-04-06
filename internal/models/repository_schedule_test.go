package models

import (
	"testing"
	"time"
)

func TestBuildClassSessionDatesAlternatesFromSecondDay(t *testing.T) {
	startDate := time.Date(2026, time.April, 8, 0, 0, 0, 0, time.UTC) // Wednesday

	dates, err := buildClassSessionDates("Sun/Wed", startDate, 4)
	if err != nil {
		t.Fatalf("buildClassSessionDates returned error: %v", err)
	}

	got := []string{
		dates[0].Format("2006-01-02"),
		dates[1].Format("2006-01-02"),
		dates[2].Format("2006-01-02"),
		dates[3].Format("2006-01-02"),
	}
	want := []string{
		"2026-04-08",
		"2026-04-12",
		"2026-04-15",
		"2026-04-19",
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("date %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestBuildClassSessionDatesRejectsNonClassDay(t *testing.T) {
	startDate := time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC) // Friday

	_, err := buildClassSessionDates("Sun/Wed", startDate, 2)
	if err == nil {
		t.Fatal("expected error for invalid start day, got nil")
	}
}
