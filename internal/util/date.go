package util

import (
	"fmt"
	"time"
)

// startOfDay returns the start of the day (00:00:00) in local timezone for the given time.
// This normalizes any time to the same day in local timezone for date-only comparison.
func startOfDay(t time.Time) time.Time {
	// Convert to local timezone first
	localTime := t.Local()
	return time.Date(localTime.Year(), localTime.Month(), localTime.Day(), 0, 0, 0, 0, time.Local)
}

// ParseDateLocal parses a date string in YYYY-MM-DD format and returns it in local timezone.
// This ensures dates from HTML date inputs are parsed consistently in local time.
func ParseDateLocal(dateStr string) (time.Time, error) {
	// Parse in UTC first (time.Parse default)
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, err
	}
	// Convert to local timezone and return start of day
	return startOfDay(t), nil
}

// ValidateNotFutureDate validates that a date is not in the future.
// It compares only the DATE (not time of day). Treats "today" as allowed.
// Returns an error if the date is after today.
// Both dates are normalized to local timezone for consistent comparison.
func ValidateNotFutureDate(d time.Time) error {
	// Normalize both dates to start of day in local timezone
	todayDay := startOfDay(time.Now())
	paymentDay := startOfDay(d)
	
	// Compare dates (not times) - today is allowed, only future dates are rejected
	if paymentDay.After(todayDay) {
		return fmt.Errorf("payment date cannot be in the future")
	}
	
	return nil
}
