package models

import (
	"database/sql"
	"fmt"

	"eighty-twenty-ops/internal/db"

	"github.com/google/uuid"
)

type MentorSessionCheck struct {
	ID              uuid.UUID `json:"id"`
	ClassSessionID  uuid.UUID `json:"class_session_id"`
	CheckedByUserID uuid.UUID `json:"checked_by_user_id"`
	Reminder1D      bool      `json:"reminder_1d"`
	Reminder1H      bool      `json:"reminder_1h"`
	ReminderTasks   bool      `json:"reminder_tasks"`
	DelayMinutes    int       `json:"delay_minutes"`
	IsAbsent        bool      `json:"is_absent"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
	CheckedByEmail  *string   `json:"checked_by_email,omitempty"`
}

type ComplianceClassSession struct {
	SessionNumber  int32               `json:"session_number"`
	ClassSessionID *uuid.UUID          `json:"class_session_id,omitempty"`
	Status         *string             `json:"status,omitempty"`
	ScheduledDate  *string             `json:"scheduled_date,omitempty"`
	ScheduledTime  *string             `json:"scheduled_time,omitempty"`
	Check          *MentorSessionCheck `json:"check,omitempty"`
}

type MentorComplianceReportRow struct {
	MentorID        uuid.UUID `json:"mentor_id"`
	MentorEmail     string    `json:"mentor_email"`
	ClassesCount    int       `json:"classes_count"`
	SessionsCount   int       `json:"sessions_count"`
	ChecksCount     int       `json:"checks_count"`
	ComplianceScore float64   `json:"compliance_score"`
	AvgDelayMinutes float64   `json:"avg_delay_minutes"`
	AbsenceCount    int       `json:"absence_count"`
	ComplaintsCount int       `json:"complaints_count"`
}

type MentorComplianceChecklistRow struct {
	ClassKey      string  `json:"class_key"`
	ClassDays     string  `json:"class_days"`
	ClassTime     string  `json:"class_time"`
	SessionNumber int32   `json:"session_number"`
	ScheduledDate string  `json:"scheduled_date"`
	ScheduledTime string  `json:"scheduled_time"`
	SessionStatus string  `json:"session_status"`
	Reminder1D    bool    `json:"reminder_1d"`
	Reminder1H    bool    `json:"reminder_1h"`
	ReminderTasks bool    `json:"reminder_tasks"`
	DelayMinutes  int     `json:"delay_minutes"`
	IsAbsent      bool    `json:"is_absent"`
	CheckedBy     *string `json:"checked_by,omitempty"`
}

type MentorClassComplianceReportRow struct {
	MentorID        uuid.UUID `json:"mentor_id"`
	MentorEmail     string    `json:"mentor_email"`
	ClassKey        string    `json:"class_key"`
	Level           int       `json:"level"`
	ClassDays       string    `json:"class_days"`
	ClassTime       string    `json:"class_time"`
	ClassNumber     int       `json:"class_number"`
	SessionsCount   int       `json:"sessions_count"`
	ChecksCount     int       `json:"checks_count"`
	ComplianceScore float64   `json:"compliance_score"`
	AvgDelayMinutes float64   `json:"avg_delay_minutes"`
	AbsenceCount    int       `json:"absence_count"`
	ComplaintsCount int       `json:"complaints_count"`
}

func ExcludeMentorFromReports(mentorID uuid.UUID, roundStatus string, excludedBy uuid.UUID, reason string) error {
	if roundStatus != "active" && roundStatus != "closed" {
		return fmt.Errorf("invalid round_status")
	}
	_, err := db.DB.Exec(`
		INSERT INTO mentor_report_exclusions (mentor_user_id, round_status, excluded_by_user_id, reason)
		VALUES ($1, $2, $3, NULLIF(TRIM($4), ''))
		ON CONFLICT (mentor_user_id, round_status) DO UPDATE
		SET excluded_by_user_id = EXCLUDED.excluded_by_user_id,
		    reason = EXCLUDED.reason,
		    created_at = NOW()
	`, mentorID, roundStatus, excludedBy, reason)
	if err != nil {
		return fmt.Errorf("failed to exclude mentor from reports: %w", err)
	}
	return nil
}

func ExcludeMentorFromReportsAll(mentorID uuid.UUID, excludedBy uuid.UUID, reason string) error {
	if err := ExcludeMentorFromReports(mentorID, "active", excludedBy, reason); err != nil {
		return err
	}
	if err := ExcludeMentorFromReports(mentorID, "closed", excludedBy, reason); err != nil {
		return err
	}
	return nil
}

func UpsertMentorSessionCheck(
	classSessionID uuid.UUID,
	checkedByUserID uuid.UUID,
	reminder1D bool,
	reminder1H bool,
	reminderTasks bool,
	delayMinutes int,
	isAbsent bool,
) (*MentorSessionCheck, error) {
	if delayMinutes < 0 {
		return nil, fmt.Errorf("delay_minutes must be >= 0")
	}

	row := &MentorSessionCheck{}
	err := db.DB.QueryRow(`
		INSERT INTO mentor_session_checks (
			class_session_id, checked_by_user_id,
			reminder_1d, reminder_1h, reminder_tasks, delay_minutes, is_absent
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (class_session_id) DO UPDATE
		SET checked_by_user_id = EXCLUDED.checked_by_user_id,
		    reminder_1d = EXCLUDED.reminder_1d,
		    reminder_1h = EXCLUDED.reminder_1h,
		    reminder_tasks = EXCLUDED.reminder_tasks,
		    delay_minutes = EXCLUDED.delay_minutes,
		    is_absent = EXCLUDED.is_absent,
		    updated_at = NOW()
		RETURNING id, class_session_id, checked_by_user_id,
		          reminder_1d, reminder_1h, reminder_tasks, delay_minutes, is_absent,
		          created_at::text, updated_at::text
	`, classSessionID, checkedByUserID, reminder1D, reminder1H, reminderTasks, delayMinutes, isAbsent).
		Scan(
			&row.ID, &row.ClassSessionID, &row.CheckedByUserID,
			&row.Reminder1D, &row.Reminder1H, &row.ReminderTasks, &row.DelayMinutes, &row.IsAbsent,
			&row.CreatedAt, &row.UpdatedAt,
		)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert mentor_session_checks: %w", err)
	}
	return row, nil
}

func GetComplianceByClassKey(classKey string) ([]*ComplianceClassSession, error) {
	rows, err := db.DB.Query(`
		SELECT gs.session_number,
		       cs.id::text,
		       cs.status,
		       cs.scheduled_date::text,
		       cs.scheduled_time::text,
		       msc.id::text,
		       msc.class_session_id::text,
		       msc.checked_by_user_id::text,
		       COALESCE(msc.reminder_1d, false),
		       COALESCE(msc.reminder_1h, false),
		       COALESCE(msc.reminder_tasks, false),
		       COALESCE(msc.delay_minutes, 0),
		       COALESCE(msc.is_absent, false),
		       COALESCE(msc.created_at::text, ''),
		       COALESCE(msc.updated_at::text, ''),
		       u.email
		FROM generate_series(1, 8) AS gs(session_number)
		LEFT JOIN class_sessions cs
		       ON cs.class_key = $1
		      AND cs.session_number = gs.session_number
		LEFT JOIN mentor_session_checks msc
		       ON msc.class_session_id = cs.id
		LEFT JOIN users u
		       ON u.id = msc.checked_by_user_id
		ORDER BY gs.session_number
	`, classKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load class compliance: %w", err)
	}
	defer rows.Close()

	sessions := make([]*ComplianceClassSession, 0, 8)
	for rows.Next() {
		var (
			sessionNum   int32
			csIDStr      sql.NullString
			status       sql.NullString
			schedDate    sql.NullString
			schedTime    sql.NullString
			checkIDStr   sql.NullString
			checkCSIDStr sql.NullString
			checkUserStr sql.NullString
			rem1d        bool
			rem1h        bool
			remTasks     bool
			delay        int
			isAbsent     bool
			createdAt    string
			updatedAt    string
			checkedEmail sql.NullString
		)
		if err := rows.Scan(
			&sessionNum, &csIDStr, &status, &schedDate, &schedTime,
			&checkIDStr, &checkCSIDStr, &checkUserStr,
			&rem1d, &rem1h, &remTasks, &delay, &isAbsent, &createdAt, &updatedAt, &checkedEmail,
		); err != nil {
			return nil, fmt.Errorf("failed to scan class compliance row: %w", err)
		}

		item := &ComplianceClassSession{SessionNumber: sessionNum}
		if csIDStr.Valid {
			id, err := uuid.Parse(csIDStr.String)
			if err == nil {
				item.ClassSessionID = &id
			}
		}
		if status.Valid {
			v := status.String
			item.Status = &v
		}
		if schedDate.Valid {
			v := schedDate.String
			item.ScheduledDate = &v
		}
		if schedTime.Valid {
			v := schedTime.String
			item.ScheduledTime = &v
		}

		if checkIDStr.Valid && checkCSIDStr.Valid && checkUserStr.Valid {
			checkID, err1 := uuid.Parse(checkIDStr.String)
			checkCSID, err2 := uuid.Parse(checkCSIDStr.String)
			checkUserID, err3 := uuid.Parse(checkUserStr.String)
			if err1 == nil && err2 == nil && err3 == nil {
				check := &MentorSessionCheck{
					ID:              checkID,
					ClassSessionID:  checkCSID,
					CheckedByUserID: checkUserID,
					Reminder1D:      rem1d,
					Reminder1H:      rem1h,
					ReminderTasks:   remTasks,
					DelayMinutes:    delay,
					IsAbsent:        isAbsent,
					CreatedAt:       createdAt,
					UpdatedAt:       updatedAt,
				}
				if checkedEmail.Valid {
					check.CheckedByEmail = &checkedEmail.String
				}
				item.Check = check
			}
		}

		sessions = append(sessions, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate class compliance rows: %w", err)
	}
	return sessions, nil
}

func GetMentorComplianceReports(roundStatus string, mentorID *uuid.UUID) ([]*MentorComplianceReportRow, error) {
	args := []interface{}{}
	i := 1

	query := `
		WITH class_owner AS (
			SELECT cg.class_key,
			       COALESCE(ma.mentor_user_id, cg.closed_mentor_user_id) AS mentor_user_id
			FROM class_groups cg
			LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
			WHERE COALESCE(ma.mentor_user_id::text, cg.closed_mentor_user_id::text) IS NOT NULL
	`
	if roundStatus != "" {
		query += fmt.Sprintf(" AND cg.round_status = $%d", i)
		args = append(args, roundStatus)
		i++
	}
	if mentorID != nil {
		query += fmt.Sprintf(" AND COALESCE(ma.mentor_user_id, cg.closed_mentor_user_id) = $%d", i)
		args = append(args, *mentorID)
		i++
	}

	query += `
		),
		class_counts AS (
			SELECT mentor_user_id, COUNT(DISTINCT class_key) AS classes_count
			FROM class_owner
			GROUP BY mentor_user_id
		),
		session_counts AS (
			SELECT co.mentor_user_id, COUNT(cs.id) AS sessions_count
			FROM class_owner co
			LEFT JOIN class_sessions cs ON cs.class_key = co.class_key
			GROUP BY co.mentor_user_id
		),
		compliance AS (
			SELECT co.mentor_user_id,
			       COUNT(msc.id) AS checks_count,
			       COALESCE(SUM(
			           (CASE WHEN msc.reminder_1d THEN 1 ELSE 0 END) +
			           (CASE WHEN msc.reminder_1h THEN 1 ELSE 0 END) +
			           (CASE WHEN msc.reminder_tasks THEN 1 ELSE 0 END)
			       ), 0) AS reminders_sent,
			       COALESCE(AVG(msc.delay_minutes::numeric), 0) AS avg_delay_minutes,
			       COALESCE(SUM(CASE WHEN msc.is_absent THEN 1 ELSE 0 END), 0) AS absence_count
			FROM class_owner co
			LEFT JOIN class_sessions cs ON cs.class_key = co.class_key
			LEFT JOIN mentor_session_checks msc ON msc.class_session_id = cs.id
			GROUP BY co.mentor_user_id
		),
		complaints AS (
			SELECT co.mentor_user_id, COUNT(*) AS complaints_count
			FROM class_owner co
			INNER JOIN followups f ON f.class_key = co.class_key
			WHERE f.type = 'complaint'
			  AND f.deleted_at IS NULL
			GROUP BY co.mentor_user_id
		)
	`
	if roundStatus != "" {
		query += fmt.Sprintf(`
		, exclusions AS (
			SELECT mentor_user_id
			FROM mentor_report_exclusions
			WHERE round_status = $%d
		)
	`, i)
		args = append(args, roundStatus)
		i++
	} else {
		query += `
		, exclusions AS (
			SELECT mentor_user_id
			FROM mentor_report_exclusions
			GROUP BY mentor_user_id
			HAVING COUNT(DISTINCT round_status) = 2
		)
	`
	}

	query += `
		SELECT u.id,
		       u.email,
		       COALESCE(cc.classes_count, 0),
		       COALESCE(sc.sessions_count, 0),
		       COALESCE(c.checks_count, 0),
		       CASE
		           WHEN COALESCE(c.checks_count, 0) = 0 THEN 0
		           ELSE ROUND((c.reminders_sent::numeric / (c.checks_count * 3)::numeric) * 100, 2)
		       END AS compliance_score,
		       COALESCE(c.avg_delay_minutes, 0),
		       COALESCE(c.absence_count, 0),
		       COALESCE(cp.complaints_count, 0)
		FROM (SELECT DISTINCT mentor_user_id FROM class_owner) m
		INNER JOIN users u ON u.id = m.mentor_user_id
		LEFT JOIN class_counts cc ON cc.mentor_user_id = m.mentor_user_id
		LEFT JOIN session_counts sc ON sc.mentor_user_id = m.mentor_user_id
		LEFT JOIN compliance c ON c.mentor_user_id = m.mentor_user_id
		LEFT JOIN complaints cp ON cp.mentor_user_id = m.mentor_user_id
		LEFT JOIN exclusions ex ON ex.mentor_user_id = m.mentor_user_id
		WHERE ex.mentor_user_id IS NULL
		ORDER BY u.email ASC
	`

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor compliance reports: %w", err)
	}
	defer rows.Close()

	var out []*MentorComplianceReportRow
	for rows.Next() {
		r := &MentorComplianceReportRow{}
		if err := rows.Scan(
			&r.MentorID,
			&r.MentorEmail,
			&r.ClassesCount,
			&r.SessionsCount,
			&r.ChecksCount,
			&r.ComplianceScore,
			&r.AvgDelayMinutes,
			&r.AbsenceCount,
			&r.ComplaintsCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor compliance report row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate mentor compliance report rows: %w", err)
	}
	return out, nil
}

func GetMentorComplianceChecklist(mentorID uuid.UUID, roundStatus string) ([]*MentorComplianceChecklistRow, error) {
	args := []interface{}{mentorID}
	query := `
		WITH class_owner AS (
			SELECT cg.class_key,
			       cg.class_days,
			       cg.class_time,
			       COALESCE(ma.mentor_user_id, cg.closed_mentor_user_id) AS mentor_user_id
			FROM class_groups cg
			LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
			WHERE COALESCE(ma.mentor_user_id, cg.closed_mentor_user_id) = $1
	`
	if roundStatus == "active" || roundStatus == "closed" {
		args = append(args, roundStatus)
		query += ` AND cg.round_status = $2`
	}
	query += `
		)
		SELECT co.class_key,
		       co.class_days,
		       co.class_time,
		       cs.session_number,
		       COALESCE(cs.scheduled_date::text, ''),
		       COALESCE(cs.scheduled_time::text, ''),
		       COALESCE(cs.status, 'scheduled'),
		       COALESCE(msc.reminder_1d, false),
		       COALESCE(msc.reminder_1h, false),
		       COALESCE(msc.reminder_tasks, false),
		       COALESCE(msc.delay_minutes, 0),
		       COALESCE(msc.is_absent, false),
		       u.email
		FROM class_owner co
		INNER JOIN class_sessions cs ON cs.class_key = co.class_key
		LEFT JOIN mentor_session_checks msc ON msc.class_session_id = cs.id
		LEFT JOIN users u ON u.id = msc.checked_by_user_id
		ORDER BY co.class_key, cs.session_number
	`

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to load mentor compliance checklist: %w", err)
	}
	defer rows.Close()

	var out []*MentorComplianceChecklistRow
	for rows.Next() {
		row := &MentorComplianceChecklistRow{}
		var checkedBy sql.NullString
		if err := rows.Scan(
			&row.ClassKey,
			&row.ClassDays,
			&row.ClassTime,
			&row.SessionNumber,
			&row.ScheduledDate,
			&row.ScheduledTime,
			&row.SessionStatus,
			&row.Reminder1D,
			&row.Reminder1H,
			&row.ReminderTasks,
			&row.DelayMinutes,
			&row.IsAbsent,
			&checkedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor compliance checklist row: %w", err)
		}
		if checkedBy.Valid {
			row.CheckedBy = &checkedBy.String
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate mentor compliance checklist rows: %w", err)
	}
	return out, nil
}

func GetMentorClassComplianceReports(roundStatus string, mentorID *uuid.UUID) ([]*MentorClassComplianceReportRow, error) {
	args := []interface{}{}
	i := 1

	query := `
		WITH class_owner AS (
			SELECT cg.class_key,
			       cg.level,
			       cg.class_days,
			       cg.class_time,
			       cg.class_number,
			       COALESCE(ma.mentor_user_id, cg.closed_mentor_user_id) AS mentor_user_id
			FROM class_groups cg
			LEFT JOIN mentor_assignments ma ON ma.class_key = cg.class_key
			WHERE COALESCE(ma.mentor_user_id::text, cg.closed_mentor_user_id::text) IS NOT NULL
	`
	if roundStatus != "" {
		query += fmt.Sprintf(" AND cg.round_status = $%d", i)
		args = append(args, roundStatus)
		i++
	}
	if mentorID != nil {
		query += fmt.Sprintf(" AND COALESCE(ma.mentor_user_id, cg.closed_mentor_user_id) = $%d", i)
		args = append(args, *mentorID)
		i++
	}
	query += `
		),
		session_counts AS (
			SELECT co.class_key, COUNT(cs.id) AS sessions_count
			FROM class_owner co
			LEFT JOIN class_sessions cs ON cs.class_key = co.class_key
			GROUP BY co.class_key
		),
		compliance AS (
			SELECT co.class_key,
			       COUNT(msc.id) AS checks_count,
			       COALESCE(SUM(
			           (CASE WHEN msc.reminder_1d THEN 1 ELSE 0 END) +
			           (CASE WHEN msc.reminder_1h THEN 1 ELSE 0 END) +
			           (CASE WHEN msc.reminder_tasks THEN 1 ELSE 0 END)
			       ), 0) AS reminders_sent,
			       COALESCE(AVG(msc.delay_minutes::numeric), 0) AS avg_delay_minutes,
			       COALESCE(SUM(CASE WHEN msc.is_absent THEN 1 ELSE 0 END), 0) AS absence_count
			FROM class_owner co
			LEFT JOIN class_sessions cs ON cs.class_key = co.class_key
			LEFT JOIN mentor_session_checks msc ON msc.class_session_id = cs.id
			GROUP BY co.class_key
		),
		complaints AS (
			SELECT f.class_key, COUNT(*) AS complaints_count
			FROM followups f
			WHERE f.type = 'complaint'
			  AND f.deleted_at IS NULL
			GROUP BY f.class_key
		)
		SELECT co.mentor_user_id,
		       u.email,
		       co.class_key,
		       co.level,
		       co.class_days,
		       co.class_time,
		       co.class_number,
		       COALESCE(sc.sessions_count, 0),
		       COALESCE(c.checks_count, 0),
		       CASE
		           WHEN COALESCE(c.checks_count, 0) = 0 THEN 0
		           ELSE ROUND((c.reminders_sent::numeric / (c.checks_count * 3)::numeric) * 100, 2)
		       END AS compliance_score,
		       COALESCE(c.avg_delay_minutes, 0),
		       COALESCE(c.absence_count, 0),
		       COALESCE(cp.complaints_count, 0)
		FROM class_owner co
		INNER JOIN users u ON u.id = co.mentor_user_id
		LEFT JOIN session_counts sc ON sc.class_key = co.class_key
		LEFT JOIN compliance c ON c.class_key = co.class_key
		LEFT JOIN complaints cp ON cp.class_key = co.class_key
		ORDER BY u.email ASC, co.level ASC, co.class_days ASC, co.class_time ASC, co.class_number ASC
	`

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query mentor class compliance reports: %w", err)
	}
	defer rows.Close()

	var out []*MentorClassComplianceReportRow
	for rows.Next() {
		r := &MentorClassComplianceReportRow{}
		if err := rows.Scan(
			&r.MentorID,
			&r.MentorEmail,
			&r.ClassKey,
			&r.Level,
			&r.ClassDays,
			&r.ClassTime,
			&r.ClassNumber,
			&r.SessionsCount,
			&r.ChecksCount,
			&r.ComplianceScore,
			&r.AvgDelayMinutes,
			&r.AbsenceCount,
			&r.ComplaintsCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan mentor class compliance row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate mentor class compliance rows: %w", err)
	}
	return out, nil
}
