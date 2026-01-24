package util

import (
	"testing"
	"time"
)

func TestValidateNotFutureDate(t *testing.T) {
	// Get today in local timezone
	today := time.Now()
	todayDay := startOfDay(today)
	
	// Test cases
	tests := []struct {
		name    string
		date    time.Time
		wantErr bool
	}{
		{
			name:    "yesterday should be allowed",
			date:    todayDay.AddDate(0, 0, -1),
			wantErr: false,
		},
		{
			name:    "today should be allowed",
			date:    todayDay,
			wantErr: false,
		},
		{
			name:    "tomorrow should be rejected",
			date:    todayDay.AddDate(0, 0, 1),
			wantErr: true,
		},
		{
			name:    "far future should be rejected",
			date:    todayDay.AddDate(1, 0, 0),
			wantErr: true,
		},
		{
			name:    "far past should be allowed",
			date:    todayDay.AddDate(-1, 0, 0),
			wantErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotFutureDate(tt.date)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNotFutureDate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && err.Error() != "payment date cannot be in the future" {
				t.Errorf("ValidateNotFutureDate() error message = %v, want 'payment date cannot be in the future'", err.Error())
			}
		})
	}
}

func TestParseDateLocal(t *testing.T) {
	tests := []struct {
		name    string
		dateStr string
		wantErr bool
	}{
		{
			name:    "valid date string",
			dateStr: "2026-01-23",
			wantErr: false,
		},
		{
			name:    "invalid date string",
			dateStr: "invalid",
			wantErr: true,
		},
		{
			name:    "empty string",
			dateStr: "",
			wantErr: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDateLocal(tt.dateStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDateLocal() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
	
	// Test that parsed date is in local timezone
	parsed, err := ParseDateLocal("2026-01-23")
	if err != nil {
		t.Fatalf("ParseDateLocal() failed: %v", err)
	}
	if parsed.Location() != time.Local {
		t.Errorf("ParseDateLocal() location = %v, want %v", parsed.Location(), time.Local)
	}
	
	// Test that parsed date is start of day
	expectedHour := 0
	if parsed.Hour() != expectedHour {
		t.Errorf("ParseDateLocal() hour = %d, want %d", parsed.Hour(), expectedHour)
	}
	if parsed.Minute() != 0 || parsed.Second() != 0 {
		t.Errorf("ParseDateLocal() should return start of day (00:00:00)")
	}
}

func TestStartOfDay(t *testing.T) {
	// Test with different times of day
	now := time.Now()
	midnight := startOfDay(now)
	
	// Should be start of day
	if midnight.Hour() != 0 || midnight.Minute() != 0 || midnight.Second() != 0 {
		t.Errorf("startOfDay() should return 00:00:00")
	}
	
	// Should be same date
	if midnight.Year() != now.Year() || midnight.Month() != now.Month() || midnight.Day() != now.Day() {
		t.Errorf("startOfDay() should preserve date")
	}
	
	// Should be in local timezone
	if midnight.Location() != time.Local {
		t.Errorf("startOfDay() location = %v, want %v", midnight.Location(), time.Local)
	}
}
