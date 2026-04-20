package models

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
)

// Student Profile Repository Functions (Milestone 4)

// SearchStudents searches for students by name or phone
func SearchStudents(query string) ([]*StudentSearchResult, error) {
	rows, err := db.DB.Query(`
		SELECT 
			l.id, 
			l.full_name, 
			l.phone, 
			COALESCE(pt.assigned_level, 0) as current_level,
			l.status
		FROM leads l
		LEFT JOIN placement_tests pt ON l.id = pt.lead_id
		WHERE 
			LOWER(l.full_name) LIKE LOWER($1) 
			OR l.phone = $2
		ORDER BY l.full_name
		LIMIT 20
	`, "%"+query+"%", query)
	if err != nil {
		return nil, fmt.Errorf("failed to search students: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []*StudentSearchResult{}
	for rows.Next() {
		r := &StudentSearchResult{}
		err := rows.Scan(&r.LeadID, &r.FullName, &r.Phone, &r.CurrentLevel, &r.Status)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, r)
	}

	return results, nil
}

// UpdateStudentBasicInfo updates the editable identity fields for a student lead.
func UpdateStudentBasicInfo(leadID uuid.UUID, fullName, phone string) error {
	now := time.Now()
	result, err := db.DB.Exec(`
		UPDATE leads
		SET full_name = $1,
		    phone = $2,
		    updated_at = $3
		WHERE id = $4
	`, strings.TrimSpace(fullName), strings.TrimSpace(phone), now, leadID)
	if err != nil {
		return fmt.Errorf("failed to update student basic info: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to confirm student update: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetStudentProfile returns the profile header information for a student
func GetStudentProfile(leadID uuid.UUID) (*StudentProfile, error) {
	profile := &StudentProfile{}
	var currentLevel sql.NullInt32
	var remainingCredits sql.NullInt32
	var isReturning sql.NullBool

	err := db.DB.QueryRow(`
		SELECT 
			l.id,
			l.full_name,
			l.phone,
			COALESCE(pt.assigned_level, 0) as current_level,
			GREATEST(COALESCE(l.levels_purchased_total, 0) - COALESCE(l.levels_consumed, 0), 0) as remaining_credits,
			l.status,
			l.is_returning
		FROM leads l
		LEFT JOIN placement_tests pt ON l.id = pt.lead_id
		WHERE l.id = $1
	`, leadID).Scan(
		&profile.LeadID,
		&profile.FullName,
		&profile.Phone,
		&currentLevel,
		&remainingCredits,
		&profile.Status,
		&isReturning,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get student profile: %w", err)
	}

	if currentLevel.Valid {
		profile.CurrentLevel = currentLevel.Int32
	}
	if remainingCredits.Valid {
		profile.RemainingCredits = remainingCredits.Int32
	}
	if isReturning.Valid {
		profile.IsReturning = isReturning.Bool
	}

	return profile, nil
}

// GetAcademicHistory returns the academic history for a student from class_enrollments
func GetAcademicHistory(leadID uuid.UUID) ([]*AcademicHistoryItem, error) {
	rows, err := db.DB.Query(`
		SELECT 
			id,
			level,
			class_days,
			class_time,
			COALESCE(mentor_name, '') as mentor_name,
			COALESCE(final_grade, '') as final_grade,
			COALESCE(outcome, '') as outcome,
			enrolled_at,
			completed_at
		FROM class_enrollments
		WHERE lead_id = $1
		ORDER BY enrolled_at DESC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get academic history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	history := []*AcademicHistoryItem{}
	for rows.Next() {
		item := &AcademicHistoryItem{}
		var completedAt sql.NullTime

		err := rows.Scan(
			&item.ID,
			&item.Level,
			&item.ClassDays,
			&item.ClassTime,
			&item.MentorName,
			&item.FinalGrade,
			&item.Outcome,
			&item.EnrolledAt,
			&completedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan academic history: %w", err)
		}

		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}

		history = append(history, item)
	}

	return history, nil
}

// GetLatestEnrollmentSchedule returns the most recent class_days/class_time/class_key for a lead.
// Used to prefill scheduling for returning students.
func GetLatestEnrollmentSchedule(leadID uuid.UUID) (string, string, string, bool, error) {
	var classDays, classTime, classKey string
	err := db.DB.QueryRow(`
		SELECT class_days, class_time, class_key
		FROM class_enrollments
		WHERE lead_id = $1
		ORDER BY enrolled_at DESC
		LIMIT 1
	`, leadID).Scan(&classDays, &classTime, &classKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", "", false, nil
		}
		return "", "", "", false, fmt.Errorf("failed to get latest enrollment schedule: %w", err)
	}
	classTime = normalizeClassTime(classTime)
	return classDays, classTime, classKey, true, nil
}

func normalizeClassTime(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 5 && value[2] == ':' {
		return value[:5]
	}
	return value
}

// GetCurrentClassStatus returns the current class status if student is in_classes
func GetCurrentClassStatus(leadID uuid.UUID) (*CurrentClassStatus, error) {
	// First check if student is in_classes
	var status string
	err := db.DB.QueryRow(`SELECT status FROM leads WHERE id = $1`, leadID).Scan(&status)
	if err != nil {
		return nil, fmt.Errorf("failed to check student status: %w", err)
	}

	if status != "in_classes" {
		return nil, nil // Not in a class, return nil
	}

	// Get class information
	currentStatus := &CurrentClassStatus{}
	var mentorName sql.NullString

	err = db.DB.QueryRow(`
		SELECT
			cg.class_key,
			cg.level,
			cg.class_days,
			cg.class_time,
			COALESCE(u.email, '') as mentor_name,
			COALESCE((
				SELECT COUNT(*)
				FROM class_sessions cs
				WHERE cs.class_key = cg.class_key
				  AND cs.status = 'completed'
			), 0) + 1 as current_session
		FROM class_memberships cm
		INNER JOIN class_groups cg ON cg.class_key = cm.class_key
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		WHERE cm.lead_id = $1
		  AND cm.left_after_session_number IS NULL
		  AND cm.removed_at IS NULL
		ORDER BY cm.created_at DESC
		LIMIT 1
	`, leadID).Scan(
		&currentStatus.ClassKey,
		&currentStatus.Level,
		&currentStatus.ClassDays,
		&currentStatus.ClassTime,
		&mentorName,
		&currentStatus.CurrentSession,
	)
	if err == sql.ErrNoRows {
		err = db.DB.QueryRow(`
		SELECT 
			cg.class_key,
			cg.level,
			cg.class_days,
			cg.class_time,
			COALESCE(u.email, '') as mentor_name,
			COALESCE((SELECT COUNT(*) FROM class_sessions WHERE class_key = cg.class_key AND status = 'completed'), 0) + 1 as current_session
		FROM leads l
		INNER JOIN scheduling s ON s.lead_id = l.id
		INNER JOIN placement_tests pt ON pt.lead_id = l.id
		INNER JOIN class_groups cg ON (
			cg.level = pt.assigned_level
			AND cg.class_days = s.class_days
			AND cg.class_time = s.class_time::text
			AND COALESCE(cg.class_number, 1) = COALESCE(s.class_group_index, 1)
		)
		LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
		LEFT JOIN users u ON u.id = ma.mentor_user_id
		WHERE l.id = $1
	`, leadID).Scan(
			&currentStatus.ClassKey,
			&currentStatus.Level,
			&currentStatus.ClassDays,
			&currentStatus.ClassTime,
			&mentorName,
			&currentStatus.CurrentSession,
		)
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get current class info: %w", err)
	}

	if mentorName.Valid {
		currentStatus.MentorName = mentorName.String
	}

	// Get attendance records
	rows, err := db.DB.Query(`
		SELECT 
			cs.session_number,
			COALESCE(a.status, 'NOT_MARKED') as status,
			cs.scheduled_date
		FROM class_sessions cs
		LEFT JOIN attendance a ON a.session_id = cs.id AND a.lead_id = $1
		WHERE cs.class_key = $2
		ORDER BY cs.session_number
	`, leadID, currentStatus.ClassKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get attendance records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	sessionDetails := []SessionAttendance{}
	stats := AttendanceStats{}

	for rows.Next() {
		var sessionNumber int32
		var status string
		var scheduledDate time.Time

		err := rows.Scan(&sessionNumber, &status, &scheduledDate)
		if err != nil {
			return nil, fmt.Errorf("failed to scan attendance: %w", err)
		}

		sessionDetails = append(sessionDetails, SessionAttendance{
			SessionNumber: sessionNumber,
			Status:        status,
			Date:          scheduledDate.Format("2006-01-02"),
		})

		// Update stats
		switch status {
		case "PRESENT":
			stats.Present++
		case "ABSENT":
			stats.Absent++
		case "LATE":
			stats.Late++
		}
		if status != "NOT_MARKED" {
			stats.Total++
		}
	}

	currentStatus.SessionDetails = sessionDetails
	currentStatus.AttendanceStats = stats

	return currentStatus, nil
}

// GetStudentNotesTimeline returns a timeline of notes, followups, and final grading notes for a student
func GetStudentNotesTimeline(leadID uuid.UUID) ([]*TimelineItem, error) {
	rows, err := db.DB.Query(`
		SELECT 
			sn.id, 
			'note' as type, 
			sn.note_text as text, 
			COALESCE(sn.class_key, '') as class_key, 
			COALESCE(sn.session_number, 0) as session, 
			sn.is_private,
			COALESCE(u.email, '') as created_by,
			sn.created_at
		FROM student_notes sn
		LEFT JOIN users u ON sn.created_by_user_id = u.id
		WHERE sn.lead_id = $1

		UNION ALL

		SELECT 
			fcn.id,
			'followup' as type,
			COALESCE(fcn.note_text, '') as text,
			COALESCE(f.class_key, '') as class_key,
			COALESCE(f.session_number, 0) as session,
			false as is_private,
			COALESCE(u.email, '') as created_by,
			fcn.created_at
		FROM followup_case_notes fcn
		INNER JOIN followups f ON f.id = fcn.case_id
		LEFT JOIN users u ON fcn.created_by_user_id = u.id
		WHERE f.lead_id = $1

		UNION ALL

		SELECT 
			f.id, 
			'followup' as type, 
			COALESCE(f.note, '') as text, 
			COALESCE(f.class_key, '') as class_key, 
			COALESCE(f.session_number, 0) as session, 
			false as is_private,
			COALESCE(u.email, '') as created_by,
			f.created_at
		FROM followups f
		LEFT JOIN users u ON f.created_by::uuid = u.id
		WHERE f.lead_id = $1
		  AND NOT EXISTS (
			SELECT 1
			FROM followup_case_notes fcn
			WHERE fcn.case_id = f.id
		  )

		UNION ALL

		SELECT
			g.id,
			'grade_note' as type,
			COALESCE(g.notes, '') as text,
			COALESCE(g.class_key, '') as class_key,
			COALESCE(g.session_number, 8) as session,
			false as is_private,
			COALESCE(u.email, '') as created_by,
			g.updated_at as created_at
		FROM grades g
		LEFT JOIN users u ON g.created_by_user_id = u.id
		WHERE g.lead_id = $1
		  AND COALESCE(TRIM(g.notes), '') <> ''

		ORDER BY created_at DESC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get notes timeline: %w", err)
	}
	defer func() { _ = rows.Close() }()

	timeline := []*TimelineItem{}
	for rows.Next() {
		item := &TimelineItem{}
		var classKey sql.NullString
		var session sql.NullInt32

		err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Text,
			&classKey,
			&session,
			&item.IsPrivate,
			&item.CreatedBy,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan timeline item: %w", err)
		}

		if classKey.Valid {
			item.ClassKey = classKey.String
		}
		if session.Valid {
			item.Session = session.Int32
		}

		timeline = append(timeline, item)
	}

	return dedupeMirroredGradeTimeline(timeline), nil
}

func dedupeMirroredGradeTimeline(timeline []*TimelineItem) []*TimelineItem {
	if len(timeline) == 0 {
		return timeline
	}

	mirroredNoteIDs := make(map[uuid.UUID]struct{})
	for _, item := range timeline {
		if item == nil || item.Type != "grade_note" {
			continue
		}
		mirroredNoteIDs[gradeMirrorNoteID(item.ID)] = struct{}{}
	}

	if len(mirroredNoteIDs) == 0 {
		return timeline
	}

	deduped := make([]*TimelineItem, 0, len(timeline))
	for _, item := range timeline {
		if item == nil {
			continue
		}
		if item.Type == "note" {
			if _, exists := mirroredNoteIDs[item.ID]; exists {
				continue
			}
		}
		deduped = append(deduped, item)
	}

	return deduped
}
