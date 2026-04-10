package util

import "time"

const CairoTimeZone = "Africa/Cairo"

// CairoLocation returns the operational timezone used for reporting and class-time logic.
func CairoLocation() *time.Location {
	loc, err := time.LoadLocation(CairoTimeZone)
	if err != nil {
		return time.Local
	}
	return loc
}

// CairoNow returns the current time in the operational Cairo timezone.
func CairoNow() time.Time {
	return time.Now().In(CairoLocation())
}

// CairoStartOfDay normalizes a timestamp to the start of its day in Cairo.
func CairoStartOfDay(t time.Time) time.Time {
	ct := t.In(CairoLocation())
	return time.Date(ct.Year(), ct.Month(), ct.Day(), 0, 0, 0, 0, CairoLocation())
}

// ParseDateCairo parses a YYYY-MM-DD string in the Cairo timezone.
func ParseDateCairo(dateStr string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", dateStr, CairoLocation())
}

// FormatDateCairo returns the YYYY-MM-DD date in Cairo for a timestamp.
func FormatDateCairo(t time.Time) string {
	return CairoStartOfDay(t).Format("2006-01-02")
}

// CairoStartOfBusinessWeek returns the Saturday that starts the Cairo business week
// containing the provided day. Friday is treated as the weekend immediately after
// the week that ended on Thursday.
func CairoStartOfBusinessWeek(t time.Time) time.Time {
	day := CairoStartOfDay(t)
	offset := 0
	switch day.Weekday() {
	case time.Saturday:
		offset = 0
	case time.Sunday:
		offset = -1
	case time.Monday:
		offset = -2
	case time.Tuesday:
		offset = -3
	case time.Wednesday:
		offset = -4
	case time.Thursday:
		offset = -5
	case time.Friday:
		offset = -6
	}
	return day.AddDate(0, 0, offset)
}

// LastCompletedCairoBusinessWeek returns the most recently completed Sat-Thu
// business week relative to the provided Cairo business day.
func LastCompletedCairoBusinessWeek(t time.Time) (time.Time, time.Time) {
	day := CairoStartOfDay(t)
	startOfContainingWeek := CairoStartOfBusinessWeek(day)
	if day.Weekday() == time.Friday {
		return startOfContainingWeek, startOfContainingWeek.AddDate(0, 0, 5)
	}
	lastWeekStart := startOfContainingWeek.AddDate(0, 0, -7)
	return lastWeekStart, lastWeekStart.AddDate(0, 0, 5)
}
