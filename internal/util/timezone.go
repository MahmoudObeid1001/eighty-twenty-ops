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
