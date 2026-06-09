package models

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
)

const placementTestSlotMinutes = 30

var (
	placementTestDayStart = mustClock("14:00")
	placementTestDayEnd   = mustClock("23:00")
)

type PlacementTestSlotConflictError struct {
	StudentSuccessUserID uuid.UUID
	TestDate             time.Time
	TestTime             string
}

func (e *PlacementTestSlotConflictError) Error() string {
	return fmt.Sprintf("placement test slot already booked for this Student Success at %s %s", e.TestDate.Format("2006-01-02"), e.TestTime)
}

func mustClock(value string) time.Time {
	clock, err := parseSessionClock(value)
	if err != nil {
		panic(err)
	}
	return clock
}

func GetStudentSuccessAvailabilityWindows(studentSuccessUserID uuid.UUID, monthStart time.Time) ([]StudentSuccessAvailabilityWindow, error) {
	monthStart = monthStartDate(monthStart)
	monthEnd := monthStart.AddDate(0, 1, 0)
	rows, err := db.DB.Query(`
		SELECT id, student_success_user_id, available_date, start_time::TEXT, end_time::TEXT, note, created_at, updated_at
		FROM student_success_availability_windows
		WHERE student_success_user_id = $1
		  AND available_date >= $2
		  AND available_date < $3
		ORDER BY available_date, start_time, end_time
	`, studentSuccessUserID, monthStart, monthEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to query student success availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	windows := make([]StudentSuccessAvailabilityWindow, 0)
	for rows.Next() {
		var item StudentSuccessAvailabilityWindow
		if err := rows.Scan(
			&item.ID,
			&item.StudentSuccessUserID,
			&item.AvailableDate,
			&item.StartTime,
			&item.EndTime,
			&item.Note,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan student success availability: %w", err)
		}
		item.StartTime = formatClockText(item.StartTime)
		item.EndTime = formatClockText(item.EndTime)
		windows = append(windows, item)
	}
	return windows, rows.Err()
}

func GetStudentSuccessAvailabilityWindowsForDate(studentSuccessUserID uuid.UUID, date time.Time) ([]StudentSuccessAvailabilityWindow, error) {
	return getStudentSuccessAvailabilityWindowsForDate(studentSuccessUserID, dateOnly(date))
}

func ReplaceStudentSuccessAvailabilityWindows(studentSuccessUserID uuid.UUID, monthStart time.Time, windows []StudentSuccessAvailabilityWindow) ([]StudentSuccessAvailabilityWindow, error) {
	monthStart = monthStartDate(monthStart)
	monthEnd := monthStart.AddDate(0, 1, 0)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	groupedByDate := make(map[string][]StudentSuccessAvailabilityWindow)

	for i, item := range windows {
		availableDate := dateOnly(item.AvailableDate)
		if availableDate.Before(monthStart) || !availableDate.Before(monthEnd) {
			return nil, fmt.Errorf("availability date %s is outside the selected month", availableDate.Format("2006-01-02"))
		}
		if availableDate.Before(today) {
			return nil, fmt.Errorf("availability cannot be submitted for past dates (%s)", availableDate.Format("2006-01-02"))
		}
		startClock, err := parseSessionClock(item.StartTime)
		if err != nil {
			return nil, fmt.Errorf("window %d start_time is invalid: %w", i+1, err)
		}
		endClock, err := parseSessionClock(item.EndTime)
		if err != nil {
			return nil, fmt.Errorf("window %d end_time is invalid: %w", i+1, err)
		}
		if !clockAfter(endClock, startClock) {
			return nil, fmt.Errorf("window %d end_time must be after start_time", i+1)
		}
		if clockBefore(startClock, placementTestDayStart) || clockAfter(endClock, placementTestDayEnd) {
			return nil, fmt.Errorf("window %d must be between 14:00 and 23:00", i+1)
		}
		groupedByDate[availableDate.Format("2006-01-02")] = append(groupedByDate[availableDate.Format("2006-01-02")], StudentSuccessAvailabilityWindow{
			AvailableDate: availableDate,
			StartTime:     startClock.Format("15:04"),
			EndTime:       endClock.Format("15:04"),
		})
	}

	for date, dateWindows := range groupedByDate {
		sort.Slice(dateWindows, func(i, j int) bool {
			if dateWindows[i].StartTime == dateWindows[j].StartTime {
				return dateWindows[i].EndTime < dateWindows[j].EndTime
			}
			return dateWindows[i].StartTime < dateWindows[j].StartTime
		})
		for i := 1; i < len(dateWindows); i++ {
			prev := dateWindows[i-1]
			curr := dateWindows[i]
			prevEnd, _ := parseSessionClock(prev.EndTime)
			currStart, _ := parseSessionClock(curr.StartTime)
			if clockBefore(currStart, prevEnd) {
				return nil, fmt.Errorf("availability windows overlap on %s (%s-%s conflicts with %s-%s)", date, prev.StartTime, prev.EndTime, curr.StartTime, curr.EndTime)
			}
		}
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin student success availability update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	deleteStart := monthStart
	if today.After(deleteStart) {
		deleteStart = today
	}
	if deleteStart.Before(monthEnd) {
		if _, err := tx.Exec(`
			DELETE FROM student_success_availability_windows
			WHERE student_success_user_id = $1
			  AND available_date >= $2
			  AND available_date < $3
		`, studentSuccessUserID, deleteStart, monthEnd); err != nil {
			return nil, fmt.Errorf("failed to clear student success availability: %w", err)
		}
	}

	now := time.Now()
	for _, item := range windows {
		note := sql.NullString{String: strings.TrimSpace(item.Note.String), Valid: item.Note.Valid && strings.TrimSpace(item.Note.String) != ""}
		if !note.Valid {
			note.String = ""
		}
		if _, err := tx.Exec(`
			INSERT INTO student_success_availability_windows (
				id,
				student_success_user_id,
				available_date,
				start_time,
				end_time,
				note,
				created_at,
				updated_at
			)
			VALUES (gen_random_uuid(), $1, $2, $3::TIME, $4::TIME, $5, $6, $6)
			ON CONFLICT DO NOTHING
		`, studentSuccessUserID, dateOnly(item.AvailableDate), formatClockText(item.StartTime), formatClockText(item.EndTime), note, now); err != nil {
			return nil, fmt.Errorf("failed to insert student success availability: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit student success availability update: %w", err)
	}
	return GetStudentSuccessAvailabilityWindows(studentSuccessUserID, monthStart)
}

func GetPlacementTestSlots(studentSuccessUserID uuid.UUID, date time.Time, now time.Time) ([]PlacementTestSlot, error) {
	date = dateOnly(date)
	if now.IsZero() {
		now = time.Now()
	}

	windows, err := getStudentSuccessAvailabilityWindowsForDate(studentSuccessUserID, date)
	if err != nil {
		return nil, err
	}
	bookings, err := getPlacementTestBookingsForSlotDate(studentSuccessUserID, date)
	if err != nil {
		return nil, err
	}

	slots := make([]PlacementTestSlot, 0, ((23-14)*60)/placementTestSlotMinutes)
	for clock := placementTestDayStart; clockBefore(clock, placementTestDayEnd); clock = clock.Add(time.Duration(placementTestSlotMinutes) * time.Minute) {
		timeText := clock.Format("15:04")
		slot := PlacementTestSlot{Time: timeText}
		slotDateTime := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, time.UTC)
		if !now.IsZero() && !slotDateTime.After(now) {
			slot.State = "past"
			slot.Disabled = true
			slot.Reason = "Past slot"
		} else if booking, ok := bookings[timeText]; ok {
			slot.State = "booked"
			slot.Disabled = true
			slot.LeadID = booking.LeadID
			slot.LeadName = booking.LeadName
			slot.LeadPhone = booking.LeadPhone
			slot.TestType = booking.TestType
			slot.Reason = "Already booked"
		} else if !slotInsideAvailability(clock, windows) {
			slot.State = "outside_availability"
			slot.Disabled = true
			slot.Reason = "Outside availability"
		} else {
			slot.State = "available"
			slot.Disabled = false
		}
		slots = append(slots, slot)
	}
	return slots, nil
}

func ValidatePlacementTestScheduleTx(tx *sql.Tx, leadID uuid.UUID, placementTest *PlacementTest) error {
	if placementTest == nil || !placementTest.TestDate.Valid || !placementTest.TestTime.Valid || placementTest.AssignedLevel.Valid {
		return nil
	}
	if !placementTest.ScheduledStudentSuccessID.Valid || strings.TrimSpace(placementTest.ScheduledStudentSuccessID.String) == "" {
		return fmt.Errorf("Student Success assignment is required before booking a placement test")
	}

	studentSuccessID, err := uuid.Parse(strings.TrimSpace(placementTest.ScheduledStudentSuccessID.String))
	if err != nil {
		return fmt.Errorf("Invalid Student Success assignment")
	}
	user, err := GetUserByID(studentSuccessID.String())
	if err != nil {
		return fmt.Errorf("failed to validate Student Success assignment: %w", err)
	}
	if user == nil || user.Role != "student_success" || !user.IsActive {
		return fmt.Errorf("Selected user is not an active Student Success")
	}

	testDate := dateOnly(placementTest.TestDate.Time)
	testClock, err := normalizePlacementTestSlotClock(placementTest.TestTime.String)
	if err != nil {
		return err
	}
	placementTest.TestDate = sql.NullTime{Time: testDate, Valid: true}
	placementTest.TestTime = sql.NullString{String: testClock.Format("15:04"), Valid: true}

	testDateTime := time.Date(testDate.Year(), testDate.Month(), testDate.Day(), testClock.Hour(), testClock.Minute(), 0, 0, time.UTC)
	if !testDateTime.After(time.Now()) {
		return fmt.Errorf("Placement test slot must be in the future")
	}

	var covered bool
	if err := tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM student_success_availability_windows
			WHERE student_success_user_id = $1
			  AND available_date = $2
			  AND start_time <= $3::TIME
			  AND end_time >= ($3::TIME + INTERVAL '30 minutes')
		)
	`, studentSuccessID, testDate, testClock.Format("15:04")).Scan(&covered); err != nil {
		return fmt.Errorf("failed to validate Student Success availability: %w", err)
	}
	if !covered {
		return fmt.Errorf("Selected slot is outside this Student Success availability")
	}

	var conflictLeadID sql.NullString
	if err := tx.QueryRow(`
		SELECT lead_id::TEXT
		FROM placement_tests
		WHERE scheduled_student_success_user_id = $1
		  AND test_date = $2
		  AND test_time = $3::TIME
		  AND assigned_level IS NULL
		  AND appointment_status = 'scheduled'
		  AND lead_id <> $4
		LIMIT 1
	`, studentSuccessID, testDate, testClock.Format("15:04"), leadID).Scan(&conflictLeadID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to validate placement test slot conflict: %w", err)
	} else if err == nil {
		return &PlacementTestSlotConflictError{
			StudentSuccessUserID: studentSuccessID,
			TestDate:             testDate,
			TestTime:             testClock.Format("15:04"),
		}
	}

	return nil
}

func IsPlacementTestSlotConflict(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*PlacementTestSlotConflictError); ok {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "uniq_active_placement_test_student_success_slot") ||
		(strings.Contains(msg, "duplicate") && strings.Contains(msg, "placement_test") && strings.Contains(msg, "student_success"))
}

func normalizePlacementTestSlotClock(value string) (time.Time, error) {
	clock, err := parseSessionClock(value)
	if err != nil {
		return time.Time{}, fmt.Errorf("Invalid placement test time")
	}
	if clock.Minute()%placementTestSlotMinutes != 0 || clock.Second() != 0 {
		return time.Time{}, fmt.Errorf("Placement test time must be a 30-minute slot")
	}
	if clockBefore(clock, placementTestDayStart) || !clockBefore(clock, placementTestDayEnd) {
		return time.Time{}, fmt.Errorf("Placement test time must be between 14:00 and 23:00")
	}
	return clock, nil
}

type placementTestBookingForSlot struct {
	LeadID    string
	LeadName  string
	LeadPhone string
	TestType  string
}

func getStudentSuccessAvailabilityWindowsForDate(studentSuccessUserID uuid.UUID, date time.Time) ([]StudentSuccessAvailabilityWindow, error) {
	rows, err := db.DB.Query(`
		SELECT id, student_success_user_id, available_date, start_time::TEXT, end_time::TEXT, note, created_at, updated_at
		FROM student_success_availability_windows
		WHERE student_success_user_id = $1
		  AND available_date = $2
		ORDER BY start_time, end_time
	`, studentSuccessUserID, dateOnly(date))
	if err != nil {
		return nil, fmt.Errorf("failed to query student success availability: %w", err)
	}
	defer func() { _ = rows.Close() }()

	windows := make([]StudentSuccessAvailabilityWindow, 0)
	for rows.Next() {
		var item StudentSuccessAvailabilityWindow
		if err := rows.Scan(
			&item.ID,
			&item.StudentSuccessUserID,
			&item.AvailableDate,
			&item.StartTime,
			&item.EndTime,
			&item.Note,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan student success availability: %w", err)
		}
		item.StartTime = formatClockText(item.StartTime)
		item.EndTime = formatClockText(item.EndTime)
		windows = append(windows, item)
	}
	return windows, rows.Err()
}

func getPlacementTestBookingsForSlotDate(studentSuccessUserID uuid.UUID, date time.Time) (map[string]placementTestBookingForSlot, error) {
	rows, err := db.DB.Query(`
		SELECT pt.test_time::TEXT, l.id::TEXT, l.full_name, l.phone, COALESCE(pt.test_type, '')
		FROM placement_tests pt
		INNER JOIN leads l ON l.id = pt.lead_id
		WHERE pt.scheduled_student_success_user_id = $1
		  AND pt.test_date = $2
		  AND pt.assigned_level IS NULL
		  AND pt.appointment_status = 'scheduled'
		  AND pt.test_time IS NOT NULL
		ORDER BY pt.test_time
	`, studentSuccessUserID, dateOnly(date))
	if err != nil {
		return nil, fmt.Errorf("failed to query placement test bookings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	bookings := make(map[string]placementTestBookingForSlot)
	for rows.Next() {
		var timeText string
		var item placementTestBookingForSlot
		if err := rows.Scan(&timeText, &item.LeadID, &item.LeadName, &item.LeadPhone, &item.TestType); err != nil {
			return nil, fmt.Errorf("failed to scan placement test booking: %w", err)
		}
		bookings[formatClockText(timeText)] = item
	}
	return bookings, rows.Err()
}

func slotInsideAvailability(slotClock time.Time, windows []StudentSuccessAvailabilityWindow) bool {
	slotEnd := slotClock.Add(time.Duration(placementTestSlotMinutes) * time.Minute)
	for _, window := range windows {
		startClock, err := parseSessionClock(window.StartTime)
		if err != nil {
			continue
		}
		endClock, err := parseSessionClock(window.EndTime)
		if err != nil {
			continue
		}
		if !clockBefore(slotClock, startClock) && !clockAfter(slotEnd, endClock) {
			return true
		}
	}
	return false
}
